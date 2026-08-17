-- name: UpsertChatMessageFeedback :exec
INSERT INTO chat_message_feedback (
    workspace_id,
    user_id,
    chat_message_id,
    sentiment,
    comment
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id, user_id, chat_message_id)
DO UPDATE SET
    sentiment = EXCLUDED.sentiment,
    comment = EXCLUDED.comment,
    updated_at = now();

-- name: ListChatMessageFeedbackByMessageIDs :many
SELECT *
FROM chat_message_feedback
WHERE chat_message_id = ANY(sqlc.arg(message_ids)::uuid[])
  AND workspace_id = sqlc.arg(workspace_id)
  AND user_id = sqlc.arg(user_id);

-- name: DeleteChatMessageFeedback :exec
DELETE FROM chat_message_feedback
WHERE workspace_id = $1
  AND user_id = $2
  AND chat_message_id = $3;
