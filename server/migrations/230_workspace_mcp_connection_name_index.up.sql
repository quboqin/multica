CREATE UNIQUE INDEX CONCURRENTLY workspace_mcp_connection_name_idx
    ON workspace_mcp_connection(workspace_id, name);
