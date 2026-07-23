CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS workspace_mcp_connection_name_idx
    ON workspace_mcp_connection(workspace_id, name);
