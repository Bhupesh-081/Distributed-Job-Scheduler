// Command job-service exposes the REST API for creating and listing jobs
// (see docs/architecture.md). Auth-gated like cmd/api — see "Auth on
// job-service routes" in docs/design-decisions.md's MVP bootstrap ledger —
// sharing the same JWT secret so a token from cmd/api's /auth/login works
// here too.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"distributed-job-scheduler/internal/authsvc"
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
	port := getenv("PORT", "8081")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		return fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}
	// job-service only validates tokens (never issues them), so the TTL
	// argument here is inert — it only affects GenerateAccessToken.
	tokens := authsvc.NewTokenIssuer(jwtSecret, 0)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{Addr: getenv("REDIS_ADDR", "localhost:6379")})
	defer rdb.Close()

	server := httpapi.NewJobServer(store.New(pool), tokens, rdb)
	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	slog.Info("job-service listening", "port", port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
