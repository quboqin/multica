ALTER TABLE creative_order_qc_report
    ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 1 CHECK (attempt > 0);

ALTER TABLE creative_order_variant_qc_resolution
    ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 1 CHECK (attempt > 0);

ALTER TABLE creative_order_qc_report
    DROP CONSTRAINT IF EXISTS creative_order_qc_report_variant_id_lane_revision_key;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'creative_order_qc_report'::regclass
          AND conname = 'creative_order_qc_report_variant_id_lane_revision_attempt_key'
    ) THEN
        ALTER TABLE creative_order_qc_report
            ADD CONSTRAINT creative_order_qc_report_variant_id_lane_revision_attempt_key
            UNIQUE (variant_id, lane, revision, attempt);
    END IF;
END $$;

ALTER TABLE creative_order_variant_qc_resolution
    DROP CONSTRAINT IF EXISTS creative_order_variant_qc_resolution_pkey;

ALTER TABLE creative_order_variant_qc_resolution
    ADD PRIMARY KEY (variant_id, revision, attempt);

CREATE INDEX IF NOT EXISTS creative_order_qc_report_variant_revision_attempt_idx
    ON creative_order_qc_report(variant_id, revision, attempt DESC);

CREATE INDEX IF NOT EXISTS creative_order_variant_qc_resolution_variant_revision_attempt_idx
    ON creative_order_variant_qc_resolution(variant_id, revision, attempt DESC);
