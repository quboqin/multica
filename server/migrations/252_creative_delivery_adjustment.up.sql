CREATE TABLE creative_delivery (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    work_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    variant SMALLINT NOT NULL CHECK (variant BETWEEN 1 AND 3),
    size TEXT NOT NULL CHECK (size IN ('1080x1080', '1200x628', '800x1000')),
    revision INT NOT NULL CHECK (revision > 0),
    base_attachment_id UUID REFERENCES attachment(id) ON DELETE SET NULL,
    final_attachment_id UUID NOT NULL REFERENCES attachment(id) ON DELETE CASCADE,
    prime_evidence_attachment_id UUID REFERENCES attachment(id) ON DELETE SET NULL,
    qc_issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(final_attachment_id),
    UNIQUE(issue_id, candidate_id, variant, size, revision)
);

CREATE INDEX creative_delivery_issue_latest_idx
    ON creative_delivery(issue_id, candidate_id, variant, size, revision DESC);

CREATE INDEX creative_delivery_work_issue_idx
    ON creative_delivery(work_issue_id, revision DESC);

CREATE TABLE creative_adjustment_request (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES creative_material_candidate(id) ON DELETE CASCADE,
    work_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    adjustment_issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    variant SMALLINT NOT NULL CHECK (variant BETWEEN 1 AND 3),
    scope TEXT NOT NULL CHECK (scope IN ('size', 'variant')),
    size TEXT CHECK (
        (scope = 'size' AND size IN ('1080x1080', '1200x628', '800x1000')) OR
        (scope = 'variant' AND size IS NULL)
    ),
    revision INT NOT NULL CHECK (revision > 0),
    instruction TEXT NOT NULL CHECK (char_length(instruction) BETWEEN 1 AND 4000),
    target_attachment_ids UUID[] NOT NULL CHECK (cardinality(target_attachment_ids) BETWEEN 1 AND 3),
    base_attachment_ids UUID[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(issue_id, candidate_id, revision)
);

CREATE INDEX creative_adjustment_request_work_issue_idx
    ON creative_adjustment_request(work_issue_id, created_at DESC);
