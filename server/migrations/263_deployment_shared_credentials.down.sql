DROP TABLE IF EXISTS credential_usage_audit;
DROP TABLE IF EXISTS credential_profile_manager;

DROP INDEX IF EXISTS credential_profile_deployment_connector_idx;
DROP INDEX IF EXISTS credential_profile_workspace_connector_idx;

CREATE UNIQUE INDEX credential_profile_workspace_connector_idx
    ON credential_profile(workspace_id, connector_id)
    WHERE status <> 'revoked';

ALTER TABLE credential_profile DROP COLUMN IF EXISTS scope;
