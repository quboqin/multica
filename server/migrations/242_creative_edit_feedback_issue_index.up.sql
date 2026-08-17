CREATE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_feedback_issue_idx
    ON creative_edit_feedback(workspace_id, issue_id, created_at DESC);
