-- name: GetCollectionAccess :one
SELECT * FROM collection_access WHERE collection_id=$1 AND workspace_id=$2;

-- name: CollectionCanRead :one
SELECT collection_can_read(sqlc.arg(collection_id)::uuid,sqlc.arg(user_id)::uuid)::boolean;

-- name: GetCollectionPermission :one
SELECT CASE WHEN NOT collection_can_read(d.collection_id,sqlc.arg(user_id)::uuid) THEN ''
    WHEN d.owner_id=sqlc.arg(user_id)::uuid THEN 'owner'
    WHEN d.scope_role='edit' AND (d.scope='workspace' OR (d.scope='project' AND EXISTS (SELECT 1 FROM project p WHERE p.id=d.project_id AND p.workspace_id=d.workspace_id))) THEN 'edit'
    WHEN EXISTS (SELECT 1 FROM collection_collaborator c WHERE c.collection_id=d.collection_id AND c.workspace_id=d.workspace_id AND c.user_id=sqlc.arg(user_id)::uuid AND c.role='edit') THEN 'edit'
    ELSE 'view' END::text AS permission
FROM collection_access d WHERE d.collection_id=sqlc.arg(collection_id) AND d.workspace_id=sqlc.arg(workspace_id);

-- name: ListCollectionReaders :many
SELECT m.user_id FROM collection_access d JOIN member m ON m.workspace_id=d.workspace_id
WHERE d.collection_id=$1 AND d.workspace_id=$2 AND collection_can_read(d.collection_id,m.user_id);

-- name: ListCollectionCollaborators :many
SELECT c.* FROM collection_collaborator c JOIN member m ON m.workspace_id=c.workspace_id AND m.user_id=c.user_id
WHERE c.collection_id=$1 AND c.workspace_id=$2 ORDER BY c.user_id;

-- name: UpdateCollectionAccess :one
UPDATE collection_access SET scope=$3,project_id=$4,scope_role=$5,revision=revision+1,updated_at=now()
WHERE collection_id=$1 AND workspace_id=$2 RETURNING *;

-- name: ClearCollectionCollaborators :exec
DELETE FROM collection_collaborator WHERE collection_id=$1 AND workspace_id=$2;

-- name: AddCollectionCollaborator :exec
INSERT INTO collection_collaborator(collection_id,workspace_id,user_id,role) VALUES($1,$2,$3,$4);

-- name: RevokeDeletedProjectCollectionShares :exec
UPDATE collection_access SET scope='private',project_id=NULL,revision=revision+1,updated_at=now()
WHERE workspace_id=$1 AND project_id=$2;


-- name: ListProjectSharedCollections :many
SELECT c.* FROM collection c JOIN collection_access a ON a.collection_id=c.id AND a.workspace_id=c.workspace_id
WHERE c.workspace_id=$1 AND a.project_id=$2 AND a.scope='project';
