ALTER TABLE creative_order_variant
    ADD COLUMN active_revision INT CHECK (active_revision > 0),
    ADD COLUMN staging_revision INT CHECK (staging_revision > 0),
    ADD COLUMN candidate_state TEXT NOT NULL DEFAULT 'selected'
        CHECK (candidate_state IN ('candidate', 'selected', 'reserve', 'rejected')),
    ADD COLUMN selection_rank INT CHECK (selection_rank BETWEEN 1 AND 5),
    ADD COLUMN primary_size TEXT NOT NULL DEFAULT '1080x1080'
        CHECK (primary_size IN ('1080x1080', '1200x628', '800x1000'));

CREATE UNIQUE INDEX creative_order_variant_item_selection_rank_idx
    ON creative_order_variant(order_item_id, selection_rank)
    WHERE selection_rank IS NOT NULL;

CREATE TABLE creative_order_variant_revision (
    variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    revision INT NOT NULL CHECK (revision > 0),
    brief JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(brief) = 'object'),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'partial', 'completed', 'failed', 'action_required', 'cancelled')),
    expected_sizes TEXT[] NOT NULL DEFAULT ARRAY['1080x1080', '1200x628', '800x1000']::text[],
    activated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (variant_id, revision),
    CHECK (cardinality(expected_sizes) BETWEEN 1 AND 3),
    CHECK (expected_sizes <@ ARRAY['1080x1080', '1200x628', '800x1000']::text[]),
    CHECK (
        cardinality(expected_sizes) =
            CASE WHEN '1080x1080' = ANY(expected_sizes) THEN 1 ELSE 0 END +
            CASE WHEN '1200x628' = ANY(expected_sizes) THEN 1 ELSE 0 END +
            CASE WHEN '800x1000' = ANY(expected_sizes) THEN 1 ELSE 0 END
    )
);

WITH current_revision AS (
    SELECT variant.id AS variant_id,
           variant.revision,
           variant.brief,
           variant.status,
           CASE
             WHEN jsonb_typeof(order_row.input_snapshot->'expected_sizes') = 'array'
               AND jsonb_array_length(order_row.input_snapshot->'expected_sizes') > 0
               AND order_row.input_snapshot->'expected_sizes' ?| ARRAY['1080x1080', '1200x628', '800x1000']
             THEN ARRAY(
               SELECT size_key
               FROM unnest(ARRAY['1080x1080', '1200x628', '800x1000']::text[]) AS size_key
               WHERE order_row.input_snapshot->'expected_sizes' ? size_key
             )
             WHEN jsonb_typeof(order_row.input_snapshot->'delivery_scope'->'expected_sizes') = 'array'
               AND jsonb_array_length(order_row.input_snapshot->'delivery_scope'->'expected_sizes') > 0
               AND order_row.input_snapshot->'delivery_scope'->'expected_sizes' ?| ARRAY['1080x1080', '1200x628', '800x1000']
             THEN ARRAY(
               SELECT size_key
               FROM unnest(ARRAY['1080x1080', '1200x628', '800x1000']::text[]) AS size_key
               WHERE order_row.input_snapshot->'delivery_scope'->'expected_sizes' ? size_key
             )
             WHEN order_row.trigger_evidence_kind = 'creative_direct_edit'
               AND jsonb_typeof(variant.brief->'expected_sizes') = 'array'
               AND jsonb_array_length(variant.brief->'expected_sizes') > 0
               AND variant.brief->'expected_sizes' ?| ARRAY['1080x1080', '1200x628', '800x1000']
             THEN ARRAY(
               SELECT size_key
               FROM unnest(ARRAY['1080x1080', '1200x628', '800x1000']::text[]) AS size_key
               WHERE variant.brief->'expected_sizes' ? size_key
             )
             WHEN order_row.trigger_evidence_kind = 'creative_direct_edit'
               AND NULLIF(variant.brief->>'target_size', '') IS NOT NULL
               AND variant.brief->>'target_size' = ANY(ARRAY['1080x1080', '1200x628', '800x1000'])
             THEN ARRAY[variant.brief->>'target_size']::text[]
             WHEN order_row.trigger_evidence_kind = 'creative_direct_edit'
               AND NULLIF(order_row.input_snapshot->>'target_size', '') IS NOT NULL
               AND order_row.input_snapshot->>'target_size' = ANY(ARRAY['1080x1080', '1200x628', '800x1000'])
             THEN ARRAY[order_row.input_snapshot->>'target_size']::text[]
             ELSE ARRAY['1080x1080', '1200x628', '800x1000']::text[]
           END AS expected_sizes
    FROM creative_order_variant variant
    JOIN creative_order_item item ON item.id = variant.order_item_id
    JOIN creative_order order_row ON order_row.id = item.order_id
)
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
SELECT variant_id, revision, brief, status, expected_sizes
FROM current_revision;

