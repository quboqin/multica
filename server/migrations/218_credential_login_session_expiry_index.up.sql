CREATE INDEX CONCURRENTLY IF NOT EXISTS credential_login_session_expiry_idx
    ON credential_login_session(expires_at)
    WHERE status = 'pending';
