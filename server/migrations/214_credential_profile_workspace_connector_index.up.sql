CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS credential_profile_workspace_connector_idx
    ON credential_profile(workspace_id, connector_id)
    WHERE status <> 'revoked';
