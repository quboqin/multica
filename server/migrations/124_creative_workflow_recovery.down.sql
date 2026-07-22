ALTER TABLE creative_edit_asset
    DROP COLUMN IF EXISTS storage_key;

DROP INDEX IF EXISTS idx_creative_material_candidate_archive_pending;

ALTER TABLE creative_material_candidate
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS next_archive_at,
    DROP COLUMN IF EXISTS archive_error,
    DROP COLUMN IF EXISTS archive_attempts,
    DROP COLUMN IF EXISTS archive_status,
    DROP COLUMN IF EXISTS archived_url;

ALTER TABLE creative_edit_job
    DROP COLUMN IF EXISTS poll_attempts;
