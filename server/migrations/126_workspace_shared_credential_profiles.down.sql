DROP INDEX IF EXISTS idx_credential_profile_workspace_connector;

DELETE FROM credential_profile
WHERE authorized_by_id IS NULL;

ALTER TABLE credential_profile
    DROP CONSTRAINT credential_profile_authorized_by_id_fkey,
    RENAME COLUMN authorized_by_id TO user_id;

ALTER TABLE credential_profile
    ALTER COLUMN user_id SET NOT NULL,
    ADD CONSTRAINT credential_profile_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES "user"(id) ON DELETE CASCADE,
    ADD CONSTRAINT credential_profile_workspace_id_user_id_fkey
        FOREIGN KEY (workspace_id, user_id)
        REFERENCES member(workspace_id, user_id)
        ON DELETE CASCADE;

CREATE INDEX idx_credential_profile_user
    ON credential_profile(workspace_id, user_id, connector_id)
    WHERE status <> 'revoked';
