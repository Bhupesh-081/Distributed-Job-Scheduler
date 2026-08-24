package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type JobRun struct {
	ID            uuid.UUID
	JobID         uuid.UUID
	Status        string
	StartTime     *time.Time
	EndTime       *time.Time
	AttemptNumber int
	ErrMsg        *string
	CreatedAt     time.Time
}

// ClaimJob atomically transitions a queued job to running. The WHERE
// status='queued' guard is what makes duplicate Kafka delivery (or a
// retried claim) safe: only the first caller sees a row come back.
func (s *Store) ClaimJob(ctx context.Context, id uuid.UUID) (Job, bool, error) {
	var j Job
	err := s.pool.QueryRow(ctx,
		`UPDATE jobs SET status = 'running', modified_time = now()
		 WHERE id = $1 AND status = 'queued'
		 RETURNING id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at`,
		id,
	).Scan(&j.ID, &j.Name, &j.ScheduledType, &j.Status, &j.ScheduledTime, &j.CronExpression,
		&j.Payload, &j.RetriesCount, &j.RetriesMax, &j.ModifiedTime, &j.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

func (s *Store) CreateJobRun(ctx context.Context, jobID uuid.UUID, attemptNumber int) (JobRun, error) {
	var r JobRun
	err := s.pool.QueryRow(ctx,
		`INSERT INTO job_runs (job_id, status, start_time, attempt_number)
		 VALUES ($1, 'running', now(), $2)
		 RETURNING id, job_id, status, start_time, end_time, attempt_number, err_msg, created_at`,
		jobID, attemptNumber,
	).Scan(&r.ID, &r.JobID, &r.Status, &r.StartTime, &r.EndTime, &r.AttemptNumber, &r.ErrMsg, &r.CreatedAt)
	return r, err
}

func (s *Store) FinishJobRun(ctx context.Context, runID uuid.UUID, status string, errMsg *string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE job_runs SET status = $2, end_time = now(), err_msg = $3 WHERE id = $1`,
		runID, status, errMsg,
	)
	return err
}

// CancelQueuedJob cancels a job that hasn't been claimed yet. Returns false
// if the job wasn't queued (already claimed/running, or already finished) —
// the caller falls back to a Redis cancel signal for the running case.
func (s *Store) CancelQueuedJob(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status = 'cancelled', modified_time = now() WHERE id = $1 AND status = 'queued'`,
		id,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) SetJobStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status = $2, modified_time = now() WHERE id = $1`,
		id, status,
	)
	return err
}

// RetryOrDeadLetter increments retries_count and either requeues the job
// (status='queued', a caller-side republish to the Kafka retry topic makes
// it run again) or dead-letters it (status='dead') if retries are
// exhausted, in one atomic statement.
func (s *Store) RetryOrDeadLetter(ctx context.Context, id uuid.UUID) (retriesCount, retriesMax int, dead bool, err error) {
	var status string
	err = s.pool.QueryRow(ctx,
		`UPDATE jobs SET
		   retries_count = retries_count + 1,
		   status = CASE WHEN retries_count + 1 > retries_max THEN 'dead' ELSE 'queued' END,
		   modified_time = now()
		 WHERE id = $1
		 RETURNING retries_count, retries_max, status`,
		id,
	).Scan(&retriesCount, &retriesMax, &status)
	return retriesCount, retriesMax, status == "dead", err
}
