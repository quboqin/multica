CREATE TABLE IF NOT EXISTS creative_material_candidate (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
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
    archived_url TEXT NOT NULL DEFAULT '',
    archive_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (archive_status IN ('pending', 'running', 'completed', 'failed')),
    archive_attempts INT NOT NULL DEFAULT 0,
    archive_error TEXT NOT NULL DEFAULT '',
    next_archive_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS creative_material_issue_candidate (
    issue_id UUID NOT NULL,
    candidate_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    source_run_id UUID,
    status TEXT NOT NULL DEFAULT 'new'
        CHECK (status IN ('new', 'selected', 'rejected', 'sent_to_edit', 'edited', 'approved', 'archived')),
    tags TEXT[] NOT NULL DEFAULT '{}',
    note TEXT NOT NULL DEFAULT '',
    selected_by UUID,
    selected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, candidate_id)
);

CREATE TABLE IF NOT EXISTS creative_material_crawl_run (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID,
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

CREATE TABLE IF NOT EXISTS creative_edit_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'completed'
        CHECK (status IN ('queued', 'running', 'partial', 'completed', 'failed')),
    prompt TEXT NOT NULL DEFAULT '',
    rules JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by_type TEXT NOT NULL DEFAULT 'member'
        CHECK (created_by_type IN ('member', 'agent')),
    created_by_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    external_provider TEXT NOT NULL DEFAULT 'mock_creative_mcp',
    external_job_id TEXT NOT NULL DEFAULT '',
    external_status TEXT NOT NULL DEFAULT '',
    stage TEXT NOT NULL DEFAULT '',
    progress INT NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
    last_poll_at TIMESTAMPTZ,
    next_poll_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    poll_attempts INT NOT NULL DEFAULT 0,
    mcp_connection_id UUID,
    process_data JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS creative_edit_job_candidate (
    job_id UUID NOT NULL,
    candidate_id UUID NOT NULL,
    PRIMARY KEY (job_id, candidate_id)
);

CREATE TABLE IF NOT EXISTS creative_edit_variant (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL,
    candidate_id UUID NOT NULL,
    variant_index INT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    qc_status TEXT NOT NULL DEFAULT 'mock'
        CHECK (qc_status IN ('mock', 'pending', 'passed', 'warning', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS creative_edit_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL,
    width INT NOT NULL,
    height INT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    asset_url TEXT NOT NULL DEFAULT '',
    source_asset_url TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'image/png',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    storage_key TEXT NOT NULL DEFAULT ''
);
