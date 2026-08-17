ALTER TABLE agent_task_queue
    ADD COLUMN IF NOT EXISTS requesting_user_id UUID;

UPDATE agent_task_queue
SET requesting_user_id = originator_user_id
WHERE requesting_user_id IS NULL
  AND originator_user_id IS NOT NULL;
