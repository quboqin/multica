CREATE TABLE creative_material_candidate (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    connector_id TEXT NOT NULL DEFAULT 'appgrowing',
    external_id TEXT,
    dedupe_key TEXT NOT NULL,
    competitor TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    asset_type TEXT NOT NULL DEFAULT 'unknown'
        CHECK (asset_type IN ('image', 'video', 'unknown')),
    preview_url TEXT NOT NULL DEFAULT '',
    resource_url TEXT NOT NULL DEFAULT '',
    poster_url TEXT NOT NULL DEFAULT '',
    original_url TEXT NOT NULL DEFAULT '',
    duration_days DOUBLE PRECISION,
    impression_estimate BIGINT,
    media_names TEXT[] NOT NULL DEFAULT '{}',
    area_names TEXT[] NOT NULL DEFAULT '{}',
    language_names TEXT[] NOT NULL DEFAULT '{}',
    platform_names TEXT[] NOT NULL DEFAULT '{}',
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, connector_id, dedupe_key)
);

CREATE INDEX idx_creative_material_candidate_workspace_seen
    ON creative_material_candidate(workspace_id, last_seen_at DESC);

CREATE TABLE creative_material_issue_candidate (
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_run_id UUID,
    status TEXT NOT NULL DEFAULT 'new'
        CHECK (status IN ('new', 'selected', 'rejected', 'sent_to_edit', 'edited', 'approved', 'archived')),
    tags TEXT[] NOT NULL DEFAULT '{}',
    note TEXT NOT NULL DEFAULT '',
    selected_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    selected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, candidate_id)
);

CREATE INDEX idx_creative_material_issue_candidate_issue
    ON creative_material_issue_candidate(issue_id, status, created_at DESC);

CREATE TABLE creative_material_crawl_run (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    connector_id TEXT NOT NULL DEFAULT 'appgrowing',
    query_summary TEXT NOT NULL DEFAULT '',
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'completed'
        CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    imported_count INT NOT NULL DEFAULT 0,
    existing_count INT NOT NULL DEFAULT 0,
    total_count INT NOT NULL DEFAULT 0,
    created_by_type TEXT NOT NULL DEFAULT 'member'
        CHECK (created_by_type IN ('member', 'agent')),
    created_by_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_creative_material_crawl_run_issue
    ON creative_material_crawl_run(issue_id, created_at DESC)
    WHERE issue_id IS NOT NULL;

CREATE TABLE creative_edit_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'completed'
        CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    prompt TEXT NOT NULL DEFAULT '',
    rules JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by_type TEXT NOT NULL DEFAULT 'member'
        CHECK (created_by_type IN ('member', 'agent')),
    created_by_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_creative_edit_job_issue
    ON creative_edit_job(issue_id, created_at DESC);

CREATE TABLE creative_edit_job_candidate (
    job_id UUID NOT NULL REFERENCES creative_edit_job(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    PRIMARY KEY (job_id, candidate_id)
);

CREATE TABLE creative_edit_variant (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES creative_edit_job(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    variant_index INT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    qc_status TEXT NOT NULL DEFAULT 'mock'
        CHECK (qc_status IN ('mock', 'pending', 'passed', 'warning', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (job_id, candidate_id, variant_index)
);

CREATE TABLE creative_edit_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES creative_edit_variant(id) ON DELETE CASCADE,
    width INT NOT NULL,
    height INT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    asset_url TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'image/png',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (variant_id, width, height)
);
