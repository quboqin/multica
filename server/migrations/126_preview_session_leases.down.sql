DROP INDEX IF EXISTS idx_preview_session_device_idle;
DROP INDEX IF EXISTS idx_preview_session_device_lease;

ALTER TABLE preview_session
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS last_active_at;

ALTER TABLE preview_session
    DROP CONSTRAINT preview_session_status_check;

ALTER TABLE preview_session
    ADD CONSTRAINT preview_session_status_check
    CHECK (status IN ('creating', 'starting', 'running', 'stopping', 'stopped', 'failed', 'expired'));
