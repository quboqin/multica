CREATE INDEX CONCURRENTLY IF NOT EXISTS preview_session_task_idx
    ON preview_session(task_id)
    WHERE task_id IS NOT NULL;
