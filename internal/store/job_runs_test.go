package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testStore connects to DATABASE_URL for a real integration test. Skipped
// when it's unset (e.g. CI without Postgres) so `go test ./...` still
// passes without a database.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set, skipping store integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return New(pool)
}

func TestClaimJobIsAtomic(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, NewJob{
		Name: "claim-test", ScheduledType: "immediate", Payload: json.RawMessage(`{}`), RetriesMax: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed1, ok1, err := s.ClaimJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok1 || claimed1.Status != "running" {
		t.Fatalf("expected first claim to succeed, got ok=%v status=%s", ok1, claimed1.Status)
	}

	_, ok2, err := s.ClaimJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok2 {
		t.Fatal("expected second claim of an already-running job to fail")
	}
}

func TestRetryOrDeadLetter(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, NewJob{
		Name: "retry-test", ScheduledType: "immediate", Payload: json.RawMessage(`{}`), RetriesMax: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	count, max, dead, err := s.RetryOrDeadLetter(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || max != 1 || dead {
		t.Fatalf("attempt 1: got count=%d max=%d dead=%v, want count=1 max=1 dead=false", count, max, dead)
	}

	count, _, dead, err = s.RetryOrDeadLetter(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || !dead {
		t.Fatalf("attempt 2: got count=%d dead=%v, want count=2 dead=true", count, dead)
	}

	got, err := s.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "dead" {
		t.Fatalf("expected job status dead, got %s", got.Status)
	}
}

func TestJobRunLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	job, err := s.CreateJob(ctx, NewJob{
		Name: "run-test", ScheduledType: "immediate", Payload: json.RawMessage(`{}`), RetriesMax: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	run, err := s.CreateJobRun(ctx, job.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "running" || run.AttemptNumber != 1 {
		t.Fatalf("unexpected run: %+v", run)
	}

	errMsg := "boom"
	if err := s.FinishJobRun(ctx, run.ID, "failed", &errMsg); err != nil {
		t.Fatal(err)
	}
}
