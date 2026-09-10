ALTER TABLE creative_order_item
    ALTER COLUMN candidate_id DROP NOT NULL,
    ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'material',
    ADD COLUMN copy_library_id UUID REFERENCES creative_resource(id) ON DELETE RESTRICT,
    ADD CONSTRAINT creative_order_item_source_check CHECK (
        (source_kind = 'material' AND candidate_id IS NOT NULL AND copy_library_id IS NULL)
        OR (source_kind = 'copy_library' AND candidate_id IS NULL AND source_analysis_id IS NULL AND copy_library_id IS NOT NULL)
    );
