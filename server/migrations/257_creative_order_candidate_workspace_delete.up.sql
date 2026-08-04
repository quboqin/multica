ALTER TABLE creative_order_item
    DROP CONSTRAINT IF EXISTS creative_order_item_candidate_id_fkey;

ALTER TABLE creative_order_item
    ADD CONSTRAINT creative_order_item_candidate_id_fkey
    FOREIGN KEY (candidate_id) REFERENCES creative_material_candidate(id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;
