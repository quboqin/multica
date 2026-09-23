-- name: LockCollectionRecord :one
-- Link writes to one record queue behind this row lock, so the duplicate check
-- and the per-field cap cannot race each other.
SELECT * FROM record WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND deleted_at IS NULL FOR UPDATE;

-- name: TouchCollectionRecord :one
-- Links are part of what a record shows, so changing them moves its revision.
UPDATE record SET revision=revision+1,updated_at=now()
WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND deleted_at IS NULL RETURNING *;

-- name: GetLiveCollectionRecord :one
SELECT r.* FROM record r
JOIN collection c ON c.id=r.collection_id AND c.workspace_id=r.workspace_id AND c.archived_at IS NULL
WHERE r.workspace_id=$1 AND r.collection_id=$2 AND r.id=$3 AND r.deleted_at IS NULL;

-- name: CountRecordFieldLinks :one
SELECT count(*)::bigint FROM record_link WHERE workspace_id=$1 AND from_record_id=$2 AND from_field_id=$3;

-- name: GetRecordLinkEdge :one
SELECT * FROM record_link
WHERE workspace_id=$1 AND from_record_id=$2 AND from_field_id=$3 AND to_type=$4 AND to_id=$5;

-- name: CreateRecordLink :one
INSERT INTO record_link (workspace_id,collection_id,from_record_id,from_field_id,to_type,to_id)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: DeleteRecordLink :execrows
DELETE FROM record_link WHERE workspace_id=$1 AND from_record_id=$2 AND id=$3;

-- name: ListRecordLinks :many
-- Every edge of the given records with what its target looks like now. A target
-- that was deleted, trashed or archived away leaves the edge and reads missing.
SELECT l.id,l.from_record_id,l.from_field_id,l.to_type,l.to_id,
 i.number AS issue_number,i.title AS issue_title,i.status AS issue_status,
 t.title AS record_title,tc.id AS record_collection_id
FROM record_link l
LEFT JOIN issue i ON l.to_type='issue' AND i.id=l.to_id AND i.workspace_id=l.workspace_id
LEFT JOIN record t ON l.to_type='record' AND t.id=l.to_id AND t.workspace_id=l.workspace_id AND t.deleted_at IS NULL
LEFT JOIN collection tc ON tc.id=t.collection_id AND tc.workspace_id=t.workspace_id AND tc.archived_at IS NULL AND collection_can_read(tc.id,sqlc.arg(user_id)::uuid)
WHERE l.workspace_id=$1 AND l.from_record_id=ANY(sqlc.arg(record_ids)::uuid[])
ORDER BY l.created_at,l.id;

-- name: ListRecordLinksTo :many
-- What points at one task or record. Only edges a reader can still open count:
-- the source record, its table and the relation field must all be live.
SELECT l.id,l.collection_id,l.from_record_id,l.from_field_id,
 r.title AS record_title,c.name AS collection_name,f.name AS field_name
FROM record_link l
JOIN record r ON r.id=l.from_record_id AND r.workspace_id=l.workspace_id AND r.deleted_at IS NULL
JOIN collection c ON c.id=l.collection_id AND c.workspace_id=l.workspace_id AND c.archived_at IS NULL
JOIN collection_field f ON f.id=l.from_field_id AND f.workspace_id=l.workspace_id AND f.archived_at IS NULL
WHERE l.workspace_id=$1 AND l.to_type=$2 AND l.to_id=$3 AND collection_can_read(c.id,sqlc.arg(user_id)::uuid)
ORDER BY lower(c.name),lower(r.title),l.id LIMIT 200;
