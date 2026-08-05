CREATE TABLE creative_market_pack_component_extraction (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    resource_id UUID NOT NULL REFERENCES creative_resource(id) ON DELETE CASCADE,
    source_attachment_id UUID NOT NULL REFERENCES attachment(id) ON DELETE RESTRICT,
    source_width INT NOT NULL CHECK (source_width > 0),
    source_height INT NOT NULL CHECK (source_height > 0),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'completed', 'failed', 'applied')),
    result JSONB NOT NULL DEFAULT '{"candidates":[]}'::jsonb
        CHECK (jsonb_typeof(result) = 'object'),
    error_message TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ
);

CREATE INDEX creative_market_pack_component_extraction_resource_idx
    ON creative_market_pack_component_extraction(resource_id, created_at DESC);
