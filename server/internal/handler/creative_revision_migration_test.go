package handler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestCreativeRevisionMigrationDoesNotInferHistoricalActiveRevision(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	schema := "creative_revision_migration_" + uuid.NewString()[:8]
	if _, err := tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`, public`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE creative_order (
  id UUID PRIMARY KEY,
  input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  trigger_evidence_kind TEXT NOT NULL DEFAULT 'manual'
);
CREATE TABLE creative_order_item (
  id UUID PRIMARY KEY,
  order_id UUID NOT NULL REFERENCES creative_order(id) ON DELETE CASCADE
);
CREATE TABLE creative_order_variant (
  id UUID PRIMARY KEY,
  order_item_id UUID NOT NULL REFERENCES creative_order_item(id) ON DELETE CASCADE,
  variant_key TEXT NOT NULL,
  revision INT NOT NULL,
  brief JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE attachment (id UUID PRIMARY KEY);
CREATE TABLE agent_runtime (id UUID PRIMARY KEY);
CREATE TABLE agent_task_queue (id UUID PRIMARY KEY);
CREATE TABLE creative_order_asset (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
  size_key TEXT NOT NULL,
  revision INT NOT NULL,
  stage TEXT NOT NULL,
  attachment_id UUID REFERENCES attachment(id),
  status TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (variant_id, size_key, revision, stage)
);
CREATE TABLE creative_order_variant_qc_resolution (
  variant_id UUID NOT NULL REFERENCES creative_order_variant(id) ON DELETE CASCADE,
  revision INT NOT NULL,
  attempt INT NOT NULL DEFAULT 1,
  outcome TEXT NOT NULL,
  PRIMARY KEY (variant_id, revision, attempt)
);
`); err != nil {
		t.Fatal(err)
	}

	orderID, itemID, variantID := uuid.New(), uuid.New(), uuid.New()
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order (id, input_snapshot)
VALUES ($1, '{"expected_sizes":["1200x628","1080x1080","1200x628","invalid"]}'::jsonb)
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_item (id, order_id) VALUES ($1, $2)
`, itemID, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_variant (id, order_item_id, variant_key, revision, status)
VALUES ($1, $2, 'V01', 3, 'action_required')
`, variantID, itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome)
VALUES ($1, 2, 'delivered'), ($1, 3, 'delivered_with_qc_risk')
`, variantID); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int{2, 3} {
		for _, size := range standardCreativeAssetSizes {
			attachmentID := uuid.New()
			if _, err := tx.Exec(ctx, `INSERT INTO attachment (id) VALUES ($1)`, attachmentID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_asset (
  variant_id, size_key, revision, stage, attachment_id, status
) VALUES ($1, $2, $3, 'delivered', $4, 'completed')
`, variantID, size, revision, attachmentID); err != nil {
				t.Fatal(err)
			}
		}
	}

	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "275_creative_variant_revisions_and_image_operations.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply creative revision migration: %v", err)
	}

	var activeRevision *int
	var stagingRevision int
	if err := tx.QueryRow(ctx, `
SELECT active_revision, staging_revision
FROM creative_order_variant
WHERE id = $1
`, variantID).Scan(&activeRevision, &stagingRevision); err != nil {
		t.Fatal(err)
	}
	if activeRevision != nil || stagingRevision != 3 {
		t.Fatalf("migrated lifecycle = active %v staging %d, want no inferred active and staging 3", activeRevision, stagingRevision)
	}
	rows, err := tx.Query(ctx, `
SELECT revision, status, activated_at IS NOT NULL
FROM creative_order_variant_revision
WHERE variant_id = $1
ORDER BY revision
`, variantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type revisionState struct {
		revision  int
		status    string
		activated bool
	}
	var revisions []revisionState
	for rows.Next() {
		var state revisionState
		if err := rows.Scan(&state.revision, &state.status, &state.activated); err != nil {
			t.Fatal(err)
		}
		revisions = append(revisions, state)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 || revisions[0] != (revisionState{revision: 2, status: "completed"}) ||
		revisions[1] != (revisionState{revision: 3, status: "action_required"}) {
		t.Fatalf("migrated revision audit rows = %#v", revisions)
	}
	var migratedSizes []string
	if err := tx.QueryRow(ctx, `
SELECT expected_sizes FROM creative_order_variant_revision
WHERE variant_id = $1 AND revision = 3
`, variantID).Scan(&migratedSizes); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(migratedSizes, []string{"1080x1080", "1200x628"}) {
		t.Fatalf("migrated expected sizes = %#v, want canonical unique valid sizes", migratedSizes)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, expected_sizes, input_fingerprint
) VALUES ($1, 3, ARRAY['1080x1080','1200x628']::text[], 'generated-contract-v1')
`, variantID); err != nil {
		t.Fatalf("insert migrated Prime contract job: %v", err)
	}
	var primeSizes []string
	var primeFingerprint string
	if err := tx.QueryRow(ctx, `
SELECT expected_sizes, input_fingerprint
FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = 3
`, variantID).Scan(&primeSizes, &primeFingerprint); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(primeSizes, []string{"1080x1080", "1200x628"}) || primeFingerprint != "generated-contract-v1" {
		t.Fatalf("migrated Prime job contract = sizes %v fingerprint %q", primeSizes, primeFingerprint)
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant_revision
SET expected_sizes = ARRAY['1080x1080', '1080x1080']::text[]
WHERE variant_id = $1 AND revision = 3
`, variantID); err == nil {
		t.Fatal("duplicate expected sizes passed the revision contract constraint")
	}
}
