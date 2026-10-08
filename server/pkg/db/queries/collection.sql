-- name: ListCollections :many
SELECT c.*,
 (SELECT count(*) FROM record r WHERE r.workspace_id=c.workspace_id AND r.collection_id=c.id AND r.deleted_at IS NULL)::bigint AS record_count
FROM collection c WHERE c.workspace_id=$1 AND (c.archived_at IS NOT NULL)=sqlc.arg(archived)::boolean AND collection_can_read(c.id,sqlc.arg(user_id)::uuid) ORDER BY c.created_at DESC,c.id;

-- name: UpdateCollection :one
UPDATE collection SET name=COALESCE(sqlc.narg(name),name),
 title_name=COALESCE(sqlc.narg(title_name),title_name),
 icon=COALESCE(sqlc.narg(icon),icon), description=COALESCE(sqlc.narg(description),description),
 archived_at=CASE WHEN sqlc.arg(archive)::boolean THEN now() ELSE archived_at END,
 revision=revision+1,updated_at=now()
WHERE workspace_id=sqlc.arg(workspace_id) AND id=sqlc.arg(id) AND archived_at IS NULL RETURNING *;

-- name: GetCollection :one
SELECT * FROM collection WHERE workspace_id=$1 AND id=$2 AND archived_at IS NULL;

-- name: LockCollection :one
SELECT * FROM collection WHERE workspace_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE;

-- name: CreateCollection :one
INSERT INTO collection (workspace_id,project_id,name,created_by,icon,description) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: ListCollectionFields :many
SELECT * FROM collection_field WHERE workspace_id=$1 AND collection_id=$2 AND archived_at IS NULL ORDER BY position,id;

-- name: CreateCollectionField :one
INSERT INTO collection_field (workspace_id,collection_id,name,type,config,position) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: GetCollectionField :one
SELECT * FROM collection_field WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND archived_at IS NULL;

-- name: CreateCollectionRecord :one
INSERT INTO record (workspace_id,collection_id,title,fields) VALUES($1,$2,$3,$4) RETURNING *;

-- name: SoftDeleteCollectionRecord :one
UPDATE record SET deleted_at=now(),revision=revision+1,updated_at=now()
WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND deleted_at IS NULL RETURNING *;

-- name: RestoreCollectionRecord :one
UPDATE record SET deleted_at=NULL,revision=revision+1,updated_at=now()
WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND deleted_at IS NOT NULL AND deleted_at > now()-interval '30 days' RETURNING *;

-- name: ListDeletedCollectionRecords :many
SELECT * FROM record WHERE workspace_id=$1 AND collection_id=$2 AND deleted_at IS NOT NULL AND deleted_at > now()-interval '30 days'
ORDER BY deleted_at DESC,id LIMIT 200;

-- name: CountDeletedCollectionRecords :one
SELECT count(*)::bigint FROM record WHERE workspace_id=$1 AND collection_id=$2 AND deleted_at IS NOT NULL AND deleted_at > now()-interval '30 days';

-- name: UpdateCollectionField :one
UPDATE collection_field SET name=COALESCE(sqlc.narg(name),name),type=COALESCE(sqlc.narg(type),type),
 config=COALESCE(sqlc.narg(config)::jsonb,config),position=COALESCE(sqlc.narg(position),position),
 archived_at=CASE WHEN sqlc.arg(archive)::boolean THEN now() ELSE archived_at END
WHERE workspace_id=sqlc.arg(workspace_id) AND collection_id=sqlc.arg(collection_id) AND id=sqlc.arg(id) AND archived_at IS NULL RETURNING *;

-- name: StripCollectionFieldOptions :execrows
-- Removed select options disappear from every live or trashed record, so a
-- deleted option never resolves to a raw id.
UPDATE record SET fields=CASE
  WHEN jsonb_typeof(fields->sqlc.arg(field_id)::text)='array' THEN
   CASE WHEN (SELECT count(*) FROM jsonb_array_elements(fields->sqlc.arg(field_id)::text) e WHERE NOT (e #>> '{}')=ANY(sqlc.arg(removed)::text[]))=0
    THEN fields-sqlc.arg(field_id)::text
    ELSE jsonb_set(fields,ARRAY[sqlc.arg(field_id)::text],(SELECT jsonb_agg(e) FROM jsonb_array_elements(fields->sqlc.arg(field_id)::text) e WHERE NOT (e #>> '{}')=ANY(sqlc.arg(removed)::text[]))) END
  ELSE fields-sqlc.arg(field_id)::text END,
 revision=revision+1,updated_at=now()
WHERE workspace_id=sqlc.arg(workspace_id) AND collection_id=sqlc.arg(collection_id) AND fields ? sqlc.arg(field_id)::text
 AND ((fields->>sqlc.arg(field_id)::text)=ANY(sqlc.arg(removed)::text[])
  OR (jsonb_typeof(fields->sqlc.arg(field_id)::text)='array' AND (fields->sqlc.arg(field_id)::text) ?| sqlc.arg(removed)::text[]));

-- name: ConvertCollectionFieldValues :execrows
-- Safe type changes only: wrap a scalar into a list, keep the first list
-- entry, or keep the scalar's text form.
UPDATE record SET fields=CASE sqlc.arg(mode)::text
  WHEN 'wrap' THEN jsonb_set(fields,ARRAY[sqlc.arg(field_id)::text],jsonb_build_array(fields->sqlc.arg(field_id)::text))
  WHEN 'first' THEN CASE WHEN jsonb_typeof(fields->sqlc.arg(field_id)::text)='array' AND jsonb_array_length(fields->sqlc.arg(field_id)::text)>0
    THEN jsonb_set(fields,ARRAY[sqlc.arg(field_id)::text],fields->sqlc.arg(field_id)::text->0) ELSE fields-sqlc.arg(field_id)::text END
  ELSE jsonb_set(fields,ARRAY[sqlc.arg(field_id)::text],to_jsonb(fields->>sqlc.arg(field_id)::text)) END,
 revision=revision+1,updated_at=now()
WHERE workspace_id=sqlc.arg(workspace_id) AND collection_id=sqlc.arg(collection_id) AND fields ? sqlc.arg(field_id)::text
 AND jsonb_typeof(fields->sqlc.arg(field_id)::text)<>'null';

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

-- name: GetArchivedCollection :one
SELECT * FROM collection WHERE workspace_id=$1 AND id=$2 AND archived_at IS NOT NULL;

-- name: RestoreCollection :one
UPDATE collection SET archived_at=NULL,revision=revision+1,updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND archived_at IS NOT NULL RETURNING *;

-- name: ListArchivedCollectionFields :many
SELECT * FROM collection_field WHERE workspace_id=$1 AND collection_id=$2 AND archived_at IS NOT NULL ORDER BY position,id;

-- name: RestoreCollectionField :one
UPDATE collection_field SET archived_at=NULL
WHERE workspace_id=$1 AND collection_id=$2 AND id=$3 AND archived_at IS NOT NULL RETURNING *;
