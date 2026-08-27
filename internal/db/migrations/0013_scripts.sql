-- A reusable script library, project-scoped like retry_policies: write a
-- Python/Bash script once, then pick it from the job-creation form instead
-- of retyping the same code into the inline editor every time.
CREATE TABLE IF NOT EXISTS scripts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    script_type TEXT NOT NULL CHECK (script_type IN ('python', 'bash')),
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_scripts_project_id ON scripts(project_id);
