ALTER TABLE agent_task_queue
    ADD COLUMN requesting_user_id UUID REFERENCES "user"(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_task_queue_requesting_user_id
    ON agent_task_queue(requesting_user_id);
