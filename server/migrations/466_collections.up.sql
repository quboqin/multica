CREATE TABLE collection (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL,
  name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_by UUID NOT NULL,
  create_request_id UUID NOT NULL,
  create_fingerprint TEXT NOT NULL CHECK (create_fingerprint ~ '^[0-9a-f]{64}$'),
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE collection_field (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL,
  collection_id UUID NOT NULL,
  name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
  type TEXT NOT NULL CHECK (type IN ('text', 'number', 'checkbox')),
  position INTEGER NOT NULL CHECK (position >= 0),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE record (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL,
  collection_id UUID NOT NULL,
  title TEXT NOT NULL DEFAULT '' CHECK (char_length(title) <= 2048),
  fields JSONB NOT NULL DEFAULT '{}'::jsonb
    CONSTRAINT record_fields_object_check CHECK (jsonb_typeof(fields) = 'object')
    CONSTRAINT record_fields_size_check CHECK (octet_length(fields::text) <= 65536),
  position DOUBLE PRECISION NOT NULL DEFAULT 0
    CHECK (position > '-Infinity'::float8 AND position < 'Infinity'::float8),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_by UUID NOT NULL,
  updated_by UUID NOT NULL,
  create_request_id UUID NOT NULL,
  create_fingerprint TEXT NOT NULL CHECK (create_fingerprint ~ '^[0-9a-f]{64}$'),
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
