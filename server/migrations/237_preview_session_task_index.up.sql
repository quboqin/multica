CREATE INDEX CONCURRENTLY preview_session_task_idx
    ON preview_session(task_id)
    WHERE task_id IS NOT NULL;
