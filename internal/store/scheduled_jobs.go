package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"distributed-job-scheduler/internal/cronexpr"
)

// ScheduledJob is a recurring-job (cron) definition. watcher-service
// expands it into an ordinary Job on each firing — it's never itself
// dispatched or executed.
type ScheduledJob struct {
	ID             uuid.UUID
	QueueID        uuid.UUID
	Name           string
	CronExpression string
	Payload        json.RawMessage
	RetriesMax     int
	RetryPolicyID  *uuid.UUID
	Active         bool
	NextRunAt      time.Time
	LastRunAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type NewScheduledJob struct {
	QueueID        uuid.UUID
	Name           string
	CronExpression string
	Payload        json.RawMessage
	RetriesMax     int
	RetryPolicyID  *uuid.UUID
	NextRunAt      time.Time
}

const scheduledJobCols = "id, queue_id, name, cron_expression, payload, retries_max, retry_policy_id, active, next_run_at, last_run_at, created_at, updated_at"

func scanScheduledJob(row pgx.Row) (ScheduledJob, error) {
	var sj ScheduledJob
	err := row.Scan(&sj.ID, &sj.QueueID, &sj.Name, &sj.CronExpression, &sj.Payload, &sj.RetriesMax,
		&sj.RetryPolicyID, &sj.Active, &sj.NextRunAt, &sj.LastRunAt, &sj.CreatedAt, &sj.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScheduledJob{}, ErrNotFound
	}
	return sj, err
}

func translateScheduledJobErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503":
			if pgErr.ConstraintName == "scheduled_jobs_retry_policy_id_fkey" {
				return ErrRetryPolicyNotFound
			}
		}
	}
	return err
}

func (s *Store) CreateScheduledJob(ctx context.Context, sj NewScheduledJob) (ScheduledJob, error) {
	out, err := scanScheduledJob(s.pool.QueryRow(ctx,
		`INSERT INTO scheduled_jobs (queue_id, name, cron_expression, payload, retries_max, retry_policy_id, next_run_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+scheduledJobCols,
		sj.QueueID, sj.Name, sj.CronExpression, sj.Payload, sj.RetriesMax, sj.RetryPolicyID, sj.NextRunAt,
	))
	if err != nil {
		return ScheduledJob{}, translateScheduledJobErr(err)
	}
	return out, nil
}

func (s *Store) GetScheduledJob(ctx context.Context, id uuid.UUID) (ScheduledJob, error) {
	return scanScheduledJob(s.pool.QueryRow(ctx, `SELECT `+scheduledJobCols+` FROM scheduled_jobs WHERE id = $1`, id))
}

func (s *Store) ListScheduledJobsForQueue(ctx context.Context, queueID uuid.UUID, limit, offset int) ([]ScheduledJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+scheduledJobCols+` FROM scheduled_jobs WHERE queue_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		queueID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduledJob
	for rows.Next() {
		sj, err := scanScheduledJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sj)
	}
	return out, rows.Err()
}

// UpdateScheduledJob is a full-field replace (name, cron_expression,
// payload, retries_max, retry_policy_id), matching queues/retry-policies'
// PATCH semantics elsewhere in this codebase. nextRunAt is recomputed by
// the caller (httpapi) since cron_expression may have changed.
func (s *Store) UpdateScheduledJob(ctx context.Context, id uuid.UUID, name, cronExpression string, payload json.RawMessage, retriesMax int, retryPolicyID *uuid.UUID, nextRunAt time.Time) (ScheduledJob, error) {
	out, err := scanScheduledJob(s.pool.QueryRow(ctx,
		`UPDATE scheduled_jobs SET name = $2, cron_expression = $3, payload = $4, retries_max = $5, retry_policy_id = $6, next_run_at = $7, updated_at = now()
		 WHERE id = $1
		 RETURNING `+scheduledJobCols,
		id, name, cronExpression, payload, retriesMax, retryPolicyID, nextRunAt,
	))
	if err != nil {
		return ScheduledJob{}, translateScheduledJobErr(err)
	}
	return out, nil
}

func (s *Store) SetScheduledJobActive(ctx context.Context, id uuid.UUID, active bool) (ScheduledJob, error) {
	return scanScheduledJob(s.pool.QueryRow(ctx,
		`UPDATE scheduled_jobs SET active = $2, updated_at = now() WHERE id = $1 RETURNING `+scheduledJobCols,
		id, active,
	))
}

func (s *Store) DeleteScheduledJob(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM scheduled_jobs WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ExpandDueScheduledJobs finds active cron definitions due to fire, and for
// each: spawns a new, ordinary `jobs` row (status='queued',
// scheduled_type='immediate', scheduled_time=now(), scheduled_job_id set
// to trace it back) and advances next_run_at — all in one transaction per
// batch, so a crash between the two can't happen. The spawned job then
// flows through the exact same dispatch/claim/execute/retry/DLQ pipeline
// as any other job; nothing downstream needs to know it came from a cron
// definition.
//
// ponytail: a cron expression is validated at create/update time, so
// Next() failing here should never happen in practice. If it somehow does,
// the whole batch's transaction is rolled back (nothing spawned, nothing
// advanced) rather than silently skipping just that row — simpler to
// reason about, and the failing definition will surface the same error
// again next tick until someone fixes it, rather than being silently
// skipped forever.
func (s *Store) ExpandDueScheduledJobs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, queue_id, name, cron_expression, payload, retries_max, retry_policy_id
		 FROM scheduled_jobs
		 WHERE active AND next_run_at <= now()
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	type due struct {
		id, queueID    uuid.UUID
		name, cronExpr string
		payload        json.RawMessage
		retriesMax     int
		retryPolicyID  *uuid.UUID
	}
	var dueRows []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.queueID, &d.name, &d.cronExpr, &d.payload, &d.retriesMax, &d.retryPolicyID); err != nil {
			rows.Close()
			return nil, err
		}
		dueRows = append(dueRows, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	spawned := make([]uuid.UUID, 0, len(dueRows))
	now := time.Now()
	for _, d := range dueRows {
		next, err := cronexpr.Next(d.cronExpr, now)
		if err != nil {
			return nil, err
		}

		var jobID uuid.UUID
		err = tx.QueryRow(ctx,
			`INSERT INTO jobs (name, scheduled_type, scheduled_time, payload, retries_max, queue_id, retry_policy_id, scheduled_job_id)
			 VALUES ($1, 'immediate', now(), $2, $3, $4, $5, $6)
			 RETURNING id`,
			d.name, d.payload, d.retriesMax, d.queueID, d.retryPolicyID, d.id,
		).Scan(&jobID)
		if err != nil {
			return nil, err
		}
		spawned = append(spawned, jobID)

		if _, err := tx.Exec(ctx,
			`UPDATE scheduled_jobs SET next_run_at = $2, last_run_at = now(), updated_at = now() WHERE id = $1`,
			d.id, next,
		); err != nil {
			return nil, err
		}
	}
	return spawned, tx.Commit(ctx)
}
