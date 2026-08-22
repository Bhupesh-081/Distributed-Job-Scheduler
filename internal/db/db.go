// Package db owns the Postgres connection pool and startup schema migration.
package db

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/0001_init.sql
var initSQL string

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate applies the schema. Every statement is idempotent (CREATE ... IF
// NOT EXISTS), so this is safe to run on every startup.
//
// ponytail: single flat migration file, no versioning table. Fine while
// there's one schema revision; move to numbered up/down files (or
// golang-migrate) the moment an existing table needs to change shape.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, initSQL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
