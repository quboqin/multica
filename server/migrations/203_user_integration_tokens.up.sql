ALTER TABLE "user"
    ADD COLUMN IF NOT EXISTS integration_tokens JSONB;

UPDATE "user"
SET integration_tokens = '{}'::jsonb
WHERE integration_tokens IS NULL;

ALTER TABLE "user"
    ALTER COLUMN integration_tokens SET DEFAULT '{}'::jsonb,
    ALTER COLUMN integration_tokens SET NOT NULL;
