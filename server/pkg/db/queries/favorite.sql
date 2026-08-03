-- name: ListFavoriteCategories :many
SELECT
    sqlc.embed(c),
    count(fi.item_id)::bigint AS favorite_count
FROM favorite_category c
LEFT JOIN favorite_item fi ON fi.category_id = c.id
  AND (
    fi.item_type = 'project'
    OR (fi.item_type = 'issue' AND EXISTS (
      SELECT 1
      FROM issue i
      WHERE i.id = fi.item_id AND i.is_active = TRUE
    ))
    OR (fi.item_type = 'attachment' AND EXISTS (
      SELECT 1
      FROM attachment a
      LEFT JOIN issue i ON i.id = a.issue_id
      LEFT JOIN comment cm ON cm.id = a.comment_id
      WHERE a.id = fi.item_id
        AND (a.issue_id IS NULL OR i.is_active = TRUE)
        AND (a.comment_id IS NULL OR cm.is_active = TRUE)
    ))
  )
WHERE c.user_id = $1 AND c.workspace_id = $2
GROUP BY c.id
ORDER BY c.is_default DESC, c.created_at ASC, lower(c.name) ASC;

-- name: CreateFavoriteCategory :one
INSERT INTO favorite_category (workspace_id, user_id, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetFavoriteCategoryForUser :one
SELECT *
FROM favorite_category
WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: UpdateFavoriteCategory :one
UPDATE favorite_category
SET name = $4, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND user_id = $3
RETURNING *;

-- name: CountFavoritesByCategory :one
SELECT count(*)::bigint
FROM favorite_item
WHERE category_id = $1
  AND user_id = $2
  AND (
    item_type = 'project'
    OR (item_type = 'issue' AND EXISTS (
      SELECT 1
      FROM issue i
      WHERE i.id = favorite_item.item_id AND i.is_active = TRUE
    ))
    OR (item_type = 'attachment' AND EXISTS (
      SELECT 1
      FROM attachment a
      LEFT JOIN issue i ON i.id = a.issue_id
      LEFT JOIN comment cm ON cm.id = a.comment_id
      WHERE a.id = favorite_item.item_id
        AND (a.issue_id IS NULL OR i.is_active = TRUE)
        AND (a.comment_id IS NULL OR cm.is_active = TRUE)
    ))
  );

-- name: DeleteFavoriteCategory :one
DELETE FROM favorite_category
WHERE id = $1 AND workspace_id = $2 AND user_id = $3
RETURNING *;

-- name: GetOrCreateDefaultFavoriteCategory :one
INSERT INTO favorite_category (workspace_id, user_id, name, is_default)
VALUES ($1, $2, 'Default', true)
ON CONFLICT (workspace_id, user_id) WHERE is_default
DO UPDATE SET updated_at = favorite_category.updated_at
RETURNING *;

-- name: ListFavorites :many
SELECT
    sqlc.embed(fi),
    sqlc.embed(c)
FROM favorite_item fi
JOIN favorite_category c ON c.id = fi.category_id
WHERE fi.workspace_id = $1 AND fi.user_id = $2
  AND (
    fi.item_type = 'project'
    OR (fi.item_type = 'issue' AND EXISTS (
      SELECT 1
      FROM issue i
      WHERE i.id = fi.item_id AND i.is_active = TRUE
    ))
    OR (fi.item_type = 'attachment' AND EXISTS (
      SELECT 1
      FROM attachment a
      LEFT JOIN issue i ON i.id = a.issue_id
      LEFT JOIN comment cm ON cm.id = a.comment_id
      WHERE a.id = fi.item_id
        AND (a.issue_id IS NULL OR i.is_active = TRUE)
        AND (a.comment_id IS NULL OR cm.is_active = TRUE)
    ))
  )
ORDER BY fi.created_at DESC;

-- name: PutFavorite :one
INSERT INTO favorite_item (workspace_id, user_id, item_type, item_id, category_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id, user_id, item_type, item_id)
DO UPDATE SET item_id = EXCLUDED.item_id
RETURNING *;

-- name: MoveFavorite :one
UPDATE favorite_item
SET category_id = $5
WHERE workspace_id = $1 AND user_id = $2 AND item_type = $3 AND item_id = $4
RETURNING *;

-- name: DeleteFavorite :exec
DELETE FROM favorite_item
WHERE workspace_id = $1 AND user_id = $2 AND item_type = $3 AND item_id = $4;
