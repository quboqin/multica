-- Keep formula definitions recoverable, but hide them from older clients.
UPDATE collection_field SET archived_at=now() WHERE type='formula' AND archived_at IS NULL;
ALTER TABLE collection_field DROP CONSTRAINT IF EXISTS collection_field_type_check;
ALTER TABLE collection_field ADD CONSTRAINT collection_field_type_check
    CHECK (type IN ('text','number','select','multi_select','date','checkbox','url','actor','multi_actor','relation')) NOT VALID;
