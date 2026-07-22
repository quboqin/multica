CREATE UNIQUE INDEX CONCURRENTLY idx_lark_login_identity_region_union
    ON lark_login_identity(region, union_id);
