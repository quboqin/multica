ALTER TABLE creative_issue_item
    ADD COLUMN creative_brief JSONB NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(creative_brief) = 'object');
