package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func setCreativeRetryForTest(t *testing.T, enabled bool) {
	t.Helper()
	if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_factory_settings(workspace_id,automatic_retry_enabled) VALUES($1,$2) ON CONFLICT(workspace_id) DO UPDATE SET automatic_retry_enabled=excluded.automatic_retry_enabled`, testWorkspaceID, enabled); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_factory_settings WHERE workspace_id=$1`, testWorkspaceID)
	})
}

func TestCreativeRetrySwitchPausesRecoveryWithoutSpendingBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	setCreativeRetryForTest(t, false)
	runOrderRecoveryForTest(t, f)
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_recovery_attempt a JOIN creative_recovery r ON r.id=a.recovery_id WHERE r.order_id=$1`, f.OrderID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("paused job dispatched: %d %v", count, err)
	}
	setCreativeRetryForTest(t, true)
	runOrderRecoveryForTest(t, f)
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_recovery_attempt a JOIN creative_recovery r ON r.id=a.recovery_id WHERE r.order_id=$1`, f.OrderID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("re-enabled job did not resume: %d %v", count, err)
	}
}

func TestCreativeRetrySwitchHoldsQueuedContinuationAndResumes(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	v := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "candidate", 1, "running", "1080x1080", []string{"1080x1080"})
	parent := addCreativeCandidateOrchestrationProductionTask(t, f, v, "failed", "candidate_primary")
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), parent); err != nil {
		t.Fatal(err)
	}
	var childID string
	if err := testPool.QueryRow(t.Context(), `SELECT id::text FROM agent_task_queue WHERE retry_of_task_id=$1`, parent.ID).Scan(&childID); err != nil {
		t.Fatal(err)
	}
	setCreativeRetryForTest(t, false)
	if _, err := testHandler.Queries.ClaimAgentTask(t.Context(), db.ClaimAgentTaskParams{AgentID: parent.AgentID, PrepareLeaseSecs: 60}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("paused queued retry was claimable: %v", err)
	}
	setCreativeRetryForTest(t, true)
	claimed, err := testHandler.Queries.ClaimAgentTask(t.Context(), db.ClaimAgentTaskParams{AgentID: parent.AgentID, PrepareLeaseSecs: 60})
	if err != nil || uuidToString(claimed.ID) != childID {
		t.Fatalf("resume claim: %s %v", uuidToString(claimed.ID), err)
	}
}

func TestCreativeRetrySwitchStopsContinuationButAllowsManualRetry(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	v := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "candidate", 1, "running", "1080x1080", []string{"1080x1080"})
	parent := addCreativeCandidateOrchestrationProductionTask(t, f, v, "failed", "candidate_primary")
	setCreativeRetryForTest(t, false)
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), parent); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE retry_of_task_id=$1`, parent.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("paused settlement dispatched: %d %v", count, err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	manual, err := testHandler.createManualCreativeTaskRetry(t.Context(), tx, parent.ID, parseUUID(testUserID))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	claimed, err := testHandler.Queries.ClaimAgentTask(t.Context(), db.ClaimAgentTaskParams{AgentID: parent.AgentID, PrepareLeaseSecs: 60})
	if err != nil || claimed.ID != manual.ID {
		t.Fatalf("explicit manual retry blocked: %v", err)
	}
}

func TestCreativeRetrySettingsRequireWorkspaceAdmin(t *testing.T) {
	setCreativeRetryForTest(t, true)
	for _, role := range []string{"member", "admin"} {
		req := httptest.NewRequest(http.MethodPatch, "/api/creative/settings", strings.NewReader(`{"automatic_retry_enabled":false}`))
		req = req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, db.Member{Role: role}))
		w := httptest.NewRecorder()
		testHandler.UpdateCreativeRetrySettings(w, req)
		want := http.StatusOK
		if role == "member" {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Fatalf("%s: %d %s", role, w.Code, w.Body.String())
		}
	}
}

func TestCreativeRetrySwitchAllowsInitialProduction(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	v := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "candidate", 1, "queued", "1080x1080", []string{"1080x1080"})
	initial := addCreativeCandidateOrchestrationProductionTask(t, f, v, "queued", "candidate_primary")
	setCreativeRetryForTest(t, false)
	claimed, err := testHandler.Queries.ClaimAgentTask(t.Context(), db.ClaimAgentTaskParams{AgentID: initial.AgentID, PrepareLeaseSecs: 60})
	if err != nil || claimed.ID != initial.ID {
		t.Fatalf("initial production incorrectly paused: %v", err)
	}
}
