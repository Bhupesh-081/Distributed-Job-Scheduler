// Package cancel is the shared "cancel:{jobID}" Redis key convention
// between job-service (writer, on POST /jobs/{id}/cancel) and
// consumer-service (reader, polled during execution). Redis is a fast-path
// signal only: Postgres jobs.status is still the source of truth, so
// losing this key just means a running job finishes instead of aborting
// early.
package cancel

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ttl is generous relative to the executor's max job timeout (5m, see
// internal/executor) so a slow-to-poll worker still sees the flag.
const ttl = 10 * time.Minute

func key(jobID uuid.UUID) string { return "cancel:" + jobID.String() }

func Request(ctx context.Context, rdb *redis.Client, jobID uuid.UUID) error {
	return rdb.Set(ctx, key(jobID), "1", ttl).Err()
}

func IsRequested(ctx context.Context, rdb *redis.Client, jobID uuid.UUID) (bool, error) {
	n, err := rdb.Exists(ctx, key(jobID)).Result()
	return n > 0, err
}

func Clear(ctx context.Context, rdb *redis.Client, jobID uuid.UUID) error {
	return rdb.Del(ctx, key(jobID)).Err()
}
