CREATE TABLE workspace_mcp_connection (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL,
    capability TEXT NOT NULL DEFAULT 'creative_edit'
        CHECK (capability IN ('creative_edit')),
    transport TEXT NOT NULL DEFAULT 'streamable_http'
        CHECK (transport IN ('streamable_http')),
    server_url TEXT NOT NULL,
    tool_create TEXT NOT NULL DEFAULT 'create_creative_job',
    tool_get TEXT NOT NULL DEFAULT 'get_creative_job',
    secret_headers_encrypted BYTEA,
    secret_header_names TEXT[] NOT NULL DEFAULT '{}',
    secret_key_version TEXT NOT NULL DEFAULT 'v1',
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    is_default BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NOT NULL,
    last_verified_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
