CREATE INDEX CONCURRENTLY creative_edit_job_mcp_connection_idx
    ON creative_edit_job(mcp_connection_id)
    WHERE mcp_connection_id IS NOT NULL;
