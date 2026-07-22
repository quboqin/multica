ALTER TABLE creative_edit_job
    ADD COLUMN IF NOT EXISTS process_data JSONB NOT NULL DEFAULT '{}'::jsonb;
