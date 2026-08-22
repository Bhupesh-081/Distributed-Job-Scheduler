package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Organization struct {
	ID          uuid.UUID
	Name        string
	OwnerUserID uuid.UUID
	CreatedAt   time.Time
}

type OrgMember struct {
	OrgID  uuid.UUID
	UserID uuid.UUID
	Role   string
}

// CreateOrganization creates the org and adds the creator as its owner member
// in one transaction, so an org can never exist without an owner membership.
func (s *Store) CreateOrganization(ctx context.Context, name string, ownerUserID uuid.UUID) (Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer tx.Rollback(ctx)

	var org Organization
	err = tx.QueryRow(ctx,
		`INSERT INTO organizations (name, owner_user_id) VALUES ($1, $2)
		 RETURNING id, name, owner_user_id, created_at`,
		name, ownerUserID,
	).Scan(&org.ID, &org.Name, &org.OwnerUserID, &org.CreatedAt)
	if err != nil {
		return Organization{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
		org.ID, ownerUserID,
	)
	if err != nil {
		return Organization{}, err
	}

	return org, tx.Commit(ctx)
}

func (s *Store) GetOrganization(ctx context.Context, id uuid.UUID) (Organization, error) {
	var org Organization
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, owner_user_id, created_at FROM organizations WHERE id = $1`,
		id,
	).Scan(&org.ID, &org.Name, &org.OwnerUserID, &org.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

// ListOrganizationsForUser returns orgs the user is a member of, newest first.
func (s *Store) ListOrganizationsForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Organization, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT o.id, o.name, o.owner_user_id, o.created_at
		 FROM organizations o
		 JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1
		 ORDER BY o.created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.OwnerUserID, &org.CreatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

func (s *Store) UpdateOrganizationName(ctx context.Context, id uuid.UUID, name string) (Organization, error) {
	var org Organization
	err := s.pool.QueryRow(ctx,
		`UPDATE organizations SET name = $2 WHERE id = $1
		 RETURNING id, name, owner_user_id, created_at`,
		id, name,
	).Scan(&org.ID, &org.Name, &org.OwnerUserID, &org.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (s *Store) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMemberRole returns the caller's role in the org, or ErrNotFound if not a member.
func (s *Store) GetMemberRole(ctx context.Context, orgID, userID uuid.UUID) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM org_members WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

func (s *Store) AddOrgMember(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)`,
		orgID, userID, role,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
