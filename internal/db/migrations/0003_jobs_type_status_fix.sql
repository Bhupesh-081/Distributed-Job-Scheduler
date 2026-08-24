-- 0002 mixed a status value ('cancelled') into scheduled_type and left out
-- 'recurring', so a cron job couldn't be represented at all. 'cancelled'
-- belongs on status (a job can be cancelled before it runs regardless of
-- type).
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_scheduled_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_scheduled_type_check
    CHECK (scheduled_type IN ('immediate', 'delayed', 'scheduled', 'recurring'));

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_status_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_status_check
    CHECK (status IN ('queued', 'running', 'success', 'failed', 'dead', 'cancelled'));
