-- Lark/Feishu web-login identity. Bot install/user bindings are stored in
-- channel_*; this table tracks cross-installation identity by cloud + union_id.

-- name: GetLarkLoginIdentityByUnionID :one
SELECT * FROM lark_login_identity
WHERE region = $1 AND union_id = $2;

-- name: UpsertLarkLoginIdentity :one
INSERT INTO lark_login_identity (
    region, union_id, open_id, multica_user_id, name, email, avatar_url
) VALUES (
    $1, $2, sqlc.narg('open_id'), $3, sqlc.narg('name'), sqlc.narg('email'), sqlc.narg('avatar_url')
)
ON CONFLICT (region, union_id) DO UPDATE SET
    open_id = COALESCE(EXCLUDED.open_id, lark_login_identity.open_id),
    multica_user_id = EXCLUDED.multica_user_id,
    name = COALESCE(EXCLUDED.name, lark_login_identity.name),
    email = COALESCE(EXCLUDED.email, lark_login_identity.email),
    avatar_url = COALESCE(EXCLUDED.avatar_url, lark_login_identity.avatar_url),
    last_login_at = now(),
    updated_at = now()
RETURNING *;
