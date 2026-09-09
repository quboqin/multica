CREATE TABLE creative_generation_measurement_epoch (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(singleton),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO creative_generation_measurement_epoch DEFAULT VALUES;

CREATE TABLE creative_initial_generated_package (
    order_item_id UUID PRIMARY KEY REFERENCES creative_order_item(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES creative_order(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    submitted_at TIMESTAMPTZ NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    target_variant_count INT NOT NULL CHECK(target_variant_count BETWEEN 1 AND 10),
    assets JSONB NOT NULL,
    CHECK(generated_at>=submitted_at)
);
CREATE INDEX creative_initial_generated_package_cohort_idx ON creative_initial_generated_package(workspace_id,submitted_at);

CREATE FUNCTION record_creative_initial_generated_package(item_id UUID) RETURNS void LANGUAGE plpgsql AS $$
DECLARE owner_id UUID; workspace UUID; submitted TIMESTAMPTZ; target_count INT;
        selected_count INT; asset_count INT; frozen_assets JSONB;
BEGIN
    IF EXISTS(SELECT 1 FROM creative_initial_generated_package WHERE order_item_id=item_id) THEN RETURN; END IF;
    SELECT o.id,o.workspace_id,o.created_at,COALESCE((o.input_snapshot->>'target_variant_count')::int,3)
      INTO owner_id,workspace,submitted,target_count
    FROM creative_order_item i JOIN creative_order o ON o.id=i.order_id
    CROSS JOIN creative_generation_measurement_epoch epoch
    WHERE i.id=item_id AND o.created_at>=epoch.started_at AND o.created_at<=now()
      AND o.status<>'cancelled' AND i.status<>'cancelled'
      AND o.trigger_evidence_kind<>'creative_direct_edit';
    IF owner_id IS NULL THEN RETURN; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('creative-initial-package:'||item_id::text,0));
    SELECT count(*) INTO selected_count FROM creative_order_variant
    WHERE order_item_id=item_id AND candidate_state='selected' AND status<>'cancelled';
    IF selected_count<>target_count THEN RETURN; END IF;
    SELECT count(*),jsonb_agg(jsonb_build_object('variant_id',variant_id,'revision',revision,
      'size_key',size_key,'asset_id',id,'attachment_id',attachment_id) ORDER BY variant_id,size_key)
    INTO asset_count,frozen_assets FROM (
      SELECT DISTINCT ON(v.id,a.size_key) v.id AS variant_id,v.revision,a.size_key,a.id,a.attachment_id
      FROM creative_order_variant v JOIN creative_order_asset a ON a.variant_id=v.id AND a.revision=v.revision
      WHERE v.order_item_id=item_id AND v.candidate_state='selected' AND v.status<>'cancelled'
        AND a.stage='generated' AND a.status='completed' AND a.attachment_id IS NOT NULL
        AND a.size_key IN ('1080x1080','1200x628','800x1000')
      ORDER BY v.id,a.size_key,a.created_at,a.id
    ) current_assets;
    IF asset_count<>3*target_count THEN RETURN; END IF;
    INSERT INTO creative_initial_generated_package(order_item_id,order_id,workspace_id,submitted_at,generated_at,target_variant_count,assets)
    VALUES(item_id,owner_id,workspace,submitted,clock_timestamp(),target_count,frozen_assets)
    ON CONFLICT(order_item_id) DO NOTHING;
END $$;

CREATE FUNCTION capture_creative_initial_generated_package() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE item_id UUID;
BEGIN
    IF TG_TABLE_NAME='creative_order_asset' THEN
      IF NEW.stage<>'generated' OR NEW.status<>'completed' OR NEW.attachment_id IS NULL THEN RETURN NEW; END IF;
      SELECT order_item_id INTO item_id FROM creative_order_variant WHERE id=NEW.variant_id;
    ELSE
      item_id:=NEW.order_item_id;
    END IF;
    PERFORM record_creative_initial_generated_package(item_id);
    RETURN NEW;
END $$;
CREATE TRIGGER creative_generated_package_asset AFTER INSERT OR UPDATE ON creative_order_asset
FOR EACH ROW EXECUTE FUNCTION capture_creative_initial_generated_package();
CREATE TRIGGER creative_generated_package_selection AFTER INSERT OR UPDATE OF candidate_state,revision ON creative_order_variant
FOR EACH ROW EXECUTE FUNCTION capture_creative_initial_generated_package();
