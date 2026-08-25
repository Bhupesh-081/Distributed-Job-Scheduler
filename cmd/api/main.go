// Command api runs the Distributed Job Scheduler REST API: auth,
// organizations/projects, queues, retry policies, workers, and the dead
// letter queue. Jobs themselves are served by the separate cmd/job-service
// (see docs/architecture.md).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"distributed-job-scheduler/internal/authsvc"
	"distributed-job-scheduler/internal/config"
	"distributed-job-scheduler/internal/db"
	"distributed-job-scheduler/internal/httpapi"
	"distributed-job-scheduler/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	st := store.New(pool)
	tokens := authsvc.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	server := httpapi.NewServer(st, tokens, cfg.RefreshTokenTTL, rdb)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	slog.Info("listening", "port", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
