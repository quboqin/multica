CREATE TABLE creative_copy_entry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    library_id UUID NOT NULL REFERENCES creative_resource(id) ON DELETE CASCADE,
    external_key TEXT NOT NULL,
    headline TEXT NOT NULL DEFAULT '',
    subheadline TEXT NOT NULL DEFAULT '',
    benefit TEXT NOT NULL DEFAULT '',
    cta TEXT NOT NULL DEFAULT '',
    legal_text TEXT NOT NULL DEFAULT '',
    copy_role TEXT NOT NULL DEFAULT '',
    market TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'disabled')),
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(library_id, external_key)
);

CREATE INDEX creative_copy_entry_library_status_idx
    ON creative_copy_entry(library_id, status, updated_at DESC);

ALTER TABLE creative_issue_item
    ADD COLUMN IF NOT EXISTS copy_entry_id UUID REFERENCES creative_copy_entry(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS copy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(copy_snapshot) = 'object');
