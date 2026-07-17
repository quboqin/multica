DROP INDEX IF EXISTS idx_creative_edit_job_pending;

ALTER TABLE creative_edit_job
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS next_poll_at,
    DROP COLUMN IF EXISTS last_poll_at,
    DROP COLUMN IF EXISTS progress,
    DROP COLUMN IF EXISTS stage,
    DROP COLUMN IF EXISTS external_status,
    DROP COLUMN IF EXISTS external_job_id,
    DROP COLUMN IF EXISTS external_provider;
