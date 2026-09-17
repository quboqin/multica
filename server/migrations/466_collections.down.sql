DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM collection LIMIT 1)
    OR EXISTS (SELECT 1 FROM collection_field LIMIT 1)
    OR EXISTS (SELECT 1 FROM record LIMIT 1) THEN
    RAISE EXCEPTION 'refusing to drop non-empty collection tables';
  END IF;
  DROP TABLE record;
  DROP TABLE collection_field;
  DROP TABLE collection;
END $$;
