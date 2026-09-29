-- name: UpsertCommentAgentGrant :exec
INSERT INTO comment_agent_grant(comment_id,agent_id,workspace_id,issue_id,permission,granted_by)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT (comment_id,agent_id) DO UPDATE SET permission=EXCLUDED.permission,granted_by=EXCLUDED.granted_by,granted_at=now();

-- name: GetCommentAgentGrant :one
SELECT * FROM comment_agent_grant WHERE comment_id=$1 AND agent_id=$2 AND workspace_id=$3;
