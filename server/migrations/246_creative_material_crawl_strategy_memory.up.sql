CREATE TABLE IF NOT EXISTS creative_material_crawl_strategy_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    connector_id TEXT NOT NULL DEFAULT 'appgrowing',
    capability TEXT NOT NULL DEFAULT 'material_search',
    scope_key TEXT NOT NULL,
    memory JSONB NOT NULL DEFAULT '{}'::jsonb,
    success_count INT NOT NULL DEFAULT 0,
    failure_count INT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    last_run_id UUID,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (scope_key <> ''),
    CHECK (jsonb_typeof(memory) = 'object'),
    UNIQUE (workspace_id, project_id, connector_id, capability, scope_key)
);

CREATE INDEX IF NOT EXISTS creative_material_crawl_strategy_memory_lookup_idx
    ON creative_material_crawl_strategy_memory(workspace_id, project_id, connector_id, capability, updated_at DESC);
