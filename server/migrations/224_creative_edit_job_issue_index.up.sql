CREATE INDEX CONCURRENTLY creative_edit_job_issue_idx
    ON creative_edit_job(issue_id, created_at DESC);
