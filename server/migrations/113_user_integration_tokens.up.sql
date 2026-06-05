ALTER TABLE "user"
    ADD COLUMN integration_tokens JSONB NOT NULL DEFAULT '{}';
