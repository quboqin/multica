UPDATE agent
SET mcp_config = CASE
        WHEN mcp_config - 'workspaceMcpRefs' - 'workspace_mcp_refs' = '{}'::jsonb THEN NULL
        ELSE mcp_config - 'workspaceMcpRefs' - 'workspace_mcp_refs'
    END,
    updated_at = now()
WHERE jsonb_typeof(mcp_config) = 'object'
  AND (mcp_config ? 'workspaceMcpRefs' OR mcp_config ? 'workspace_mcp_refs');
