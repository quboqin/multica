ALTER TABLE collection ADD CONSTRAINT collection_pkey PRIMARY KEY USING INDEX collection_id_uidx;
ALTER TABLE collection_field ADD CONSTRAINT collection_field_pkey PRIMARY KEY USING INDEX collection_field_id_uidx;
ALTER TABLE record ADD CONSTRAINT record_pkey PRIMARY KEY USING INDEX record_id_uidx;
