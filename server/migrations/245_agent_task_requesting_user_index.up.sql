CREATE INDEX CONCURRENTLY IF NOT EXISTS agent_task_queue_requesting_user_id_idx
    ON agent_task_queue(requesting_user_id)
    WHERE requesting_user_id IS NOT NULL;
