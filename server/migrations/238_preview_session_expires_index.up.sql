CREATE INDEX CONCURRENTLY preview_session_expires_idx
    ON preview_session(expires_at)
    WHERE expires_at IS NOT NULL
      AND status IN ('creating', 'starting', 'running', 'stopping');
