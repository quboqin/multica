-- name: ListCollections :many
SELECT * FROM collection WHERE workspace_id=$1 AND archived_at IS NULL ORDER BY created_at DESC,id;

-- name: GetCollection :one
SELECT * FROM collection WHERE workspace_id=$1 AND id=$2 AND archived_at IS NULL;

-- name: LockCollection :one
SELECT * FROM collection WHERE workspace_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE;

-- name: CreateCollection :one
INSERT INTO collection (workspace_id,project_id,name,created_by) VALUES($1,$2,$3,$4) RETURNING *;

-- name: ListCollectionFields :many
SELECT * FROM collection_field WHERE workspace_id=$1 AND collection_id=$2 AND archived_at IS NULL ORDER BY position,id;

-- name: CreateCollectionField :one
INSERT INTO collection_field (workspace_id,collection_id,name,type,config,position) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: GetCollectionField :one
SELECT * FROM collection_field WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND archived_at IS NULL;

-- name: CreateCollectionRecord :one
INSERT INTO record (workspace_id,collection_id,title) VALUES($1,$2,$3) RETURNING *;

-- name: SetCollectionRecordField :one
UPDATE record SET fields=CASE WHEN sqlc.arg(unset)::boolean THEN fields-sqlc.arg(field_id)::text
 ELSE jsonb_set(fields,ARRAY[sqlc.arg(field_id)::text],sqlc.arg(value)::jsonb) END,
 revision=revision+1,updated_at=now()
WHERE workspace_id=sqlc.arg(workspace_id) AND collection_id=sqlc.arg(collection_id) AND id=sqlc.arg(id) AND deleted_at IS NULL
 AND (NOT sqlc.arg(compare_value)::boolean OR COALESCE(fields->sqlc.arg(field_id)::text,'null'::jsonb)=sqlc.arg(expected_value)::jsonb OR COALESCE(fields->sqlc.arg(field_id)::text,'null'::jsonb)=sqlc.arg(value)::jsonb)
RETURNING *;

-- name: UpdateCollectionRecordTitle :one
UPDATE record SET title=sqlc.arg(title),revision=revision+1,updated_at=now()
WHERE workspace_id=sqlc.arg(workspace_id) AND collection_id=sqlc.arg(collection_id) AND id=sqlc.arg(id) AND deleted_at IS NULL
 AND (title=sqlc.arg(title_base) OR title=sqlc.arg(title)) RETURNING *;
