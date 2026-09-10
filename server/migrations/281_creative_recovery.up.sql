CREATE TABLE creative_recovery (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES creative_order(id) ON DELETE CASCADE,
    order_item_id UUID NOT NULL REFERENCES creative_order_item(id) ON DELETE CASCADE,
    variant_id UUID REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    revision INT CHECK (revision > 0),
    stage TEXT NOT NULL CHECK (stage IN ('planning','candidate_selection','production','prime','qc')),
    size_key TEXT NOT NULL DEFAULT '' CHECK (size_key IN ('','1080x1080','1200x628','800x1000')),
    recovery_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','waiting','resolved','manual_required','cancelled')),
    reason_code TEXT NOT NULL,
    source_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    result_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    resolved_asset_id UUID REFERENCES creative_order_asset(id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ,
    attempt INT NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts INT NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (attempt <= max_attempts),
    CHECK ((variant_id IS NULL) = (revision IS NULL)),
    CHECK (variant_id IS NOT NULL OR size_key=''),
    CHECK ((stage IN ('planning','candidate_selection'))=(variant_id IS NULL)),
    CHECK ((lease_token IS NULL)=(lease_expires_at IS NULL)),
    CHECK ((status='running') = (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)),
    FOREIGN KEY (variant_id,revision) REFERENCES creative_order_variant_revision(variant_id,revision),
    FOREIGN KEY(order_id,workspace_id) REFERENCES creative_order(id,workspace_id) ON DELETE CASCADE,
    FOREIGN KEY(order_item_id,order_id) REFERENCES creative_order_item(id,order_id) ON DELETE CASCADE,
    FOREIGN KEY(variant_id,order_item_id) REFERENCES creative_order_variant(id,order_item_id) ON DELETE CASCADE,
    UNIQUE NULLS NOT DISTINCT(order_item_id,variant_id,revision,stage,size_key)
);
CREATE INDEX creative_recovery_due_idx ON creative_recovery(next_retry_at,id)
    WHERE status IN ('pending','waiting','running');
CREATE INDEX creative_recovery_order_idx ON creative_recovery(order_id,order_item_id,created_at);

CREATE TABLE creative_recovery_attempt (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recovery_id UUID NOT NULL REFERENCES creative_recovery(id) ON DELETE CASCADE,
    attempt INT NOT NULL CHECK (attempt > 0),
    lease_token UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running','queued','resolved','failed','cancelled')),
    source_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    result_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    result_asset_id UUID REFERENCES creative_order_asset(id) ON DELETE SET NULL,
    reason_code TEXT NOT NULL,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE(recovery_id,attempt)
);

CREATE TABLE creative_order_recovery_scan (
    order_id UUID PRIMARY KEY REFERENCES creative_order(id) ON DELETE CASCADE,
    checked_at TIMESTAMPTZ,
    next_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX creative_order_recovery_scan_due_idx ON creative_order_recovery_scan(next_check_at,order_id);

CREATE FUNCTION wake_creative_order_recovery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO creative_order_recovery_scan(order_id,next_check_at) VALUES(NEW.id,now())
    ON CONFLICT(order_id) DO UPDATE SET next_check_at=LEAST(creative_order_recovery_scan.next_check_at,now());
    RETURN NEW;
END;
$$;
CREATE TRIGGER creative_order_recovery_wake AFTER INSERT OR UPDATE ON creative_order
FOR EACH ROW EXECUTE FUNCTION wake_creative_order_recovery();

INSERT INTO creative_order_recovery_scan(order_id,next_check_at)
SELECT o.id,CASE WHEN EXISTS(SELECT 1 FROM creative_order_variant v JOIN creative_order_item i ON i.id=v.order_item_id WHERE i.order_id=o.id)
AND NOT EXISTS(SELECT 1 FROM creative_order_variant v JOIN creative_order_item i ON i.id=v.order_item_id
 WHERE i.order_id=o.id AND i.status<>'cancelled' AND v.status<>'cancelled' AND v.candidate_state IN ('candidate','selected') AND v.revision IS DISTINCT FROM v.active_revision)
THEN now()+interval '1 day' ELSE now() END FROM creative_order o;
