ALTER TABLE creative_order_variant
    DROP CONSTRAINT creative_order_variant_selection_rank_check,
    ADD CONSTRAINT creative_order_variant_selection_rank_check CHECK (selection_rank BETWEEN 1 AND 5);
