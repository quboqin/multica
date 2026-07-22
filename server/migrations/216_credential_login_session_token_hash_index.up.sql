CREATE UNIQUE INDEX CONCURRENTLY credential_login_session_token_hash_idx
    ON credential_login_session(token_hash);
