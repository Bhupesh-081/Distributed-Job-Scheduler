-- Tracks whether the Watcher has already published a job to Kafka, so
-- polling doesn't republish the same due job every tick, and so a job that
-- was dispatched but never progressed (crash, lost message) is detectable
-- for stuck-job recovery.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dispatched_at TIMESTAMPTZ;

-- Due-job dispatch: queued jobs not yet dispatched, ordered by when they're due.
CREATE INDEX IF NOT EXISTS idx_jobs_due ON jobs(scheduled_time)
    WHERE status = 'queued' AND dispatched_at IS NULL;

-- Stuck-job recovery: queued jobs that were dispatched but haven't moved on.
CREATE INDEX IF NOT EXISTS idx_jobs_stuck ON jobs(modified_time)
    WHERE status = 'queued' AND dispatched_at IS NOT NULL;
