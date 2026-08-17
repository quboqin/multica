CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS creative_edit_variant_unique_idx
    ON creative_edit_variant(job_id, candidate_id, variant_index);
