DROP INDEX IF EXISTS creative_order_asset_operation_idx;

ALTER TABLE creative_order_asset
    DROP COLUMN IF EXISTS operation_id;

DROP TABLE IF EXISTS creative_prime_composition_job;
DROP TABLE IF EXISTS creative_image_operation_attempt;
DROP INDEX IF EXISTS creative_image_operation_single_inflight_idx;
DROP TABLE IF EXISTS creative_image_operation;

ALTER TABLE creative_order_variant
    DROP CONSTRAINT IF EXISTS creative_order_variant_staging_revision_fkey,
    DROP CONSTRAINT IF EXISTS creative_order_variant_active_revision_fkey;

-- The legacy schema can expose only one revision. Project the durable active
-- package back before dropping revision history, so a failed newer staging
-- revision cannot replace the version users were actually receiving.
UPDATE creative_order_variant variant
SET revision = active_revision.revision,
    brief = active_revision.brief,
    status = active_revision.status,
    updated_at = GREATEST(variant.updated_at, active_revision.updated_at)
FROM creative_order_variant_revision active_revision
WHERE variant.active_revision IS NOT NULL
  AND active_revision.variant_id = variant.id
  AND active_revision.revision = variant.active_revision;

DROP TABLE IF EXISTS creative_order_variant_revision;

DROP INDEX IF EXISTS creative_order_variant_item_selection_rank_idx;

ALTER TABLE creative_order_variant
    DROP COLUMN IF EXISTS primary_size,
    DROP COLUMN IF EXISTS selection_rank,
    DROP COLUMN IF EXISTS candidate_state,
    DROP COLUMN IF EXISTS staging_revision,
    DROP COLUMN IF EXISTS active_revision;
