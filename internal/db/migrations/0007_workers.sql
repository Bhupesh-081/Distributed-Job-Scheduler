-- Worker identity + heartbeat/liveness tracking (MVP bootstrap ledger item
-- 3). Each consumer-service process registers itself with a self-generated
-- id at startup; job_runs.worker_id records which instance ran an attempt.
CREATE TABLE IF NOT EXISTS workers (
    id                UUID PRIMARY KEY,
    hostname          TEXT NOT NULL,
    pid               INT NOT NULL,
    concurrency       INT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'stopped')),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    stopped_at        TIMESTAMPTZ,
    last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workers_status ON workers(status);

-- Heartbeat history (in-flight job count over time), separate from workers'
-- single current-state row so the dashboard can show recent activity, not
-- just "alive right now".
CREATE TABLE IF NOT EXISTS worker_heartbeats (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_id       UUID NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    in_flight_count INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_worker_heartbeats_worker_id ON worker_heartbeats(worker_id, heartbeat_at DESC);

ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS worker_id UUID REFERENCES workers(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_job_runs_worker_id ON job_runs(worker_id) WHERE worker_id IS NOT NULL;
