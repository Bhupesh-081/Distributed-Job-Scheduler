package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DispatchDueJobs atomically claims up to limit queued, undispatched jobs
// whose scheduled_time has passed and marks them dispatched. The caller is
// expected to publish each returned ID to Kafka right after.
func (s *Store) DispatchDueJobs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`WITH due AS (
		   SELECT id FROM jobs
		   WHERE status = 'queued' AND dispatched_at IS NULL AND scheduled_time <= now()
		   ORDER BY scheduled_time
		   LIMIT $1
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE jobs SET dispatched_at = now()
		 FROM due WHERE jobs.id = due.id
		 RETURNING jobs.id`,
		limit,
	)
	return scanIDs(rows, err)
}

// RecoverStuckJobs resets dispatched_at on queued jobs that were dispatched
// more than staleAfter ago but never progressed (crash, lost Kafka
// message), so DispatchDueJobs picks them up again on the next tick.
func (s *Store) RecoverStuckJobs(ctx context.Context, staleAfter time.Duration, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`WITH stuck AS (
		   SELECT id FROM jobs
		   WHERE status = 'queued' AND dispatched_at IS NOT NULL
		     AND modified_time < now() - $1::interval
		   LIMIT $2
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE jobs SET dispatched_at = NULL, modified_time = now()
		 FROM stuck WHERE jobs.id = stuck.id
		 RETURNING jobs.id`,
		staleAfter.String(), limit,
	)
	return scanIDs(rows, err)
}

func scanIDs(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}, err error) ([]uuid.UUID, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
