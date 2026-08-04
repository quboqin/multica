ALTER TABLE creative_order
    ADD COLUMN IF NOT EXISTS submission_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS creative_order_workspace_submission_key_key
    ON creative_order (workspace_id, submission_key)
    WHERE submission_key <> '';

CREATE UNIQUE INDEX IF NOT EXISTS issue_workspace_creative_submission_key_key
    ON issue (workspace_id, (metadata ->> 'creative_submission_key'))
    WHERE metadata ? 'creative_submission_key'
      AND metadata ->> 'creative_submission_key' <> '';
