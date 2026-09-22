-- name: GetDocumentAccess :one
SELECT * FROM document_access WHERE issue_id=$1 AND workspace_id=$2;

-- name: DocumentCanRead :one
SELECT document_can_read(sqlc.arg(issue_id)::uuid,sqlc.arg(user_id)::uuid)::boolean;

-- name: GetDocumentPermission :one
SELECT CASE WHEN NOT document_can_read(d.issue_id,sqlc.arg(user_id)::uuid) THEN ''
    WHEN d.owner_id=sqlc.arg(user_id)::uuid THEN 'owner'
    WHEN d.scope_role='edit' AND (d.scope='workspace' OR (d.scope='project' AND EXISTS (SELECT 1 FROM project p WHERE p.id=d.project_id AND p.workspace_id=d.workspace_id))) THEN 'edit'
    WHEN EXISTS (SELECT 1 FROM document_collaborator c WHERE c.issue_id=d.issue_id AND c.workspace_id=d.workspace_id AND c.user_id=sqlc.arg(user_id)::uuid AND c.role='edit') THEN 'edit'
    ELSE 'view' END::text AS permission
FROM document_access d WHERE d.issue_id=sqlc.arg(issue_id) AND d.workspace_id=sqlc.arg(workspace_id);

-- name: ListDocumentReaders :many
SELECT m.user_id FROM document_access d JOIN member m ON m.workspace_id=d.workspace_id
WHERE d.issue_id=$1 AND d.workspace_id=$2 AND document_can_read(d.issue_id,m.user_id);

-- name: ListDocumentCollaborators :many
SELECT c.* FROM document_collaborator c JOIN member m ON m.workspace_id=c.workspace_id AND m.user_id=c.user_id
WHERE c.issue_id=$1 AND c.workspace_id=$2 ORDER BY c.user_id;

-- name: UpdateDocumentAccess :one
UPDATE document_access SET scope=$3,project_id=$4,scope_role=$5,revision=revision+1,updated_at=now()
WHERE issue_id=$1 AND workspace_id=$2 RETURNING *;

-- name: ClearDocumentCollaborators :exec
DELETE FROM document_collaborator WHERE issue_id=$1 AND workspace_id=$2;

-- name: AddDocumentCollaborator :exec
INSERT INTO document_collaborator(issue_id,workspace_id,user_id,role) VALUES($1,$2,$3,$4);

-- name: SetDocumentAuditActor :exec
SELECT set_config('multica.document_actor_type',sqlc.arg(actor_type)::text,true),
    set_config('multica.document_actor_id',sqlc.arg(actor_id)::text,true),
    set_config('multica.document_action',sqlc.arg(action)::text,true),
    set_config('multica.document_restored_from',sqlc.arg(restored_from)::text,true);

-- name: SetDocumentOwner :exec
SELECT set_config('multica.document_owner',sqlc.arg(owner_id)::text,true);

-- name: ListDocumentVersions :many
SELECT issue_id,workspace_id,version,title,actor_type,actor_id,action,restored_from,created_at
FROM document_version WHERE issue_id=sqlc.arg(issue_id) AND workspace_id=sqlc.arg(workspace_id)
AND (sqlc.arg(before_version)::bigint=0 OR version<sqlc.arg(before_version))
ORDER BY version DESC LIMIT 50;

-- name: GetDocumentVersion :one
SELECT * FROM document_version WHERE issue_id=$1 AND workspace_id=$2 AND version=$3;

-- name: RestoreDocumentVersion :one
UPDATE issue SET title=$3,description=$4,revision=revision+1,updated_at=now()
WHERE id=$1 AND workspace_id=$2 AND kind='doc' RETURNING *;

-- name: DeleteDocumentAccess :exec
DELETE FROM document_access WHERE workspace_id=$1 AND issue_id=$2;

-- name: DeleteDocumentVersions :exec
DELETE FROM document_version WHERE workspace_id=$1 AND issue_id=$2;

-- name: RevokeDeletedProjectDocumentShares :exec
UPDATE document_access SET scope='private',project_id=NULL,revision=revision+1,updated_at=now()
WHERE workspace_id=$1 AND project_id=$2;

-- name: ListOwnedDocuments :many
SELECT i.* FROM issue i JOIN document_access d ON d.issue_id=i.id AND d.workspace_id=i.workspace_id
WHERE i.workspace_id=$1 AND d.owner_id=$2 AND i.kind='doc' ORDER BY i.position,i.created_at,i.id;

-- name: ListProjectSharedDocuments :many
SELECT i.* FROM issue i JOIN document_access d ON d.issue_id=i.id AND d.workspace_id=i.workspace_id
WHERE i.workspace_id=$1 AND d.project_id=$2 AND d.scope='project';

-- name: ListReadableIssueIDs :many
SELECT id FROM issue WHERE workspace_id=sqlc.arg(workspace_id)
AND id=ANY(sqlc.arg(issue_ids)::uuid[])
AND (kind<>'doc' OR document_can_read(id,sqlc.arg(user_id)::uuid));
