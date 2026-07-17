CREATE TABLE credential_profile (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    connector_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'active', 'need_reauth', 'revoked')),
    last_used_at TIMESTAMPTZ,
    expires_hint TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, user_id)
        REFERENCES member(workspace_id, user_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_credential_profile_user
    ON credential_profile(workspace_id, user_id, connector_id)
    WHERE status <> 'revoked';

CREATE INDEX idx_credential_profile_status
    ON credential_profile(workspace_id, status);

CREATE TABLE credential_secret (
    profile_id UUID PRIMARY KEY REFERENCES credential_profile(id) ON DELETE CASCADE,
    ciphertext BYTEA NOT NULL,
    key_version TEXT NOT NULL DEFAULT 'v1',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE credential_login_session (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES credential_profile(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    connector_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    browser_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'completed', 'expired', 'abandoned')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_credential_login_session_profile
    ON credential_login_session(profile_id, status);

CREATE INDEX idx_credential_login_session_expiry
    ON credential_login_session(expires_at)
    WHERE status = 'pending';
