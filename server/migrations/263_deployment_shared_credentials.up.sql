ALTER TABLE credential_profile
    ADD COLUMN scope TEXT NOT NULL DEFAULT 'workspace'
        CHECK (scope IN ('workspace', 'deployment'));

DROP INDEX IF EXISTS credential_profile_workspace_connector_idx;

WITH canonical AS (
    SELECT profile.id
    FROM credential_profile profile
    JOIN workspace ON workspace.id = profile.workspace_id
    WHERE profile.connector_id = 'appgrowing'
      AND profile.status <> 'revoked'
    ORDER BY
        CASE profile.status
            WHEN 'active' THEN 0
            WHEN 'need_reauth' THEN 1
            WHEN 'pending' THEN 2
            ELSE 3
        END,
        profile.last_used_at DESC NULLS LAST,
        profile.updated_at DESC,
        profile.created_at DESC
    LIMIT 1
)
UPDATE credential_profile profile
SET status = 'revoked',
    scope = 'workspace',
    updated_at = now()
WHERE profile.connector_id = 'appgrowing'
  AND profile.id <> ALL (SELECT id FROM canonical);

WITH canonical AS (
    SELECT profile.id
    FROM credential_profile profile
    JOIN workspace ON workspace.id = profile.workspace_id
    WHERE profile.connector_id = 'appgrowing'
      AND profile.status <> 'revoked'
    ORDER BY
        CASE profile.status
            WHEN 'active' THEN 0
            WHEN 'need_reauth' THEN 1
            WHEN 'pending' THEN 2
            ELSE 3
        END,
        profile.last_used_at DESC NULLS LAST,
        profile.updated_at DESC,
        profile.created_at DESC
    LIMIT 1
)
UPDATE credential_profile profile
SET scope = 'deployment'
FROM canonical
WHERE profile.id = canonical.id;

CREATE UNIQUE INDEX credential_profile_workspace_connector_idx
    ON credential_profile(workspace_id, connector_id)
    WHERE scope = 'workspace' AND status <> 'revoked';

CREATE UNIQUE INDEX credential_profile_deployment_connector_idx
    ON credential_profile(connector_id)
    WHERE scope = 'deployment' AND status <> 'revoked';

CREATE TABLE credential_profile_manager (
    profile_id UUID NOT NULL REFERENCES credential_profile(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    granted_by_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, user_id)
);

INSERT INTO credential_profile_manager (profile_id, user_id, granted_by_id)
SELECT profile.id, profile.authorized_by_id, profile.authorized_by_id
FROM credential_profile profile
JOIN "user" app_user ON app_user.id = profile.authorized_by_id
ON CONFLICT DO NOTHING;

WITH canonical AS (
    SELECT id
    FROM credential_profile
    WHERE connector_id = 'appgrowing'
      AND scope = 'deployment'
      AND status <> 'revoked'
    LIMIT 1
)
INSERT INTO credential_profile_manager (profile_id, user_id, granted_by_id)
SELECT canonical.id, legacy.authorized_by_id, legacy.authorized_by_id
FROM canonical
JOIN credential_profile legacy ON legacy.connector_id = 'appgrowing'
JOIN "user" app_user ON app_user.id = legacy.authorized_by_id
ON CONFLICT DO NOTHING;

WITH fallback_manager AS (
    SELECT DISTINCT ON (profile.id)
        profile.id AS profile_id,
        member.user_id
    FROM credential_profile profile
    JOIN member ON member.workspace_id = profile.workspace_id
    WHERE member.role IN ('owner', 'admin')
      AND NOT EXISTS (
          SELECT 1
          FROM credential_profile_manager manager
          WHERE manager.profile_id = profile.id
      )
    ORDER BY profile.id, CASE member.role WHEN 'owner' THEN 0 ELSE 1 END, member.created_at
)
INSERT INTO credential_profile_manager (profile_id, user_id, granted_by_id)
SELECT profile_id, user_id, user_id
FROM fallback_manager
ON CONFLICT DO NOTHING;

CREATE TABLE credential_usage_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES credential_profile(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    requested_by_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    connector_id TEXT NOT NULL,
    capability TEXT NOT NULL,
    outcome TEXT NOT NULL
        CHECK (outcome IN ('completed', 'need_reauth', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX credential_usage_audit_workspace_created_idx
    ON credential_usage_audit(workspace_id, created_at DESC);

CREATE INDEX credential_usage_audit_profile_created_idx
    ON credential_usage_audit(profile_id, created_at DESC);
