-- name: CreateCredentialProfile :one
INSERT INTO credential_profile (
    workspace_id, authorized_by_id, connector_id, label, status, expires_hint
)
VALUES (
    $1, $2, $3, $4, $5, sqlc.narg('expires_hint')
)
RETURNING *;

-- name: ListCredentialProfilesForWorkspace :many
SELECT * FROM credential_profile
WHERE workspace_id = $1
  AND status <> 'revoked'
ORDER BY updated_at DESC, created_at DESC;

-- name: GetCredentialProfileForWorkspace :one
SELECT * FROM credential_profile
WHERE id = $1
  AND workspace_id = $2
  AND status <> 'revoked';

-- name: GetCredentialProfileForWorkspaceByConnector :one
SELECT * FROM credential_profile
WHERE workspace_id = $1
  AND connector_id = $2
  AND status <> 'revoked';

-- name: GetActiveCredentialProfileForWorkspaceByConnector :one
SELECT * FROM credential_profile
WHERE workspace_id = $1
  AND connector_id = $2
  AND status = 'active';

-- name: UpdateCredentialProfileStatus :one
UPDATE credential_profile
SET status = $2,
    expires_hint = sqlc.narg('expires_hint'),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateCredentialProfileAuthorizedBy :one
UPDATE credential_profile
SET authorized_by_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: TouchCredentialProfileLastUsed :one
UPDATE credential_profile
SET last_used_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RevokeCredentialProfileForWorkspace :one
UPDATE credential_profile
SET status = 'revoked',
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
RETURNING *;

-- name: UpsertCredentialSecret :exec
INSERT INTO credential_secret (profile_id, ciphertext, key_version, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (profile_id) DO UPDATE SET
    ciphertext = EXCLUDED.ciphertext,
    key_version = EXCLUDED.key_version,
    updated_at = now();

-- name: GetCredentialSecret :one
SELECT * FROM credential_secret
WHERE profile_id = $1;

-- name: DeleteCredentialSecret :exec
DELETE FROM credential_secret
WHERE profile_id = $1;

-- name: AbortPendingCredentialLoginSessions :execrows
UPDATE credential_login_session
SET status = 'abandoned',
    updated_at = now()
WHERE profile_id = $1
  AND status = 'pending';

-- name: CreateCredentialLoginSession :one
INSERT INTO credential_login_session (
    profile_id, user_id, connector_id, token_hash, browser_url, expires_at
)
VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetCredentialLoginSession :one
SELECT * FROM credential_login_session
WHERE id = $1;

-- name: GetPendingCredentialLoginSessionByTokenHash :one
SELECT * FROM credential_login_session
WHERE token_hash = $1
  AND status = 'pending'
  AND expires_at > now();

-- name: CompleteCredentialLoginSession :one
UPDATE credential_login_session
SET status = 'completed',
    consumed_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ExpireCredentialLoginSessions :execrows
UPDATE credential_login_session
SET status = 'expired',
    updated_at = now()
WHERE status = 'pending'
  AND expires_at <= $1;
