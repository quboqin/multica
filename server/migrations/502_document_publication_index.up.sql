CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS document_publication_version_idx ON document_publication (issue_id, document_revision, action);
