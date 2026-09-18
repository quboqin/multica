CREATE INDEX CONCURRENTLY collection_field_page_idx ON collection_field (workspace_id, collection_id, position, id) WHERE archived_at IS NULL;
