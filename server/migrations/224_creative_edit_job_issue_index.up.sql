CREATE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_job_issue_idx
    ON creative_edit_job(issue_id, created_at DESC);
