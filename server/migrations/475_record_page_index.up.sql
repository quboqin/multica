CREATE INDEX CONCURRENTLY record_page_idx ON record (workspace_id, collection_id, created_at, id) WHERE deleted_at IS NULL;
