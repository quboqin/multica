ALTER TABLE creative_material_crawl_run
    ADD COLUMN IF NOT EXISTS diagnostics JSONB NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(diagnostics) = 'object');
