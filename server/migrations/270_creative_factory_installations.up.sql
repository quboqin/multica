CREATE TABLE creative_factory_installation (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'ready'
        CHECK (status IN ('initializing', 'ready', 'needs_setup', 'failed')),
    schema_version INT NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    template_version INT NOT NULL DEFAULT 1 CHECK (template_version > 0),
    runtime_id UUID REFERENCES agent_runtime(id) ON DELETE SET NULL,
    market_pack_id UUID REFERENCES creative_resource(id) ON DELETE SET NULL,
    copy_library_id UUID REFERENCES creative_resource(id) ON DELETE SET NULL,
    squad_id UUID REFERENCES squad(id) ON DELETE SET NULL,
    orchestration_skill_id UUID REFERENCES skill(id) ON DELETE SET NULL,
    role_agents JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(role_agents) = 'object'),
    role_skills JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(role_skills) = 'object'),
    config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    initialized_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    initialized_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX creative_factory_installation_status_idx
    ON creative_factory_installation(status, updated_at DESC);
