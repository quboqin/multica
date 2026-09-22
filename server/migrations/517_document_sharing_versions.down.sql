DROP TRIGGER IF EXISTS document_version_capture ON issue;
DROP FUNCTION IF EXISTS capture_document_version();
DROP FUNCTION IF EXISTS document_can_read(UUID,UUID);
DROP TABLE IF EXISTS document_version;
DROP TABLE IF EXISTS document_collaborator;
DROP TABLE IF EXISTS document_access;
