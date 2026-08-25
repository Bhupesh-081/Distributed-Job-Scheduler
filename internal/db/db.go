// Package db owns the Postgres connection pool and startup schema migration.
package db

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/0001_init.sql
var initSQL string

//go:embed migrations/0002_jobs.sql
var jobsSQL string

//go:embed migrations/0003_jobs_type_status_fix.sql
var jobsTypeStatusFixSQL string

//go:embed migrations/0004_jobs_dispatch.sql
var jobsDispatchSQL string

//go:embed migrations/0005_queues.sql
var queuesSQL string

//go:embed migrations/0006_retry_policies.sql
var retryPoliciesSQL string

//go:embed migrations/0007_workers.sql
var workersSQL string

//go:embed migrations/0008_job_logs.sql
var jobLogsSQL string

//go:embed migrations/0009_dead_letter_queue.sql
var dlqSQL string

//go:embed migrations/0010_scheduled_jobs.sql
var scheduledJobsSQL string

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate applies the schema. Every statement is idempotent (CREATE ... IF
// NOT EXISTS), so this is safe to run on every startup.
//
// ponytail: numbered flat files, no versioning table, applied in full every
// startup. Fine while each file only adds new tables; move to golang-migrate
// (up/down pairs) the moment an existing table needs to change shape.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	for _, stmt := range []string{initSQL, jobsSQL, jobsTypeStatusFixSQL, jobsDispatchSQL, queuesSQL, retryPoliciesSQL, workersSQL, jobLogsSQL, dlqSQL, scheduledJobsSQL} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}
