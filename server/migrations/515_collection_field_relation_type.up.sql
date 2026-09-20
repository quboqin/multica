-- Collection fields gain the relation type. It is collection-only: issue
-- properties keep their own whitelist. NOT VALID skips the table scan; the new
-- list is a superset of the old one, so every existing row already passes.
ALTER TABLE collection_field DROP CONSTRAINT IF EXISTS collection_field_type_check;

ALTER TABLE collection_field ADD CONSTRAINT collection_field_type_check
    CHECK (type IN ('text','number','select','multi_select','date','checkbox','url','actor','multi_actor','relation')) NOT VALID;
