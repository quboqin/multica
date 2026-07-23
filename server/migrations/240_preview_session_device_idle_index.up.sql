CREATE INDEX CONCURRENTLY IF NOT EXISTS preview_session_device_idle_idx
    ON preview_session(last_active_at)
    WHERE provider = 'local_device'
      AND status IN ('creating', 'starting', 'running', 'sleeping');
