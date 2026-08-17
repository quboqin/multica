CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_lark_login_identity_user
    ON lark_login_identity(multica_user_id);
