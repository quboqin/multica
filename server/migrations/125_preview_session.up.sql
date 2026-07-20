-- Preview sessions are workspace-scoped, issue-owned records for interactive
-- application previews. Phase 0 registers an already-running external Web URL;
-- later providers can reuse the lifecycle fields for managed environments.
CREATE TABLE preview_session (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    platform TEXT NOT NULL,
    provider TEXT NOT NULL,
    title TEXT NOT NULL,
    preview_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'creating'
        CHECK (status IN ('creating', 'starting', 'running', 'stopping', 'stopped', 'failed', 'expired')),
    creator_type TEXT NOT NULL CHECK (creator_type IN ('member', 'agent')),
    creator_id UUID NOT NULL,
    error_message TEXT,
    expires_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    stopped_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_preview_session_workspace_created
    ON preview_session(workspace_id, created_at DESC);

CREATE INDEX idx_preview_session_issue_created
    ON preview_session(issue_id, created_at DESC);

CREATE INDEX idx_preview_session_task
    ON preview_session(task_id)
    WHERE task_id IS NOT NULL;

CREATE INDEX idx_preview_session_expires
    ON preview_session(expires_at)
    WHERE expires_at IS NOT NULL
      AND status IN ('creating', 'starting', 'running', 'stopping');
