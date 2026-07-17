ALTER TABLE credential_profile
    DROP CONSTRAINT credential_profile_workspace_id_user_id_fkey,
    DROP CONSTRAINT credential_profile_user_id_fkey;

ALTER TABLE credential_profile
    RENAME COLUMN user_id TO authorized_by_id;

ALTER TABLE credential_profile
    ALTER COLUMN authorized_by_id DROP NOT NULL,
    ADD CONSTRAINT credential_profile_authorized_by_id_fkey
        FOREIGN KEY (authorized_by_id) REFERENCES "user"(id) ON DELETE SET NULL;

DROP INDEX idx_credential_profile_user;

WITH ranked_profiles AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY workspace_id, connector_id
               ORDER BY updated_at DESC, created_at DESC, id DESC
           ) AS rank
    FROM credential_profile
    WHERE status <> 'revoked'
)
UPDATE credential_profile AS profile
SET status = 'revoked', updated_at = now()
FROM ranked_profiles
WHERE profile.id = ranked_profiles.id
  AND ranked_profiles.rank > 1;

CREATE UNIQUE INDEX idx_credential_profile_workspace_connector
    ON credential_profile(workspace_id, connector_id)
    WHERE status <> 'revoked';
