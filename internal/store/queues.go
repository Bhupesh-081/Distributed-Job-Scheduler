package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Queue struct {
	ID                   uuid.UUID
	ProjectID            uuid.UUID
	Name                 string
	Priority             int
	ConcurrencyLimit     int
	Paused               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DefaultRetryPolicyID *uuid.UUID
}

type NewQueue struct {
	ProjectID        uuid.UUID
	Name             string
	Priority         int
	ConcurrencyLimit int
}

const queueCols = "id, project_id, name, priority, concurrency_limit, paused, created_at, updated_at, default_retry_policy_id"

func scanQueue(row pgx.Row) (Queue, error) {
	var q Queue
	err := row.Scan(&q.ID, &q.ProjectID, &q.Name, &q.Priority, &q.ConcurrencyLimit, &q.Paused, &q.CreatedAt, &q.UpdatedAt, &q.DefaultRetryPolicyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Queue{}, ErrNotFound
	}
	return q, err
}

func (s *Store) CreateQueue(ctx context.Context, q NewQueue) (Queue, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO queues (project_id, name, priority, concurrency_limit)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+queueCols,
		q.ProjectID, q.Name, q.Priority, q.ConcurrencyLimit,
	)
	out, err := scanQueue(row)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Queue{}, ErrConflict
	}
	return out, err
}

func (s *Store) GetQueue(ctx context.Context, id uuid.UUID) (Queue, error) {
	return scanQueue(s.pool.QueryRow(ctx, `SELECT `+queueCols+` FROM queues WHERE id = $1`, id))
}

func (s *Store) ListQueuesForProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]Queue, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+queueCols+` FROM queues WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []Queue
	for rows.Next() {
		q, err := scanQueue(rows)
		if err != nil {
			return nil, err
		}
		queues = append(queues, q)
	}
	return queues, rows.Err()
}

// UpdateQueueConfig updates the mutable queue settings (name, priority,
// concurrency_limit, default_retry_policy_id; nil clears it). Pause/resume
// go through SetQueuePaused instead, since those are a single-field toggle
// callers hit without knowing the rest.
func (s *Store) UpdateQueueConfig(ctx context.Context, id uuid.UUID, name string, priority, concurrencyLimit int, defaultRetryPolicyID *uuid.UUID) (Queue, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE queues SET name = $2, priority = $3, concurrency_limit = $4, default_retry_policy_id = $5, updated_at = now()
		 WHERE id = $1
		 RETURNING `+queueCols,
		id, name, priority, concurrencyLimit, defaultRetryPolicyID,
	)
	out, err := scanQueue(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return Queue{}, ErrConflict
			case "23503":
				return Queue{}, ErrRetryPolicyNotFound
			}
		}
		return Queue{}, err
	}
	return out, nil
}

func (s *Store) SetQueuePaused(ctx context.Context, id uuid.UUID, paused bool) (Queue, error) {
	return scanQueue(s.pool.QueryRow(ctx,
		`UPDATE queues SET paused = $2, updated_at = now() WHERE id = $1 RETURNING `+queueCols,
		id, paused,
	))
}

func (s *Store) DeleteQueue(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM queues WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrConflict
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type QueueStats struct {
	Queued    int
	Running   int
	Success   int
	Failed    int
	Dead      int
	Cancelled int
}

func (s *Store) GetQueueStats(ctx context.Context, id uuid.UUID) (QueueStats, error) {
	var st QueueStats
	err := s.pool.QueryRow(ctx,
		`SELECT
		   count(*) FILTER (WHERE status = 'queued'),
		   count(*) FILTER (WHERE status = 'running'),
		   count(*) FILTER (WHERE status = 'success'),
		   count(*) FILTER (WHERE status = 'failed'),
		   count(*) FILTER (WHERE status = 'dead'),
		   count(*) FILTER (WHERE status = 'cancelled')
		 FROM jobs WHERE queue_id = $1`,
		id,
	).Scan(&st.Queued, &st.Running, &st.Success, &st.Failed, &st.Dead, &st.Cancelled)
	return st, err
}
