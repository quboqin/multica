CREATE TABLE workspace_capability (
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    capability_key TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, capability_key)
);

CREATE INDEX workspace_capability_key_idx
    ON workspace_capability (capability_key, workspace_id);
