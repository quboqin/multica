CREATE TABLE creative_order_variant_qc_resolution (
    variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    revision INT NOT NULL CHECK (revision > 0),
    outcome TEXT NOT NULL CHECK (outcome IN ('delivered', 'action_required')),
    finalized_by_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    failure_summary JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(failure_summary) = 'object'),
    inbox_item_id UUID REFERENCES inbox_item(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (variant_id, revision)
);

CREATE INDEX creative_order_variant_qc_resolution_issue_idx
    ON creative_order_variant_qc_resolution(issue_id, created_at DESC)
    WHERE issue_id IS NOT NULL;
