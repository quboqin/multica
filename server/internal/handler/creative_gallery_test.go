package handler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func galleryTestTransaction(t *testing.T) (pgx.Tx, pgtype.UUID, pgtype.UUID, pgtype.UUID) {
	t.Helper()
	if testPool == nil {
		t.Skip("database not available")
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(t.Context()) })
	_, err = tx.Exec(t.Context(), `
CREATE TEMP TABLE creative_order (id UUID PRIMARY KEY, workspace_id UUID, issue_id UUID, status TEXT DEFAULT 'completed', input_snapshot JSONB DEFAULT '{}', trigger_evidence_kind TEXT DEFAULT 'manual', created_by UUID, updated_at TIMESTAMPTZ DEFAULT now());
CREATE TEMP TABLE creative_order_item (id UUID PRIMARY KEY, order_id UUID, adopted_variant_id UUID, adopted_at TIMESTAMPTZ, adopted_by UUID, updated_at TIMESTAMPTZ DEFAULT now());
CREATE TEMP TABLE creative_order_variant (id UUID PRIMARY KEY, order_item_id UUID, variant_key TEXT, revision INT DEFAULT 1, status TEXT, brief JSONB DEFAULT '{}', active_revision INT, staging_revision INT DEFAULT 1, candidate_state TEXT DEFAULT 'selected', updated_at TIMESTAMPTZ DEFAULT now());
CREATE TEMP TABLE creative_order_variant_revision (variant_id UUID, revision INT, brief JSONB DEFAULT '{}', status TEXT, expected_sizes TEXT[], activated_at TIMESTAMPTZ, created_at TIMESTAMPTZ DEFAULT now(), updated_at TIMESTAMPTZ DEFAULT now(), PRIMARY KEY (variant_id, revision));
CREATE TEMP TABLE creative_order_asset (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), variant_id UUID, asset_family_id UUID DEFAULT gen_random_uuid(), size_key TEXT, revision INT, stage TEXT, attachment_id UUID, derived_from_asset_id UUID, metadata JSONB DEFAULT '{}', evidence JSONB DEFAULT '{}', status TEXT, created_at TIMESTAMPTZ DEFAULT now(), updated_at TIMESTAMPTZ DEFAULT now(), UNIQUE(variant_id, size_key, revision, stage));
CREATE TEMP TABLE creative_order_qc_report (variant_id UUID, revision INT, lane TEXT DEFAULT 'visual', attempt INT DEFAULT 1, status TEXT);
CREATE TEMP TABLE creative_order_variant_qc_resolution (variant_id UUID, revision INT, attempt INT DEFAULT 1, outcome TEXT, created_at TIMESTAMPTZ DEFAULT now());
CREATE TEMP TABLE creative_feedback_event (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id UUID, issue_id UUID, idempotency_key TEXT DEFAULT '', actor_type TEXT DEFAULT 'member', actor_id UUID, subject_type TEXT DEFAULT 'variant', subject_id UUID, event_type TEXT DEFAULT 'decision', decision TEXT DEFAULT 'accepted', reason_codes TEXT[] DEFAULT '{}', comment TEXT DEFAULT '', annotation JSONB DEFAULT '{}', context_snapshot JSONB DEFAULT '{}', undo_of_id UUID, created_at TIMESTAMPTZ DEFAULT clock_timestamp());
`)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, userID, orderID, itemID := parseUUID(uuid.NewString()), parseUUID(uuid.NewString()), parseUUID(uuid.NewString()), parseUUID(uuid.NewString())
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order (id, workspace_id, created_by) VALUES ($1, $2, $3)`, orderID, workspaceID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_item (id, order_id) VALUES ($1, $2)`, itemID, orderID); err != nil {
		t.Fatal(err)
	}
	return tx, workspaceID, userID, itemID
}

