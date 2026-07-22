CREATE INDEX CONCURRENTLY preview_session_issue_created_idx
    ON preview_session(issue_id, created_at DESC);
