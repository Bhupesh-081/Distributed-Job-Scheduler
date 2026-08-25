package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Job struct {
	ID             uuid.UUID
	Name           string
	ScheduledType  string
	Status         string
	ScheduledTime  *time.Time
	CronExpression *string
	Payload        json.RawMessage
	RetriesCount   int
	RetriesMax     int
	ModifiedTime   time.Time
	CreatedAt      time.Time
	QueueID        *uuid.UUID
	RetryPolicyID  *uuid.UUID
	// ScheduledJobID traces this job back to the cron definition that
	// spawned it (nil for immediate/delayed/scheduled jobs and any job
	// created before scheduled_jobs existed). Only ExpandDueScheduledJobs
	// sets it — not exposed on NewJob, since a caller creating a job
	// directly can't claim to be a cron firing.
	ScheduledJobID *uuid.UUID
}

type NewJob struct {
	Name           string
	ScheduledType  string
	ScheduledTime  *time.Time
	CronExpression *string
	Payload        json.RawMessage
	RetriesMax     int
	// QueueID is nullable at the store layer (legacy/internal callers may
	// leave a job unscoped); job-service's HTTP layer requires it on every
	// request it accepts.
	QueueID *uuid.UUID
	// RetryPolicyID overrides the queue's default_retry_policy_id when set.
	RetryPolicyID *uuid.UUID
}

const jobCols = "id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at, queue_id, retry_policy_id, scheduled_job_id"

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Name, &j.ScheduledType, &j.Status, &j.ScheduledTime, &j.CronExpression,
		&j.Payload, &j.RetriesCount, &j.RetriesMax, &j.ModifiedTime, &j.CreatedAt, &j.QueueID, &j.RetryPolicyID, &j.ScheduledJobID)
	return j, err
}

// ErrQueueNotFound is returned when a job references a queue_id that
// doesn't exist (FK violation on jobs.queue_id).
var ErrQueueNotFound = errors.New("queue not found")

func translateJobFK(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		switch pgErr.ConstraintName {
		case "jobs_queue_id_fkey":
			return ErrQueueNotFound
		case "jobs_retry_policy_id_fkey":
			return ErrRetryPolicyNotFound
		}
	}
	return err
}

func (s *Store) CreateJob(ctx context.Context, j NewJob) (Job, error) {
	out, err := scanJob(s.pool.QueryRow(ctx,
		`INSERT INTO jobs (name, scheduled_type, scheduled_time, cron_expression, payload, retries_max, queue_id, retry_policy_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+jobCols,
		j.Name, j.ScheduledType, j.ScheduledTime, j.CronExpression, j.Payload, j.RetriesMax, j.QueueID, j.RetryPolicyID,
	))
	if err != nil {
		return Job{}, translateJobFK(err)
	}
	return out, nil
}

// CreateJobsBatch inserts all jobs in one transaction: all-or-nothing, so a
// batch request never leaves a partial set of jobs behind.
func (s *Store) CreateJobsBatch(ctx context.Context, jobs []NewJob) ([]Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	out := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		row, err := scanJob(tx.QueryRow(ctx,
			`INSERT INTO jobs (name, scheduled_type, scheduled_time, cron_expression, payload, retries_max, queue_id, retry_policy_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING `+jobCols,
			j.Name, j.ScheduledType, j.ScheduledTime, j.CronExpression, j.Payload, j.RetriesMax, j.QueueID, j.RetryPolicyID,
		))
		if err != nil {
			return nil, translateJobFK(err)
		}
		out = append(out, row)
	}
	return out, tx.Commit(ctx)
}

func (s *Store) GetJob(ctx context.Context, id uuid.UUID) (Job, error) {
	out, err := scanJob(s.pool.QueryRow(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return out, err
}

// ListJobs returns jobs newest-first, optionally filtered by status and/or queue.
func (s *Store) ListJobs(ctx context.Context, status string, queueID *uuid.UUID, limit, offset int) ([]Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+jobCols+` FROM jobs
		 WHERE ($1 = '' OR status = $1) AND ($2::uuid IS NULL OR queue_id = $2)
		 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		status, queueID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
