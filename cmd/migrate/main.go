// Command migrate applies the Postgres schema (internal/db.Migrate)
// standalone, without booting a full service — used by `make migrate` and
// by checkpoints that need the schema present before any service runs.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"distributed-job-scheduler/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	slog.Info("migrated")
	return nil
}
