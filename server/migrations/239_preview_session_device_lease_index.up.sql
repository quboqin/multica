CREATE INDEX CONCURRENTLY preview_session_device_lease_idx
    ON preview_session(workspace_id, provider, preview_url, lease_expires_at)
    WHERE provider = 'local_device'
      AND status IN ('creating', 'starting', 'running', 'sleeping');
