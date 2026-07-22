-- name: CreatePreviewSession :one
INSERT INTO preview_session (
    workspace_id, issue_id, task_id, platform, provider, title, preview_url,
    status, creator_type, creator_id, expires_at, started_at, last_active_at,
    lease_expires_at
) VALUES (
    $1, $2, sqlc.narg('task_id'), $3, $4, $5, $6,
    'running', $7, $8, sqlc.narg('expires_at'), now(),
    sqlc.narg('last_active_at'), sqlc.narg('lease_expires_at')
)
RETURNING *;

-- name: ListPreviewSessionsByIssue :many
SELECT * FROM preview_session
WHERE issue_id = $1
  AND workspace_id = $2
ORDER BY created_at DESC, id DESC;

-- name: GetPreviewSessionInWorkspace :one
SELECT * FROM preview_session
WHERE id = $1
  AND workspace_id = $2;

-- name: StopPreviewSession :one
UPDATE preview_session
SET status = CASE
        WHEN status IN ('creating', 'starting', 'running', 'sleeping', 'stopping') THEN 'stopped'
        ELSE status
    END,
    stopped_at = CASE
        WHEN status IN ('creating', 'starting', 'running', 'sleeping', 'stopping') THEN COALESCE(stopped_at, now())
        ELSE stopped_at
    END,
    updated_at = CASE
        WHEN status IN ('creating', 'starting', 'running', 'sleeping', 'stopping') THEN now()
        ELSE updated_at
    END
WHERE id = $1
  AND workspace_id = $2
RETURNING *;

-- name: LockPreviewDeviceResource :exec
SELECT pg_advisory_xact_lock(hashtextextended($1, 0));

-- name: GetConflictingPreviewDeviceLease :one
SELECT * FROM preview_session
WHERE workspace_id = $1
  AND provider = 'local_device'
  AND preview_url = $2
  AND id <> $3
  AND status IN ('creating', 'starting', 'running', 'sleeping')
  AND lease_expires_at > now()
ORDER BY lease_expires_at DESC
LIMIT 1
FOR UPDATE;

-- name: SleepOtherPreviewDeviceSessions :exec
UPDATE preview_session
SET status = 'sleeping',
    lease_expires_at = NULL,
    updated_at = now()
WHERE workspace_id = $1
  AND provider = 'local_device'
  AND preview_url = $2
  AND id <> $3
  AND status IN ('creating', 'starting', 'running', 'sleeping')
  AND (lease_expires_at IS NULL OR lease_expires_at <= now());

-- name: TouchPreviewDeviceSession :one
UPDATE preview_session
SET status = 'running',
    last_active_at = now(),
    lease_expires_at = now() + INTERVAL '5 minutes',
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND provider = 'local_device'
  AND status IN ('creating', 'starting', 'running', 'sleeping')
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: SwitchPreviewDeviceSession :one
UPDATE preview_session
SET preview_url = $3,
    status = 'running',
    last_active_at = now(),
    lease_expires_at = now() + INTERVAL '5 minutes',
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND provider = 'local_device'
  AND status IN ('creating', 'starting', 'running', 'sleeping')
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;
