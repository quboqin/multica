ALTER TABLE creative_edit_job
    ADD COLUMN IF NOT EXISTS poll_attempts INT NOT NULL DEFAULT 0;

ALTER TABLE creative_material_candidate
    ADD COLUMN IF NOT EXISTS archived_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS archive_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (archive_status IN ('pending', 'running', 'completed', 'failed')),
    ADD COLUMN IF NOT EXISTS archive_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS archive_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS next_archive_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_creative_material_candidate_archive_pending
    ON creative_material_candidate(next_archive_at, created_at)
    WHERE archive_status IN ('pending', 'running');

ALTER TABLE creative_edit_asset
    ADD COLUMN IF NOT EXISTS storage_key TEXT NOT NULL DEFAULT '';
