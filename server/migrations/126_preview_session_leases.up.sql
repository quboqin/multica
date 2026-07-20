ALTER TABLE preview_session
    DROP CONSTRAINT preview_session_status_check;

ALTER TABLE preview_session
    ADD CONSTRAINT preview_session_status_check
    CHECK (status IN ('creating', 'starting', 'running', 'sleeping', 'stopping', 'stopped', 'failed', 'expired'));

ALTER TABLE preview_session
    ADD COLUMN last_active_at TIMESTAMPTZ,
    ADD COLUMN lease_expires_at TIMESTAMPTZ;

UPDATE preview_session
SET last_active_at = COALESCE(updated_at, started_at, created_at),
    lease_expires_at = CASE
        WHEN provider = 'local_device' AND status = 'running' THEN now() + INTERVAL '5 minutes'
        ELSE NULL
    END,
    expires_at = CASE
        WHEN provider = 'local_device' AND expires_at IS NULL THEN now() + INTERVAL '4 hours'
        ELSE expires_at
    END;

CREATE INDEX idx_preview_session_device_lease
    ON preview_session(workspace_id, provider, preview_url, lease_expires_at)
    WHERE provider = 'local_device'
      AND status IN ('creating', 'starting', 'running', 'sleeping');

CREATE INDEX idx_preview_session_device_idle
    ON preview_session(last_active_at)
    WHERE provider = 'local_device'
      AND status IN ('creating', 'starting', 'running', 'sleeping');
