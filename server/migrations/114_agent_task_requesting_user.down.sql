DROP INDEX IF EXISTS idx_agent_task_queue_requesting_user_id;

ALTER TABLE agent_task_queue
    DROP COLUMN requesting_user_id;
