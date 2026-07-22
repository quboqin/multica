CREATE INDEX CONCURRENTLY workspace_mcp_connection_workspace_idx
    ON workspace_mcp_connection(workspace_id, capability, status, created_at);
