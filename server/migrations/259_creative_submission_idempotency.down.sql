DROP INDEX IF EXISTS issue_workspace_creative_submission_key_key;
DROP INDEX IF EXISTS creative_order_workspace_submission_key_key;

ALTER TABLE creative_order
    DROP COLUMN IF EXISTS submission_key;
