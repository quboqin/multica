-- name: CreateAttachment :one
WITH active_issue AS (
  SELECT i.id
  FROM issue i
  WHERE i.id = sqlc.narg(issue_id)::uuid
    AND i.workspace_id = $2
    AND i.is_active = TRUE
  FOR KEY SHARE
), active_comment AS (
  SELECT c.id
  FROM comment c
  WHERE c.id = sqlc.narg(comment_id)::uuid
    AND c.workspace_id = $2
    AND c.is_active = TRUE
  FOR KEY SHARE
)
INSERT INTO attachment (
  id, workspace_id, issue_id, comment_id, chat_session_id,
  uploader_type, uploader_id, filename, url, content_type, size_bytes
)
SELECT
  $1, $2, sqlc.narg(issue_id), sqlc.narg(comment_id), sqlc.narg(chat_session_id),
  $3, $4, $5, $6, $7, $8
WHERE (sqlc.narg(issue_id)::uuid IS NULL OR EXISTS (SELECT 1 FROM active_issue))
  AND (sqlc.narg(comment_id)::uuid IS NULL OR EXISTS (SELECT 1 FROM active_comment))
RETURNING *;

-- name: ListAttachmentsByIssue :many
SELECT a.* FROM attachment a
WHERE a.issue_id = $1
  AND a.workspace_id = $2
  AND EXISTS (
    SELECT 1 FROM issue i
    WHERE i.id = a.issue_id AND i.is_active = TRUE
  )
  AND (
    a.comment_id IS NULL
    OR EXISTS (
      SELECT 1 FROM comment c
      WHERE c.id = a.comment_id AND c.is_active = TRUE
    )
  )
ORDER BY a.created_at ASC;

-- name: ListAttachmentsByComment :many
SELECT a.* FROM attachment a
JOIN comment c ON c.id = a.comment_id AND c.is_active = TRUE
JOIN issue i ON i.id = c.issue_id AND i.is_active = TRUE
WHERE a.comment_id = $1 AND a.workspace_id = $2
ORDER BY a.created_at ASC;

-- name: GetAttachment :one
SELECT a.* FROM attachment a
WHERE a.id = $1
  AND a.workspace_id = $2
  AND (
    a.issue_id IS NULL
    OR EXISTS (
      SELECT 1 FROM issue i
      WHERE i.id = a.issue_id AND i.is_active = TRUE
    )
  )
  AND (
    a.comment_id IS NULL
    OR EXISTS (
      SELECT 1 FROM comment c
      WHERE c.id = a.comment_id AND c.is_active = TRUE
    )
  );

-- name: GetAttachmentByIDOnly :one
-- Used by the download endpoint, which derives workspace context from the
-- attachment row itself rather than from request headers/query params. The
-- caller still has to verify the requester is a member of the returned
-- workspace_id before serving the bytes. Attachments belonging to logically
-- deleted comments are hidden, while the underlying row and object remain
-- available for audit or a future restore flow.
SELECT a.* FROM attachment a
WHERE a.id = $1
  AND (
    a.issue_id IS NULL
    OR EXISTS (
      SELECT 1 FROM issue i
      WHERE i.id = a.issue_id AND i.is_active = TRUE
    )
  )
  AND (
    a.comment_id IS NULL
    OR EXISTS (
      SELECT 1 FROM comment c
      WHERE c.id = a.comment_id AND c.is_active = TRUE
    )
  );

-- name: ListAttachmentsByCommentIDs :many
SELECT a.* FROM attachment a
JOIN comment c ON c.id = a.comment_id AND c.is_active = TRUE
JOIN issue i ON i.id = c.issue_id AND i.is_active = TRUE
WHERE a.comment_id = ANY($1::uuid[]) AND a.workspace_id = $2
ORDER BY a.created_at ASC;

-- name: ListAttachmentURLsByIssueOrComments :many
SELECT a.url FROM attachment a
WHERE a.issue_id = $1
   OR a.comment_id IN (SELECT c.id FROM comment c WHERE c.issue_id = $1);

-- name: ListAttachmentURLsByCommentID :many
SELECT url FROM attachment
WHERE comment_id = $1;

-- name: LinkAttachmentsToComment :exec
UPDATE attachment
SET comment_id = $1
WHERE issue_id = $2
  AND comment_id IS NULL
  AND id = ANY($3::uuid[]);

-- name: ReplaceCommentAttachments :exec
UPDATE attachment
SET comment_id = CASE
  WHEN id = ANY(sqlc.arg(attachment_ids)::uuid[]) THEN $1
  ELSE NULL
END
WHERE issue_id = $2
  AND (
    comment_id = $1
    OR (comment_id IS NULL AND id = ANY(sqlc.arg(attachment_ids)::uuid[]))
  );

-- name: LinkAttachmentsToChatMessage :exec
UPDATE attachment
SET chat_message_id = $1
WHERE chat_session_id = $2
  AND chat_message_id IS NULL
  AND id = ANY($3::uuid[]);

-- name: ListAttachmentsByChatMessage :many
SELECT * FROM attachment
WHERE chat_message_id = $1 AND workspace_id = $2
ORDER BY created_at ASC;

-- name: ListAttachmentsByChatMessageIDs :many
SELECT * FROM attachment
WHERE chat_message_id = ANY($1::uuid[]) AND workspace_id = $2
ORDER BY created_at ASC;

-- name: LinkAttachmentsToIssue :exec
UPDATE attachment
SET issue_id = $1
WHERE workspace_id = $2
  AND issue_id IS NULL
  AND id = ANY($3::uuid[]);

-- name: DeleteAttachment :exec
DELETE FROM attachment WHERE id = $1 AND workspace_id = $2;
