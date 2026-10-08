-- Sharing and cancelled ingestion are user-visible decisions; do not invent
-- previous approval state when rolling back this data normalization.
SELECT 1;
