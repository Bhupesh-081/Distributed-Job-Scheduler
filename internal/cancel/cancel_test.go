package cancel

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set, skipping cancel integration tests")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func TestRequestIsRequestedClear(t *testing.T) {
	rdb := testClient(t)
	ctx := context.Background()
	jobID := uuid.New()

	requested, err := IsRequested(ctx, rdb, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if requested {
		t.Fatal("expected no cancel request initially")
	}

	if err := Request(ctx, rdb, jobID); err != nil {
		t.Fatal(err)
	}
	requested, err = IsRequested(ctx, rdb, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if !requested {
		t.Fatal("expected cancel request to be visible after Request")
	}

	if err := Clear(ctx, rdb, jobID); err != nil {
		t.Fatal(err)
	}
	requested, err = IsRequested(ctx, rdb, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if requested {
		t.Fatal("expected cancel request to be gone after Clear")
	}
}
