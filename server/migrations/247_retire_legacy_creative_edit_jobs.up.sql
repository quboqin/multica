DROP TABLE IF EXISTS creative_edit_feedback;
DROP TABLE IF EXISTS creative_edit_asset;
DROP TABLE IF EXISTS creative_edit_variant;
DROP TABLE IF EXISTS creative_edit_job_candidate;
DROP TABLE IF EXISTS creative_edit_job;

ALTER TABLE workspace_mcp_connection
    DROP CONSTRAINT IF EXISTS workspace_mcp_connection_capability_check;

UPDATE workspace_mcp_connection
SET capability = 'agent_runtime'
WHERE capability = 'creative_edit';

ALTER TABLE workspace_mcp_connection
    ALTER COLUMN capability SET DEFAULT 'agent_runtime';

ALTER TABLE workspace_mcp_connection
    ADD CONSTRAINT workspace_mcp_connection_capability_check
    CHECK (capability IN ('agent_runtime'));

ALTER TABLE workspace_mcp_connection
    ALTER COLUMN tool_create SET DEFAULT 'tools/list',
    ALTER COLUMN tool_get SET DEFAULT 'tools/call';
