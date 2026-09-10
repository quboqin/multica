DROP INDEX IF EXISTS creative_order_item_adopted_variant_idx;

ALTER TABLE creative_order_item
    DROP CONSTRAINT IF EXISTS creative_order_item_adopted_variant_fkey,
    DROP CONSTRAINT IF EXISTS creative_order_item_adoption_timestamp_check,
    DROP COLUMN IF EXISTS adopted_by,
    DROP COLUMN IF EXISTS adopted_at,
    DROP COLUMN IF EXISTS adopted_variant_id;

ALTER TABLE creative_order_variant
    DROP CONSTRAINT IF EXISTS creative_order_variant_item_id_id_key;
