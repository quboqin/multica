package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

type creativePrimeComposeRequest struct {
	VariantID string `json:"variant_id"`
	Async     bool   `json:"async,omitempty"`
	// Force is a human-requested Prime-only recomposition. It reuses the
	// canonical generated bases and frozen template contract, replacing only
	// the deterministic Prime package before visual QC runs again.
	Force bool `json:"force,omitempty"`
}

// ComposeCreativeOrderPrime is the explicit handoff from the bound Prime
// Skill to the backend-owned composer. Generated asset writes stay inert so a
// producer cannot accidentally start two composition jobs for one variant.
func (h *Handler) ComposeCreativeOrderPrime(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var input creativePrimeComposeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative Prime composition request")
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	composeContext := r.Context()
	if r.Header.Get("X-Actor-Source") == "task_token" {
		if input.Force {
			writeError(w, http.StatusForbidden, "task tokens cannot force creative Prime composition")
			return
		}
		taskID := parseUUID(strings.TrimSpace(r.Header.Get("X-Task-ID")))
		agentID := parseUUID(strings.TrimSpace(r.Header.Get("X-Agent-ID")))
		if !taskID.Valid || !agentID.Valid {
			writeError(w, http.StatusForbidden, "task is not authorized for this creative Prime composition")
			return
		}
		composeContext = context.WithValue(composeContext, creativePrimeComposeTaskFenceContextKey{}, creativePrimeComposeTaskFence{
			TaskID: taskID, AgentID: agentID,
		})
	}
	revision, jobStatus, generatedComplete, err := h.queueCreativePrimeComposition(
		composeContext, workspaceID, orderID, variantID, input.Force,
	)
	if err != nil {
		var immutableErr *creativePrimeImmutableRevisionError
		var cancelledErr *creativePrimeCancelledError
		var authorizationErr *creativePrimeTaskAuthorizationError
		if errors.As(err, &authorizationErr) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if errors.As(err, &immutableErr) || errors.As(err, &cancelledErr) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if !generatedComplete {
		writeJSON(w, http.StatusConflict, map[string]any{
			"variant_id": uuidToString(variantID), "revision": revision,
			"composed": false, "completed": false, "status": "waiting_for_generated_assets",
		})
		return
	}
	if jobStatus == "completed" {
		writeJSON(w, http.StatusOK, map[string]any{
			"variant_id": uuidToString(variantID), "revision": revision,
			"composed": true, "completed": true, "status": "completed",
		})
		return
	}
	if jobStatus == "failed" || jobStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative Prime composition is "+jobStatus+"; use an explicit force retry on a staging revision")
		return
	}
	if input.Async {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"variant_id": uuidToString(variantID), "revision": revision,
			"composed":  false,
			"completed": false,
			"status":    "composition_queued",
		})
		return
	}
	claim, claimed, err := h.claimCreativePrimeComposition(r.Context(), variantID, revision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !claimed {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"variant_id": uuidToString(variantID), "revision": revision,
			"composed": false, "completed": false, "status": "composition_in_progress",
		})
		return
	}
	if err := h.processCreativePrimeCompositionClaim(context.WithoutCancel(r.Context()), claim); err != nil {
		var handoffErr *creativeQCHandoffError
		if errors.As(err, &handoffErr) {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"variant_id": uuidToString(variantID), "revision": revision,
		"composed":  true,
		"completed": true,
		"status":    "completed",
	})
}

func (h *Handler) runCreativeOrderPrimeComposition(ctx context.Context, workspaceID, orderID, variantID, userID pgtype.UUID, force bool, primeClaim *creativePrimeCompositionClaim) (bool, error) {
	composeContext, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	defer h.publish(protocol.EventCreativeMaterialsUpdated, uuidToString(workspaceID), "member", uuidToString(userID), map[string]any{"scope": "order", "order_id": uuidToString(orderID)})
	composed, err := h.composeCreativeOrderVariantPrime(composeContext, workspaceID, orderID, variantID, userID, force, primeClaim)
	if err != nil {
		var immutableErr *creativePrimeImmutableRevisionError
		if errors.As(err, &immutableErr) {
			return false, err
		}
		var cancelledErr *creativePrimeCancelledError
		if errors.As(err, &cancelledErr) {
			return false, err
		}
		var handoffErr *creativeQCHandoffError
		if errors.As(err, &handoffErr) {
			h.markCreativeQCHandoffFailed(composeContext, variantID, handoffErr)
			return false, err
		}
		h.markCreativePrimeCompositionFailed(composeContext, variantID, err)
		return false, err
	}
	return composed, nil
}
