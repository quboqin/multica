CREATE TABLE IF NOT EXISTS creative_order_diagnostic_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    task_id UUID,
    attachment_id UUID NOT NULL REFERENCES attachment(id) ON DELETE CASCADE,
    size_key TEXT NOT NULL CHECK (size_key IN ('1080x1080', '1200x628', '800x1000')),
    revision INT NOT NULL CHECK (revision > 0),
    workflow TEXT NOT NULL CHECK (workflow <> ''),
    label TEXT NOT NULL CHECK (label <> ''),
    filename TEXT NOT NULL CHECK (filename <> ''),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(variant_id, revision, workflow, size_key, label, filename)
);

CREATE INDEX IF NOT EXISTS creative_order_diagnostic_asset_variant_idx
    ON creative_order_diagnostic_asset(variant_id, revision DESC, updated_at DESC);
