// Command watcher-service polls Postgres for two things: due jobs to
// publish onto the Kafka "run" topic (delayed/scheduled jobs whose time has
// come, and immediate jobs job-service already inserted as due-now), and
// queued jobs stuck mid-dispatch (crash, lost Kafka message) to reset for
// redispatch. See docs/architecture.md.
//
// ponytail: single instance, no leader election. Two watcher-service
// replicas would double-publish due jobs. Fine for MVP; add a Redis lock
// (see design-decisions.md) before running more than one.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"distributed-job-scheduler/internal/db"
	kafkapkg "distributed-job-scheduler/internal/kafka"
	"distributed-job-scheduler/internal/store"
)

const (
	pollInterval  = time.Second // NFR: a job starts within 2s of its scheduled time.
	dispatchBatch = 500
	recoverBatch  = 100
)

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
	staleAfter := 30 * time.Second
	if v, _ := strconv.Atoi(os.Getenv("STUCK_JOB_THRESHOLD_SECONDS")); v > 0 {
		staleAfter = time.Duration(v) * time.Second
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

	slog.Info("watcher-service running", "poll_interval", pollInterval, "stuck_job_threshold", staleAfter, "brokers", brokers)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("watcher-service stopped")
			return nil
		case <-ticker.C:
			tick(ctx, st, producer, staleAfter)
		}
	}
}

func tick(ctx context.Context, st *store.Store, producer *kafkapkg.Producer, staleAfter time.Duration) {
	recovered, err := st.RecoverStuckJobs(ctx, staleAfter, recoverBatch)
	if err != nil {
		slog.Error("recover stuck jobs", "error", err)
	} else if len(recovered) > 0 {
		slog.Warn("recovered stuck jobs", "count", len(recovered), "job_ids", recovered)
	}

	due, err := st.DispatchDueJobs(ctx, dispatchBatch)
	if err != nil {
		slog.Error("dispatch due jobs", "error", err)
		return
	}
	for _, id := range due {
		if err := producer.PublishJob(ctx, kafkapkg.TopicRun, id); err != nil {
			slog.Error("publish due job", "job_id", id, "error", err)
		}
	}
	if len(due) > 0 {
		slog.Info("dispatched due jobs", "count", len(due))
	}
}
