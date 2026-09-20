-- 511's rollback drops the edges, so a relation field left in the catalog would
-- be an empty column of a type older servers cannot read. Archive them first.
UPDATE collection_field SET archived_at=now() WHERE type='relation' AND archived_at IS NULL;

ALTER TABLE collection_field DROP CONSTRAINT IF EXISTS collection_field_type_check;

ALTER TABLE collection_field ADD CONSTRAINT collection_field_type_check
    CHECK (type IN ('text','number','select','multi_select','date','checkbox','url','actor','multi_actor')) NOT VALID;
