// Package store holds all SQL access, keyed by entity.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrRetryPolicyNotFound = errors.New("retry policy not found")

type Store struct {
	pool *pgxpool.Pool
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, so a handful of
// helpers (e.g. retryOrDeadLetter) can run either directly against the
// pool or inside a caller's transaction without duplicating SQL.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}
