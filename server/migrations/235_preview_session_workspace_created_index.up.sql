CREATE INDEX CONCURRENTLY IF NOT EXISTS preview_session_workspace_created_idx
    ON preview_session(workspace_id, created_at DESC);
