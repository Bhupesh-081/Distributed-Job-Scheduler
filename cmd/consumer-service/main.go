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

// retryDelay is the fallback when store.EffectiveRetryPolicy finds no
// policy (unscoped job, or a queue with no default_retry_policy_id).
const retryDelay = 5 * time.Second

// cancelPollInterval also drives TouchRunningJob's liveness touch, well
// under watcher-service's 30s stale threshold so a healthy job in a long
// payload never looks abandoned.
const cancelPollInterval = 2 * time.Second

const heartbeatInterval = 10 * time.Second

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

	hostname, _ := os.Hostname()
	workerID := uuid.New()
	if _, err := st.RegisterWorker(ctx, workerID, hostname, os.Getpid(), concurrency); err != nil {
		return fmt.Errorf("register worker: %w", err)
	}

	w := &worker{id: workerID, store: st, producer: producer, redis: rdb, sem: make(chan struct{}, concurrency)}

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

	wg.Go(func() { w.heartbeatLoop(ctx) })

	slog.Info("consumer-service running", "worker_id", workerID, "concurrency", concurrency, "brokers", brokers)
	wg.Wait()

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()
	if err := st.MarkWorkerStopped(stopCtx, workerID); err != nil {
		slog.Error("mark worker stopped", "error", err)
	}
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
	id       uuid.UUID
	store    *store.Store
	producer *kafkapkg.Producer
	redis    *redis.Client
	sem      chan struct{}
	inFlight atomic.Int32
}

func (w *worker) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.store.Heartbeat(context.Background(), w.id, int(w.inFlight.Load())); err != nil {
				slog.Error("heartbeat", "worker_id", w.id, "error", err)
			}
		}
	}
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
			w.inFlight.Add(1)
			w.handle(context.Background(), topic, jobID)
			w.inFlight.Add(-1)
			if err := consumer.Commit(context.Background(), msg); err != nil {
				slog.Error("commit message", "topic", topic, "job_id", jobID, "error", err)
			}
		})
	}
	inFlight.Wait()
}

// handle logs errors but never returns one: a bad job shouldn't take down
// the consumer loop.
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

	run, err := w.store.CreateJobRun(ctx, jobID, job.RetriesCount+1, w.id)
	if err != nil {
		log.Error("create job run", "error", err)
		return
	}
	w.jobLog(ctx, log, jobID, &run.ID, "info", fmt.Sprintf("claimed by worker %s, attempt %d", w.id, run.AttemptNumber))

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
				// Proves to RecoverStuckRunningJobs that this job is still
				// being worked on, not abandoned by a dead worker.
				if err := w.store.TouchRunningJob(context.Background(), jobID); err != nil {
					log.Error("touch running job", "error", err)
				}
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
	if output != "" {
		w.jobLog(ctx, log, jobID, &run.ID, "info", "output: "+output)
	}

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
		w.jobLog(ctx, log, jobID, &run.ID, "warn", "cancelled by user")
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
		w.jobLog(ctx, log, jobID, &run.ID, "info", "attempt succeeded")
		return
	}

	errMsg := runErr.Error()
	if err := w.store.FinishJobRun(ctx, run.ID, "failed", &errMsg); err != nil {
		log.Error("finish job run", "error", err)
	}
	w.jobLog(ctx, log, jobID, &run.ID, "error", "attempt failed: "+errMsg)

	count, max, dead, err := w.store.RetryOrDeadLetter(ctx, jobID)
	if err != nil {
		log.Error("retry or dead-letter", "error", err)
		return
	}
	if dead {
		log.Warn("job dead-lettered", "retries", count, "max", max)
		w.jobLog(ctx, log, jobID, &run.ID, "error", fmt.Sprintf("dead-lettered after %d/%d retries", count, max))
		if _, err := w.store.CreateDLQEntry(ctx, jobID, job.QueueID, &errMsg, count); err != nil {
			log.Error("create dlq entry", "error", err)
		}
		return
	}

	delay := retryDelay
	if policy, ok, err := w.store.EffectiveRetryPolicy(ctx, jobID); err != nil {
		log.Error("get effective retry policy", "error", err)
	} else if ok {
		delay = policy.Delay(count)
	}

	log.Info("job requeued for retry", "attempt", count, "max", max, "delay", delay)
	w.jobLog(ctx, log, jobID, &run.ID, "warn", fmt.Sprintf("requeued for retry %d/%d in %s", count, max, delay))
	time.AfterFunc(delay, func() {
		if err := w.producer.PublishJob(context.Background(), kafkapkg.TopicRetry, jobID); err != nil {
			slog.Error("publish retry", "job_id", jobID, "error", err)
		}
	})
}

// jobLog only logs on error; a lost log line shouldn't affect job execution.
func (w *worker) jobLog(ctx context.Context, log *slog.Logger, jobID uuid.UUID, runID *uuid.UUID, level, message string) {
	if _, err := w.store.CreateJobLog(ctx, jobID, runID, level, message); err != nil {
		log.Error("write job log", "error", err)
	}
}
