package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Project struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	Name      string
	CreatedAt time.Time
}

func (s *Store) CreateProject(ctx context.Context, orgID uuid.UUID, name string) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`INSERT INTO projects (org_id, name) VALUES ($1, $2)
		 RETURNING id, org_id, name, created_at`,
		orgID, name,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Project{}, ErrConflict
		}
		return Project{}, err
	}
	return p, nil
}

func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, created_at FROM projects WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (s *Store) ListProjectsForOrg(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, name, created_at FROM projects
		 WHERE org_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		orgID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateProjectName(ctx context.Context, id uuid.UUID, name string) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`UPDATE projects SET name = $2 WHERE id = $1
		 RETURNING id, org_id, name, created_at`,
		id, name,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (s *Store) DeleteProject(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
