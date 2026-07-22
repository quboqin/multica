-- name: ListMilestones :many
SELECT * FROM milestone
WHERE workspace_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY position ASC, created_at ASC;

-- name: GetMilestone :one
SELECT * FROM milestone
WHERE id = $1;

-- name: GetMilestoneInWorkspace :one
SELECT * FROM milestone
WHERE id = $1 AND workspace_id = $2;

-- name: CreateMilestone :one
INSERT INTO milestone (
    workspace_id, title, description, start_date, end_date,
    status, position, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: UpdateMilestone :one
UPDATE milestone SET
    title = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    start_date = sqlc.narg('start_date'),
    end_date = sqlc.narg('end_date'),
    status = COALESCE(sqlc.narg('status'), status),
    position = COALESCE(sqlc.narg('position'), position),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
RETURNING *;

-- name: DeleteMilestone :exec
DELETE FROM milestone WHERE id = $1 AND workspace_id = $2;
