package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Script is a reusable, project-scoped snippet - the job-creation form's
// "load from library" picker reads from these instead of the caller
// retyping the same code into the inline editor every time.
type Script struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Name       string
	ScriptType string
	Content    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type NewScript struct {
	ProjectID  uuid.UUID
	Name       string
	ScriptType string
	Content    string
}

const scriptCols = "id, project_id, name, script_type, content, created_at, updated_at"

func scanScript(row pgx.Row) (Script, error) {
	var s Script
	err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &s.ScriptType, &s.Content, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Script{}, ErrNotFound
	}
	return s, err
}

func (s *Store) CreateScript(ctx context.Context, n NewScript) (Script, error) {
	out, err := scanScript(s.pool.QueryRow(ctx,
		`INSERT INTO scripts (project_id, name, script_type, content)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+scriptCols,
		n.ProjectID, n.Name, n.ScriptType, n.Content,
	))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Script{}, ErrConflict
	}
	return out, err
}

func (s *Store) GetScript(ctx context.Context, id uuid.UUID) (Script, error) {
	return scanScript(s.pool.QueryRow(ctx, `SELECT `+scriptCols+` FROM scripts WHERE id = $1`, id))
}

func (s *Store) ListScriptsForProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]Script, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+scriptCols+` FROM scripts WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Script
	for rows.Next() {
		sc, err := scanScript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *Store) UpdateScript(ctx context.Context, id uuid.UUID, name, scriptType, content string) (Script, error) {
	out, err := scanScript(s.pool.QueryRow(ctx,
		`UPDATE scripts SET name = $2, script_type = $3, content = $4, updated_at = now()
		 WHERE id = $1
		 RETURNING `+scriptCols,
		id, name, scriptType, content,
	))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Script{}, ErrConflict
	}
	return out, err
}

func (s *Store) DeleteScript(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM scripts WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
