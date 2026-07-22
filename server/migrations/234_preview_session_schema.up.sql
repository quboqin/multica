CREATE TABLE preview_session (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    task_id UUID,
    platform TEXT NOT NULL,
    provider TEXT NOT NULL,
    title TEXT NOT NULL,
    preview_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'creating'
        CHECK (status IN ('creating', 'starting', 'running', 'sleeping', 'stopping', 'stopped', 'failed', 'expired')),
    creator_type TEXT NOT NULL CHECK (creator_type IN ('member', 'agent')),
    creator_id UUID NOT NULL,
    error_message TEXT,
    expires_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    stopped_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_active_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ
);
