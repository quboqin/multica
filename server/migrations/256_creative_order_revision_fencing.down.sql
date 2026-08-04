ALTER TABLE creative_order_qc_report
    DROP CONSTRAINT IF EXISTS creative_order_qc_report_variant_id_lane_revision_key;

ALTER TABLE creative_order_qc_report
    DROP COLUMN IF EXISTS revision;

ALTER TABLE creative_order_qc_report
    ADD CONSTRAINT creative_order_qc_report_variant_id_lane_key
    UNIQUE (variant_id, lane);

ALTER TABLE creative_order_variant
    DROP COLUMN IF EXISTS revision;
