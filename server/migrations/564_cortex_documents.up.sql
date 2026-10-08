ALTER TABLE issue ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'task';
ALTER TABLE issue ADD COLUMN IF NOT EXISTS document_revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE issue ADD CONSTRAINT issue_kind_check CHECK (kind IN ('task', 'doc', 'knowledge', 'workflow_run'));
ALTER TABLE issue ADD CONSTRAINT issue_document_revision_check CHECK (document_revision > 0);

-- This gate also protects documents from older servers in a rolling deployment.
-- Only a transaction which has compared the body version may replace a body.
CREATE FUNCTION guard_document_write() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER document_write_guard BEFORE UPDATE ON issue FOR EACH ROW EXECUTE FUNCTION guard_document_write();

CREATE TABLE document_publication (
  issue_id UUID NOT NULL,
  workspace_id UUID NOT NULL,
  document_revision BIGINT NOT NULL,
  actor_id UUID NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('review', 'publish', 'draft')),
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ingestion_state TEXT NOT NULL DEFAULT 'not_requested' CHECK (ingestion_state IN ('not_requested', 'pending', 'complete'))
);
