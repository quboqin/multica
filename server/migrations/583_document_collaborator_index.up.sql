CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS document_collaborator_issue_user_idx ON document_collaborator (issue_id,user_id);
