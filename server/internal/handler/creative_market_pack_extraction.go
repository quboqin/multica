package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const marketPackComponentExtractionEvidenceKind = "creative_market_pack_component_extraction"

type creativeMarketPackExtractionCandidate struct {
	ID                   string   `json:"id"`
	Label                string   `json:"label"`
	Kind                 string   `json:"kind"`
	SuggestedComponentID string   `json:"suggested_component_id"`
	SuggestedRole        string   `json:"suggested_role"`
	Content              string   `json:"content"`
	Rect                 [4]int   `json:"rect"`
	Confidence           float64  `json:"confidence"`
	Evidence             []string `json:"evidence"`
}

type creativeMarketPackExtractionResult struct {
	Summary    string                                  `json:"summary"`
	Candidates []creativeMarketPackExtractionCandidate `json:"candidates"`
}

type creativeMarketPackExtractionResponse struct {
	ID                 string                             `json:"id"`
	ResourceID         string                             `json:"resource_id"`
	SourceAttachmentID string                             `json:"source_attachment_id"`
	SourceURL          string                             `json:"source_url"`
	SourceFilename     string                             `json:"source_filename"`
	SourceWidth        int                                `json:"source_width"`
	SourceHeight       int                                `json:"source_height"`
	Status             string                             `json:"status"`
	Result             creativeMarketPackExtractionResult `json:"result"`
	ErrorMessage       string                             `json:"error_message"`
	TaskID             string                             `json:"task_id"`
	AgentID            string                             `json:"agent_id"`
	CreatedAt          string                             `json:"created_at"`
	UpdatedAt          string                             `json:"updated_at"`
}

type creativeMarketPackExtractionTaskContext struct {
	Workflow           string `json:"workflow"`
	ExtractionID       string `json:"extraction_id"`
	ResourceID         string `json:"resource_id"`
	SourceAttachmentID string `json:"source_attachment_id"`
	SourceWidth        int    `json:"source_width"`
	SourceHeight       int    `json:"source_height"`
}

