package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DeadLetterEntry struct {
	ID           uuid.UUID
	JobID        uuid.UUID
	QueueID      *uuid.UUID
	FinalError   *string
	RetriesCount int
	MovedAt      time.Time
}

const dlqCols = "id, job_id, queue_id, final_error, retries_count, moved_at"

func scanDLQEntry(row pgx.Row) (DeadLetterEntry, error) {
	var e DeadLetterEntry
	err := row.Scan(&e.ID, &e.JobID, &e.QueueID, &e.FinalError, &e.RetriesCount, &e.MovedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeadLetterEntry{}, ErrNotFound
	}
	return e, err
}

// CreateDLQEntry records a job's move to the dead letter queue. queueID is
// a snapshot of the job's queue at the time of dead-lettering (nullable for
// an unscoped job), independent of whether that queue is later renamed or
// deleted.
func (s *Store) CreateDLQEntry(ctx context.Context, jobID uuid.UUID, queueID *uuid.UUID, finalError *string, retriesCount int) (DeadLetterEntry, error) {
	return scanDLQEntry(s.pool.QueryRow(ctx,
		`INSERT INTO dead_letter_queue (job_id, queue_id, final_error, retries_count)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+dlqCols,
		jobID, queueID, finalError, retriesCount,
	))
}

func (s *Store) GetDLQEntry(ctx context.Context, id uuid.UUID) (DeadLetterEntry, error) {
	return scanDLQEntry(s.pool.QueryRow(ctx, `SELECT `+dlqCols+` FROM dead_letter_queue WHERE id = $1`, id))
}

// ListDLQForQueue returns a queue's dead-letter entries newest-first.
func (s *Store) ListDLQForQueue(ctx context.Context, queueID uuid.UUID, limit, offset int) ([]DeadLetterEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+dlqCols+` FROM dead_letter_queue WHERE queue_id = $1 ORDER BY moved_at DESC LIMIT $2 OFFSET $3`,
		queueID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeadLetterEntry
	for rows.Next() {
		e, err := scanDLQEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReplayDLQEntry re-queues the original job (status='queued', retries_count
// reset, re-dispatched immediately) and removes the DLQ entry; a
// successful replay's audit trail is the job's own job_runs/job_logs
// history from here on. Returns the job's ID.
func (s *Store) ReplayDLQEntry(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var jobID uuid.UUID
	if err := tx.QueryRow(ctx, `DELETE FROM dead_letter_queue WHERE id = $1 RETURNING job_id`, id).Scan(&jobID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE jobs SET status = 'queued', retries_count = 0, dispatched_at = NULL, scheduled_time = now(), modified_time = now()
		 WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return uuid.Nil, err
	}
	if tag.RowsAffected() == 0 {
		return uuid.Nil, ErrNotFound
	}
	return jobID, tx.Commit(ctx)
}

func (s *Store) DeleteDLQEntry(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM dead_letter_queue WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
