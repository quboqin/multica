CREATE TABLE creative_edit_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES creative_edit_job(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    variant_id UUID NOT NULL REFERENCES creative_edit_variant(id) ON DELETE CASCADE,
    decision TEXT NOT NULL
        CHECK (decision IN ('accepted', 'rejected', 'needs_revision')),
    reason_codes TEXT[] NOT NULL,
    suggestion TEXT NOT NULL DEFAULT ''
        CHECK (char_length(suggestion) <= 2000),
    process_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_by_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (cardinality(reason_codes) > 0),
    CHECK (reason_codes <@ ARRAY[
        'ready_to_publish',
        'copy_accurate',
        'benefit_clear',
        'layout_match',
        'brand_complete',
        'copy_error',
        'copy_too_long',
        'benefit_mismatch',
        'layout_mismatch',
        'missing_content',
        'brand_compliance',
        'visual_quality',
        'other'
    ]::TEXT[])
);

CREATE INDEX idx_creative_edit_feedback_issue
    ON creative_edit_feedback(workspace_id, issue_id, created_at DESC);

CREATE INDEX idx_creative_edit_feedback_variant
    ON creative_edit_feedback(variant_id, created_at DESC);
