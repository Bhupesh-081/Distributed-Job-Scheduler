package heartbeat

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestIsAlive(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		lastPoll time.Time
		want     bool
	}{
		{"just polled", now, true},
		{"29s ago", now.Add(-29 * time.Second), true},
		{"exactly at threshold", now.Add(-StaleAfter), true},
		{"31s ago", now.Add(-31 * time.Second), false},
		{"long dead", now.Add(-time.Hour), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isAlive(c.lastPoll, now); got != c.want {
				t.Fatalf("isAlive(%v) = %v, want %v", c.lastPoll, got, c.want)
			}
		})
	}
}

func testClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set, skipping heartbeat integration tests")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func TestReportAliveThenGet(t *testing.T) {
	rdb := testClient(t)
	ctx := context.Background()

	if err := rdb.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}

	status, err := Get(ctx, rdb)
	if err != nil {
		t.Fatal(err)
	}
	if status.Alive {
		t.Fatal("expected not alive before any ReportAlive")
	}

	if err := ReportAlive(ctx, rdb); err != nil {
		t.Fatal(err)
	}
	status, err = Get(ctx, rdb)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Alive {
		t.Fatal("expected alive right after ReportAlive")
	}
	if status.LastPollAt.IsZero() {
		t.Fatal("expected a non-zero LastPollAt")
	}
}
