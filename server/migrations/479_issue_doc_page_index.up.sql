CREATE INDEX CONCURRENTLY issue_doc_page_idx ON issue (workspace_id, created_at, id) WHERE kind = 'doc';
