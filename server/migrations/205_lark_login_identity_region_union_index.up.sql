CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_lark_login_identity_region_union
    ON lark_login_identity(region, union_id);
