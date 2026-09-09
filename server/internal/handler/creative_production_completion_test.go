package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreativeProductionCompletionRequiresItsGeneratedPhase(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "ready"}[complete], func(t *testing.T) {
			f := createCreativeCountFixture(t, 1)
			variant := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "candidate", 1, "running", "1080x1080", []string{"1080x1080"})
			addCreativeCandidateOrchestrationProductionTask(t, f, variant, "running", "candidate_primary")
			var taskID string
			if err := testPool.QueryRow(t.Context(), `SELECT task_id::text FROM creative_task_binding WHERE variant_id=$1 AND workflow='creative_production'`, variant).Scan(&taskID); err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context=jsonb_set(context,'{expected_sizes}','["1080x1080"]'::jsonb) WHERE id=$1`, taskID); err != nil {
				t.Fatal(err)
			}
			if complete {
				addCreativeCandidateOrchestrationAsset(t, variant, "1080x1080", "generated")
			} else {
				if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_image_operation(variant_id,revision,size_key,operation_kind,idempotency_key,status,error_type,error_message) VALUES($1,1,'1080x1080','generation','completion-test','failed','provider_5xx','provider 503: no compatible accounts')`, variant); err != nil {
					t.Fatal(err)
				}
			}
			request := newDaemonTokenRequest(http.MethodPost, "/api/daemon/tasks/"+taskID+"/complete", TaskCompleteRequest{Output: "turn ended"}, testWorkspaceID, "completion-test")
			request = withURLParam(request, "taskId", taskID)
			response := httptest.NewRecorder()
			testHandler.CompleteTask(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("completion: %d %s", response.Code, response.Body.String())
			}
			var status, reason, message string
			if err := testPool.QueryRow(t.Context(), `SELECT status,COALESCE(failure_reason,''),COALESCE(error,'') FROM agent_task_queue WHERE id=$1`, taskID).Scan(&status, &reason, &message); err != nil {
				t.Fatal(err)
			}
			if complete && status != "completed" {
				t.Fatalf("completed primary incorrectly requires future sizes: %s %s", status, message)
			}
			if !complete && (status != "failed" || reason != "creative_output_missing" || !strings.Contains(message, "provider 503")) {
				t.Fatalf("missing images accepted or reason lost: %s %s %s", status, reason, message)
			}
		})
	}
}
