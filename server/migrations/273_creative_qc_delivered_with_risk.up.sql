ALTER TABLE creative_order_variant_qc_resolution
    DROP CONSTRAINT IF EXISTS creative_order_variant_qc_resolution_outcome_check;

ALTER TABLE creative_order_variant_qc_resolution
    ADD CONSTRAINT creative_order_variant_qc_resolution_outcome_check
    CHECK (outcome IN ('delivered', 'action_required', 'delivered_with_qc_risk'));
