CREATE OR REPLACE FUNCTION guard_document_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.kind <> OLD.kind THEN
    RAISE EXCEPTION 'issue kind is immutable' USING ERRCODE = '23514';
  END IF;
  IF NEW.kind = 'doc' AND TG_OP = 'UPDATE' THEN
    IF NEW.description IS DISTINCT FROM OLD.description THEN
      IF current_setting('multica.document_write', true) IS DISTINCT FROM NEW.id::text || ':' || OLD.document_revision::text THEN
        RAISE EXCEPTION 'document_version_required: read the document and save with expected_document_revision' USING ERRCODE = '40001';
      END IF;
      NEW.document_revision := OLD.document_revision + 1;
    ELSE
      NEW.document_revision := OLD.document_revision;
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND
       current_setting('multica.document_transition', true) IS DISTINCT FROM NEW.id::text THEN
      RAISE EXCEPTION 'use the authorized document transition endpoint' USING ERRCODE = '23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS document_write_guard ON issue;
CREATE TRIGGER document_write_guard BEFORE UPDATE ON issue FOR EACH ROW EXECUTE FUNCTION guard_document_write();