func seedGalleryVariant(t *testing.T, tx pgx.Tx, itemID pgtype.UUID, risk bool) pgtype.UUID {
	t.Helper()
	id := parseUUID(uuid.NewString())
	status, visual, outcome := "completed", "passed", "delivered"
	if risk {
		status, visual, outcome = "action_required", "failed", "delivered_with_qc_risk"
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_variant (id, order_item_id, variant_key, status) VALUES ($1, $2, $3, $4)`, id, itemID, uuidToString(id), status); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_variant_revision (variant_id, revision, status, expected_sizes) VALUES ($1, 1, $2, $3)`, id, status, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_qc_report (variant_id, revision, status) VALUES ($1, 1, $2)`, id, visual); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome) VALUES ($1, 1, $2)`, id, outcome); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, status, attachment_id) VALUES ($1, $2, 1, 'primed', 'completed', $3)`, id, size, parseUUID(uuid.NewString())); err != nil {
			t.Fatal(err)
		}
	}
	if !risk {
		if _, err := copyCreativePrimedAssetsToDelivered(t.Context(), tx, id, 1, standardCreativeAssetSizes); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestCreativeGalleryRiskConfirmationAndIndependentVariants(t *testing.T) {
	tx, workspaceID, userID, itemID := galleryTestTransaction(t)
	riskID := seedGalleryVariant(t, tx, itemID, true)
	input := creativeGalleryInput{VariantID: uuidToString(riskID), Revision: 1, IdempotencyKey: "risk-entry"}
	if _, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, riskID, input); err == nil {
		t.Fatal("risk was accepted without acknowledgement")
	}
	input.QCRiskAcknowledged, input.QCRiskReason = true, "Reviewed all three sizes"
	event, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, riskID, input)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Revision int      `json:"revision"`
		Assets   []string `json:"delivery_asset_ids"`
		Sizes    []string `json:"delivery_package_sizes"`
	}
	if err := json.Unmarshal(event.ContextSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 1 || len(snapshot.Assets) != 3 || len(snapshot.Sizes) != 3 {
		t.Fatalf("incomplete bound package: %s", event.ContextSnapshot)
	}
	var active int
	if err := tx.QueryRow(t.Context(), `SELECT active_revision FROM creative_order_variant WHERE id = $1`, riskID).Scan(&active); err != nil || active != 1 {
		t.Fatalf("activation = %d, %v", active, err)
	}
	secondID := seedGalleryVariant(t, tx, itemID, false)
	if _, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, secondID, creativeGalleryInput{Revision: 1, IdempotencyKey: "second-entry"}); err != nil {
		t.Fatal(err)
	}
	again, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, riskID, input)
	if err != nil || again.ID != event.ID {
		t.Fatalf("idempotent confirmation: %+v %v", again, err)
	}
	var count int
	var adopted pgtype.UUID
	if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM creative_feedback_event`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("membership count = %d, %v", count, err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT adopted_variant_id FROM creative_order_item WHERE id = $1`, itemID).Scan(&adopted); err != nil || adopted.Valid {
		t.Fatal("gallery confirmation changed single-variant adoption")
	}
}

func TestCreativeGalleryRepairsExistingEntryAndPreservesDeliveredAssets(t *testing.T) {
	tx, workspaceID, userID, itemID := galleryTestTransaction(t)
	id := seedGalleryVariant(t, tx, itemID, false)
	var existingID string
	if err := tx.QueryRow(t.Context(), `INSERT INTO creative_feedback_event (workspace_id, actor_id, subject_id, context_snapshot) VALUES ($1, $2, $3, '{"revision":1,"action":"add_to_gallery"}') RETURNING id::text`, workspaceID, userID, id).Scan(&existingID); err != nil {
		t.Fatal(err)
	}
	var before []string
	if err := tx.QueryRow(t.Context(), `SELECT array_agg(id::text ORDER BY id) FROM creative_order_asset WHERE variant_id = $1 AND stage = 'delivered'`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	event, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, id, creativeGalleryInput{Revision: 1, IdempotencyKey: "repair"})
	if err != nil || event.ID != existingID {
		t.Fatalf("repair created another membership: %s %v", event.ID, err)
	}
	var unchanged bool
	if err := tx.QueryRow(t.Context(), `SELECT array_agg(id::text ORDER BY id) = $2::text[] FROM creative_order_asset WHERE variant_id = $1 AND stage = 'delivered'`, id, before).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("repair replaced existing delivery assets")
	}
	if _, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, id, creativeGalleryInput{Revision: 2}); err == nil {
		t.Fatal("membership silently changed revision")
	}
}

func TestCreativeGalleryRejectsMissingSizesAndCrossWorkspace(t *testing.T) {
	tx, workspaceID, userID, itemID := galleryTestTransaction(t)
	id := seedGalleryVariant(t, tx, itemID, true)
	input := creativeGalleryInput{Revision: 1, QCRiskAcknowledged: true, QCRiskReason: "Reviewed", IdempotencyKey: "incomplete"}
	if _, err := confirmCreativeGalleryEntry(t.Context(), tx, parseUUID(uuid.NewString()), userID, id, input); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-workspace lookup = %v", err)
	}
	if _, err := tx.Exec(t.Context(), `DELETE FROM pg_temp.creative_order_asset WHERE variant_id = $1 AND size_key = '800x1000'`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, id, input); err == nil {
		t.Fatal("incomplete Prime package was accepted")
	}
	var count int
	if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM creative_feedback_event`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid membership was persisted")
	}
}

func TestCreativeGalleryRemovalAndReentryDoNotResurrectOlderMembership(t *testing.T) {
	tx, workspaceID, userID, itemID := galleryTestTransaction(t)
	id := seedGalleryVariant(t, tx, itemID, false)
	oldID := parseUUID(uuid.NewString())
	latestID := parseUUID(uuid.NewString())
	for _, entryID := range []pgtype.UUID{oldID, latestID} {
		if _, err := tx.Exec(t.Context(), `INSERT INTO creative_feedback_event (id, workspace_id, actor_id, subject_id, context_snapshot) VALUES ($1, $2, $3, $4, '{"revision":1}')`, entryID, workspaceID, userID, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_feedback_event (workspace_id, actor_id, subject_id, event_type, decision, undo_of_id) VALUES ($1, $2, $3, 'undo', '', $4)`, workspaceID, userID, id, latestID); err != nil {
		t.Fatal(err)
	}
	event, err := confirmCreativeGalleryEntry(t.Context(), tx, workspaceID, userID, id, creativeGalleryInput{Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == uuidToString(oldID) || event.ID == uuidToString(latestID) {
		t.Fatal("reentry reused old membership")
	}
	var count int
	if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM creative_feedback_event`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("membership history = %d, %v", count, err)
	}
}
