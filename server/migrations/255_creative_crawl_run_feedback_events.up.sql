ALTER TABLE creative_material_crawl_run
    ADD COLUMN IF NOT EXISTS autopilot_run_id UUID REFERENCES autopilot_run(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS rerun_of_id UUID REFERENCES creative_material_crawl_run(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS error_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';

ALTER TABLE creative_material_crawl_run
    DROP CONSTRAINT IF EXISTS creative_material_crawl_run_status_check;

ALTER TABLE creative_material_crawl_run
    ADD CONSTRAINT creative_material_crawl_run_status_check
    CHECK (status IN ('queued', 'running', 'completed', 'partial', 'failed', 'action_required', 'cancelled'));

ALTER TABLE creative_material_crawl_run
    ADD CONSTRAINT creative_material_crawl_run_finished_after_started_check
    CHECK (finished_at IS NULL OR started_at IS NULL OR finished_at >= started_at);

ALTER TABLE creative_material_issue_candidate
    ADD COLUMN IF NOT EXISTS analysis_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (analysis_status IN ('pending', 'running', 'completed', 'failed')),
    ADD COLUMN IF NOT EXISTS analysis_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS analyzed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS creative_material_crawl_run_workspace_created_idx
    ON creative_material_crawl_run(workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS creative_material_crawl_run_autopilot_idx
    ON creative_material_crawl_run(autopilot_run_id)
    WHERE autopilot_run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS creative_material_issue_candidate_run_analysis_idx
    ON creative_material_issue_candidate(source_run_id, analysis_status);

CREATE TABLE IF NOT EXISTS creative_material_crawl_run_candidate (
    run_id UUID NOT NULL REFERENCES creative_material_crawl_run(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    is_new_in_run BOOLEAN NOT NULL DEFAULT false,
    analysis_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (analysis_status IN ('pending', 'running', 'completed', 'failed')),
    analysis_error TEXT NOT NULL DEFAULT '',
    analyzed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, candidate_id)
);

CREATE INDEX IF NOT EXISTS creative_material_crawl_run_candidate_analysis_idx
    ON creative_material_crawl_run_candidate(run_id, analysis_status);

INSERT INTO creative_material_crawl_run_candidate (
    run_id, candidate_id, workspace_id, is_new_in_run, analysis_status, analysis_error, analyzed_at
)
SELECT ic.source_run_id, ic.candidate_id, ic.workspace_id,
       c.created_at >= cr.created_at, ic.analysis_status, ic.analysis_error, ic.analyzed_at
FROM creative_material_issue_candidate ic
JOIN creative_material_crawl_run cr ON cr.id = ic.source_run_id
JOIN creative_material_candidate c ON c.id = ic.candidate_id
JOIN workspace w ON w.id = ic.workspace_id
WHERE ic.source_run_id IS NOT NULL AND cr.workspace_id = ic.workspace_id
ON CONFLICT (run_id, candidate_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS creative_feedback_event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('member', 'agent')),
    actor_id UUID NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('candidate', 'recommended_copy', 'variant', 'asset', 'qc')),
    subject_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('viewed', 'shortlisted', 'decision', 'replacement', 'annotation', 'undo')),
    decision TEXT NOT NULL DEFAULT '' CHECK (decision IN ('', 'selected', 'rejected', 'accepted', 'needs_revision', 'replaced', 'abandoned', 'reported')),
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    comment TEXT NOT NULL DEFAULT '' CHECK (char_length(comment) <= 4000),
    annotation JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(annotation) = 'object'),
    context_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(context_snapshot) = 'object'),
    undo_of_id UUID REFERENCES creative_feedback_event(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (cardinality(reason_codes) <= 12)
);

CREATE INDEX IF NOT EXISTS creative_feedback_event_workspace_created_idx
    ON creative_feedback_event(workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS creative_feedback_event_subject_idx
    ON creative_feedback_event(workspace_id, subject_type, subject_id, created_at DESC);

CREATE INDEX IF NOT EXISTS creative_feedback_event_issue_idx
    ON creative_feedback_event(issue_id, created_at DESC)
    WHERE issue_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS creative_source_analysis (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    analysis_version INT NOT NULL CHECK (analysis_version > 0),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    summary TEXT NOT NULL DEFAULT '',
    result JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    trigger_evidence_kind TEXT NOT NULL DEFAULT '',
    trigger_evidence_ref_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE(candidate_id, analysis_version),
    CHECK (completed_at IS NULL OR completed_at >= created_at)
);

CREATE INDEX IF NOT EXISTS creative_source_analysis_candidate_idx
    ON creative_source_analysis(workspace_id, candidate_id, analysis_version DESC);

CREATE TABLE IF NOT EXISTS creative_order (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID UNIQUE REFERENCES issue(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'queued', 'running', 'partial', 'completed', 'failed', 'action_required', 'cancelled')),
    input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input_snapshot) = 'object'),
    trigger_evidence_kind TEXT NOT NULL DEFAULT '',
    trigger_evidence_ref_id UUID,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS creative_order_workspace_updated_idx
    ON creative_order(workspace_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS creative_order_item (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES creative_order(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED,
    source_analysis_id UUID REFERENCES creative_source_analysis(id) ON DELETE SET NULL,
    copy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(copy_snapshot) = 'object'),
    direction TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready', 'running', 'partial', 'completed', 'failed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(order_id, candidate_id)
);

CREATE TABLE IF NOT EXISTS creative_order_variant (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id UUID NOT NULL REFERENCES creative_order_item(id) ON DELETE CASCADE,
    variant_key TEXT NOT NULL CHECK (variant_key <> ''),
    brief JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(brief) = 'object'),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'partial', 'completed', 'failed', 'action_required', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(order_item_id, variant_key)
);

CREATE TABLE IF NOT EXISTS creative_order_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    asset_family_id UUID NOT NULL DEFAULT gen_random_uuid(),
    size_key TEXT NOT NULL CHECK (size_key IN ('1080x1080', '1200x628', '800x1000')),
    revision INT NOT NULL DEFAULT 1 CHECK (revision > 0),
    stage TEXT NOT NULL DEFAULT 'generated' CHECK (stage IN ('generated', 'primed', 'delivered')),
    attachment_id UUID REFERENCES attachment(id) ON DELETE SET NULL,
    derived_from_asset_id UUID REFERENCES creative_order_asset(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(evidence) = 'object'),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(variant_id, size_key, revision, stage)
);

CREATE TABLE IF NOT EXISTS creative_order_qc_report (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    lane TEXT NOT NULL CHECK (lane IN ('technical', 'visual')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'passed', 'warning', 'failed')),
    findings JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(findings) = 'object'),
    trigger_evidence_kind TEXT NOT NULL DEFAULT '',
    trigger_evidence_ref_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(variant_id, lane)
);

CREATE INDEX IF NOT EXISTS creative_order_variant_item_idx
    ON creative_order_variant(order_item_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS creative_order_asset_variant_idx
    ON creative_order_asset(variant_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS creative_order_asset_family_idx
    ON creative_order_asset(asset_family_id, revision DESC);