-- Preserve historical delivery coordinates for audit and rollback, but do
-- not infer activation from legacy delivered assets. Before this migration
-- there was no revision-scoped activation record, and a delivered package may
-- be a failed-QC risk package awaiting explicit adoption. New workflow writes
-- active_revision only through the revision activation transaction.
WITH historical_delivery AS (
    SELECT variant.id AS variant_id,
           asset.revision,
           variant.brief,
           current_revision.expected_sizes,
           max(asset.updated_at) AS delivered_at
    FROM creative_order_variant variant
    JOIN creative_order_variant_revision current_revision
      ON current_revision.variant_id = variant.id
     AND current_revision.revision = variant.revision
    JOIN creative_order_asset asset
      ON asset.variant_id = variant.id
     AND asset.revision <> variant.revision
     AND asset.stage = 'delivered'
     AND asset.status = 'completed'
     AND asset.attachment_id IS NOT NULL
     AND asset.size_key = ANY(current_revision.expected_sizes)
    GROUP BY variant.id, asset.revision, variant.brief, current_revision.expected_sizes
    HAVING count(DISTINCT asset.size_key) = cardinality(current_revision.expected_sizes)
)
INSERT INTO creative_order_variant_revision (
    variant_id, revision, brief, status, expected_sizes, created_at, updated_at
)
SELECT variant_id, revision, brief, 'completed', expected_sizes, delivered_at, delivered_at
FROM historical_delivery
ON CONFLICT (variant_id, revision) DO NOTHING;

UPDATE creative_order_variant
SET staging_revision = revision;

ALTER TABLE creative_order_variant
    ADD CONSTRAINT creative_order_variant_active_revision_fkey
        FOREIGN KEY (id, active_revision)
        REFERENCES creative_order_variant_revision(variant_id, revision)
        DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT creative_order_variant_staging_revision_fkey
        FOREIGN KEY (id, staging_revision)
        REFERENCES creative_order_variant_revision(variant_id, revision)
        DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE creative_image_operation (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    size_key TEXT NOT NULL CHECK (size_key IN ('1080x1080', '1200x628', '800x1000')),
    revision INT NOT NULL CHECK (revision > 0),
    operation_kind TEXT NOT NULL
        CHECK (operation_kind IN ('generation', 'visual_rework', 'direct_edit', 'canvas_repair')),
    idempotency_key TEXT NOT NULL CHECK (btrim(idempotency_key) <> ''),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'unknown', 'completed', 'failed', 'cancelled')),
    model TEXT NOT NULL DEFAULT '',
    runtime_id UUID REFERENCES agent_runtime(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    prompt_sha256 TEXT NOT NULL DEFAULT '',
    input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input_snapshot) = 'object'),
    provider_request_id TEXT NOT NULL DEFAULT '',
    result_receipt JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_receipt) = 'object'),
    error_type TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    output_attachment_id UUID REFERENCES attachment(id) ON DELETE SET NULL,
    output_asset_id UUID REFERENCES creative_order_asset(id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (variant_id, idempotency_key),
    UNIQUE (variant_id, revision, size_key, operation_kind),
    UNIQUE (id, variant_id, size_key, revision),
    FOREIGN KEY (variant_id, revision)
        REFERENCES creative_order_variant_revision(variant_id, revision)
        DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX creative_image_operation_variant_revision_idx
    ON creative_image_operation(variant_id, revision, size_key, updated_at DESC);

CREATE INDEX creative_image_operation_unsettled_idx
    ON creative_image_operation(status, updated_at)
    WHERE status IN ('queued', 'running', 'unknown');

CREATE UNIQUE INDEX creative_image_operation_single_inflight_idx
    ON creative_image_operation(variant_id, revision, size_key)
    WHERE status IN ('queued', 'running', 'unknown');

CREATE TABLE creative_image_operation_attempt (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL REFERENCES creative_image_operation(id) ON DELETE CASCADE,
    attempt INT NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'unknown', 'completed', 'failed', 'cancelled')),
    runtime_id UUID REFERENCES agent_runtime(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    provider_request_id TEXT NOT NULL DEFAULT '',
    provider_status TEXT NOT NULL DEFAULT '',
    http_status INT CHECK (http_status BETWEEN 100 AND 599),
    exit_code INT,
    error_type TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    result_receipt JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_receipt) = 'object'),
    output_attachment_id UUID REFERENCES attachment(id) ON DELETE SET NULL,
    duration_ms BIGINT CHECK (duration_ms >= 0),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (operation_id, attempt)
);

CREATE INDEX creative_image_operation_attempt_request_idx
    ON creative_image_operation_attempt(provider_request_id)
    WHERE provider_request_id <> '';

-- Prime composition is deterministic platform work, not an image-model
-- attempt. Persist it separately so a task that finishes after registering
-- every generated size cannot strand the variant between generation and
-- Prime. The lease makes a crashed worker recoverable without concurrent
-- duplicate composition across server replicas.
CREATE TABLE creative_prime_composition_job (
    variant_id UUID NOT NULL,
    revision INT NOT NULL CHECK (revision > 0),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    attempt INT NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    expected_sizes TEXT[] NOT NULL DEFAULT '{}'::TEXT[],
    input_fingerprint TEXT NOT NULL DEFAULT '',
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    composed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (variant_id, revision),
    FOREIGN KEY (variant_id, revision)
        REFERENCES creative_order_variant_revision(variant_id, revision)
        ON DELETE CASCADE,
    CHECK ((lease_token IS NULL) = (lease_expires_at IS NULL)),
    CHECK (status = 'running' OR lease_token IS NULL)
);

CREATE INDEX creative_prime_composition_job_recovery_idx
    ON creative_prime_composition_job(status, lease_expires_at, updated_at)
    WHERE status IN ('queued', 'running');

ALTER TABLE creative_order_asset
    ADD COLUMN operation_id UUID,
    ADD CONSTRAINT creative_order_asset_operation_fkey
        FOREIGN KEY (operation_id, variant_id, size_key, revision)
        REFERENCES creative_image_operation(id, variant_id, size_key, revision)
        DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX creative_order_asset_operation_idx
    ON creative_order_asset(operation_id)
    WHERE operation_id IS NOT NULL;
