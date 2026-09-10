ALTER TABLE creative_order ADD CONSTRAINT creative_order_id_workspace_key UNIQUE(id,workspace_id);
ALTER TABLE creative_order_item ADD CONSTRAINT creative_order_item_id_order_key UNIQUE(id,order_id);
ALTER TABLE creative_order_variant ADD CONSTRAINT creative_order_variant_id_item_key UNIQUE(id,order_item_id);

CREATE TABLE creative_task_binding (
    task_id UUID PRIMARY KEY REFERENCES agent_task_queue(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    order_id UUID REFERENCES creative_order(id) ON DELETE CASCADE,
    order_item_id UUID REFERENCES creative_order_item(id) ON DELETE CASCADE,
    variant_id UUID REFERENCES creative_order_variant(id) ON DELETE CASCADE,
    revision INT CHECK (revision > 0),
    workflow TEXT NOT NULL CHECK (workflow <> ''),
    production_phase TEXT NOT NULL DEFAULT '',
    item_key TEXT NOT NULL DEFAULT '',
    qc_attempt INT CHECK (qc_attempt > 0),
    binding_status TEXT NOT NULL CHECK (binding_status IN ('bound','unresolved')),
    binding_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((variant_id IS NULL) = (revision IS NULL)),
    CHECK (variant_id IS NULL OR order_item_id IS NOT NULL),
    CHECK ((binding_status='bound')=(order_id IS NOT NULL)),
    FOREIGN KEY(order_id,workspace_id) REFERENCES creative_order(id,workspace_id) ON DELETE CASCADE,
    FOREIGN KEY(order_item_id,order_id) REFERENCES creative_order_item(id,order_id) ON DELETE CASCADE,
    FOREIGN KEY(variant_id,order_item_id) REFERENCES creative_order_variant(id,order_item_id) ON DELETE CASCADE
);

COMMENT ON COLUMN creative_task_binding.revision IS 'Requested revision; tasks may target a version before its revision record exists. Image operations enforce the actual revision foreign key.';

CREATE INDEX creative_task_binding_order_idx
    ON creative_task_binding(order_id, workflow, created_at DESC, task_id DESC);
CREATE INDEX creative_task_binding_item_idx
    ON creative_task_binding(order_item_id, workflow, created_at DESC, task_id DESC);
CREATE INDEX creative_task_binding_variant_idx
    ON creative_task_binding(variant_id, revision, workflow, created_at DESC, task_id DESC);

CREATE TABLE creative_task_size (
    task_id UUID NOT NULL REFERENCES creative_task_binding(task_id) ON DELETE CASCADE,
    size_key TEXT NOT NULL CHECK (size_key IN ('1080x1080', '1200x628', '800x1000')),
    is_required BOOLEAN NOT NULL,
    is_expected BOOLEAN NOT NULL,
    PRIMARY KEY (task_id, size_key)
);

-- Runtime context is an input snapshot. This is its single relational write
-- boundary, including native fanout, retries, and rolling CLI upgrades.
CREATE FUNCTION sync_creative_task_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    linked_order creative_order%ROWTYPE;
    linked_item creative_order_item%ROWTYPE;
    linked_variant creative_order_variant%ROWTYPE;
    target_revision INT;
    uuid_pattern CONSTANT TEXT := '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
    sizes JSONB;
    required_sizes JSONB;
    source_workspace UUID;
    binding_error TEXT := '';
BEGIN
    IF NEW.context->>'type' IS DISTINCT FROM 'creative_domain_task' THEN
        DELETE FROM creative_task_binding WHERE task_id=NEW.id;
        RETURN NEW;
    END IF;
    SELECT workspace_id INTO source_workspace FROM agent WHERE id=NEW.agent_id;
    IF NEW.context->>'variant_id' ~ uuid_pattern THEN
        SELECT * INTO linked_variant FROM creative_order_variant WHERE id = (NEW.context->>'variant_id')::uuid;
        IF FOUND THEN
            SELECT * INTO linked_item FROM creative_order_item WHERE id = linked_variant.order_item_id;
            target_revision := CASE WHEN NEW.context->>'revision' ~ '^[1-9][0-9]{0,8}$'
                THEN (NEW.context->>'revision')::int ELSE linked_variant.revision END;
        END IF;
    END IF;
    IF linked_item.id IS NULL AND NEW.context->>'creative_order_item_id' ~ uuid_pattern THEN
        SELECT * INTO linked_item FROM creative_order_item WHERE id = (NEW.context->>'creative_order_item_id')::uuid;
    END IF;
    IF linked_item.id IS NOT NULL THEN
        SELECT * INTO linked_order FROM creative_order WHERE id = linked_item.order_id;
    ELSIF NEW.context->>'creative_order_id' ~ uuid_pattern THEN
        SELECT * INTO linked_order FROM creative_order WHERE id = (NEW.context->>'creative_order_id')::uuid;
    END IF;
    IF linked_order.id IS NULL THEN binding_error := 'subject_not_found'; END IF;
    IF NULLIF(NEW.context->>'variant_id','') IS NOT NULL AND linked_variant.id IS NULL THEN binding_error := 'variant_not_found'; END IF;
    IF source_workspace IS DISTINCT FROM linked_order.workspace_id
       OR (NEW.context->>'creative_order_id' ~ uuid_pattern AND (NEW.context->>'creative_order_id')::uuid <> linked_order.id)
       OR (linked_item.id IS NOT NULL AND NEW.context->>'creative_order_item_id' ~ uuid_pattern
           AND (NEW.context->>'creative_order_item_id')::uuid <> linked_item.id) THEN
        binding_error := 'scope_mismatch';
    END IF;
    IF binding_error<>'' THEN
        INSERT INTO creative_task_binding(task_id,workspace_id,workflow,binding_status,binding_error,created_at)
        VALUES(NEW.id,source_workspace,COALESCE(NULLIF(NEW.context->>'workflow',''),'unspecified'),'unresolved',binding_error,NEW.created_at)
        ON CONFLICT(task_id) DO UPDATE SET workspace_id=EXCLUDED.workspace_id,order_id=NULL,order_item_id=NULL,variant_id=NULL,revision=NULL,
          workflow=EXCLUDED.workflow,binding_status='unresolved',binding_error=EXCLUDED.binding_error,updated_at=now();
        DELETE FROM creative_task_size WHERE task_id=NEW.id;
        RETURN NEW;
    END IF;
    INSERT INTO creative_task_binding(task_id, workspace_id, order_id, order_item_id, variant_id, revision,
        workflow, production_phase, item_key, qc_attempt, binding_status, created_at)
    VALUES (NEW.id, linked_order.workspace_id, linked_order.id, linked_item.id, linked_variant.id, target_revision,
        COALESCE(NULLIF(NEW.context->>'workflow',''),'unspecified'), COALESCE(NEW.context->>'production_phase',''), COALESCE(NEW.context->>'item_key',''),
        CASE WHEN NEW.context->>'qc_attempt' ~ '^[1-9][0-9]{0,8}$' THEN (NEW.context->>'qc_attempt')::int END, 'bound', NEW.created_at)
    ON CONFLICT (task_id) DO UPDATE SET
        workspace_id=EXCLUDED.workspace_id, order_id=EXCLUDED.order_id, order_item_id=EXCLUDED.order_item_id,
        variant_id=EXCLUDED.variant_id, revision=EXCLUDED.revision, workflow=EXCLUDED.workflow,
        production_phase=EXCLUDED.production_phase, item_key=EXCLUDED.item_key, qc_attempt=EXCLUDED.qc_attempt,
        binding_status='bound',binding_error='',updated_at=now();
    sizes := CASE WHEN jsonb_typeof(NEW.context->'expected_sizes')='array' THEN NEW.context->'expected_sizes' ELSE '[]'::jsonb END;
    required_sizes := CASE
      WHEN NEW.context->>'workflow'='creative_production' THEN CASE
        WHEN jsonb_typeof(NEW.context->'late_receipt_recoveries')='array' THEN jsonb_path_query_array(NEW.context,'$.late_receipt_recoveries[*].size_key')
        WHEN jsonb_typeof(NEW.context->'late_receipt_recovery')='object' THEN jsonb_build_array(NEW.context->'late_receipt_recovery'->>'size_key')
        WHEN jsonb_typeof(NEW.context->'missing_sizes')='array' THEN NEW.context->'missing_sizes'
        WHEN jsonb_typeof(NEW.context->'qc_visual_rework'->'target_sizes')='array' THEN NEW.context->'qc_visual_rework'->'target_sizes'
        ELSE sizes END
      WHEN NEW.context->>'workflow'='creative_direct_edit' THEN CASE
        WHEN jsonb_typeof(NEW.context->'edit_sizes')='array' THEN NEW.context->'edit_sizes'
        WHEN jsonb_typeof(NEW.context->'direct_edit'->'edit_sizes')='array' THEN NEW.context->'direct_edit'->'edit_sizes'
        WHEN NULLIF(NEW.context->>'target_size','') IS NOT NULL THEN jsonb_build_array(NEW.context->>'target_size')
        WHEN NULLIF(NEW.context->'direct_edit'->>'target_size','') IS NOT NULL THEN jsonb_build_array(NEW.context->'direct_edit'->>'target_size')
        ELSE sizes END
      ELSE sizes END;
    DELETE FROM creative_task_size WHERE task_id=NEW.id;
    INSERT INTO creative_task_size(task_id,size_key,is_required,is_expected)
    SELECT NEW.id, size_key, required_sizes ? size_key, sizes ? size_key FROM jsonb_array_elements_text(sizes || required_sizes) AS size_key
    WHERE size_key IN ('1080x1080','1200x628','800x1000') GROUP BY size_key;
    RETURN NEW;
END;
$$;

CREATE TRIGGER creative_task_binding_sync
AFTER INSERT OR UPDATE OF context, agent_id ON agent_task_queue
FOR EACH ROW EXECUTE FUNCTION sync_creative_task_binding();

UPDATE agent_task_queue SET context=context
WHERE context->>'type'='creative_domain_task';
