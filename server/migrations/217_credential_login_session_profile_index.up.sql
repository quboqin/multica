CREATE INDEX CONCURRENTLY IF NOT EXISTS credential_login_session_profile_idx
    ON credential_login_session(profile_id, status);
