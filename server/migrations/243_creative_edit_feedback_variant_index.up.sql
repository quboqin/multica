CREATE INDEX CONCURRENTLY creative_edit_feedback_variant_idx
    ON creative_edit_feedback(variant_id, created_at DESC);