func (h *Handler) CreateCreativeMarketPackComponentExtraction(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	var input struct {
		AttachmentID string `json:"attachment_id"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid component extraction request")
		return
	}
	attachmentID, ok := parseUUIDOrBadRequest(w, input.AttachmentID, "attachment_id")
	if !ok {
		return
	}
	if input.Width < 1 || input.Height < 1 || input.Width > 20000 || input.Height > 20000 {
		writeError(w, http.StatusBadRequest, "image dimensions must be between 1 and 20000 pixels")
		return
	}
	if _, err := h.requireCreativeResource(r.Context(), workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	attachment, err := h.Queries.GetAttachmentByIDOnly(r.Context(), attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID || !strings.HasPrefix(attachment.ContentType, "image/") {
		writeError(w, http.StatusNotFound, "image attachment not found")
		return
	}

	var extractionID string
	if err := h.DB.QueryRow(r.Context(), `
INSERT INTO creative_market_pack_component_extraction (
  workspace_id, resource_id, source_attachment_id, source_width, source_height, created_by
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text
`, workspaceID, resourceID, attachmentID, input.Width, input.Height, userID).Scan(&extractionID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create component extraction")
		return
	}

	agent, err := h.resolveMarketPackComponentExtractionAgent(r.Context(), workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.failCreativeMarketPackExtraction(r.Context(), parseUUID(extractionID), "No enabled agent with market_pack_component_extraction capability is configured.")
		response, _ := h.loadCreativeMarketPackExtraction(r.Context(), workspaceID, parseUUID(extractionID))
		writeJSON(w, http.StatusAccepted, response)
		return
	}
	if err != nil || h.TaskService == nil {
		h.failCreativeMarketPackExtraction(r.Context(), parseUUID(extractionID), "Component extraction task service is unavailable.")
		response, _ := h.loadCreativeMarketPackExtraction(r.Context(), workspaceID, parseUUID(extractionID))
		writeJSON(w, http.StatusAccepted, response)
		return
	}
	taskContext, _ := json.Marshal(map[string]any{
		"type":                 "creative_domain_task",
		"workflow":             "creative_market_pack_component_extraction",
		"extraction_id":        extractionID,
		"resource_id":          uuidToString(resourceID),
		"source_attachment_id": input.AttachmentID,
		"source_width":         input.Width,
		"source_height":        input.Height,
	})
	tasks, err := h.TaskService.EnqueueDirectTaskFanout(r.Context(), service.DirectTaskFanout{
		Agent:                agent,
		RequestingUserID:     userID,
		Attribution:          attribution.DirectHumanRun(userID, attribution.EvidenceKind(marketPackComponentExtractionEvidenceKind), parseUUID(extractionID)),
		TriggerEvidenceKind:  marketPackComponentExtractionEvidenceKind,
		TriggerEvidenceRefID: parseUUID(extractionID),
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: input.AttachmentID,
			Context: taskContext,
		}},
	})
	if err != nil || len(tasks) != 1 {
		message := "Component extraction could not be queued."
		if err != nil {
			message = fmt.Sprintf("Component extraction could not be queued: %s", err)
		}
		h.failCreativeMarketPackExtraction(r.Context(), parseUUID(extractionID), message)
	}
	response, loadErr := h.loadCreativeMarketPackExtraction(r.Context(), workspaceID, parseUUID(extractionID))
	if loadErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to read component extraction")
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) GetLatestCreativeMarketPackComponentExtraction(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	if _, err := h.requireCreativeResource(r.Context(), workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	var extractionID pgtype.UUID
	if err := h.DB.QueryRow(r.Context(), `
SELECT id FROM creative_market_pack_component_extraction
WHERE workspace_id = $1 AND resource_id = $2
ORDER BY created_at DESC
LIMIT 1
`, workspaceID, resourceID).Scan(&extractionID); errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read component extraction")
		return
	}
	response, err := h.loadCreativeMarketPackExtraction(r.Context(), workspaceID, extractionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read component extraction")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetCreativeMarketPackComponentExtraction(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	extractionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "extractionId"), "extraction_id")
	if !ok {
		return
	}
	if _, err := h.requireCreativeResource(r.Context(), workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	response, err := h.loadCreativeMarketPackExtraction(r.Context(), workspaceID, extractionID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && response.ResourceID != uuidToString(resourceID)) {
		writeError(w, http.StatusNotFound, "component extraction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read component extraction")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) PutCreativeMarketPackComponentExtraction(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	extractionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "extractionId"), "extraction_id")
	if !ok {
		return
	}
	actorType, _ := h.resolveActor(r, uuidToString(userID), uuidToString(workspaceID))
	if actorType != "agent" {
		writeError(w, http.StatusForbidden, "component extraction results must be written by the assigned agent")
		return
	}
	if !h.requestTaskMatchesMarketPackExtraction(r.Context(), r.Header.Get("X-Task-ID"), workspaceID, resourceID, extractionID) {
		writeError(w, http.StatusForbidden, "task is not assigned to this component extraction")
		return
	}
	var input struct {
		Status       string                                  `json:"status"`
		Summary      string                                  `json:"summary"`
		Candidates   []creativeMarketPackExtractionCandidate `json:"candidates"`
		ErrorMessage string                                  `json:"error_message"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid component extraction result")
		return
	}
	input.Status = strings.TrimSpace(input.Status)
	if input.Status != "completed" && input.Status != "failed" {
		writeError(w, http.StatusBadRequest, "status must be completed or failed")
		return
	}
	var width, height int
	if err := h.DB.QueryRow(r.Context(), `
SELECT source_width, source_height
FROM creative_market_pack_component_extraction
WHERE id = $1 AND resource_id = $2 AND workspace_id = $3
`, extractionID, resourceID, workspaceID).Scan(&width, &height); err != nil {
		writeError(w, http.StatusNotFound, "component extraction not found")
		return
	}
	result, err := normalizeCreativeMarketPackExtractionResult(input.Summary, input.Candidates, width, height)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	encoded, _ := json.Marshal(result)
	errorMessage := strings.TrimSpace(input.ErrorMessage)
	if input.Status == "failed" && errorMessage == "" {
		errorMessage = "Component extraction failed without an error message."
	}
	if _, err := h.DB.Exec(r.Context(), `
UPDATE creative_market_pack_component_extraction
SET status = $4, result = $5::jsonb, error_message = $6,
    completed_at = now(), updated_at = now()
WHERE id = $1 AND resource_id = $2 AND workspace_id = $3
`, extractionID, resourceID, workspaceID, input.Status, encoded, errorMessage); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save component extraction result")
		return
	}
	response, err := h.loadCreativeMarketPackExtraction(r.Context(), workspaceID, extractionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read component extraction result")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "market_pack_component_extraction", "resource_id": uuidToString(resourceID), "extraction_id": uuidToString(extractionID),
	})
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) ApplyCreativeMarketPackComponentExtraction(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	extractionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "extractionId"), "extraction_id")
	if !ok {
		return
	}
	result, err := h.DB.Exec(r.Context(), `
UPDATE creative_market_pack_component_extraction
SET status = 'applied', applied_at = now(), updated_at = now()
WHERE id = $1 AND resource_id = $2 AND workspace_id = $3 AND status IN ('completed', 'applied')
`, extractionID, resourceID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark component extraction applied")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "component extraction is not ready to apply")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeCreativeMarketPackExtractionResult(summary string, candidates []creativeMarketPackExtractionCandidate, width, height int) (creativeMarketPackExtractionResult, error) {
	if len(candidates) > 50 {
		return creativeMarketPackExtractionResult{}, errors.New("component extraction cannot contain more than 50 candidates")
	}
	seen := map[string]struct{}{}
	for index := range candidates {
		candidate := &candidates[index]
		candidate.ID = strings.TrimSpace(candidate.ID)
		if candidate.ID == "" {
			candidate.ID = fmt.Sprintf("candidate_%d", index+1)
		}
		if _, exists := seen[candidate.ID]; exists {
			return creativeMarketPackExtractionResult{}, errors.New("component candidate ids must be unique")
		}
		seen[candidate.ID] = struct{}{}
		candidate.Label = strings.TrimSpace(candidate.Label)
		candidate.Kind = strings.TrimSpace(candidate.Kind)
		candidate.SuggestedComponentID = strings.TrimSpace(candidate.SuggestedComponentID)
		candidate.SuggestedRole = strings.TrimSpace(candidate.SuggestedRole)
		candidate.Content = strings.TrimSpace(candidate.Content)
		if candidate.Label == "" || len([]rune(candidate.Label)) > 120 {
			return creativeMarketPackExtractionResult{}, errors.New("each component candidate needs a concise label")
		}
		if candidate.Kind != "image" && candidate.Kind != "qr" && candidate.Kind != "text" {
			return creativeMarketPackExtractionResult{}, errors.New("component candidate kind must be image, qr, or text")
		}
		if len([]rune(candidate.Content)) > 2000 {
			return creativeMarketPackExtractionResult{}, errors.New("component candidate content is too long")
		}
		if candidate.Rect[0] < 0 || candidate.Rect[1] < 0 || candidate.Rect[2] <= candidate.Rect[0] || candidate.Rect[3] <= candidate.Rect[1] || candidate.Rect[2] > width || candidate.Rect[3] > height {
			return creativeMarketPackExtractionResult{}, errors.New("component candidate rect is outside the source image")
		}
		if candidate.Confidence < 0 || candidate.Confidence > 1 {
			return creativeMarketPackExtractionResult{}, errors.New("component candidate confidence must be between 0 and 1")
		}
		candidate.Evidence = uniqueNonEmptyStrings(candidate.Evidence)
		if len(candidate.Evidence) > 6 {
			candidate.Evidence = candidate.Evidence[:6]
		}
	}
	return creativeMarketPackExtractionResult{Summary: strings.TrimSpace(summary), Candidates: candidates}, nil
}

