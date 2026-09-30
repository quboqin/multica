-- Review no longer controls access. Migrated, owner-only documents use draft;
-- previously queued publication ingestion must not bypass their new audience.
DO $$
DECLARE doc RECORD;
BEGIN
 FOR doc IN SELECT i.id FROM issue i JOIN document_access d ON d.issue_id=i.id
   WHERE i.kind='doc' AND d.scope='private' AND i.status IN ('reviewing','published')
     AND NOT EXISTS (SELECT 1 FROM document_collaborator c WHERE c.issue_id=i.id)
 LOOP
  PERFORM set_config('multica.document_transition',doc.id::text,true);
  UPDATE issue SET status='draft',revision=revision+1 WHERE id=doc.id;
 END LOOP;
END $$;
UPDATE document_publication SET ingestion_state='not_requested' WHERE ingestion_state='pending';
