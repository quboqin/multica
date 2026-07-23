CREATE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_job_mcp_connection_idx
    ON creative_edit_job(mcp_connection_id)
    WHERE mcp_connection_id IS NOT NULL;
