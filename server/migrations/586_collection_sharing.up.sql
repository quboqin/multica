CREATE TABLE collection_access (
    collection_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    scope TEXT NOT NULL DEFAULT 'private' CHECK (scope IN ('private','project','workspace')),
    project_id UUID,
    scope_role TEXT NOT NULL DEFAULT 'view' CHECK (scope_role IN ('view','edit')),
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'project') = (project_id IS NOT NULL))
);
CREATE TABLE collection_collaborator (
    collection_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('view','edit'))
);
-- Existing tables become private to their creator, as documents do.
INSERT INTO collection_access(collection_id,workspace_id,owner_id)
SELECT id,workspace_id,created_by FROM collection;
CREATE FUNCTION collection_can_read(table_id UUID, reader_id UUID) RETURNS BOOLEAN
LANGUAGE SQL STABLE AS $$
 SELECT EXISTS (
  SELECT 1 FROM collection_access a JOIN member m ON m.workspace_id=a.workspace_id AND m.user_id=reader_id
  WHERE a.collection_id=table_id AND (a.owner_id=reader_id OR a.scope='workspace'
   OR (a.scope='project' AND EXISTS (SELECT 1 FROM project p WHERE p.id=a.project_id AND p.workspace_id=a.workspace_id))
   OR EXISTS (SELECT 1 FROM collection_collaborator c WHERE c.collection_id=a.collection_id AND c.workspace_id=a.workspace_id AND c.user_id=reader_id))
 );
$$;
CREATE FUNCTION initialize_collection_access() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO collection_access(collection_id,workspace_id,owner_id) VALUES(NEW.id,NEW.workspace_id,NEW.created_by);
 RETURN NEW;
END $$;
CREATE TRIGGER collection_access_create AFTER INSERT ON collection FOR EACH ROW EXECUTE FUNCTION initialize_collection_access();
ALTER TABLE pinned_item DROP CONSTRAINT IF EXISTS pinned_item_item_type_check;
ALTER TABLE pinned_item ADD CONSTRAINT pinned_item_item_type_check CHECK (item_type IN ('issue','project','view','collection'));
