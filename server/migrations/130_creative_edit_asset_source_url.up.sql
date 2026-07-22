ALTER TABLE creative_edit_asset
    ADD COLUMN IF NOT EXISTS source_asset_url TEXT NOT NULL DEFAULT '';

UPDATE creative_edit_asset
SET source_asset_url = asset_url
WHERE source_asset_url = '' AND asset_url <> '';
