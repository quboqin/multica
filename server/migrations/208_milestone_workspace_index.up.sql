CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_milestone_workspace
    ON milestone(workspace_id, position, created_at);
