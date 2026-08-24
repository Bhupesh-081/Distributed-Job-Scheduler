// Command consumer-service reads the "run"/"retry" Kafka topics, atomically
// claims each job in Postgres, and executes it (see internal/executor and
// docs/architecture.md). This is the Worker service from the assignment
// brief: concurrent execution, graceful shutdown, at-least-once delivery
// made safe by the atomic claim.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"distributed-job-scheduler/internal/cancel"
	"distributed-job-scheduler/internal/db"
	"distributed-job-scheduler/internal/executor"
	kafkapkg "distributed-job-scheduler/internal/kafka"
	"distributed-job-scheduler/internal/store"
)

// ponytail: fixed 5s retry delay, no linear/exponential backoff yet —
// retry_policies table is deferred (see docs/design-decisions.md MVP
// ledger). Upgrade: read a strategy off the job/queue once that table
// exists.
const retryDelay = 5 * time.Second

// How often a running job's execution is checked against the Redis cancel
// flag. Coarse on purpose — cancellation is best-effort, not a hard SLA.
const cancelPollInterval = 2 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	concurrency, _ := strconv.Atoi(os.Getenv("CONCURRENCY"))
	if concurrency <= 0 {
		concurrency = 5
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	st := store.New(pool)
	brokers := kafkapkg.Brokers()
	producer := kafkapkg.NewProducer(brokers)
	defer producer.Close()

	rdb := redis.NewClient(&redis.Options{Addr: getenv("REDIS_ADDR", "localhost:6379")})
	defer rdb.Close()

	w := &worker{store: st, producer: producer, redis: rdb, sem: make(chan struct{}, concurrency)}

	var wg sync.WaitGroup
	for _, topic := range []string{kafkapkg.TopicRun, kafkapkg.TopicRetry} {
		// Distinct group ID per topic: two Readers sharing one group ID would
		// each subscribe to a different single topic, which is invalid
		// consumer-group usage (a group's members must agree on subscription).
		consumer := kafkapkg.NewConsumer(brokers, topic, "consumer-service-"+topic)
		defer consumer.Close()
		wg.Add(1)
		go func(topic string) {
			defer wg.Done()
			w.consumeLoop(ctx, topic, consumer)
		}(topic)
	}

	slog.Info("consumer-service running", "concurrency", concurrency, "brokers", brokers)
	wg.Wait()
	slog.Info("consumer-service stopped")
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type worker struct {
	store    *store.Store
	producer *kafkapkg.Producer
	redis    *redis.Client
	sem      chan struct{}
}

// consumeLoop fetches messages one at a time (kafka-go readers aren't safe
// for concurrent Fetch) and hands each to a goroutine gated by the
// semaphore, which is what makes execution concurrent within one topic.
func (w *worker) consumeLoop(ctx context.Context, topic string, consumer *kafkapkg.Consumer) {
	var inFlight sync.WaitGroup
fetchLoop:
	for {
		jobID, msg, err := consumer.ReadJob(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break fetchLoop
			}
			slog.Error("fetch message", "topic", topic, "error", err)
			continue
		}

		select {
		case w.sem <- struct{}{}:
		case <-ctx.Done():
			break fetchLoop
		}

		inFlight.Go(func() {
			defer func() { <-w.sem }()
			w.handle(context.Background(), topic, jobID)
			if err := consumer.Commit(context.Background(), msg); err != nil {
				slog.Error("commit message", "topic", topic, "job_id", jobID, "error", err)
			}
		})
	}
	inFlight.Wait()
}

// handle claims, runs, and records the outcome of one job. Errors are
// logged, never fatal to the worker — a bad job shouldn't take down the
// consumer loop.
func (w *worker) handle(ctx context.Context, topic string, jobID uuid.UUID) {
	log := slog.With("job_id", jobID, "topic", topic)

	job, claimed, err := w.store.ClaimJob(ctx, jobID)
	if err != nil {
		log.Error("claim job", "error", err)
		return
	}
	if !claimed {
		log.Info("job already claimed, skipping")
		return
	}

	run, err := w.store.CreateJobRun(ctx, jobID, job.RetriesCount+1)
	if err != nil {
		log.Error("create job run", "error", err)
		return
	}

	execCtx, stopExec := context.WithCancel(ctx)
	defer stopExec()
	var wasCancelled atomic.Bool
	cancelPollDone := make(chan struct{})
	go func() {
		defer close(cancelPollDone)
		ticker := time.NewTicker(cancelPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-execCtx.Done():
				return
			case <-ticker.C:
				if requested, err := cancel.IsRequested(context.Background(), w.redis, jobID); err != nil {
					log.Error("check cancel flag", "error", err)
				} else if requested {
					wasCancelled.Store(true)
					stopExec()
					return
				}
			}
		}
	}()

	output, runErr := executor.Run(execCtx, job.Payload)
	stopExec() // unblock the poller (execCtx.Done()) so cancelPollDone closes below
	<-cancelPollDone
	log.Info("job executed", "attempt", run.AttemptNumber, "success", runErr == nil, "output", output)

	if wasCancelled.Load() {
		errMsg := "cancelled by user"
		if err := w.store.FinishJobRun(ctx, run.ID, "failed", &errMsg); err != nil {
			log.Error("finish job run", "error", err)
		}
		if err := w.store.SetJobStatus(ctx, jobID, "cancelled"); err != nil {
			log.Error("set job status", "error", err)
		}
		if err := cancel.Clear(ctx, w.redis, jobID); err != nil {
			log.Error("clear cancel flag", "error", err)
		}
		log.Info("job cancelled")
		return
	}

	if runErr == nil {
		if err := w.store.FinishJobRun(ctx, run.ID, "success", nil); err != nil {
			log.Error("finish job run", "error", err)
		}
		if err := w.store.SetJobStatus(ctx, jobID, "success"); err != nil {
			log.Error("set job status", "error", err)
		}
		return
	}

	errMsg := runErr.Error()
	if err := w.store.FinishJobRun(ctx, run.ID, "failed", &errMsg); err != nil {
		log.Error("finish job run", "error", err)
	}

	count, max, dead, err := w.store.RetryOrDeadLetter(ctx, jobID)
	if err != nil {
		log.Error("retry or dead-letter", "error", err)
		return
	}
	if dead {
		log.Warn("job dead-lettered", "retries", count, "max", max)
		return
	}

	log.Info("job requeued for retry", "attempt", count, "max", max, "delay", retryDelay)
	time.AfterFunc(retryDelay, func() {
		if err := w.producer.PublishJob(context.Background(), kafkapkg.TopicRetry, jobID); err != nil {
			slog.Error("publish retry", "job_id", jobID, "error", err)
		}
	})
}
