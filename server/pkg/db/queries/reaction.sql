-- name: AddReaction :one
WITH active_comment AS (
    SELECT c.id
    FROM comment c
    WHERE c.id = $1
      AND c.workspace_id = $2
      AND c.is_active = TRUE
    FOR KEY SHARE
)
INSERT INTO comment_reaction (comment_id, workspace_id, actor_type, actor_id, emoji)
SELECT $1, $2, $3, $4, $5
FROM active_comment
ON CONFLICT (comment_id, actor_type, actor_id, emoji) DO UPDATE SET created_at = comment_reaction.created_at
RETURNING *;

-- name: RemoveReaction :exec
DELETE FROM comment_reaction
WHERE comment_id = $1 AND actor_type = $2 AND actor_id = $3 AND emoji = $4;

-- name: ListReactionsByCommentIDs :many
SELECT * FROM comment_reaction
WHERE comment_id = ANY($1::uuid[])
ORDER BY created_at ASC;
