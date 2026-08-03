-- name: ListPinnedItems :many
SELECT p.* FROM pinned_item p
WHERE p.workspace_id = $1 AND p.user_id = $2
  AND (
    p.item_type <> 'issue'
    OR EXISTS (
      SELECT 1 FROM issue i
      WHERE i.id = p.item_id AND i.is_active = TRUE
    )
  )
ORDER BY p.position ASC, p.created_at ASC;

-- name: CreatePinnedItem :one
INSERT INTO pinned_item (workspace_id, user_id, item_type, item_id, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeletePinnedItem :exec
DELETE FROM pinned_item
WHERE workspace_id = $1 AND user_id = $2 AND item_type = $3 AND item_id = $4;

-- name: UpdatePinnedItemPosition :exec
UPDATE pinned_item SET position = $1
WHERE id = $2 AND workspace_id = $3 AND user_id = $4;

-- name: GetMaxPinnedItemPosition :one
SELECT COALESCE(MAX(position), 0)::float8 AS max_position
FROM pinned_item
WHERE workspace_id = $1 AND user_id = $2;

-- name: DeletePinnedItemsByItem :exec
DELETE FROM pinned_item
WHERE item_type = $1 AND item_id = $2;
