CREATE UNIQUE INDEX CONCURRENTLY collection_create_uidx ON collection (workspace_id, created_by, create_request_id);
