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
	WorkerID      *uuid.UUID
}

// ClaimJob atomically transitions a queued job to running. The WHERE
// status='queued' guard is what makes duplicate Kafka delivery (or a
// retried claim) safe: only the first caller sees a row come back.
//
// If the job belongs to a queue, the claim is also blocked while the queue
// is paused or already at its concurrency_limit. The job stays 'queued'
// with dispatched_at still set, so watcher-service's stuck-job recovery
// picks it up again once a slot frees (see RecoverStuckJobs); no separate
// backoff/retry path needed here.
func (s *Store) ClaimJob(ctx context.Context, id uuid.UUID) (Job, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx)

	var queueID *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT queue_id FROM jobs WHERE id = $1 AND status = 'queued'`, id).Scan(&queueID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}

	if queueID != nil {
		// Serializes concurrent claims for this queue so the count check
		// below can't race two claims past the limit at once. Advisory
		// locks don't block other queues, so this doesn't become a
		// global bottleneck.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, queueID.String()); err != nil {
			return Job{}, false, err
		}
		var paused bool
		var limit, running int
		err := tx.QueryRow(ctx,
			`SELECT paused, concurrency_limit,
			        (SELECT count(*) FROM jobs WHERE queue_id = $1 AND status = 'running')
			 FROM queues WHERE id = $1`,
			queueID,
		).Scan(&paused, &limit, &running)
		if err != nil {
			return Job{}, false, err
		}
		if paused || running >= limit {
			return Job{}, false, nil
		}
	}

	j, err := scanJob(tx.QueryRow(ctx,
		`UPDATE jobs SET status = 'running', modified_time = now()
		 WHERE id = $1 AND status = 'queued'
		 RETURNING `+jobCols,
		id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, tx.Commit(ctx)
}

func (s *Store) CreateJobRun(ctx context.Context, jobID uuid.UUID, attemptNumber int, workerID uuid.UUID) (JobRun, error) {
	var r JobRun
	err := s.pool.QueryRow(ctx,
		`INSERT INTO job_runs (job_id, status, start_time, attempt_number, worker_id)
		 VALUES ($1, 'running', now(), $2, $3)
		 RETURNING id, job_id, status, start_time, end_time, attempt_number, err_msg, created_at, worker_id`,
		jobID, attemptNumber, workerID,
	).Scan(&r.ID, &r.JobID, &r.Status, &r.StartTime, &r.EndTime, &r.AttemptNumber, &r.ErrMsg, &r.CreatedAt, &r.WorkerID)
	return r, err
}

// TouchRunningJob refreshes a running job's modified_time, called
// periodically by consumer-service while a job executes so
// RecoverStuckRunningJobs can tell "still being worked on" apart from "the
// worker that claimed this is gone."
func (s *Store) TouchRunningJob(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET modified_time = now() WHERE id = $1 AND status = 'running'`, id)
	return err
}

func (s *Store) FinishJobRun(ctx context.Context, runID uuid.UUID, status string, errMsg *string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE job_runs SET status = $2, end_time = now(), err_msg = $3 WHERE id = $1`,
		runID, status, errMsg,
	)
	return err
}

// CancelQueuedJob cancels a job that hasn't been claimed yet. Returns false
// if the job wasn't queued (already claimed/running, or already finished);
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
	return retryOrDeadLetter(ctx, s.pool, id)
}

// retryOrDeadLetter backs both RetryOrDeadLetter (against the pool, from
// consumer-service) and RecoverStuckRunningJobs (against a transaction);
// both need the exact same queued-vs-dead decision.
func retryOrDeadLetter(ctx context.Context, q querier, id uuid.UUID) (retriesCount, retriesMax int, dead bool, err error) {
	var status string
	err = q.QueryRow(ctx,
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
