CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS collection_collaborator_idx ON collection_collaborator (collection_id,user_id);
