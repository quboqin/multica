ALTER TABLE creative_order_item
    ALTER COLUMN candidate_id SET NOT NULL,
    DROP CONSTRAINT creative_order_item_source_check,
    DROP COLUMN copy_library_id,
    DROP COLUMN source_kind;
