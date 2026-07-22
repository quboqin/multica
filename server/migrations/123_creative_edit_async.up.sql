ALTER TABLE creative_edit_job
    ADD COLUMN IF NOT EXISTS external_provider TEXT NOT NULL DEFAULT 'mock_creative_mcp',
    ADD COLUMN IF NOT EXISTS external_job_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS external_status TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS stage TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS progress INT NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
    ADD COLUMN IF NOT EXISTS last_poll_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS next_poll_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_creative_edit_job_pending
    ON creative_edit_job(workspace_id, status, next_poll_at)
    WHERE status IN ('queued', 'running');
