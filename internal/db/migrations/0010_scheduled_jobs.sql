-- Recurring (cron) job definitions - the one core requirement left off the
-- MVP bootstrap ledger. A scheduled_jobs row is a template that
-- watcher-service expands into an ordinary `jobs` row on each firing;
-- everything downstream (dispatch, claim, execute, retry, DLQ, logs)
-- reuses the existing pipeline unchanged - a scheduled_jobs row itself is
-- never dispatched or executed.
CREATE TABLE IF NOT EXISTS scheduled_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id        UUID NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    cron_expression TEXT NOT NULL,
    payload         JSONB NOT NULL,
    retries_max     INT NOT NULL DEFAULT 3,
    retry_policy_id UUID REFERENCES retry_policies(id) ON DELETE SET NULL,
    active          BOOLEAN NOT NULL DEFAULT true,
    next_run_at     TIMESTAMPTZ NOT NULL,
    last_run_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (queue_id, name)
);

-- watcher-service's due-cron-definitions query.
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_due ON scheduled_jobs(next_run_at) WHERE active;

-- Traces a spawned job back to the cron definition that created it
-- (nullable: every job created before this migration, and every
-- immediate/delayed/scheduled job, has no scheduled_job_id).
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS scheduled_job_id UUID REFERENCES scheduled_jobs(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_jobs_scheduled_job_id ON jobs(scheduled_job_id) WHERE scheduled_job_id IS NOT NULL;
