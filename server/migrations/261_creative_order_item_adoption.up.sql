ALTER TABLE creative_order_variant
    ADD CONSTRAINT creative_order_variant_item_id_id_key
    UNIQUE (order_item_id, id);

ALTER TABLE creative_order_item
    ADD COLUMN adopted_variant_id UUID,
    ADD COLUMN adopted_at TIMESTAMPTZ,
    ADD COLUMN adopted_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    ADD CONSTRAINT creative_order_item_adoption_timestamp_check
        CHECK ((adopted_variant_id IS NULL) = (adopted_at IS NULL)),
    ADD CONSTRAINT creative_order_item_adopted_variant_fkey
        FOREIGN KEY (id, adopted_variant_id)
        REFERENCES creative_order_variant(order_item_id, id)
        DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX creative_order_item_adopted_variant_idx
    ON creative_order_item(adopted_variant_id)
    WHERE adopted_variant_id IS NOT NULL;
