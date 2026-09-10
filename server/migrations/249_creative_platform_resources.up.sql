CREATE TABLE creative_resource (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('copy_library', 'market_pack')),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),
    published_version INT CHECK (published_version > 0 AND published_version <= version),
    config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX creative_resource_workspace_kind_idx
    ON creative_resource(workspace_id, kind, updated_at DESC)
    WHERE status <> 'archived';

CREATE TABLE creative_resource_revision (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id UUID NOT NULL REFERENCES creative_resource(id) ON DELETE CASCADE,
    version INT NOT NULL CHECK (version > 0),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(resource_id, version)
);

CREATE TABLE creative_resource_file (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id UUID NOT NULL REFERENCES creative_resource(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    attachment_id UUID NOT NULL REFERENCES attachment(id) ON DELETE RESTRICT,
    role TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_version INT NOT NULL CHECK (created_version > 0),
    removed_version INT CHECK (removed_version > created_version),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(resource_id, attachment_id, role, created_version)
);

CREATE INDEX creative_resource_file_active_idx
    ON creative_resource_file(resource_id, role, created_at)
    WHERE removed_version IS NULL;

CREATE TABLE creative_issue_context (
    issue_id UUID PRIMARY KEY REFERENCES issue(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    market_pack_id UUID REFERENCES creative_resource(id) ON DELETE RESTRICT,
    orchestration_skill_id UUID NOT NULL REFERENCES skill(id) ON DELETE RESTRICT,
    squad_id UUID REFERENCES squad(id) ON DELETE RESTRICT,
    snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(snapshot) = 'object'),
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX creative_issue_context_workspace_idx
    ON creative_issue_context(workspace_id, updated_at DESC);

CREATE TABLE creative_issue_item (
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    work_issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    revision INT NOT NULL DEFAULT 1 CHECK (revision > 0),
    status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready', 'running', 'published', 'blocked')),
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(issue_id, candidate_id)
);

CREATE INDEX creative_issue_item_work_issue_idx
    ON creative_issue_item(work_issue_id)
    WHERE work_issue_id IS NOT NULL;

INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by, created_at)
SELECT id, version, name, description, config, created_by, created_at
FROM creative_resource;
