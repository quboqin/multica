ALTER TABLE creative_feedback_event
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS creative_feedback_event_workspace_idempotency_idx
    ON creative_feedback_event(workspace_id, idempotency_key)
    WHERE idempotency_key <> '';

