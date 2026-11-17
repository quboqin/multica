WITH checks AS (
  SELECT
    (SELECT count(*) = 3 FROM schema_migrations WHERE version IN (
      '590_collection_record_parent', '591_collection_record_parent_index', '592_collection_email_field'
    )) AS migrations_applied,
    EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema()
      AND table_name = 'record' AND column_name = 'parent_id' AND data_type = 'uuid' AND is_nullable = 'YES') AS parent_column_compatible,
    EXISTS (SELECT 1 FROM pg_index i WHERE i.indexrelid = to_regclass('record_parent_index')
      AND i.indrelid = 'record'::regclass AND i.indisvalid AND i.indisready) AS parent_index_valid,
    EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'collection_field'::regclass
      AND conname = 'collection_field_type_check' AND pg_get_constraintdef(oid) LIKE '%email%') AS email_type_supported,
    (SELECT count(*) FROM record r WHERE r.deleted_at IS NULL AND r.parent_id IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM record p WHERE p.id = r.parent_id AND p.id <> r.id
        AND p.workspace_id = r.workspace_id AND p.collection_id = r.collection_id AND p.deleted_at IS NULL)) AS invalid_parents
)
SELECT jsonb_build_object(
  'compatible', migrations_applied AND parent_column_compatible AND parent_index_valid AND email_type_supported AND invalid_parents = 0,
  'migrations_applied', migrations_applied,
  'parent_column_compatible', parent_column_compatible,
  'parent_index_valid', parent_index_valid,
  'email_type_supported', email_type_supported,
  'invalid_parents', invalid_parents,
  'collections', (SELECT count(*) FROM collection),
  'records', (SELECT count(*) FROM record),
  'fields', (SELECT count(*) FROM collection_field)
) FROM checks;
