package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DispatchDueJobs atomically claims up to limit queued, undispatched jobs
// whose scheduled_time has passed and marks them dispatched. The caller is
// expected to publish each returned ID to Kafka right after.
//
// Jobs in a paused queue are skipped (left undispatched, so they're
// reconsidered every tick until the queue resumes). Ordering is by queue
// priority (highest first, unscoped jobs treated as priority 0) then
// scheduled_time, so when there's more due work than one tick's limit,
// higher-priority queues get published to Kafka first.
func (s *Store) DispatchDueJobs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`WITH due AS (
		   SELECT j.id FROM jobs j
		   LEFT JOIN queues q ON q.id = j.queue_id
		   WHERE j.status = 'queued' AND j.dispatched_at IS NULL AND j.scheduled_time <= now()
		     AND coalesce(q.paused, false) = false
		   ORDER BY coalesce(q.priority, 0) DESC, j.scheduled_time
		   LIMIT $1
		   FOR UPDATE OF j SKIP LOCKED
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

// RecoveredRun is one job RecoverStuckRunningJobs found and handled.
type RecoveredRun struct {
	JobID uuid.UUID
	Dead  bool // true if this exhausted retries and was dead-lettered
}

// RecoverStuckRunningJobs finds jobs stuck in status='running' whose worker
// presumably crashed (no progress in staleAfter) — the gap RecoverStuckJobs
// doesn't cover, since that one only ever matches status='queued'.
// "No progress" means jobs.modified_time, which consumer-service refreshes
// every 2s while a job actively executes (TouchRunningJob) — this is what
// keeps a long-running-but-healthy job from being mistaken for a crashed
// one; without that periodic touch, modified_time would be frozen at claim
// time and any payload running longer than staleAfter would look
// identical to an abandoned job. A crash is treated exactly like a genuine
// execution failure: the orphaned job_runs row is closed out and
// retryOrDeadLetter's usual queued-vs-dead decision applies, so a payload
// that reliably kills its worker (OOM, etc.) still dead-letters eventually
// instead of retrying forever. On requeue, dispatched_at is cleared so
// this same watcher-service tick's DispatchDueJobs picks it back up
// immediately, same pattern as ExpandDueScheduledJobs.
func (s *Store) RecoverStuckRunningJobs(ctx context.Context, staleAfter time.Duration, limit int) ([]RecoveredRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, queue_id FROM jobs
		 WHERE status = 'running' AND modified_time < now() - $1::interval
		 LIMIT $2
		 FOR UPDATE SKIP LOCKED`,
		staleAfter.String(), limit,
	)
	if err != nil {
		return nil, err
	}
	type stuck struct {
		id      uuid.UUID
		queueID *uuid.UUID
	}
	var stuckRows []stuck
	for rows.Next() {
		var st stuck
		if err := rows.Scan(&st.id, &st.queueID); err != nil {
			rows.Close()
			return nil, err
		}
		stuckRows = append(stuckRows, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	const crashMsg = "worker crashed or was lost mid-execution (recovered by watcher-service)"

	out := make([]RecoveredRun, 0, len(stuckRows))
	for _, st := range stuckRows {
		var jobRunID *uuid.UUID
		err := tx.QueryRow(ctx,
			`UPDATE job_runs SET status = 'failed', end_time = now(), err_msg = $2
			 WHERE job_id = $1 AND status = 'running'
			 RETURNING id`,
			st.id, crashMsg,
		).Scan(&jobRunID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO job_logs (job_id, job_run_id, level, message) VALUES ($1, $2, 'error', $3)`,
			st.id, jobRunID, crashMsg,
		); err != nil {
			return nil, err
		}

		retriesCount, _, dead, err := retryOrDeadLetter(ctx, tx, st.id)
		if err != nil {
			return nil, err
		}

		if dead {
			if _, err := tx.Exec(ctx,
				`INSERT INTO dead_letter_queue (job_id, queue_id, final_error, retries_count) VALUES ($1, $2, $3, $4)`,
				st.id, st.queueID, crashMsg, retriesCount,
			); err != nil {
				return nil, err
			}
		} else if _, err := tx.Exec(ctx, `UPDATE jobs SET dispatched_at = NULL WHERE id = $1`, st.id); err != nil {
			return nil, err
		}

		out = append(out, RecoveredRun{JobID: st.id, Dead: dead})
	}
	return out, tx.Commit(ctx)
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
