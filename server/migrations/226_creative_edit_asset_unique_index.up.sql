CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_asset_unique_idx
    ON creative_edit_asset(variant_id, width, height);
