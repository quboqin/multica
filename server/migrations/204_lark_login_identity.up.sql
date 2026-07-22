CREATE TABLE lark_login_identity (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    region           TEXT NOT NULL
        CHECK (region IN ('feishu', 'lark')),
    union_id         TEXT NOT NULL,
    open_id          TEXT,
    multica_user_id  UUID NOT NULL,
    name             TEXT,
    email            TEXT,
    avatar_url       TEXT,
    last_login_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
