-- FR-028: a relation field keeps its edges here rather than in record.fields,
-- so a target can list what points at it and a later rollup needs no table
-- change. An edge outlives its target: deleting a task or a record leaves the
-- edge in place and readers report it as missing.
CREATE TABLE IF NOT EXISTS record_link (
 id UUID NOT NULL DEFAULT gen_random_uuid(), workspace_id UUID NOT NULL, collection_id UUID NOT NULL,
 from_record_id UUID NOT NULL, from_field_id UUID NOT NULL,
 to_type TEXT NOT NULL CHECK (to_type IN ('issue','record')), to_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
