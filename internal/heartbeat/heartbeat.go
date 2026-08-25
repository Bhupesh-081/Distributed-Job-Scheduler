// Package heartbeat tracks watcher-service's own liveness in Redis: a
// single "watcher:last_poll" key, refreshed every poll tick, read by
// cmd/api's health endpoint. Separate from the Postgres-backed
// workers/worker_heartbeats tables (see internal/store), which track
// consumer-service instances, not watcher-service itself — watcher-service
// runs as a single instance today (no leader election, see
// docs/design-decisions.md), so Postgres round-trip cost for its own
// liveness isn't worth it; Redis is already the fast-path signal for
// exactly this kind of disposable liveness data in this codebase (see
// internal/cancel).
package heartbeat

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const key = "watcher:last_poll"

// StaleAfter is how long since the last reported poll before the watcher
// is considered dead. 3x watcher-service's 1s poll interval would be
// plenty; 30s matches the same threshold used for stuck-job recovery and
// worker-heartbeat staleness elsewhere, so one number to remember.
const StaleAfter = 30 * time.Second

// ttl is a safety net only: since ReportAlive is called every tick, the
// key is refreshed long before this expiry would ever matter in practice.
const ttl = 5 * time.Minute

// ReportAlive records "now" as the watcher's last poll time.
func ReportAlive(ctx context.Context, rdb *redis.Client) error {
	return rdb.Set(ctx, key, time.Now().Format(time.RFC3339), ttl).Err()
}

type Status struct {
	Alive      bool
	LastPollAt time.Time // zero if never reported (or the key expired)
}

// Get reports whether the watcher's last reported poll is within
// StaleAfter of now.
func Get(ctx context.Context, rdb *redis.Client) (Status, error) {
	val, err := rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	lastPoll, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return Status{}, err
	}
	return Status{Alive: isAlive(lastPoll, time.Now()), LastPollAt: lastPoll}, nil
}

// isAlive is the pure comparison, split out so the staleness rule itself
// is testable without a Redis connection.
func isAlive(lastPoll, now time.Time) bool {
	return now.Sub(lastPoll) <= StaleAfter
}
