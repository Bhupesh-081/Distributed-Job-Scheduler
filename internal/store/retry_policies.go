package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RetryPolicy configures the delay before a failed job's next attempt.
// Strategy is one of "fixed", "linear", "exponential"; BaseDelaySeconds is
// the per-strategy unit (the flat delay for fixed, the per-attempt
// increment for linear, the base for exponential); MaxDelaySeconds caps
// linear/exponential growth.
type RetryPolicy struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	Name             string
	Strategy         string
	BaseDelaySeconds int
	MaxDelaySeconds  *int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Delay computes the wait before retry attempt number `attempt` (1-indexed:
// the first retry after the original failure is attempt 1).
func (p RetryPolicy) Delay(attempt int) time.Duration {
	secs := p.BaseDelaySeconds
	switch p.Strategy {
	case "linear":
		secs = p.BaseDelaySeconds * attempt
	case "exponential":
		// ponytail: clamp to avoid int overflow on a pathological retries_max; MaxDelaySeconds is the real cap in practice.
		exp := min(max(attempt-1, 0), 30)
		secs = p.BaseDelaySeconds * (1 << exp)
	}
	if p.MaxDelaySeconds != nil && secs > *p.MaxDelaySeconds {
		secs = *p.MaxDelaySeconds
	}
	return time.Duration(secs) * time.Second
}

type NewRetryPolicy struct {
	ProjectID        uuid.UUID
	Name             string
	Strategy         string
	BaseDelaySeconds int
	MaxDelaySeconds  *int
}

const retryPolicyCols = "id, project_id, name, strategy, base_delay_seconds, max_delay_seconds, created_at, updated_at"

func scanRetryPolicy(row pgx.Row) (RetryPolicy, error) {
	var p RetryPolicy
	err := row.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Strategy, &p.BaseDelaySeconds, &p.MaxDelaySeconds, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RetryPolicy{}, ErrNotFound
	}
	return p, err
}

func (s *Store) CreateRetryPolicy(ctx context.Context, p NewRetryPolicy) (RetryPolicy, error) {
	out, err := scanRetryPolicy(s.pool.QueryRow(ctx,
		`INSERT INTO retry_policies (project_id, name, strategy, base_delay_seconds, max_delay_seconds)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+retryPolicyCols,
		p.ProjectID, p.Name, p.Strategy, p.BaseDelaySeconds, p.MaxDelaySeconds,
	))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return RetryPolicy{}, ErrConflict
	}
	return out, err
}

func (s *Store) GetRetryPolicy(ctx context.Context, id uuid.UUID) (RetryPolicy, error) {
	return scanRetryPolicy(s.pool.QueryRow(ctx, `SELECT `+retryPolicyCols+` FROM retry_policies WHERE id = $1`, id))
}

func (s *Store) ListRetryPoliciesForProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]RetryPolicy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+retryPolicyCols+` FROM retry_policies WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RetryPolicy
	for rows.Next() {
		p, err := scanRetryPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateRetryPolicy(ctx context.Context, id uuid.UUID, name, strategy string, baseDelaySeconds int, maxDelaySeconds *int) (RetryPolicy, error) {
	out, err := scanRetryPolicy(s.pool.QueryRow(ctx,
		`UPDATE retry_policies SET name = $2, strategy = $3, base_delay_seconds = $4, max_delay_seconds = $5, updated_at = now()
		 WHERE id = $1
		 RETURNING `+retryPolicyCols,
		id, name, strategy, baseDelaySeconds, maxDelaySeconds,
	))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return RetryPolicy{}, ErrConflict
	}
	return out, err
}

func (s *Store) DeleteRetryPolicy(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM retry_policies WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EffectiveRetryPolicy resolves the policy that governs a job's retries: the
// job's own retry_policy_id if set, else its queue's default_retry_policy_id.
// ok is false when neither is set (or the job is unscoped), meaning the
// caller should fall back to its own hardcoded default.
func (s *Store) EffectiveRetryPolicy(ctx context.Context, jobID uuid.UUID) (RetryPolicy, bool, error) {
	var id, projectID *uuid.UUID
	var name, strategy *string
	var baseDelay, maxDelay *int
	var createdAt, updatedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT rp.id, rp.project_id, rp.name, rp.strategy, rp.base_delay_seconds, rp.max_delay_seconds, rp.created_at, rp.updated_at
		 FROM jobs j
		 LEFT JOIN queues q ON q.id = j.queue_id
		 LEFT JOIN retry_policies rp ON rp.id = coalesce(j.retry_policy_id, q.default_retry_policy_id)
		 WHERE j.id = $1`,
		jobID,
	).Scan(&id, &projectID, &name, &strategy, &baseDelay, &maxDelay, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RetryPolicy{}, false, nil
	}
	if err != nil {
		return RetryPolicy{}, false, err
	}
	if id == nil {
		return RetryPolicy{}, false, nil
	}
	return RetryPolicy{
		ID: *id, ProjectID: *projectID, Name: *name, Strategy: *strategy,
		BaseDelaySeconds: *baseDelay, MaxDelaySeconds: maxDelay,
		CreatedAt: *createdAt, UpdatedAt: *updatedAt,
	}, true, nil
}
