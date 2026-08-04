DROP INDEX IF EXISTS creative_feedback_event_workspace_idempotency_idx;

ALTER TABLE creative_feedback_event
    DROP COLUMN IF EXISTS idempotency_key;

