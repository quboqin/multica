CREATE UNIQUE INDEX CONCURRENTLY collection_field_name_uidx ON collection_field (workspace_id, collection_id, lower(name)) WHERE archived_at IS NULL;
