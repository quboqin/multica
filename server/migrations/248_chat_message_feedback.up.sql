CREATE TABLE chat_message_feedback (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    chat_message_id UUID NOT NULL REFERENCES chat_message(id) ON DELETE CASCADE,
    sentiment       TEXT CHECK (sentiment IS NULL OR sentiment IN ('positive', 'negative')),
    comment         TEXT NOT NULL DEFAULT '' CHECK (char_length(comment) <= 2000),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chat_message_feedback_negative_comment_check
        CHECK (sentiment = 'positive' OR char_length(btrim(comment)) > 0),
    CONSTRAINT chat_message_feedback_content_check
        CHECK (sentiment IS NOT NULL OR char_length(btrim(comment)) > 0),
    CONSTRAINT chat_message_feedback_user_message_key
        UNIQUE (workspace_id, user_id, chat_message_id)
);

CREATE INDEX idx_chat_message_feedback_workspace_created
    ON chat_message_feedback(workspace_id, created_at DESC);
