ALTER TABLE creative_issue_item
    DROP COLUMN IF EXISTS copy_entry_id,
    DROP COLUMN IF EXISTS copy_snapshot;

DROP TABLE IF EXISTS creative_copy_entry;
