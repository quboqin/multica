CREATE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_feedback_variant_idx
    ON creative_edit_feedback(variant_id, created_at DESC);
