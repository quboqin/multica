DROP TRIGGER IF EXISTS document_write_guard ON issue;
DROP FUNCTION IF EXISTS guard_document_write();
DROP TABLE IF EXISTS document_publication;
ALTER TABLE issue DROP COLUMN IF EXISTS document_revision;
ALTER TABLE issue DROP COLUMN IF EXISTS kind;
