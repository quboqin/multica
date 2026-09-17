CREATE UNIQUE INDEX CONCURRENTLY record_create_uidx ON record (workspace_id, collection_id, created_by, create_request_id);
