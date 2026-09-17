CREATE INDEX CONCURRENTLY collection_page_idx ON collection (workspace_id, created_at, id) WHERE archived_at IS NULL;
