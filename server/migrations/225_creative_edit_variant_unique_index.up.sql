CREATE UNIQUE INDEX CONCURRENTLY creative_edit_variant_unique_idx
    ON creative_edit_variant(job_id, candidate_id, variant_index);
