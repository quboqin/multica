ALTER TABLE collection ENABLE ROW LEVEL SECURITY;
ALTER TABLE collection FORCE ROW LEVEL SECURITY;
CREATE POLICY collection_workspace_policy ON collection
  USING (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid)
  WITH CHECK (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE collection_field ENABLE ROW LEVEL SECURITY;
ALTER TABLE collection_field FORCE ROW LEVEL SECURITY;
CREATE POLICY collection_field_workspace_policy ON collection_field
  USING (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid)
  WITH CHECK (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE record ENABLE ROW LEVEL SECURITY;
ALTER TABLE record FORCE ROW LEVEL SECURITY;
CREATE POLICY record_workspace_policy ON record
  USING (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid)
  WITH CHECK (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid);
