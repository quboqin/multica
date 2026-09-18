CREATE TABLE collection (
 id UUID NOT NULL DEFAULT gen_random_uuid(), workspace_id UUID NOT NULL, project_id UUID,
 name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80), description TEXT NOT NULL DEFAULT '',
 icon TEXT NOT NULL DEFAULT '', created_by UUID NOT NULL, revision BIGINT NOT NULL DEFAULT 1,
 archived_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE collection_field (
 id UUID NOT NULL DEFAULT gen_random_uuid(), workspace_id UUID NOT NULL, collection_id UUID NOT NULL,
 name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 32), type TEXT NOT NULL CHECK(type IN ('text','number','select','multi_select','date','checkbox','url','actor','multi_actor')),
 config JSONB NOT NULL DEFAULT '{}', position DOUBLE PRECISION NOT NULL DEFAULT 0, archived_at TIMESTAMPTZ
);
CREATE TABLE record (
 id UUID NOT NULL DEFAULT gen_random_uuid(), workspace_id UUID NOT NULL, collection_id UUID NOT NULL,
 title TEXT NOT NULL DEFAULT '' CHECK(length(title)<=2048), fields JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(fields)='object' AND octet_length(fields::text)<=65536),
 revision BIGINT NOT NULL DEFAULT 1, position DOUBLE PRECISION NOT NULL DEFAULT 0,
 deleted_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE issue_view ADD COLUMN IF NOT EXISTS collection_id UUID;
