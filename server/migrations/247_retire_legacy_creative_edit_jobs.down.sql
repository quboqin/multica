ALTER TABLE workspace_mcp_connection
    DROP CONSTRAINT IF EXISTS workspace_mcp_connection_capability_check;

ALTER TABLE workspace_mcp_connection
    ALTER COLUMN capability SET DEFAULT 'creative_edit';

UPDATE workspace_mcp_connection
SET capability = 'creative_edit'
WHERE capability = 'agent_runtime';

ALTER TABLE workspace_mcp_connection
    ADD CONSTRAINT workspace_mcp_connection_capability_check
    CHECK (capability IN ('creative_edit'));

ALTER TABLE workspace_mcp_connection
    ALTER COLUMN tool_create SET DEFAULT 'create_creative_job',
    ALTER COLUMN tool_get SET DEFAULT 'get_creative_job';
