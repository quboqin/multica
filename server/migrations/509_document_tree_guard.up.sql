CREATE OR REPLACE FUNCTION guard_document_write() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_kind TEXT; parent_workspace UUID; parent_project UUID;
BEGIN
 IF NEW.parent_issue_id IS NOT NULL AND (TG_OP='INSERT' OR NEW.parent_issue_id IS DISTINCT FROM OLD.parent_issue_id) THEN
  IF NEW.kind='doc' THEN PERFORM pg_advisory_xact_lock(hashtextextended(NEW.workspace_id::text,531)); END IF;
  SELECT kind,workspace_id,project_id INTO parent_kind,parent_workspace,parent_project FROM issue WHERE id=NEW.parent_issue_id;
  IF parent_workspace IS DISTINCT FROM NEW.workspace_id OR (parent_kind='doc') IS DISTINCT FROM (NEW.kind='doc') OR (NEW.kind='doc' AND parent_project IS DISTINCT FROM NEW.project_id) THEN
   RAISE EXCEPTION 'parent must belong to the same object kind, workspace, and document project' USING ERRCODE='23514';
  END IF;
  IF NEW.kind='doc' AND EXISTS(WITH RECURSIVE ancestors AS (SELECT id,parent_issue_id FROM issue WHERE id=NEW.parent_issue_id UNION SELECT i.id,i.parent_issue_id FROM issue i JOIN ancestors a ON i.id=a.parent_issue_id) SELECT 1 FROM ancestors WHERE id=NEW.id) THEN
   RAISE EXCEPTION 'document tree cannot contain cycles' USING ERRCODE='23514';
  END IF;
 END IF;
  IF TG_OP = 'UPDATE' AND NEW.kind <> OLD.kind THEN
    RAISE EXCEPTION 'issue kind is immutable' USING ERRCODE = '23514';
  END IF;
  IF NEW.kind = 'doc' AND TG_OP = 'UPDATE' THEN
    IF NEW.description IS DISTINCT FROM OLD.description THEN
      IF current_setting('multica.document_media', true) = NEW.id::text THEN
        NEW.document_revision := OLD.document_revision;
      ELSE
      IF current_setting('multica.document_write', true) IS DISTINCT FROM NEW.id::text || ':' || OLD.document_revision::text THEN
        RAISE EXCEPTION 'document_version_required: read the document and save with expected_document_revision' USING ERRCODE = '40001';
      END IF;
      NEW.document_revision := OLD.document_revision + 1;
      END IF;
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
CREATE TRIGGER document_write_guard BEFORE INSERT OR UPDATE ON issue FOR EACH ROW EXECUTE FUNCTION guard_document_write();