func (h *Handler) resolveMarketPackComponentExtractionAgent(ctx context.Context, workspaceID pgtype.UUID) (db.Agent, error) {
	var agentID pgtype.UUID
	err := h.DB.QueryRow(ctx, `
SELECT agent.id
FROM agent
JOIN agent_skill binding ON binding.agent_id = agent.id AND binding.enabled
JOIN skill capability ON capability.id = binding.skill_id
  AND capability.workspace_id = agent.workspace_id
WHERE agent.workspace_id = $1
  AND agent.archived_at IS NULL
  AND capability.config->>'kind' = 'creative_role'
  AND capability.config->>'capability' = 'market_pack_component_extraction'
ORDER BY agent.created_at, agent.id
LIMIT 1
`, workspaceID).Scan(&agentID)
	if err != nil {
		return db.Agent{}, err
	}
	return h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
}

func (h *Handler) requestTaskMatchesMarketPackExtraction(ctx context.Context, taskID string, workspaceID, resourceID, extractionID pgtype.UUID) bool {
	parsedTaskID, err := parseUUIDString(taskID)
	if err != nil {
		return false
	}
	var matches bool
	err = h.DB.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM agent_task_queue task
  JOIN agent ON agent.id = task.agent_id
  WHERE task.id = $1 AND agent.workspace_id = $2
    AND task.trigger_evidence_kind = $3 AND task.trigger_evidence_ref_id = $4
    AND task.context->>'resource_id' = $5
    AND task.context->>'extraction_id' = $6
)
`, parsedTaskID, workspaceID, marketPackComponentExtractionEvidenceKind, extractionID, uuidToString(resourceID), uuidToString(extractionID)).Scan(&matches)
	return err == nil && matches
}

func (h *Handler) failCreativeMarketPackExtraction(ctx context.Context, extractionID pgtype.UUID, message string) {
	_, _ = h.DB.Exec(ctx, `
