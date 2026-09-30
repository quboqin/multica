CREATE INDEX CONCURRENTLY IF NOT EXISTS record_link_target_idx ON record_link(workspace_id,to_type,to_id);
