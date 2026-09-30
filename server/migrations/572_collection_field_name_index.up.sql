CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS collection_field_name_idx ON collection_field(collection_id,lower(name)) WHERE archived_at IS NULL;
