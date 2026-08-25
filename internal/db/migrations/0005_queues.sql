-- Project/queue scoping (MVP bootstrap ledger item 1, design-decisions.md).
-- jobs.queue_id stays nullable at the DB level: existing MVP rows created
-- before this migration have no queue. New jobs are required to set it at
-- the application layer (job-service validates queue_id on create).
CREATE TABLE IF NOT EXISTS queues (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    priority          INT NOT NULL DEFAULT 0,
    concurrency_limit INT NOT NULL DEFAULT 5 CHECK (concurrency_limit > 0),
    paused            BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_queues_project_id ON queues(project_id);

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS queue_id UUID REFERENCES queues(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS idx_jobs_queue_id ON jobs(queue_id) WHERE queue_id IS NOT NULL;

-- Concurrency-limit check in ClaimJob counts running jobs per queue.
CREATE INDEX IF NOT EXISTS idx_jobs_queue_running ON jobs(queue_id) WHERE status = 'running';
