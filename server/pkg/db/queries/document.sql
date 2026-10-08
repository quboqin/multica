-- name: ListDocuments :many
SELECT sqlc.embed(issue), sharing.owner_id FROM issue JOIN document_access sharing ON sharing.issue_id=issue.id AND sharing.workspace_id=issue.workspace_id
WHERE issue.workspace_id = sqlc.arg(workspace_id) AND issue.kind = 'doc'
  AND document_can_read(issue.id, sqlc.arg(user_id)::uuid)
  AND (sqlc.narg(project_id)::uuid IS NULL OR issue.project_id = sqlc.narg(project_id) OR EXISTS (SELECT 1 FROM document_access d WHERE d.issue_id=issue.id AND d.project_id=sqlc.narg(project_id)))
ORDER BY issue.position, issue.created_at, issue.id;

-- name: AuthorizeDocumentWrite :exec
SELECT set_config('multica.document_write', sqlc.arg(document_write)::text, true);

-- name: AuthorizeDocumentTransition :exec
SELECT set_config('multica.document_transition', sqlc.arg(issue_id)::text, true);

-- name: LockDocumentTree :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(workspace_id)::text, 531));

-- name: MoveDocument :one
UPDATE issue SET parent_issue_id = sqlc.narg(parent_issue_id), position = sqlc.arg(position),
 revision = revision + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND kind = 'doc'
RETURNING *;

-- name: TransitionDocument :one
UPDATE issue SET status = sqlc.arg(status), revision = revision + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND kind = 'doc'
RETURNING *;

-- name: RecordDocumentTransition :exec
INSERT INTO document_publication (issue_id, workspace_id, document_revision, actor_id, action, body, ingestion_state)
VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (issue_id,document_revision,action) DO UPDATE SET actor_id=EXCLUDED.actor_id, body=EXCLUDED.body, ingestion_state=EXCLUDED.ingestion_state, created_at=now();

-- name: SeedDocumentStatuses :exec
INSERT INTO issue_status (workspace_id, key, name, description, category, color, is_system, position)
VALUES (sqlc.arg(workspace_id), 'draft', 'Draft', '', 'unstarted', '#6b7280', FALSE, 0),
 (sqlc.arg(workspace_id), 'reviewing', 'Reviewing', '', 'started', '#f59e0b', FALSE, 0),
 (sqlc.arg(workspace_id), 'published', 'Published', '', 'done', '#22c55e', FALSE, 0)
ON CONFLICT DO NOTHING;

-- name: DeleteDocumentPublications :exec
DELETE FROM document_publication WHERE workspace_id=$1 AND issue_id=$2;

-- name: DetachProjectCollections :exec
UPDATE collection SET project_id=NULL,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND project_id=$2;

-- name: CancelDocumentIngestion :exec
UPDATE document_publication SET ingestion_state='not_requested'
WHERE workspace_id=$1 AND issue_id=$2 AND ingestion_state='pending';
