DELETE FROM pinned_item WHERE item_type='collection';
ALTER TABLE pinned_item DROP CONSTRAINT IF EXISTS pinned_item_item_type_check;
ALTER TABLE pinned_item ADD CONSTRAINT pinned_item_item_type_check CHECK (item_type IN ('issue','project','view'));
DROP TRIGGER IF EXISTS collection_access_create ON collection;
DROP FUNCTION IF EXISTS initialize_collection_access();
DROP FUNCTION IF EXISTS collection_can_read(UUID,UUID);
DROP TABLE IF EXISTS collection_collaborator;
DROP TABLE IF EXISTS collection_access;
