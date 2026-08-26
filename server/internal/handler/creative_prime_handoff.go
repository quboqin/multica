package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
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
	if input.Async {
		requestContext := context.WithoutCancel(r.Context())
		go h.runCreativeOrderPrimeComposition(requestContext, workspaceID, orderID, variantID, userID, input.Force)
		writeJSON(w, http.StatusAccepted, map[string]any{
			"variant_id": uuidToString(variantID),
			"composed":   false,
			"completed":  false,
			"status":     "composition_started",
		})
		return
	}
	composed, err := h.runCreativeOrderPrimeComposition(context.WithoutCancel(r.Context()), workspaceID, orderID, variantID, userID, input.Force)
	if err != nil {
		var handoffErr *creativeQCHandoffError
		if errors.As(err, &handoffErr) {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if !composed {
		writeJSON(w, http.StatusConflict, map[string]any{
			"variant_id": uuidToString(variantID),
			"composed":   false,
			"completed":  false,
			"status":     "waiting_for_generated_assets",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"variant_id": uuidToString(variantID),
		"composed":   composed,
		"completed":  true,
	})
}

func (h *Handler) runCreativeOrderPrimeComposition(ctx context.Context, workspaceID, orderID, variantID, userID pgtype.UUID, force bool) (bool, error) {
	composeContext, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	defer h.publish(protocol.EventCreativeMaterialsUpdated, uuidToString(workspaceID), "member", uuidToString(userID), map[string]any{"scope": "order", "order_id": uuidToString(orderID)})
	composed, err := h.composeCreativeOrderVariantPrime(composeContext, workspaceID, orderID, variantID, userID, force)
	if err != nil {
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
