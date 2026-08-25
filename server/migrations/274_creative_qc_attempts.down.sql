WITH ranked_reports AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY variant_id, lane, revision
               ORDER BY attempt DESC, updated_at DESC, id DESC
           ) AS row_number
    FROM creative_order_qc_report
)
DELETE FROM creative_order_qc_report report
USING ranked_reports ranked
WHERE report.id = ranked.id
  AND ranked.row_number > 1;

WITH ranked_resolutions AS (
    SELECT variant_id, revision, attempt,
           row_number() OVER (
               PARTITION BY variant_id, revision
               ORDER BY attempt DESC, created_at DESC
           ) AS row_number
    FROM creative_order_variant_qc_resolution
)
DELETE FROM creative_order_variant_qc_resolution resolution
USING ranked_resolutions ranked
WHERE resolution.variant_id = ranked.variant_id
  AND resolution.revision = ranked.revision
  AND resolution.attempt = ranked.attempt
  AND ranked.row_number > 1;

DROP INDEX IF EXISTS creative_order_variant_qc_resolution_variant_revision_attempt_idx;
DROP INDEX IF EXISTS creative_order_qc_report_variant_revision_attempt_idx;

ALTER TABLE creative_order_qc_report
    DROP CONSTRAINT IF EXISTS creative_order_qc_report_variant_id_lane_revision_attempt_key;

ALTER TABLE creative_order_qc_report
    ADD CONSTRAINT creative_order_qc_report_variant_id_lane_revision_key
    UNIQUE (variant_id, lane, revision);

ALTER TABLE creative_order_variant_qc_resolution
    DROP CONSTRAINT IF EXISTS creative_order_variant_qc_resolution_pkey;

ALTER TABLE creative_order_variant_qc_resolution
    ADD PRIMARY KEY (variant_id, revision);

ALTER TABLE creative_order_variant_qc_resolution
    DROP COLUMN IF EXISTS attempt;

ALTER TABLE creative_order_qc_report
    DROP COLUMN IF EXISTS attempt;
