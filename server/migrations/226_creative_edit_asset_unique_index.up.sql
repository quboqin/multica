CREATE UNIQUE INDEX CONCURRENTLY creative_edit_asset_unique_idx
    ON creative_edit_asset(variant_id, width, height);
