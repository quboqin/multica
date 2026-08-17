CREATE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_job_pending_idx
    ON creative_edit_job(workspace_id, status, next_poll_at)
    WHERE status IN ('queued', 'running');
