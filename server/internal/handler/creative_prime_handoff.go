package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type creativePrimeComposeRequest struct {
	VariantID string `json:"variant_id"`
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
	composeContext, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	defer cancel()
	composed, err := h.composeCreativeOrderVariantPrime(composeContext, workspaceID, orderID, variantID, userID)
	if err != nil {
		var handoffErr *creativeQCHandoffError
		if errors.As(err, &handoffErr) {
			h.markCreativeQCHandoffFailed(context.WithoutCancel(r.Context()), variantID, handoffErr)
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		h.markCreativePrimeCompositionFailed(context.WithoutCancel(r.Context()), variantID, err)
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
