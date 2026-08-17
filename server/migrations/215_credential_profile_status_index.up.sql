CREATE INDEX CONCURRENTLY IF NOT EXISTS credential_profile_status_idx
    ON credential_profile(workspace_id, status);
