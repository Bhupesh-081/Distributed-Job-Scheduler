-- Configurable retry strategies (MVP bootstrap ledger item 2). Replaces
-- consumer-service's hardcoded 5s retry delay with a per-project policy a
-- queue can default to and a job can override.
CREATE TABLE IF NOT EXISTS retry_policies (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    strategy           TEXT NOT NULL CHECK (strategy IN ('fixed', 'linear', 'exponential')),
    base_delay_seconds INT NOT NULL CHECK (base_delay_seconds > 0),
    max_delay_seconds  INT CHECK (max_delay_seconds IS NULL OR max_delay_seconds >= base_delay_seconds),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_retry_policies_project_id ON retry_policies(project_id);

ALTER TABLE queues ADD COLUMN IF NOT EXISTS default_retry_policy_id UUID REFERENCES retry_policies(id) ON DELETE SET NULL;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS retry_policy_id UUID REFERENCES retry_policies(id) ON DELETE SET NULL;
