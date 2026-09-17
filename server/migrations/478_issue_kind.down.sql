DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM issue WHERE kind <> 'task') THEN
    RAISE EXCEPTION 'Cannot remove issue.kind while non-task data exists';
  END IF;
END $$;
ALTER TABLE issue DROP COLUMN kind;
