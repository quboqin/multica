CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS workspace_mcp_connection_default_idx
    ON workspace_mcp_connection(workspace_id, capability)
    WHERE is_default AND status = 'active';
