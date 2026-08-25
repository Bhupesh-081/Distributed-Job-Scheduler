-- Real DLQ audit record (MVP bootstrap ledger item 5), replacing
-- jobs.status='dead' as the only trace of a dead-lettered job. Append-only
-- (like job_runs/job_logs): a job that's replayed and dies again gets a
-- second row, which is itself useful audit history.
CREATE TABLE IF NOT EXISTS dead_letter_queue (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id        UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    queue_id      UUID REFERENCES queues(id) ON DELETE SET NULL,
    final_error   TEXT,
    retries_count INT NOT NULL,
    moved_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dlq_queue_id ON dead_letter_queue(queue_id) WHERE queue_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dlq_job_id ON dead_letter_queue(job_id);
