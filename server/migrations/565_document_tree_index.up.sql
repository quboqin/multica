CREATE INDEX CONCURRENTLY IF NOT EXISTS issue_document_tree_idx ON issue (workspace_id, project_id, parent_issue_id, position, id) WHERE kind = 'doc';
