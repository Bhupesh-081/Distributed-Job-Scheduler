-- MVP job schema (flat: no project/queue scoping yet, no retry_policies/
-- workers/job_logs/dead_letter_queue tables yet). See "MVP bootstrap
-- ledger" in docs/design-decisions.md for what's deliberately deferred and
-- when to add it back.
CREATE TABLE IF NOT EXISTS jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    scheduled_type  TEXT NOT NULL CHECK (scheduled_type IN ('immediate', 'delayed', 'scheduled', 'cancelled')),
    status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'success', 'failed', 'dead')),
    scheduled_time  TIMESTAMPTZ,
    cron_expression TEXT,
    payload         JSONB,
    retries_count   INT NOT NULL DEFAULT 0,
    retries_max     INT NOT NULL DEFAULT 3,
    modified_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta            JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_modified_time ON jobs(modified_time);
CREATE INDEX IF NOT EXISTS idx_jobs_status_scheduled_time ON jobs(status, scheduled_time);

CREATE TABLE IF NOT EXISTS job_runs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id         UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    status         TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'failed')),
    start_time     TIMESTAMPTZ,
    end_time       TIMESTAMPTZ,
    attempt_number INT NOT NULL DEFAULT 1,
    err_msg        TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_runs_job_id ON job_runs(job_id);
CREATE INDEX IF NOT EXISTS idx_job_runs_status ON job_runs(status);
