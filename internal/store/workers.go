package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Worker struct {
	ID              uuid.UUID
	Hostname        string
	PID             int
	Concurrency     int
	Status          string
	StartedAt       time.Time
	StoppedAt       *time.Time
	LastHeartbeatAt time.Time
}

const workerCols = "id, hostname, pid, concurrency, status, started_at, stopped_at, last_heartbeat_at"

func scanWorker(row pgx.Row) (Worker, error) {
	var w Worker
	err := row.Scan(&w.ID, &w.Hostname, &w.PID, &w.Concurrency, &w.Status, &w.StartedAt, &w.StoppedAt, &w.LastHeartbeatAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Worker{}, ErrNotFound
	}
	return w, err
}

// RegisterWorker records a new consumer-service instance. id is generated
// by the caller (one per process lifetime), not the DB, so job_runs can
// reference it before the row necessarily commits.
func (s *Store) RegisterWorker(ctx context.Context, id uuid.UUID, hostname string, pid, concurrency int) (Worker, error) {
	return scanWorker(s.pool.QueryRow(ctx,
		`INSERT INTO workers (id, hostname, pid, concurrency)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+workerCols,
		id, hostname, pid, concurrency,
	))
}

// Heartbeat updates the worker's liveness timestamp and appends a row to
// the heartbeat history (in-flight job count at the time of the beat).
func (s *Store) Heartbeat(ctx context.Context, id uuid.UUID, inFlight int) error {
	if _, err := s.pool.Exec(ctx, `UPDATE workers SET last_heartbeat_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO worker_heartbeats (worker_id, in_flight_count) VALUES ($1, $2)`,
		id, inFlight,
	)
	return err
}

func (s *Store) MarkWorkerStopped(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE workers SET status = 'stopped', stopped_at = now() WHERE id = $1`,
		id,
	)
	return err
}

// ReapStaleWorkers marks workers 'stopped' whose last heartbeat is older
// than staleAfter, catching a crashed process that never got to run its
// graceful-shutdown MarkWorkerStopped call. Mirrors RecoverStuckJobs: called
// from watcher-service's existing poll tick, no separate reaper needed.
func (s *Store) ReapStaleWorkers(ctx context.Context, staleAfter time.Duration, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`WITH stale AS (
		   SELECT id FROM workers
		   WHERE status = 'active' AND last_heartbeat_at < now() - $1::interval
		   LIMIT $2
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE workers SET status = 'stopped', stopped_at = now()
		 FROM stale WHERE workers.id = stale.id
		 RETURNING workers.id`,
		staleAfter.String(), limit,
	)
	return scanIDs(rows, err)
}

func (s *Store) GetWorker(ctx context.Context, id uuid.UUID) (Worker, error) {
	return scanWorker(s.pool.QueryRow(ctx, `SELECT `+workerCols+` FROM workers WHERE id = $1`, id))
}

// ListWorkers returns workers newest-first, optionally filtered by status.
func (s *Store) ListWorkers(ctx context.Context, status string, limit, offset int) ([]Worker, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+workerCols+` FROM workers
		 WHERE ($1 = '' OR status = $1)
		 ORDER BY started_at DESC LIMIT $2 OFFSET $3`,
		status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Worker
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

type WorkerHeartbeat struct {
	HeartbeatAt   time.Time
	InFlightCount int
}

// ListWorkerHeartbeats returns a worker's most recent heartbeats, newest first.
func (s *Store) ListWorkerHeartbeats(ctx context.Context, workerID uuid.UUID, limit int) ([]WorkerHeartbeat, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT heartbeat_at, in_flight_count FROM worker_heartbeats
		 WHERE worker_id = $1 ORDER BY heartbeat_at DESC LIMIT $2`,
		workerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkerHeartbeat
	for rows.Next() {
		var h WorkerHeartbeat
		if err := rows.Scan(&h.HeartbeatAt, &h.InFlightCount); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
