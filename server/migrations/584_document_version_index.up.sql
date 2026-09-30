CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS document_version_issue_version_idx ON document_version (issue_id,version DESC);
