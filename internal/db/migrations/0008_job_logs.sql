-- Structured per-attempt execution logs (MVP bootstrap ledger item 4).
-- job_runs.err_msg stays as the one-line failure summary; job_logs adds a
-- browsable log stream (claim, execution output, outcome) per attempt.
CREATE TABLE IF NOT EXISTS job_logs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id     UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    job_run_id UUID REFERENCES job_runs(id) ON DELETE CASCADE,
    level      TEXT NOT NULL CHECK (level IN ('info', 'warn', 'error')),
    message    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_logs_job_id ON job_logs(job_id, created_at);
CREATE INDEX IF NOT EXISTS idx_job_logs_job_run_id ON job_logs(job_run_id) WHERE job_run_id IS NOT NULL;