UPDATE creative_market_pack_component_extraction
SET status = 'failed', error_message = $2, completed_at = now(), updated_at = now()
WHERE id = $1
`, extractionID, strings.TrimSpace(message))
}

func (h *Handler) loadCreativeMarketPackExtraction(ctx context.Context, workspaceID, extractionID pgtype.UUID) (creativeMarketPackExtractionResponse, error) {
	var response creativeMarketPackExtractionResponse
	var resultJSON, storedStatus, taskStatus, taskError string
	err := h.DB.QueryRow(ctx, `
SELECT extraction.id::text, extraction.resource_id::text, extraction.source_attachment_id::text,
       extraction.source_width, extraction.source_height, extraction.status, extraction.result::text,
       extraction.error_message, extraction.created_at::text, extraction.updated_at::text,
       COALESCE(task.id::text, ''), COALESCE(task.agent_id::text, ''),
       COALESCE(task.status, ''), COALESCE(task.error, '')
FROM creative_market_pack_component_extraction extraction
LEFT JOIN LATERAL (
  SELECT id, agent_id, status, error
  FROM agent_task_queue
  WHERE trigger_evidence_kind = $3 AND trigger_evidence_ref_id = extraction.id
  ORDER BY created_at DESC
  LIMIT 1
) task ON true
WHERE extraction.id = $1 AND extraction.workspace_id = $2
`, extractionID, workspaceID, marketPackComponentExtractionEvidenceKind).Scan(
		&response.ID, &response.ResourceID, &response.SourceAttachmentID,
		&response.SourceWidth, &response.SourceHeight, &storedStatus, &resultJSON,
		&response.ErrorMessage, &response.CreatedAt, &response.UpdatedAt,
		&response.TaskID, &response.AgentID, &taskStatus, &taskError,
	)
	if err != nil {
		return response, err
	}
	if err := json.Unmarshal([]byte(resultJSON), &response.Result); err != nil {
		return response, err
	}
	response.Status = derivedMarketPackExtractionStatus(storedStatus, taskStatus)
	if response.Status == "failed" && response.ErrorMessage == "" {
		response.ErrorMessage = strings.TrimSpace(taskError)
		if response.ErrorMessage == "" {
			response.ErrorMessage = "Component extraction task failed."
		}
	}
	attachmentID, err := parseUUIDString(response.SourceAttachmentID)
	if err != nil {
		return response, err
	}
	attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID {
		return response, errors.New("component extraction source attachment is unavailable")
	}
	attachmentResponse := h.attachmentToResponse(attachment)
	response.SourceURL = attachmentResponse.MarkdownURL
	response.SourceFilename = attachmentResponse.Filename
	return response, nil
}

func derivedMarketPackExtractionStatus(storedStatus, taskStatus string) string {
	if storedStatus == "completed" || storedStatus == "failed" || storedStatus == "applied" {
		return storedStatus
	}
	switch taskStatus {
	case "running":
		return "running"
	case "failed", "cancelled":
		return "failed"
	default:
		return "pending"
	}
}
