CREATE UNIQUE INDEX CONCURRENTLY credential_profile_workspace_connector_idx
    ON credential_profile(workspace_id, connector_id)
    WHERE status <> 'revoked';
