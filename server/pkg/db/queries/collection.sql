-- name: SetCollectionWorkspaceContext :exec
SELECT set_config('app.workspace_id', $1, true);

-- name: LockCollectionWorkspace :one
SELECT id FROM workspace WHERE id = $1 FOR KEY SHARE;

-- name: LockCollectionMember :one
SELECT id, workspace_id, user_id, role, created_at
FROM member
WHERE workspace_id = $1 AND user_id = $2
FOR SHARE;

-- name: CreateCollectionIfAbsent :one
INSERT INTO collection (
  id, workspace_id, name, created_by, create_request_id, create_fingerprint
) VALUES (
  COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
  sqlc.arg('workspace_id'),
  sqlc.arg('name'),
  sqlc.arg('created_by'),
  sqlc.arg('create_request_id'),
  sqlc.arg('create_fingerprint')
)
ON CONFLICT (workspace_id, created_by, create_request_id) DO NOTHING
RETURNING *;

-- name: GetCollectionByCreateRequest :one
SELECT * FROM collection
WHERE workspace_id = $1 AND created_by = $2 AND create_request_id = $3;

-- name: CreateCollectionField :one
INSERT INTO collection_field (
  id, workspace_id, collection_id, name, type, position
) VALUES (
  COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
  sqlc.arg('workspace_id'),
  sqlc.arg('collection_id'),
  sqlc.arg('name'),
  sqlc.arg('type'),
  sqlc.arg('position')
)
RETURNING *;

-- name: ListCollectionsFirstPage :many
SELECT * FROM collection
WHERE workspace_id = $1 AND archived_at IS NULL
ORDER BY created_at ASC, id ASC
LIMIT $2;

-- name: ListCollectionsPage :many
SELECT * FROM collection
WHERE workspace_id = $1
  AND archived_at IS NULL
  AND (created_at, id) > (
    sqlc.arg('after_created_at')::timestamptz,
    sqlc.arg('after_id')::uuid
  )
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CountCollections :one
SELECT COUNT(*) FROM collection
WHERE workspace_id = $1 AND archived_at IS NULL;

-- name: GetCollectionInWorkspace :one
SELECT * FROM collection
WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL;

-- name: LockCollectionInWorkspace :one
SELECT * FROM collection
WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
FOR SHARE;

-- name: ListCollectionFields :many
SELECT * FROM collection_field
WHERE workspace_id = $1 AND collection_id = $2 AND archived_at IS NULL
ORDER BY position ASC, id ASC;

-- name: CreateRecordIfAbsent :one
INSERT INTO record (
  id, workspace_id, collection_id, title, fields, created_by, updated_by,
  create_request_id, create_fingerprint
) VALUES (
  COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
  sqlc.arg('workspace_id'),
  sqlc.arg('collection_id'),
  sqlc.arg('title'),
  sqlc.arg('fields'),
  sqlc.arg('created_by'),
  sqlc.arg('updated_by'),
  sqlc.arg('create_request_id'),
  sqlc.arg('create_fingerprint')
)
ON CONFLICT (workspace_id, collection_id, created_by, create_request_id) DO NOTHING
RETURNING *;

-- name: GetRecordByCreateRequest :one
SELECT * FROM record
WHERE workspace_id = $1
  AND collection_id = $2
  AND created_by = $3
  AND create_request_id = $4;

-- name: ListRecordsFirstPage :many
SELECT * FROM record
WHERE workspace_id = $1 AND collection_id = $2 AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC
LIMIT $3;

-- name: ListRecordsPage :many
SELECT * FROM record
WHERE workspace_id = $1
  AND collection_id = $2
  AND deleted_at IS NULL
  AND (created_at, id) > (
    sqlc.arg('after_created_at')::timestamptz,
    sqlc.arg('after_id')::uuid
  )
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: CountRecords :one
SELECT COUNT(*) FROM record
WHERE workspace_id = $1 AND collection_id = $2 AND deleted_at IS NULL;

-- name: GetRecordInCollection :one
SELECT * FROM record
WHERE id = $1
  AND workspace_id = $2
  AND collection_id = $3
  AND deleted_at IS NULL;

-- name: UpdateRecordTitleCAS :one
UPDATE record
SET title = $5, updated_by = $4, revision = revision + 1, updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND collection_id = $3
  AND deleted_at IS NULL
  AND revision = $6
RETURNING *;

-- name: UpdateRecordFieldsCAS :one
UPDATE record
SET fields = $5, updated_by = $4, revision = revision + 1, updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND collection_id = $3
  AND deleted_at IS NULL
  AND revision = $6
RETURNING *;

-- name: DeleteWorkspaceRecords :exec
DELETE FROM record WHERE workspace_id = $1;

-- name: DeleteWorkspaceCollectionFields :exec
DELETE FROM collection_field WHERE workspace_id = $1;

-- name: DeleteWorkspaceCollections :exec
DELETE FROM collection WHERE workspace_id = $1;
