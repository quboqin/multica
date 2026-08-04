ALTER TABLE creative_order_variant
    ADD COLUMN IF NOT EXISTS revision INT NOT NULL DEFAULT 1 CHECK (revision > 0);

ALTER TABLE creative_order_qc_report
    ADD COLUMN IF NOT EXISTS revision INT NOT NULL DEFAULT 1 CHECK (revision > 0);

ALTER TABLE creative_order_qc_report
    DROP CONSTRAINT IF EXISTS creative_order_qc_report_variant_id_lane_key;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'creative_order_qc_report'::regclass
          AND conname = 'creative_order_qc_report_variant_id_lane_revision_key'
    ) THEN
        ALTER TABLE creative_order_qc_report
            ADD CONSTRAINT creative_order_qc_report_variant_id_lane_revision_key
            UNIQUE (variant_id, lane, revision);
    END IF;
END $$;
