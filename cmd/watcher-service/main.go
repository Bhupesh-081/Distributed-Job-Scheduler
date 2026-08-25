// Command watcher-service polls Postgres for due jobs to publish onto the
// Kafka "run" topic (delayed/scheduled jobs whose time has come, and
// immediate jobs job-service already inserted as due-now), expands due
// scheduled_jobs (cron) definitions into new ordinary jobs, and resets
// jobs stuck mid-dispatch (crash, lost Kafka message) or mid-execution
// (crashed worker) for redispatch. It also reaps stale consumer-service
// workers, and reports its own liveness to Redis (see internal/heartbeat)
// so cmd/api's health endpoint can tell whether this single-instance
// service is still ticking. See docs/architecture.md.
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

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"distributed-job-scheduler/internal/db"
	"distributed-job-scheduler/internal/heartbeat"
	kafkapkg "distributed-job-scheduler/internal/kafka"
	"distributed-job-scheduler/internal/store"
)

const (
	pollInterval  = time.Second // NFR: a job starts within 2s of its scheduled time.
	dispatchBatch = 500
	recoverBatch  = 100
	// workerStaleAfter is 3x consumer-service's heartbeat interval (10s):
	// long enough that one missed beat doesn't flag a live worker as dead.
	workerStaleAfter = 30 * time.Second
	reapBatch        = 50
	cronBatch        = 100
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

	rdb := redis.NewClient(&redis.Options{Addr: getenv("REDIS_ADDR", "localhost:6379")})
	defer rdb.Close()

	slog.Info("watcher-service running", "poll_interval", pollInterval, "stuck_job_threshold", staleAfter, "brokers", brokers)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("watcher-service stopped")
			return nil
		case <-ticker.C:
			if err := heartbeat.ReportAlive(ctx, rdb); err != nil {
				slog.Error("report heartbeat", "error", err)
			}
			tick(ctx, st, producer, staleAfter)
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func tick(ctx context.Context, st *store.Store, producer *kafkapkg.Producer, staleAfter time.Duration) {
	recovered, err := st.RecoverStuckJobs(ctx, staleAfter, recoverBatch)
	if err != nil {
		slog.Error("recover stuck jobs", "error", err)
	} else if len(recovered) > 0 {
		slog.Warn("recovered stuck jobs", "count", len(recovered), "job_ids", recovered)
	}

	if recoveredRunning, err := st.RecoverStuckRunningJobs(ctx, staleAfter, recoverBatch); err != nil {
		slog.Error("recover stuck running jobs", "error", err)
	} else if len(recoveredRunning) > 0 {
		var requeued, dead []uuid.UUID
		for _, r := range recoveredRunning {
			if r.Dead {
				dead = append(dead, r.JobID)
			} else {
				requeued = append(requeued, r.JobID)
			}
		}
		slog.Warn("recovered jobs stuck running (crashed worker)", "requeued", requeued, "dead_lettered", dead)
	}

	if reaped, err := st.ReapStaleWorkers(ctx, workerStaleAfter, reapBatch); err != nil {
		slog.Error("reap stale workers", "error", err)
	} else if len(reaped) > 0 {
		slog.Warn("reaped stale workers", "count", len(reaped), "worker_ids", reaped)
	}

	// Expanded before DispatchDueJobs, in the same tick, so a freshly
	// spawned cron firing (scheduled_time=now()) gets dispatched to Kafka
	// immediately rather than waiting a full extra tick.
	if spawned, err := st.ExpandDueScheduledJobs(ctx, cronBatch); err != nil {
		slog.Error("expand scheduled jobs", "error", err)
	} else if len(spawned) > 0 {
		slog.Info("expanded scheduled jobs", "count", len(spawned), "job_ids", spawned)
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
