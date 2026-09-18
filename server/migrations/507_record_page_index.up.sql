CREATE INDEX CONCURRENTLY IF NOT EXISTS record_page_idx ON record(workspace_id,collection_id,created_at DESC,id) WHERE deleted_at IS NULL;
