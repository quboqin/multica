CREATE INDEX CONCURRENTLY IF NOT EXISTS record_parent_index ON record(workspace_id, collection_id, parent_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
