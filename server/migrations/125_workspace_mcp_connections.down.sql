DROP INDEX IF EXISTS idx_creative_edit_job_mcp_connection;
ALTER TABLE creative_edit_job DROP COLUMN IF EXISTS mcp_connection_id;
DROP TABLE IF EXISTS workspace_mcp_connection;
