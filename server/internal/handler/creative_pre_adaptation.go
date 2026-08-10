package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const creativePreAdaptationEvidenceKind = "creative_source_analysis"

type creativePreAdaptationTaskContext struct {
	Type               string `json:"type"`
	Workflow           string `json:"workflow"`
	SourceAnalysisID   string `json:"source_analysis_id"`
	CandidateID        string `json:"candidate_id"`
	MarketPackID       string `json:"market_pack_id"`
	MarketPackVersion  int    `json:"market_pack_version"`
	CopyLibraryID      string `json:"copy_library_id"`
	CopyLibraryVersion int    `json:"copy_library_version"`
}

type creativePreAdaptationInput struct {
	Status       string          `json:"status"`
	Summary      string          `json:"summary"`
	Result       json.RawMessage `json:"result"`
	ErrorCode    string          `json:"error_code"`
	ErrorMessage string          `json:"error_message"`
}

type creativePreAdaptationRetryRequest struct {
	MarketPackID string `json:"market_pack_id"`
}

type creativePreAdaptationSourceTextBlock struct {
	ID           string `json:"id"`
	Location     string `json:"location"`
	Role         string `json:"role"`
	SourceText   string `json:"source_text"`
	Purpose      string `json:"purpose"`
	SemanticKind string `json:"semantic_kind"`
}

type creativePreAdaptationVisualBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type creativePreAdaptationSourceVisualRegion struct {
	ID             string                            `json:"id"`
	Location       string                            `json:"location"`
	Kind           string                            `json:"kind"`
	SourceBlockIDs []string                          `json:"source_block_ids"`
	VisualBounds   creativePreAdaptationVisualBounds `json:"visual_bounds"`
}

// A calculated replacement remains a recommendation until a user accepts it
// for the current order. The formula and its frozen inputs make that decision
// inspectable without pretending the value came from the copy library.
type creativePreAdaptationCalculation struct {
	RuleKey string   `json:"rule_key"`
	Formula string   `json:"formula"`
	Inputs  []string `json:"inputs"`
	Result  string   `json:"result"`
}

type creativePreAdaptationTextReplacement struct {
	BlockID             string                            `json:"block_id"`
	VisualRegionID      string                            `json:"visual_region_id"`
	Location            string                            `json:"location"`
	Role                string                            `json:"role"`
	SourceText          string                            `json:"source_text"`
	ReplacementText     string                            `json:"replacement_text"`
	SourceKeys          []string                          `json:"source_keys"`
	Status              string                            `json:"status"`
	Note                string                            `json:"note"`
	RecommendationBasis []string                          `json:"recommendation_basis"`
	Calculation         *creativePreAdaptationCalculation `json:"calculation"`
}

type creativePreAdaptationRepaymentPlanValues struct {
	Principal          string `json:"principal"`
	Tenor              string `json:"tenor"`
	TotalInterest      string `json:"total_interest"`
	TotalRepayment     string `json:"total_repayment"`
	MonthlyInstallment string `json:"monthly_installment"`
}

type creativePreAdaptationRepaymentPlanSelection struct {
	ID          string                                   `json:"id"`
	PlanKey     string                                   `json:"plan_key"`
	Principal   int64                                    `json:"principal"`
	TenorMonths int                                      `json:"tenor_months"`
	Values      creativePreAdaptationRepaymentPlanValues `json:"values"`
}

// A numeric layout describes how approved first-party plan values occupy a
// competitor-inspired numeric area. Each source block remains traceable: text
// that is not an actual repayment label or value must be filled as approved
// copy instead of being silently absorbed into a numeric region.
type creativePreAdaptationNumericLayout struct {
	ID                string   `json:"id"`
	VisualRegionID    string   `json:"visual_region_id"`
	SourceBlockIDs    []string `json:"source_block_ids"`
	Location          string   `json:"location"`
	LayoutKind        string   `json:"layout_kind"`
	ScenarioIDs       []string `json:"scenario_ids"`
	TargetColumns     []string `json:"target_columns"`
	RenderInstruction string   `json:"render_instruction"`
}

type creativePreAdaptationCompletedResult struct {
	MarketPackID            string                                        `json:"market_pack_id"`
	MarketPackVersion       int                                           `json:"market_pack_version"`
	CopyLibraryID           string                                        `json:"copy_library_id"`
	CopyLibraryVersion      int                                           `json:"copy_library_version"`
	TextReplacements        []creativePreAdaptationTextReplacement        `json:"text_replacements"`
	RepaymentPlanSelections []creativePreAdaptationRepaymentPlanSelection `json:"repayment_plan_selections"`
	NumericLayouts          []creativePreAdaptationNumericLayout          `json:"numeric_layouts"`
	AnalysisHighlights      []string                                      `json:"analysis_highlights"`
	ProductionPrompt        string                                        `json:"production_prompt"`
}

// enqueueCreativePreAdaptation intentionally runs after the source analysis
// completes. The latter is market-neutral; this task freezes one explicit
// market pack and its bound copy library before making any recommendation.
func (h *Handler) enqueueCreativePreAdaptation(ctx context.Context, sourceTask db.AgentTaskQueue, workspaceRaw string) {
	if h.TaskService == nil || !sourceTask.TriggerEvidenceKind.Valid || sourceTask.TriggerEvidenceKind.String != "creative_crawl_run_analysis" {
		return
	}
	var sourceContext creativeReferenceAnalysisTaskContext
	if json.Unmarshal(sourceTask.Context, &sourceContext) != nil || sourceContext.Workflow != "creative_reference_analysis" {
		return
	}
	workspaceID, err := parseUUIDString(strings.TrimSpace(workspaceRaw))
	if err != nil {
		return
	}
	candidateID, err := parseUUIDString(strings.TrimSpace(sourceContext.CandidateID))
	if err != nil {
		return
	}
	var analysisID pgtype.UUID
	err = h.DB.QueryRow(ctx, `
SELECT id FROM creative_source_analysis
WHERE workspace_id = $1 AND candidate_id = $2 AND analysis_version = $3 AND status = 'completed'
ORDER BY completed_at DESC LIMIT 1
`, workspaceID, candidateID, sourceContext.AnalysisVersion).Scan(&analysisID)
	if err != nil {
		return
	}
	marketPack, copyLibrary, err := h.defaultCreativePreAdaptationResources(ctx, workspaceID)
	if err != nil {
		// An explicit default is a prerequisite, not a reason to fail the
		// already-valid source analysis.
		return
	}
	agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: sourceTask.AgentID, WorkspaceID: workspaceID})
	if err != nil || agent.ArchivedAt.Valid {
		return
	}
	_, _ = h.enqueueCreativePreAdaptationTask(ctx, agent, sourceTask.RequestingUserID, analysisID, candidateID, marketPack, copyLibrary)
}

// enqueueCreativePreAdaptationTask is used both by the automatic handoff after
// reference analysis and by an explicit order-draft retry. The task key freezes
// the market and copy-library versions, and keeps repeated user clicks idempotent.
func (h *Handler) enqueueCreativePreAdaptationTask(
	ctx context.Context,
	agent db.Agent,
	requestingUserID, analysisID, candidateID pgtype.UUID,
	marketPack, copyLibrary creativeResourceResponse,
) (db.AgentTaskQueue, error) {
	if h.TaskService == nil {
		return db.AgentTaskQueue{}, errors.New("pre-adaptation task service is unavailable")
	}
	contextJSON, _ := json.Marshal(creativePreAdaptationTaskContext{
		Type: "creative_domain_task", Workflow: "creative_pre_adaptation", SourceAnalysisID: uuidToString(analysisID), CandidateID: uuidToString(candidateID),
		MarketPackID: marketPack.ID, MarketPackVersion: marketPack.PublishedVersion,
		CopyLibraryID: copyLibrary.ID, CopyLibraryVersion: copyLibrary.PublishedVersion,
	})
	itemKey := fmt.Sprintf("%s:%s:v%d:%s:v%d", uuidToString(analysisID), marketPack.ID, marketPack.PublishedVersion, copyLibrary.ID, copyLibrary.PublishedVersion)
	tasks, err := h.TaskService.EnqueueDirectTaskFanout(ctx, service.DirectTaskFanout{
		Agent: agent, RequestingUserID: requestingUserID,
		Attribution:         attribution.DirectHumanRun(requestingUserID, attribution.EvidenceKind(creativePreAdaptationEvidenceKind), analysisID),
		TriggerEvidenceKind: creativePreAdaptationEvidenceKind, TriggerEvidenceRefID: analysisID,
		Items: []service.DirectTaskFanoutItem{{ItemKey: itemKey, Context: contextJSON}},
	})
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	if len(tasks) != 1 {
		return db.AgentTaskQueue{}, errors.New("pre-adaptation task was not queued")
	}
	return tasks[0], nil
}

func (h *Handler) defaultCreativePreAdaptationResources(ctx context.Context, workspaceID pgtype.UUID) (creativeResourceResponse, creativeResourceResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT r.id::text, r.workspace_id::text, r.kind, rr.name, rr.description, r.status,
       rr.version, rr.version, rr.config::text, r.created_by::text, r.created_at::text, rr.created_at::text
FROM creative_resource r
JOIN creative_resource_revision rr ON rr.resource_id = r.id AND rr.version = r.published_version
WHERE r.workspace_id = $1 AND r.kind = 'market_pack' AND r.status <> 'archived'
  AND r.published_version IS NOT NULL
  AND COALESCE(rr.config->>'pre_adaptation_default', 'false') = 'true'
ORDER BY rr.created_at DESC, r.id LIMIT 2
`, workspaceID)
	if err != nil {
		return creativeResourceResponse{}, creativeResourceResponse{}, err
	}
	defer rows.Close()
	packs := []creativeResourceResponse{}
	for rows.Next() {
		pack, scanErr := scanCreativeResource(rows)
		if scanErr != nil {
			return creativeResourceResponse{}, creativeResourceResponse{}, scanErr
		}
		packs = append(packs, pack)
	}
	if err := rows.Err(); err != nil || len(packs) != 1 {
		return creativeResourceResponse{}, creativeResourceResponse{}, errors.New("exactly one market pack must be marked pre_adaptation_default")
	}
	var config map[string]any
	if json.Unmarshal(packs[0].Config, &config) != nil {
		return creativeResourceResponse{}, creativeResourceResponse{}, errors.New("default market pack config is invalid")
	}
	libraryID, err := parseUUIDString(strings.TrimSpace(fmt.Sprint(config["copy_library_id"])))
	if err != nil {
		return creativeResourceResponse{}, creativeResourceResponse{}, errors.New("default market pack must bind a published copy library")
	}
	library, err := h.loadPublishedCreativeResource(ctx, workspaceID, libraryID, "copy_library")
	if err != nil {
		return creativeResourceResponse{}, creativeResourceResponse{}, errors.New("default market pack copy library must have a published version")
	}
	return packs[0], library, nil
}

func (h *Handler) GetCreativePreAdaptationContext(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	analysisID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "source_analysis_id")
	if !ok {
		return
	}
	marketPackID, ok := parseUUIDOrBadRequest(w, r.URL.Query().Get("market_pack_id"), "market_pack_id")
	if !ok {
		return
	}
	copyLibraryID, ok := parseUUIDOrBadRequest(w, r.URL.Query().Get("copy_library_id"), "copy_library_id")
	if !ok {
		return
	}
	marketVersion, err := parsePositiveInt(r.URL.Query().Get("market_pack_version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "market_pack_version must be positive")
		return
	}
	libraryVersion, err := parsePositiveInt(r.URL.Query().Get("copy_library_version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "copy_library_version must be positive")
		return
	}
	analysis, err := h.loadCreativeSourceAnalysisByID(r.Context(), workspaceID, analysisID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "source analysis not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source analysis")
		return
	}
	marketPack, err := h.loadCreativeResourceVersion(r.Context(), workspaceID, marketPackID, "market_pack", marketVersion)
	if err != nil {
		writeError(w, http.StatusConflict, "frozen market pack is unavailable")
		return
	}
	copyLibrary, err := h.loadCreativeResourceVersion(r.Context(), workspaceID, copyLibraryID, "copy_library", libraryVersion)
	if err != nil {
		writeError(w, http.StatusConflict, "frozen copy library is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source_analysis": analysis, "market_pack": marketPack, "copy_library": copyLibrary})
}

// RetryCreativePreAdaptation queues the missing market-specific work without
// re-running crawl or reference analysis. This covers analyses that completed
// before a market pack was selected or before its default was configured.
func (h *Handler) RetryCreativePreAdaptation(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	analysisID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "source_analysis_id")
	if !ok {
		return
	}
	var input creativePreAdaptationRetryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid pre-adaptation retry")
		return
	}
	marketPackID, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(input.MarketPackID), "market_pack_id")
	if !ok {
		return
	}
	analysis, err := h.loadCreativeSourceAnalysisByID(r.Context(), workspaceID, analysisID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "source analysis not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source analysis")
		return
	}
	if analysis.Status != "completed" {
		writeError(w, http.StatusConflict, "reference analysis must complete before pre-adaptation")
		return
	}
	if !creativePreAdaptationSourceHasVisualRegions(analysis.Result) {
		writeError(w, http.StatusConflict, "source analysis must be updated with visual regions before preparing copy")
		return
	}
	marketPack, err := h.loadPublishedCreativeResource(r.Context(), workspaceID, marketPackID, "market_pack")
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "selected market pack is unavailable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load selected market pack")
		return
	}
	var marketConfig map[string]any
	if json.Unmarshal(marketPack.Config, &marketConfig) != nil {
		writeError(w, http.StatusConflict, "selected market pack is invalid")
		return
	}
	copyLibraryID, err := parseUUIDString(strings.TrimSpace(fmt.Sprint(marketConfig["copy_library_id"])))
	if err != nil {
		writeError(w, http.StatusConflict, "selected market pack must bind a published copy library")
		return
	}
	copyLibrary, err := h.loadPublishedCreativeResource(r.Context(), workspaceID, copyLibraryID, "copy_library")
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "selected market pack copy library is unavailable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load selected copy library")
		return
	}
	if creativePreAdaptationManualRequiredForResources(analysis.Result, marketPack, copyLibrary) {
		writeError(w, http.StatusConflict, "pre-adaptation requires manual confirmation")
		return
	}
	candidateID, err := parseUUIDString(analysis.CandidateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "source analysis has an invalid candidate")
		return
	}
	agent, err := h.resolveReferenceAnalysisAgent(r.Context(), workspaceID, pgtype.UUID{})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "no enabled reference analysis agent is configured")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve pre-adaptation agent")
		return
	}
	task, err := h.enqueueCreativePreAdaptationTask(r.Context(), agent, userID, analysisID, candidateID, marketPack, copyLibrary)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue pre-adaptation: "+err.Error())
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "pre_adaptation_queued", "source_analysis_id": uuidToString(analysisID), "task_id": uuidToString(task.ID)})
	writeJSON(w, http.StatusOK, map[string]string{"task_id": uuidToString(task.ID), "status": task.Status})
}

func (h *Handler) PutCreativePreAdaptation(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	analysisID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "source_analysis_id")
	if !ok {
		return
	}
	var input creativePreAdaptationInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid pre-adaptation")
		return
	}
	input, err := normalizeCreativePreAdaptation(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Status == "completed" {
		var completed creativePreAdaptationCompletedResult
		if err := json.Unmarshal(input.Result, &completed); err != nil {
			writeError(w, http.StatusBadRequest, "completed pre-adaptation result is invalid: "+err.Error())
			return
		}
		copyLibraryID, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(completed.CopyLibraryID), "copy_library_id")
		if !ok || completed.CopyLibraryVersion < 1 {
			if ok {
				writeError(w, http.StatusBadRequest, "copy_library_version must be positive")
			}
			return
		}
		copyLibrary, loadErr := h.loadCreativeResourceVersion(r.Context(), workspaceID, copyLibraryID, "copy_library", completed.CopyLibraryVersion)
		if errors.Is(loadErr, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "frozen copy library is unavailable")
			return
		}
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load frozen copy library")
			return
		}
		marketPackID, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(completed.MarketPackID), "market_pack_id")
		if !ok || completed.MarketPackVersion < 1 {
			if ok {
				writeError(w, http.StatusBadRequest, "market_pack_version must be positive")
			}
			return
		}
		marketPack, loadErr := h.loadCreativeResourceVersion(r.Context(), workspaceID, marketPackID, "market_pack", completed.MarketPackVersion)
		if errors.Is(loadErr, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "frozen market pack is unavailable")
			return
		}
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load frozen market pack")
			return
		}
		analysis, loadErr := h.loadCreativeSourceAnalysisByID(r.Context(), workspaceID, analysisID)
		if errors.Is(loadErr, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "source analysis not found")
			return
		}
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load source analysis")
			return
		}
		if err := normalizeCreativePreAdaptationDerivedFields(&completed, analysis.Result, marketPack.Config, copyLibrary.Config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		input.Result, _ = json.Marshal(completed)
		if err := validateCompletedCreativePreAdaptation(analysis.Result, input.Result); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateCreativePreAdaptationCalculationRules(analysis.Result, input.Result, marketPack.Config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateCreativePreAdaptationCopyBindings(analysis.Result, input.Result, copyLibrary.Config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	adaptation := map[string]any{"status": input.Status, "summary": input.Summary, "result": json.RawMessage(input.Result), "error_code": input.ErrorCode, "error_message": input.ErrorMessage}
	encoded, _ := json.Marshal(adaptation)
	result, err := h.DB.Exec(r.Context(), `
UPDATE creative_source_analysis
SET result = jsonb_set(result, '{adaptation}', $3::jsonb, true)
WHERE id = $1 AND workspace_id = $2
`, analysisID, workspaceID, encoded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save pre-adaptation")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "source analysis not found")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "pre_adaptation", "source_analysis_id": uuidToString(analysisID)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// The model chooses an approved plan key, amount, and tenor. Once those match
// one row exactly, the server owns display formatting so a harmless separator
// typo cannot turn an otherwise valid plan choice into a failed creative task.
func normalizeCreativePreAdaptationPlanDisplay(adaptation *creativePreAdaptationCompletedResult, copyLibraryRaw json.RawMessage) error {
	var library composableCopyLibraryConfig
	if err := json.Unmarshal(copyLibraryRaw, &library); err != nil {
		return errors.New("frozen copy library config is invalid")
	}
	approvedByKey := map[string]composableCopyLibraryRepaymentPlanEntry{}
	for _, entry := range library.RepaymentPlan.Entries {
		if entry.Status == "approved" {
			approvedByKey[strings.TrimSpace(entry.Key)] = entry
		}
	}
	for index := range adaptation.RepaymentPlanSelections {
		selection := &adaptation.RepaymentPlanSelections[index]
		entry, exists := approvedByKey[strings.TrimSpace(selection.PlanKey)]
		if !exists || selection.Principal != entry.Principal || selection.TenorMonths != entry.TenorMonths {
			return fmt.Errorf("repayment plan selection %q does not match the approved amount and tenor", strings.TrimSpace(selection.ID))
		}
		selection.Values = creativePreAdaptationRepaymentPlanValues{
			Principal: creativeFormatRupiah(entry.Principal), Tenor: fmt.Sprintf("%d Bulan", entry.TenorMonths), TotalInterest: creativeFormatRupiah(entry.TotalInterest),
			TotalRepayment: creativeFormatRupiah(entry.TotalRepayment), MonthlyInstallment: creativeFormatRupiah(entry.MonthlyInstallment),
		}
	}
	selectionIDByPlanKey := map[string]string{}
	for _, selection := range adaptation.RepaymentPlanSelections {
		selectionIDByPlanKey[strings.TrimSpace(selection.PlanKey)] = strings.TrimSpace(selection.ID)
	}
	for layoutIndex := range adaptation.NumericLayouts {
		layout := &adaptation.NumericLayouts[layoutIndex]
		for scenarioIndex, reference := range layout.ScenarioIDs {
			if selectionIDByPlanKey[reference] != "" {
				layout.ScenarioIDs[scenarioIndex] = selectionIDByPlanKey[reference]
			}
		}
	}
	return nil
}

func normalizeCreativePreAdaptationDerivedFields(adaptation *creativePreAdaptationCompletedResult, sourceRaw, marketPackRaw, copyLibraryRaw json.RawMessage) error {
	sourceBlocks, regionsByBlockID, hasVisualRegions, err := creativePreAdaptationSourceIndex(sourceRaw)
	if err != nil {
		return err
	}
	copyLibrary, fragmentsByKey, err := creativePreAdaptationApprovedFragments(copyLibraryRaw)
	if err != nil {
		return err
	}
	calculationRules, err := creativePreAdaptationCalculationRules(marketPackRaw)
	if err != nil {
		return err
	}

	normalizeCreativePreAdaptationCopyChoices(adaptation, sourceBlocks, regionsByBlockID, hasVisualRegions, fragmentsByKey, calculationRules)
	normalizeCreativePreAdaptationPlanChoices(adaptation, copyLibrary.RepaymentPlan, sourceBlocks, regionsByBlockID, hasVisualRegions)
	ensureCreativePreAdaptationPromptDefaults(adaptation)
	return nil
}

func creativePreAdaptationSourceIndex(sourceRaw json.RawMessage) (map[string]creativePreAdaptationSourceTextBlock, map[string]creativePreAdaptationSourceVisualRegion, bool, error) {
	var source struct {
		TextBlocks    []creativePreAdaptationSourceTextBlock    `json:"text_blocks"`
		VisualRegions []creativePreAdaptationSourceVisualRegion `json:"visual_regions"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return nil, nil, false, errors.New("source analysis result is invalid")
	}
	if len(source.TextBlocks) == 0 {
		return nil, nil, false, errors.New("completed pre-adaptation requires source analysis text_blocks")
	}
	blocks := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		block.ID = strings.TrimSpace(block.ID)
		block.Location = strings.TrimSpace(block.Location)
		block.Role = strings.TrimSpace(block.Role)
		block.Purpose = strings.TrimSpace(block.Purpose)
		block.SemanticKind = strings.TrimSpace(block.SemanticKind)
		if block.ID == "" || block.Location == "" || block.Role == "" || !validCreativeTextRole(block.Role) {
			return nil, nil, false, errors.New("source analysis contains an invalid text block")
		}
		if _, exists := blocks[block.ID]; exists {
			return nil, nil, false, errors.New("source analysis contains duplicate text blocks")
		}
		blocks[block.ID] = block
	}
	regionsByBlockID, hasVisualRegions, err := validatedCreativePreAdaptationVisualRegions(source.VisualRegions, blocks)
	if err != nil {
		return nil, nil, false, err
	}
	return blocks, regionsByBlockID, hasVisualRegions, nil
}

type creativePreAdaptationApprovedFragment struct {
	text          string
	semanticGroup string
}

func creativePreAdaptationApprovedFragments(copyLibraryRaw json.RawMessage) (composableCopyLibraryConfig, map[string]creativePreAdaptationApprovedFragment, error) {
	var library composableCopyLibraryConfig
	if err := json.Unmarshal(copyLibraryRaw, &library); err != nil {
		return library, nil, errors.New("frozen copy library config is invalid")
	}
	fragmentsByKey := map[string]creativePreAdaptationApprovedFragment{}
	for _, fragment := range library.Fragments {
		if fragment.Status != "approved" || strings.TrimSpace(fragment.ID) == "" || strings.TrimSpace(fragment.Key) == "" {
			continue
		}
		fragmentsByKey[strings.TrimSpace(fragment.Key)] = creativePreAdaptationApprovedFragment{
			text: strings.TrimSpace(fragment.Text), semanticGroup: strings.TrimSpace(fragment.SemanticGroup),
		}
	}
	return library, fragmentsByKey, nil
}

func creativePreAdaptationCalculationRules(marketPackRaw json.RawMessage) (map[string]creativePreAdaptationCalculationRule, error) {
	var marketPack struct {
		CalculationRules []creativePreAdaptationCalculationRule `json:"calculation_rules"`
	}
	if err := json.Unmarshal(marketPackRaw, &marketPack); err != nil {
		return nil, errors.New("frozen market pack config is invalid")
	}
	rulesByKey := make(map[string]creativePreAdaptationCalculationRule, len(marketPack.CalculationRules))
	for _, rule := range marketPack.CalculationRules {
		rule.Key = strings.TrimSpace(rule.Key)
		if rule.Key != "" {
			rulesByKey[rule.Key] = rule
		}
	}
	return rulesByKey, nil
}

func normalizeCreativePreAdaptationCopyChoices(
	adaptation *creativePreAdaptationCompletedResult,
	blocks map[string]creativePreAdaptationSourceTextBlock,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
	hasVisualRegions bool,
	fragmentsByKey map[string]creativePreAdaptationApprovedFragment,
	calculationRules map[string]creativePreAdaptationCalculationRule,
) {
	replacements := make([]creativePreAdaptationTextReplacement, 0, len(adaptation.TextReplacements))
	seenBlocks := map[string]bool{}
	usedSourceKeys := map[string]bool{}
	for _, replacement := range adaptation.TextReplacements {
		blockID := strings.TrimSpace(replacement.BlockID)
		block, exists := blocks[blockID]
		if !exists || seenBlocks[blockID] {
			continue
		}
		seenBlocks[blockID] = true
		replacement = anchoredCreativePreAdaptationReplacement(replacement, block, regionsByBlockID, hasVisualRegions)
		replacement.Status = normalizedCreativePreAdaptationReplacementStatus(replacement)
		switch replacement.Status {
		case "ready":
			normalizeReadyCreativePreAdaptationReplacement(&replacement, block, fragmentsByKey, usedSourceKeys)
		case "calculated":
			normalizeCalculatedCreativePreAdaptationReplacement(&replacement, block, calculationRules)
		case "recommended":
			normalizeRecommendedCreativePreAdaptationReplacement(&replacement)
		default:
			markCreativePreAdaptationReplacementMissing(&replacement, "没有可自动采用的内容，可由用户选择文案或手动填写。")
		}
		replacements = append(replacements, replacement)
	}
	adaptation.TextReplacements = replacements
}

func anchoredCreativePreAdaptationReplacement(
	replacement creativePreAdaptationTextReplacement,
	block creativePreAdaptationSourceTextBlock,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
	hasVisualRegions bool,
) creativePreAdaptationTextReplacement {
	replacement.BlockID = strings.TrimSpace(block.ID)
	replacement.Location = strings.TrimSpace(block.Location)
	replacement.Role = strings.TrimSpace(block.Role)
	replacement.SourceText = block.SourceText
	replacement.Note = strings.TrimSpace(replacement.Note)
	replacement.ReplacementText = strings.TrimSpace(replacement.ReplacementText)
	replacement.RecommendationBasis = trimCreativePreAdaptationStrings(replacement.RecommendationBasis)
	replacement.SourceKeys = trimCreativePreAdaptationStrings(replacement.SourceKeys)
	replacement.VisualRegionID = strings.TrimSpace(replacement.VisualRegionID)
	if hasVisualRegions {
		replacement.VisualRegionID = strings.TrimSpace(regionsByBlockID[replacement.BlockID].ID)
	}
	return replacement
}

func normalizeReadyCreativePreAdaptationReplacement(
	replacement *creativePreAdaptationTextReplacement,
	block creativePreAdaptationSourceTextBlock,
	fragmentsByKey map[string]creativePreAdaptationApprovedFragment,
	usedSourceKeys map[string]bool,
) {
	if len(replacement.SourceKeys) != 1 {
		markCreativePreAdaptationReplacementMissing(replacement, "没有绑定唯一已审核文案，可由用户选择文案或手动填写。")
		return
	}
	sourceKey := strings.TrimSpace(replacement.SourceKeys[0])
	fragment, exists := fragmentsByKey[sourceKey]
	if !exists {
		markCreativePreAdaptationReplacementMissing(replacement, "文案库中没有找到可用来源，可由用户选择文案或手动填写。")
		return
	}
	if usedSourceKeys[sourceKey] {
		markCreativePreAdaptationReplacementMissing(replacement, "同一条已审核文案已用于其他区域，可由用户重新选择。")
		return
	}
	if !creativeFragmentSupportsSourceSemanticKind(fragment.semanticGroup, creativeSourceTextBlockSemanticKind(block)) {
		markCreativePreAdaptationReplacementMissing(replacement, "已审核文案与当前文字语义不匹配，可由用户重新选择。")
		return
	}
	usedSourceKeys[sourceKey] = true
	replacement.Status = "ready"
	replacement.SourceKeys = []string{sourceKey}
	replacement.ReplacementText = fragment.text
	replacement.RecommendationBasis = nil
	replacement.Calculation = nil
	if replacement.Note == "" {
		replacement.Note = "来自冻结文案库。"
	}
}

func normalizeCalculatedCreativePreAdaptationReplacement(replacement *creativePreAdaptationTextReplacement, block creativePreAdaptationSourceTextBlock, calculationRules map[string]creativePreAdaptationCalculationRule) {
	if replacement.Calculation == nil {
		markCreativePreAdaptationReplacementMissing(replacement, "没有可验证的计算结果，可由用户选择文案或手动填写。")
		return
	}
	calculation := replacement.Calculation
	calculation.RuleKey = strings.TrimSpace(calculation.RuleKey)
	calculation.Formula = strings.TrimSpace(calculation.Formula)
	calculation.Result = strings.TrimSpace(calculation.Result)
	calculation.Inputs = trimCreativePreAdaptationStrings(calculation.Inputs)
	rule, exists := calculationRules[calculation.RuleKey]
	semanticKind := creativeSourceTextBlockSemanticKind(block)
	expectedFormula := strings.TrimSpace(rule.Formulas[semanticKind])
	if !exists || expectedFormula == "" || calculation.Formula != expectedFormula || calculation.Result == "" || len(calculation.Inputs) == 0 {
		markCreativePreAdaptationReplacementMissing(replacement, "没有可验证的计算结果，可由用户选择文案或手动填写。")
		return
	}
	replacement.Status = "calculated"
	replacement.SourceKeys = nil
	replacement.ReplacementText = calculation.Result
	replacement.RecommendationBasis = nil
}

func normalizeRecommendedCreativePreAdaptationReplacement(replacement *creativePreAdaptationTextReplacement) {
	if strings.TrimSpace(replacement.ReplacementText) == "" || len(replacement.RecommendationBasis) == 0 {
		markCreativePreAdaptationReplacementMissing(replacement, "没有安全推荐，可由用户选择文案或手动填写。")
		return
	}
	replacement.Status = "recommended"
	replacement.SourceKeys = nil
	replacement.Calculation = nil
}

func markCreativePreAdaptationReplacementMissing(replacement *creativePreAdaptationTextReplacement, note string) {
	replacement.Status = "missing"
	replacement.ReplacementText = ""
	replacement.SourceKeys = nil
	replacement.RecommendationBasis = nil
	replacement.Calculation = nil
	replacement.Note = note
}

func normalizeCreativePreAdaptationNumericLayoutKinds(adaptation *creativePreAdaptationCompletedResult) {
	for index := range adaptation.NumericLayouts {
		layout := &adaptation.NumericLayouts[index]
		layout.LayoutKind = inferredCreativePreAdaptationNumericLayoutKind(*layout)
	}
}

func normalizeCreativePreAdaptationPlanChoices(
	adaptation *creativePreAdaptationCompletedResult,
	plan composableCopyLibraryRepaymentPlan,
	blocks map[string]creativePreAdaptationSourceTextBlock,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
	hasVisualRegions bool,
) {
	selections, selectionIDByPlanKey, selectionsByID := normalizedCreativePreAdaptationPlanSelections(plan, adaptation.RepaymentPlanSelections)
	numericBlocks := creativePreAdaptationNumericBlocks(blocks, regionsByBlockID)
	layouts := make([]creativePreAdaptationNumericLayout, 0, len(adaptation.NumericLayouts))
	coveredNumericBlocks := map[string]bool{}
	usedSelectionIDs := map[string]bool{}
	for _, layout := range adaptation.NumericLayouts {
		normalized, ok := normalizedCreativePreAdaptationNumericLayout(layout, selectionsByID, selectionIDByPlanKey, numericBlocks, regionsByBlockID, hasVisualRegions, coveredNumericBlocks)
		if !ok {
			continue
		}
		for _, scenarioID := range normalized.ScenarioIDs {
			usedSelectionIDs[scenarioID] = true
		}
		layouts = append(layouts, normalized)
	}
	filteredSelections := make([]creativePreAdaptationRepaymentPlanSelection, 0, len(selections))
	for _, selection := range selections {
		if usedSelectionIDs[selection.ID] {
			filteredSelections = append(filteredSelections, selection)
		}
	}
	adaptation.RepaymentPlanSelections = filteredSelections
	adaptation.NumericLayouts = layouts
	adaptation.TextReplacements = creativePreAdaptationCopyTextReplacements(adaptation.TextReplacements, regionsByBlockID, hasVisualRegions, coveredNumericBlocks)
	ensureCreativePreAdaptationMissingReplacements(adaptation, blocks, regionsByBlockID, hasVisualRegions, coveredNumericBlocks)
}

func normalizedCreativePreAdaptationPlanSelections(
	plan composableCopyLibraryRepaymentPlan,
	selections []creativePreAdaptationRepaymentPlanSelection,
) ([]creativePreAdaptationRepaymentPlanSelection, map[string]string, map[string]creativePreAdaptationRepaymentPlanSelection) {
	approvedByKey := map[string]composableCopyLibraryRepaymentPlanEntry{}
	for _, entry := range plan.Entries {
		if entry.Status == "approved" {
			approvedByKey[strings.TrimSpace(entry.Key)] = entry
		}
	}
	normalized := make([]creativePreAdaptationRepaymentPlanSelection, 0, len(selections))
	selectionIDByPlanKey := map[string]string{}
	selectionsByID := map[string]creativePreAdaptationRepaymentPlanSelection{}
	for _, selection := range selections {
		planKey := strings.TrimSpace(selection.PlanKey)
		entry, exists := approvedByKey[planKey]
		if !exists {
			continue
		}
		selection.ID = strings.TrimSpace(selection.ID)
		if selection.ID == "" {
			selection.ID = planKey
		}
		if selectionsByID[selection.ID].ID != "" {
			continue
		}
		selection.PlanKey = planKey
		selection.Principal = entry.Principal
		selection.TenorMonths = entry.TenorMonths
		selection.Values = creativePreAdaptationRepaymentPlanValues{
			Principal:          creativeFormatRupiah(entry.Principal),
			Tenor:              fmt.Sprintf("%d Bulan", entry.TenorMonths),
			TotalInterest:      creativeFormatRupiah(entry.TotalInterest),
			TotalRepayment:     creativeFormatRupiah(entry.TotalRepayment),
			MonthlyInstallment: creativeFormatRupiah(entry.MonthlyInstallment),
		}
		normalized = append(normalized, selection)
		selectionIDByPlanKey[planKey] = selection.ID
		selectionsByID[selection.ID] = selection
	}
	return normalized, selectionIDByPlanKey, selectionsByID
}

func creativePreAdaptationNumericBlocks(
	blocks map[string]creativePreAdaptationSourceTextBlock,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
) map[string]creativePreAdaptationSourceTextBlock {
	numericBlocks := map[string]creativePreAdaptationSourceTextBlock{}
	for id, block := range blocks {
		region, hasRegion := regionsByBlockID[id]
		if block.Role == "plan_field" || (hasRegion && region.Kind == "numeric" && block.Role == "supporting") {
			numericBlocks[id] = block
		}
	}
	return numericBlocks
}

func normalizedCreativePreAdaptationNumericLayout(
	layout creativePreAdaptationNumericLayout,
	selectionsByID map[string]creativePreAdaptationRepaymentPlanSelection,
	selectionIDByPlanKey map[string]string,
	numericBlocks map[string]creativePreAdaptationSourceTextBlock,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
	hasVisualRegions bool,
	coveredNumericBlocks map[string]bool,
) (creativePreAdaptationNumericLayout, bool) {
	layout.ID = strings.TrimSpace(layout.ID)
	if layout.ID == "" {
		return layout, false
	}
	layout.Location = strings.TrimSpace(layout.Location)
	layout.LayoutKind = inferredCreativePreAdaptationNumericLayoutKind(layout)
	layout.SourceBlockIDs = trimCreativePreAdaptationStrings(layout.SourceBlockIDs)
	layout.ScenarioIDs = normalizedCreativePreAdaptationScenarioIDs(layout.ScenarioIDs, selectionsByID, selectionIDByPlanKey)
	layout.TargetColumns = normalizedCreativePreAdaptationTargetColumns(layout.TargetColumns)
	if len(layout.SourceBlockIDs) == 0 || len(layout.ScenarioIDs) == 0 || len(layout.TargetColumns) == 0 {
		return layout, false
	}
	layoutRegionID := ""
	for _, blockID := range layout.SourceBlockIDs {
		if _, exists := numericBlocks[blockID]; !exists || coveredNumericBlocks[blockID] {
			return layout, false
		}
		if hasVisualRegions {
			region, exists := regionsByBlockID[blockID]
			if !exists || region.Kind != "numeric" || (layoutRegionID != "" && layoutRegionID != region.ID) {
				return layout, false
			}
			layoutRegionID = region.ID
		}
	}
	if hasVisualRegions {
		layout.VisualRegionID = layoutRegionID
		if layout.Location == "" {
			layout.Location = strings.TrimSpace(regionsByBlockID[layout.SourceBlockIDs[0]].Location)
		}
	}
	if layout.Location == "" {
		layout.Location = "数值区域"
	}
	layout.RenderInstruction = creativePreAdaptationNumericRenderInstruction(layout, selectionsByID)
	for _, blockID := range layout.SourceBlockIDs {
		coveredNumericBlocks[blockID] = true
	}
	return layout, true
}

func normalizedCreativePreAdaptationScenarioIDs(
	raw []string,
	selectionsByID map[string]creativePreAdaptationRepaymentPlanSelection,
	selectionIDByPlanKey map[string]string,
) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if selectionIDByPlanKey[value] != "" {
			value = selectionIDByPlanKey[value]
		}
		if selectionsByID[value].ID == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizedCreativePreAdaptationTargetColumns(raw []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(raw))
	for _, column := range raw {
		column = strings.TrimSpace(column)
		if !validCreativeNumericColumn(column) || seen[column] {
			continue
		}
		seen[column] = true
		normalized = append(normalized, column)
	}
	return normalized
}

func creativePreAdaptationNumericRenderInstruction(layout creativePreAdaptationNumericLayout, selectionsByID map[string]creativePreAdaptationRepaymentPlanSelection) string {
	rows := make([]string, 0, len(layout.ScenarioIDs))
	for _, scenarioID := range layout.ScenarioIDs {
		selection := selectionsByID[scenarioID]
		values := make([]string, 0, len(layout.TargetColumns))
		for _, column := range layout.TargetColumns {
			if value := creativePreAdaptationPlanColumnValue(selection, column); value != "" {
				values = append(values, creativePreAdaptationPlanColumnLabel(column)+" "+value)
			}
		}
		if len(values) > 0 {
			rows = append(rows, strings.Join(values, " / "))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return "按已审核还款计划展示：" + strings.Join(rows, "；") + "。"
}

func creativePreAdaptationPlanColumnLabel(column string) string {
	switch column {
	case "principal":
		return "本金"
	case "tenor":
		return "期限"
	case "monthly_installment":
		return "月还"
	case "total_interest":
		return "总利息"
	case "total_repayment":
		return "总还款"
	default:
		return column
	}
}

func creativePreAdaptationPlanColumnValue(selection creativePreAdaptationRepaymentPlanSelection, column string) string {
	switch column {
	case "principal":
		return selection.Values.Principal
	case "tenor":
		return selection.Values.Tenor
	case "monthly_installment":
		return selection.Values.MonthlyInstallment
	case "total_interest":
		return selection.Values.TotalInterest
	case "total_repayment":
		return selection.Values.TotalRepayment
	default:
		return ""
	}
}

func creativePreAdaptationCopyTextReplacements(
	replacements []creativePreAdaptationTextReplacement,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
	hasVisualRegions bool,
	coveredNumericBlocks map[string]bool,
) []creativePreAdaptationTextReplacement {
	filtered := make([]creativePreAdaptationTextReplacement, 0, len(replacements))
	for _, replacement := range replacements {
		blockID := strings.TrimSpace(replacement.BlockID)
		if coveredNumericBlocks[blockID] {
			continue
		}
		if hasVisualRegions && regionsByBlockID[blockID].Kind == "numeric" {
			continue
		}
		filtered = append(filtered, replacement)
	}
	return filtered
}

func ensureCreativePreAdaptationMissingReplacements(
	adaptation *creativePreAdaptationCompletedResult,
	blocks map[string]creativePreAdaptationSourceTextBlock,
	regionsByBlockID map[string]creativePreAdaptationSourceVisualRegion,
	hasVisualRegions bool,
	coveredNumericBlocks map[string]bool,
) {
	replacementsByBlockID := map[string]bool{}
	for _, replacement := range adaptation.TextReplacements {
		replacementsByBlockID[strings.TrimSpace(replacement.BlockID)] = true
	}
	blockIDs := make([]string, 0, len(blocks))
	for blockID := range blocks {
		blockIDs = append(blockIDs, blockID)
	}
	sort.Strings(blockIDs)
	for _, blockID := range blockIDs {
		if replacementsByBlockID[blockID] || coveredNumericBlocks[blockID] {
			continue
		}
		replacement := anchoredCreativePreAdaptationReplacement(creativePreAdaptationTextReplacement{
			BlockID: blockID,
			Status:  "missing",
		}, blocks[blockID], regionsByBlockID, hasVisualRegions)
		markCreativePreAdaptationReplacementMissing(&replacement, "没有可自动采用的内容，可由用户选择文案或手动填写。")
		adaptation.TextReplacements = append(adaptation.TextReplacements, replacement)
	}
}

func ensureCreativePreAdaptationPromptDefaults(adaptation *creativePreAdaptationCompletedResult) {
	highlights := trimCreativePreAdaptationStrings(adaptation.AnalysisHighlights)
	for len(highlights) < 3 {
		highlights = append(highlights, "待用户确认画面文字与数值后进入生产。")
	}
	if len(highlights) > 5 {
		highlights = highlights[:5]
	}
	adaptation.AnalysisHighlights = highlights
	basePrompt := strings.TrimSpace(adaptation.ProductionPrompt)
	if basePrompt == "" {
		basePrompt = "保留原图主体视觉和版式层级；按本页逐块确认结果替换、留空或重排文字与数值。"
	}
	lines := []string{basePrompt}
	for _, replacement := range adaptation.TextReplacements {
		text := strings.TrimSpace(replacement.ReplacementText)
		if text != "" && !strings.Contains(basePrompt, text) {
			lines = append(lines, fmt.Sprintf("%s：%s", replacement.Location, text))
		}
	}
	for _, layout := range adaptation.NumericLayouts {
		instruction := strings.TrimSpace(layout.RenderInstruction)
		if instruction != "" && !strings.Contains(strings.Join(lines, "\n"), instruction) {
			lines = append(lines, fmt.Sprintf("%s：%s", layout.Location, instruction))
		}
	}
	adaptation.ProductionPrompt = strings.Join(lines, "\n")
}

func normalizeCreativePreAdaptationSourceAnchors(adaptation *creativePreAdaptationCompletedResult, sourceRaw json.RawMessage) error {
	var source struct {
		TextBlocks []creativePreAdaptationSourceTextBlock `json:"text_blocks"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return errors.New("source analysis result is invalid")
	}
	blocks := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		block.ID = strings.TrimSpace(block.ID)
		if block.ID != "" {
			blocks[block.ID] = block
		}
	}
	for index := range adaptation.TextReplacements {
		replacement := &adaptation.TextReplacements[index]
		blockID := strings.TrimSpace(replacement.BlockID)
		block, exists := blocks[blockID]
		if !exists {
			replacement.BlockID = blockID
			continue
		}
		replacement.BlockID = blockID
		replacement.Location = strings.TrimSpace(block.Location)
		replacement.Role = strings.TrimSpace(block.Role)
		replacement.SourceText = block.SourceText
	}
	return nil
}

func inferredCreativePreAdaptationNumericLayoutKind(layout creativePreAdaptationNumericLayout) string {
	kind := normalizedCreativeNumericLayoutKind(layout.LayoutKind)
	if validCreativeCanonicalNumericLayoutKind(kind) {
		return kind
	}
	if allCreativeNumericColumns(layout.TargetColumns, "tenor") && (nonEmptyStringCount(layout.ScenarioIDs) > 1 || nonEmptyStringCount(layout.SourceBlockIDs) > 1) {
		return "option_buttons"
	}
	switch kind {
	case "tenor", "term", "duration", "tenor_button", "tenor_buttons", "period_option", "period_options":
		return "option_buttons"
	case "repayment_table", "repayment_summary", "loan_summary", "installment_table", "summary_table":
		return "table"
	case "principal", "amount", "amount_box", "loan_amount", "principal_amount", "credit_limit", "limit":
		return "single_value"
	}
	columnCount := nonEmptyStringCount(layout.TargetColumns)
	scenarioCount := nonEmptyStringCount(layout.ScenarioIDs)
	sourceBlockCount := nonEmptyStringCount(layout.SourceBlockIDs)
	if columnCount == 1 && scenarioCount <= 1 {
		return "single_value"
	}
	if columnCount == 1 {
		return "card_grid"
	}
	if scenarioCount == 1 && columnCount <= 2 && sourceBlockCount <= 2 {
		return "table_row"
	}
	return "table"
}

func normalizedCreativeNumericLayoutKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func validCreativeCanonicalNumericLayoutKind(value string) bool {
	switch value {
	case "table", "card_grid", "comparison", "single_card", "single_value", "option_buttons", "table_row":
		return true
	default:
		return false
	}
}

func allCreativeNumericColumns(columns []string, column string) bool {
	hasColumn := false
	for _, value := range columns {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.TrimSpace(value) != column {
			return false
		}
		hasColumn = true
	}
	return hasColumn
}

func nonEmptyStringCount(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func (h *Handler) loadCreativeSourceAnalysisByID(ctx context.Context, workspaceID, analysisID pgtype.UUID) (creativeSourceAnalysisResponse, error) {
	return scanCreativeSourceAnalysis(h.DB.QueryRow(ctx, `
SELECT id::text, workspace_id::text, candidate_id::text, analysis_version, status, summary, result::text,
  error_code, error_message, trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_at::text, COALESCE(completed_at::text, '')
FROM creative_source_analysis WHERE id = $1 AND workspace_id = $2
`, analysisID, workspaceID))
}

func (h *Handler) loadCreativeResourceVersion(ctx context.Context, workspaceID, resourceID pgtype.UUID, kind string, version int) (creativeResourceResponse, error) {
	return scanCreativeResource(h.DB.QueryRow(ctx, `
SELECT r.id::text, r.workspace_id::text, r.kind, rr.name, rr.description, r.status,
       rr.version, rr.version, rr.config::text, r.created_by::text, r.created_at::text, rr.created_at::text
FROM creative_resource r JOIN creative_resource_revision rr ON rr.resource_id = r.id AND rr.version = $4
WHERE r.id = $1 AND r.workspace_id = $2 AND r.kind = $3 AND r.status <> 'archived'
`, resourceID, workspaceID, kind, version))
}

func normalizeCreativePreAdaptation(input creativePreAdaptationInput) (creativePreAdaptationInput, error) {
	input.Status, input.Summary = strings.TrimSpace(input.Status), strings.TrimSpace(input.Summary)
	input.ErrorCode, input.ErrorMessage = strings.TrimSpace(input.ErrorCode), strings.TrimSpace(input.ErrorMessage)
	if input.Status != "completed" && input.Status != "unavailable" && input.Status != "failed" {
		return input, errors.New("invalid pre-adaptation status")
	}
	if len(input.Summary) > 4000 || len(input.ErrorMessage) > 4000 {
		return input, errors.New("invalid pre-adaptation")
	}
	var err error
	input.Result, err = normalizedOptionalJSONObject(input.Result)
	if err != nil {
		return input, errors.New("result must be an object")
	}
	if input.Status == "completed" && (input.Summary == "" || string(input.Result) == "{}") {
		return input, errors.New("completed pre-adaptation requires summary and result")
	}
	if input.Status != "completed" && input.ErrorMessage == "" {
		return input, errors.New("unavailable or failed pre-adaptation requires error_message")
	}
	return input, nil
}

func creativePreAdaptationManualRequiredForResources(sourceRaw json.RawMessage, marketPack, copyLibrary creativeResourceResponse) bool {
	var source struct {
		Adaptation struct {
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
			Result    struct {
				MarketPackID       string `json:"market_pack_id"`
				MarketPackVersion  int    `json:"market_pack_version"`
				CopyLibraryID      string `json:"copy_library_id"`
				CopyLibraryVersion int    `json:"copy_library_version"`
			} `json:"result"`
		} `json:"adaptation"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return false
	}
	return strings.TrimSpace(source.Adaptation.Status) == "unavailable" &&
		strings.TrimSpace(source.Adaptation.ErrorCode) == "manual_confirmation_required" &&
		strings.TrimSpace(source.Adaptation.Result.MarketPackID) == marketPack.ID &&
		source.Adaptation.Result.MarketPackVersion == marketPack.PublishedVersion &&
		strings.TrimSpace(source.Adaptation.Result.CopyLibraryID) == copyLibrary.ID &&
		source.Adaptation.Result.CopyLibraryVersion == copyLibrary.PublishedVersion
}

func validateCompletedCreativePreAdaptation(sourceRaw, adaptationRaw json.RawMessage) error {
	var source struct {
		TextBlocks    []creativePreAdaptationSourceTextBlock    `json:"text_blocks"`
		VisualRegions []creativePreAdaptationSourceVisualRegion `json:"visual_regions"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return errors.New("source analysis result is invalid")
	}
	if len(source.TextBlocks) == 0 {
		return errors.New("completed pre-adaptation requires source analysis text_blocks")
	}
	blocks := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		block.ID = strings.TrimSpace(block.ID)
		block.Location = strings.TrimSpace(block.Location)
		block.Role = strings.TrimSpace(block.Role)
		block.Purpose = strings.TrimSpace(block.Purpose)
		block.SemanticKind = strings.TrimSpace(block.SemanticKind)
		if block.ID == "" || block.Location == "" || block.Role == "" || !validCreativeTextRole(block.Role) {
			return errors.New("source analysis contains an invalid text block")
		}
		if _, exists := blocks[block.ID]; exists {
			return errors.New("source analysis contains duplicate text blocks")
		}
		blocks[block.ID] = block
	}
	regionsByBlockID, hasVisualRegions, err := validatedCreativePreAdaptationVisualRegions(source.VisualRegions, blocks)
	if err != nil {
		return err
	}

	var adaptation creativePreAdaptationCompletedResult
	if err := json.Unmarshal(adaptationRaw, &adaptation); err != nil {
		return errors.New("completed pre-adaptation result is invalid")
	}
	if len(adaptation.AnalysisHighlights) < 3 || len(adaptation.AnalysisHighlights) > 5 {
		return errors.New("completed pre-adaptation requires 3-5 analysis highlights")
	}
	for _, highlight := range adaptation.AnalysisHighlights {
		if strings.TrimSpace(highlight) == "" {
			return errors.New("completed pre-adaptation contains an empty analysis highlight")
		}
	}
	if strings.TrimSpace(adaptation.ProductionPrompt) == "" {
		return errors.New("completed pre-adaptation requires a filled production prompt")
	}
	if strings.TrimSpace(adaptation.MarketPackID) == "" || adaptation.MarketPackVersion < 1 {
		return errors.New("completed pre-adaptation requires frozen market pack identity")
	}

	// Every visible source block has one explicit destination. A plan_field is
	// not automatically numeric: source analysis uses that broad role for form
	// labels such as a name or occupation as well. A supporting label inside a
	// declared numeric visual region, however, is part of the same display
	// relationship as the value it names (for example, "Total pembayaran").
	// It must stay with that numeric layout instead of being split into copy.
	numericBlocks := map[string]creativePreAdaptationSourceTextBlock{}
	for id, block := range blocks {
		region, hasRegion := regionsByBlockID[id]
		if block.Role == "plan_field" || (hasRegion && region.Kind == "numeric" && block.Role == "supporting") {
			numericBlocks[id] = block
		}
	}
	replacements := make(map[string]creativePreAdaptationTextReplacement, len(adaptation.TextReplacements))
	for _, replacement := range adaptation.TextReplacements {
		replacement.BlockID = strings.TrimSpace(replacement.BlockID)
		replacement.VisualRegionID = strings.TrimSpace(replacement.VisualRegionID)
		replacement.Location = strings.TrimSpace(replacement.Location)
		replacement.Role = strings.TrimSpace(replacement.Role)
		replacement.ReplacementText = strings.TrimSpace(replacement.ReplacementText)
		replacement.Note = strings.TrimSpace(replacement.Note)
		replacement.RecommendationBasis = trimCreativePreAdaptationStrings(replacement.RecommendationBasis)
		block, exists := blocks[replacement.BlockID]
		if !exists || replacement.Location != block.Location || replacement.Role != block.Role || replacement.SourceText != block.SourceText {
			return errors.New("completed pre-adaptation text replacement does not match source analysis")
		}
		replacement.Status = normalizedCreativePreAdaptationReplacementStatus(replacement)
		if !validCreativePreAdaptationReplacementStatus(replacement.Status) {
			return errors.New("completed pre-adaptation text replacement has an invalid status")
		}
		if err := validateCreativePreAdaptationReplacementProposal(replacement); err != nil {
			return err
		}
		if _, duplicate := replacements[replacement.BlockID]; duplicate {
			return errors.New("completed pre-adaptation contains duplicate text replacements")
		}
		if hasVisualRegions {
			region, exists := regionsByBlockID[replacement.BlockID]
			if !exists || replacement.VisualRegionID != region.ID {
				return errors.New("completed pre-adaptation text replacement does not match its visual region")
			}
			if region.Kind != "copy" && replacement.Status != "missing" {
				return errors.New("completed pre-adaptation text replacement does not match its visual region")
			}
		}
		if replacement.Status != "missing" && !strings.Contains(adaptation.ProductionPrompt, replacement.ReplacementText) {
			return errors.New("completed pre-adaptation production prompt omits a replacement text")
		}
		replacements[replacement.BlockID] = replacement
	}
	coveredNumericBlocks := map[string]bool{}
	seenLayouts := map[string]bool{}
	for _, layout := range adaptation.NumericLayouts {
		layout.ID = strings.TrimSpace(layout.ID)
		layout.VisualRegionID = strings.TrimSpace(layout.VisualRegionID)
		layout.Location = strings.TrimSpace(layout.Location)
		layout.LayoutKind = strings.TrimSpace(layout.LayoutKind)
		layout.RenderInstruction = strings.TrimSpace(layout.RenderInstruction)
		if layout.ID == "" || layout.Location == "" || !validCreativeNumericLayoutKind(layout.LayoutKind) || layout.RenderInstruction == "" || seenLayouts[layout.ID] {
			return errors.New("completed pre-adaptation contains an invalid numeric layout")
		}
		seenLayouts[layout.ID] = true
		if len(layout.SourceBlockIDs) == 0 || len(layout.ScenarioIDs) == 0 || len(layout.TargetColumns) == 0 {
			return errors.New("completed pre-adaptation numeric layout is incomplete")
		}
		// One visible repayment field can supply one label and one value per
		// selected scenario/column. Additional source blocks are unrelated text
		// that the model must visibly replace with approved copy.
		if len(layout.SourceBlockIDs) > len(layout.ScenarioIDs)*len(layout.TargetColumns)*2 {
			return errors.New("completed pre-adaptation numeric layout covers more source blocks than its repayment fields can display")
		}
		layoutRegionID := ""
		for _, blockID := range layout.SourceBlockIDs {
			blockID = strings.TrimSpace(blockID)
			if _, exists := numericBlocks[blockID]; !exists || coveredNumericBlocks[blockID] || replacements[blockID].BlockID != "" {
				return errors.New("completed pre-adaptation numeric layout does not match source plan fields")
			}
			if hasVisualRegions {
				region, exists := regionsByBlockID[blockID]
				if !exists || region.Kind != "numeric" || (layoutRegionID != "" && layoutRegionID != region.ID) {
					return errors.New("completed pre-adaptation numeric layout does not match one visual region")
				}
				layoutRegionID = region.ID
			}
			coveredNumericBlocks[blockID] = true
		}
		if hasVisualRegions && layout.VisualRegionID != layoutRegionID {
			return errors.New("completed pre-adaptation numeric layout does not match its visual region")
		}
		seenColumns := map[string]bool{}
		for _, column := range layout.TargetColumns {
			column = strings.TrimSpace(column)
			if !validCreativeNumericColumn(column) || seenColumns[column] {
				return errors.New("completed pre-adaptation numeric layout has invalid target columns")
			}
			seenColumns[column] = true
		}
		if !strings.Contains(adaptation.ProductionPrompt, layout.RenderInstruction) {
			return errors.New("completed pre-adaptation production prompt omits a numeric layout instruction")
		}
	}
	for blockID := range blocks {
		if _, replaced := replacements[blockID]; replaced {
			continue
		}
		if coveredNumericBlocks[blockID] {
			continue
		}
		return errors.New("completed pre-adaptation must visibly replace every source text block")
	}
	if len(numericBlocks) == 0 && len(adaptation.NumericLayouts) > 0 {
		return errors.New("completed pre-adaptation has numeric layouts without numeric source blocks")
	}
	return nil
}

func validatedCreativePreAdaptationVisualRegions(regions []creativePreAdaptationSourceVisualRegion, blocks map[string]creativePreAdaptationSourceTextBlock) (map[string]creativePreAdaptationSourceVisualRegion, bool, error) {
	if len(regions) == 0 {
		return nil, false, nil
	}
	regionsByBlockID := make(map[string]creativePreAdaptationSourceVisualRegion, len(blocks))
	regionIDs := make(map[string]bool, len(regions))
	for _, region := range regions {
		region.ID = strings.TrimSpace(region.ID)
		region.Location = strings.TrimSpace(region.Location)
		region.Kind = strings.TrimSpace(region.Kind)
		if region.ID == "" || region.Location == "" || (region.Kind != "copy" && region.Kind != "numeric") || regionIDs[region.ID] || len(region.SourceBlockIDs) == 0 {
			return nil, false, errors.New("source analysis contains an invalid visual region")
		}
		if region.VisualBounds.X < 0 || region.VisualBounds.Y < 0 || region.VisualBounds.Width <= 0 || region.VisualBounds.Height <= 0 || region.VisualBounds.X+region.VisualBounds.Width > 1000 || region.VisualBounds.Y+region.VisualBounds.Height > 1000 {
			return nil, false, errors.New("source analysis contains an invalid visual region bounds")
		}
		regionIDs[region.ID] = true
		for index, blockID := range region.SourceBlockIDs {
			blockID = strings.TrimSpace(blockID)
			region.SourceBlockIDs[index] = blockID
			if _, exists := blocks[blockID]; !exists || regionsByBlockID[blockID].ID != "" {
				return nil, false, errors.New("source analysis visual regions do not uniquely cover text blocks")
			}
			regionsByBlockID[blockID] = region
		}
	}
	if len(regionsByBlockID) != len(blocks) {
		return nil, false, errors.New("source analysis visual regions do not cover every text block")
	}
	return regionsByBlockID, true, nil
}

func trimCreativePreAdaptationStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func validCreativePreAdaptationReplacementStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case "ready", "missing", "recommended", "calculated":
		return true
	default:
		return false
	}
}

func normalizedCreativePreAdaptationReplacementStatus(replacement creativePreAdaptationTextReplacement) string {
	if status := strings.TrimSpace(replacement.Status); status != "" {
		return status
	}
	if strings.TrimSpace(replacement.ReplacementText) == "" && len(replacement.SourceKeys) == 0 {
		return "missing"
	}
	return "ready"
}

func validateCreativePreAdaptationReplacementProposal(replacement creativePreAdaptationTextReplacement) error {
	switch replacement.Status {
	case "ready":
		if replacement.ReplacementText == "" || len(replacement.SourceKeys) == 0 {
			return errors.New("ready pre-adaptation text replacement must bind approved copy")
		}
		if len(replacement.RecommendationBasis) != 0 || replacement.Calculation != nil {
			return errors.New("ready pre-adaptation text replacement must not carry a recommendation")
		}
	case "missing":
		if replacement.ReplacementText != "" || len(replacement.SourceKeys) != 0 || len(replacement.RecommendationBasis) != 0 || replacement.Calculation != nil {
			return errors.New("missing pre-adaptation text replacement must not contain copy or a proposal")
		}
	case "recommended":
		if replacement.ReplacementText == "" || len(replacement.SourceKeys) != 0 || len(replacement.RecommendationBasis) == 0 || replacement.Calculation != nil {
			return errors.New("recommended pre-adaptation text replacement requires text and a non-library basis")
		}
	case "calculated":
		if replacement.ReplacementText == "" || len(replacement.SourceKeys) != 0 || replacement.Calculation == nil {
			return errors.New("calculated pre-adaptation text replacement requires a formula result")
		}
		calculation := replacement.Calculation
		calculation.RuleKey = strings.TrimSpace(calculation.RuleKey)
		calculation.Formula = strings.TrimSpace(calculation.Formula)
		calculation.Result = strings.TrimSpace(calculation.Result)
		calculation.Inputs = trimCreativePreAdaptationStrings(calculation.Inputs)
		if calculation.RuleKey == "" || calculation.Formula == "" || calculation.Result != replacement.ReplacementText || len(calculation.Inputs) == 0 {
			return errors.New("calculated pre-adaptation text replacement has an invalid formula trace")
		}
	}
	return nil
}

type creativePreAdaptationCalculationRule struct {
	Key      string            `json:"key"`
	Formulas map[string]string `json:"formulas"`
}

func creativePreAdaptationSourceHasVisualRegions(sourceRaw json.RawMessage) bool {
	var source struct {
		TextBlocks    []creativePreAdaptationSourceTextBlock    `json:"text_blocks"`
		VisualRegions []creativePreAdaptationSourceVisualRegion `json:"visual_regions"`
	}
	if json.Unmarshal(sourceRaw, &source) != nil || len(source.TextBlocks) == 0 {
		return false
	}
	blocks := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		block.ID = strings.TrimSpace(block.ID)
		if block.ID == "" {
			return false
		}
		blocks[block.ID] = block
	}
	_, hasVisualRegions, err := validatedCreativePreAdaptationVisualRegions(source.VisualRegions, blocks)
	return err == nil && hasVisualRegions
}

func validateCreativePreAdaptationCalculationRules(sourceRaw, adaptationRaw, marketPackRaw json.RawMessage) error {
	var source struct {
		TextBlocks []creativePreAdaptationSourceTextBlock `json:"text_blocks"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return errors.New("source analysis result is invalid")
	}
	semanticKindByBlockID := make(map[string]string, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		semanticKindByBlockID[strings.TrimSpace(block.ID)] = strings.TrimSpace(block.SemanticKind)
	}
	var marketPack struct {
		CalculationRules []creativePreAdaptationCalculationRule `json:"calculation_rules"`
	}
	if err := json.Unmarshal(marketPackRaw, &marketPack); err != nil {
		return errors.New("frozen market pack config is invalid")
	}
	rulesByKey := make(map[string]creativePreAdaptationCalculationRule, len(marketPack.CalculationRules))
	for _, rule := range marketPack.CalculationRules {
		rule.Key = strings.TrimSpace(rule.Key)
		if rule.Key != "" {
			rulesByKey[rule.Key] = rule
		}
	}
	var adaptation creativePreAdaptationCompletedResult
	if err := json.Unmarshal(adaptationRaw, &adaptation); err != nil {
		return errors.New("completed pre-adaptation result is invalid")
	}
	for _, replacement := range adaptation.TextReplacements {
		if normalizedCreativePreAdaptationReplacementStatus(replacement) != "calculated" || replacement.Calculation == nil {
			continue
		}
		calculation := replacement.Calculation
		rule, exists := rulesByKey[strings.TrimSpace(calculation.RuleKey)]
		if !exists {
			return fmt.Errorf("pre-adaptation block %q references an unknown calculation rule", strings.TrimSpace(replacement.BlockID))
		}
		semanticKind := semanticKindByBlockID[strings.TrimSpace(replacement.BlockID)]
		expectedFormula := strings.TrimSpace(rule.Formulas[semanticKind])
		if expectedFormula == "" || strings.TrimSpace(calculation.Formula) != expectedFormula {
			return fmt.Errorf("pre-adaptation block %q calculation does not match the frozen %s rule", strings.TrimSpace(replacement.BlockID), semanticKind)
		}
	}
	return nil
}

func validateCreativePreAdaptationCopyBindings(sourceRaw, adaptationRaw, copyLibraryRaw json.RawMessage) error {
	var source struct {
		TextBlocks []creativePreAdaptationSourceTextBlock `json:"text_blocks"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return errors.New("source analysis result is invalid")
	}
	sourceBlocksByID := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		sourceBlocksByID[strings.TrimSpace(block.ID)] = block
	}

	var adaptation creativePreAdaptationCompletedResult
	if err := json.Unmarshal(adaptationRaw, &adaptation); err != nil {
		return errors.New("completed pre-adaptation result is invalid")
	}
	if strings.TrimSpace(adaptation.CopyLibraryID) == "" || adaptation.CopyLibraryVersion < 1 {
		return errors.New("completed pre-adaptation requires frozen copy library identity")
	}
	var library composableCopyLibraryConfig
	if err := json.Unmarshal(copyLibraryRaw, &library); err != nil {
		return errors.New("frozen copy library config is invalid")
	}

	type approvedFragment struct {
		text          string
		semanticGroup string
	}
	fragmentsByKey := map[string]approvedFragment{}
	for _, fragment := range library.Fragments {
		if fragment.Status != "approved" || strings.TrimSpace(fragment.ID) == "" || strings.TrimSpace(fragment.Key) == "" {
			continue
		}
		fragmentsByKey[strings.TrimSpace(fragment.Key)] = approvedFragment{
			text: strings.TrimSpace(fragment.Text), semanticGroup: strings.TrimSpace(fragment.SemanticGroup),
		}
	}

	planValues, planLabels, err := validateCreativePreAdaptationRepaymentPlanSelections(library.RepaymentPlan, adaptation.RepaymentPlanSelections)
	if err != nil {
		return err
	}
	if err := validateCreativePreAdaptationNumericLayouts(adaptation.NumericLayouts, adaptation.RepaymentPlanSelections, planValues, planLabels); err != nil {
		return err
	}

	usedSourceKeys := map[string]string{}
	for _, replacement := range adaptation.TextReplacements {
		replacement.Status = normalizedCreativePreAdaptationReplacementStatus(replacement)
		replacement.ReplacementText = strings.TrimSpace(replacement.ReplacementText)
		replacement.RecommendationBasis = trimCreativePreAdaptationStrings(replacement.RecommendationBasis)
		if err := validateCreativePreAdaptationReplacementProposal(replacement); err != nil {
			return fmt.Errorf("pre-adaptation block %q is invalid: %w", replacement.BlockID, err)
		}
		if replacement.Status == "missing" || replacement.Status == "recommended" || replacement.Status == "calculated" {
			continue
		}
		replacementText := replacement.ReplacementText
		sourceKeys := map[string]bool{}
		for _, sourceKey := range replacement.SourceKeys {
			if sourceKey = strings.TrimSpace(sourceKey); sourceKey != "" {
				sourceKeys[sourceKey] = true
			}
		}
		if len(sourceKeys) != 1 {
			return fmt.Errorf("pre-adaptation block %q must bind exactly one approved copy library source", replacement.BlockID)
		}
		if len(sourceKeys) == 0 {
			return fmt.Errorf("pre-adaptation block %q has no copy library source keys", replacement.BlockID)
		}
		var sourceKey string
		for sourceKey = range sourceKeys {
		}
		fragment, exists := fragmentsByKey[sourceKey]
		if !exists {
			return fmt.Errorf("pre-adaptation block %q references unknown copy library key %q", replacement.BlockID, sourceKey)
		}
		if fragment.text != replacementText {
			return fmt.Errorf("pre-adaptation block %q replacement text does not match its approved copy library source", replacement.BlockID)
		}
		if previousBlockID := usedSourceKeys[sourceKey]; previousBlockID != "" {
			return fmt.Errorf("pre-adaptation blocks %q and %q reuse approved copy source %q", previousBlockID, replacement.BlockID, sourceKey)
		}
		if sourceBlock, exists := sourceBlocksByID[strings.TrimSpace(replacement.BlockID)]; exists {
			semanticKind := creativeSourceTextBlockSemanticKind(sourceBlock)
			if !creativeFragmentSupportsSourceSemanticKind(fragment.semanticGroup, semanticKind) {
				return fmt.Errorf("pre-adaptation block %q requires approved %s copy, not %s", replacement.BlockID, semanticKind, fragment.semanticGroup)
			}
		}
		usedSourceKeys[sourceKey] = replacement.BlockID
	}
	return nil
}

// A source block's visual role is deliberately broad. Financial compatibility
// comes from its explicit semantic kind, with purpose-based fallback for
// analyses created before semantic_kind was introduced.
func creativeSourceTextBlockSemanticKind(block creativePreAdaptationSourceTextBlock) string {
	if semanticKind := strings.TrimSpace(block.SemanticKind); validCreativeSourceSemanticKind(semanticKind) {
		return semanticKind
	}
	purpose := strings.ToLower(strings.TrimSpace(block.Purpose))
	switch {
	case strings.Contains(purpose, "日利息金额") || strings.Contains(purpose, "daily interest amount"):
		return "daily_interest_amount"
	case strings.Contains(purpose, "日利息字段") || strings.Contains(purpose, "daily interest label"):
		return "daily_interest_label"
	case strings.Contains(purpose, "标注") || strings.Contains(purpose, "label"):
		return "copy"
	case strings.Contains(purpose, "固定利率") || strings.Contains(purpose, "利率数值") || strings.Contains(purpose, "interest rate"):
		return "interest_rate"
	case strings.Contains(purpose, "借款金额") || strings.Contains(purpose, "本金") || strings.Contains(purpose, "principal"):
		return "principal"
	case strings.Contains(purpose, "期限") || strings.Contains(purpose, "tenor"):
		return "tenor"
	case strings.Contains(purpose, "月供") || strings.Contains(purpose, "monthly installment"):
		return "monthly_installment"
	case strings.Contains(purpose, "总利息") || strings.Contains(purpose, "total interest"):
		return "total_interest"
	case strings.Contains(purpose, "总还款") || strings.Contains(purpose, "total repayment"):
		return "total_repayment"
	}
	return ""
}

func validCreativeSourceSemanticKind(value string) bool {
	switch value {
	case "copy", "principal", "tenor", "monthly_installment", "total_interest", "total_repayment", "interest_rate", "daily_interest_amount", "daily_interest_label":
		return true
	default:
		return false
	}
}

func creativeFragmentSupportsSourceSemanticKind(fragmentGroup, sourceKind string) bool {
	if sourceKind == "" || sourceKind == "copy" {
		return true
	}
	fragmentGroup = strings.TrimSpace(fragmentGroup)
	switch sourceKind {
	case "principal":
		return fragmentGroup == "principal" || fragmentGroup == "limit"
	case "tenor":
		return fragmentGroup == "tenor"
	default:
		return fragmentGroup == sourceKind
	}
}

func validateCreativePreAdaptationNumericLayouts(layouts []creativePreAdaptationNumericLayout, selections []creativePreAdaptationRepaymentPlanSelection, values, _ map[string]string) error {
	if len(layouts) == 0 && len(selections) == 0 {
		return nil
	}
	selectionsByID := map[string]creativePreAdaptationRepaymentPlanSelection{}
	for _, selection := range selections {
		selectionsByID[strings.TrimSpace(selection.ID)] = selection
	}
	usedSelections := map[string]bool{}
	for _, layout := range layouts {
		if len(layout.ScenarioIDs) == 0 || len(layout.TargetColumns) == 0 {
			return errors.New("pre-adaptation numeric layout is incomplete")
		}
		for _, selectionID := range layout.ScenarioIDs {
			selectionID = strings.TrimSpace(selectionID)
			_, exists := selectionsByID[selectionID]
			if !exists {
				return errors.New("pre-adaptation numeric layout has an unknown repayment plan selection")
			}
			usedSelections[selectionID] = true
			for _, column := range layout.TargetColumns {
				column = strings.TrimSpace(column)
				valueKey := "plan:" + selectionID + ":" + column
				value, valueExists := values[valueKey]
				if !valueExists || !strings.Contains(layout.RenderInstruction, value) {
					return fmt.Errorf("pre-adaptation numeric layout %q does not contain approved %s values", layout.ID, column)
				}
			}
		}
	}
	if len(usedSelections) != len(selectionsByID) {
		return errors.New("pre-adaptation contains an unused repayment plan selection")
	}
	return nil
}

func validateCreativePreAdaptationRepaymentPlanSelections(plan composableCopyLibraryRepaymentPlan, selections []creativePreAdaptationRepaymentPlanSelection) (map[string]string, map[string]string, error) {
	values := map[string]string{}
	labels := map[string]string{}
	if len(selections) == 0 {
		return values, labels, nil
	}
	labels["plan:label:principal"] = strings.TrimSpace(plan.Labels.Principal)
	labels["plan:label:tenor"] = strings.TrimSpace(plan.Labels.Tenor)
	labels["plan:label:monthly_installment"] = strings.TrimSpace(plan.Labels.MonthlyInstallment)
	labels["plan:label:total_interest"] = strings.TrimSpace(plan.Labels.TotalInterest)
	labels["plan:label:total_repayment"] = strings.TrimSpace(plan.Labels.TotalRepayment)
	approvedByKey := map[string]composableCopyLibraryRepaymentPlanEntry{}
	for _, entry := range plan.Entries {
		if entry.Status == "approved" {
			approvedByKey[strings.TrimSpace(entry.Key)] = entry
		}
	}
	seen := map[string]bool{}
	for _, selection := range selections {
		selection.ID = strings.TrimSpace(selection.ID)
		entry, exists := approvedByKey[strings.TrimSpace(selection.PlanKey)]
		if selection.ID == "" || !exists || seen[selection.ID] {
			return nil, nil, errors.New("pre-adaptation has an invalid or duplicate repayment plan selection")
		}
		seen[selection.ID] = true
		if selection.Principal != entry.Principal || selection.TenorMonths != entry.TenorMonths {
			return nil, nil, fmt.Errorf("repayment plan selection %q does not match the approved amount and tenor", selection.ID)
		}
		expected := creativePreAdaptationRepaymentPlanValues{
			Principal: creativeFormatRupiah(entry.Principal), Tenor: fmt.Sprintf("%d Bulan", entry.TenorMonths), TotalInterest: creativeFormatRupiah(entry.TotalInterest),
			TotalRepayment: creativeFormatRupiah(entry.TotalRepayment), MonthlyInstallment: creativeFormatRupiah(entry.MonthlyInstallment),
		}
		if selection.Values != expected {
			return nil, nil, fmt.Errorf("repayment plan selection %q does not match the approved plan row", selection.ID)
		}
		prefix := "plan:" + selection.ID + ":"
		values[prefix+"principal"] = expected.Principal
		values[prefix+"tenor"] = expected.Tenor
		values[prefix+"total_interest"] = expected.TotalInterest
		values[prefix+"total_repayment"] = expected.TotalRepayment
		values[prefix+"monthly_installment"] = expected.MonthlyInstallment
	}
	return values, labels, nil
}

func creativeFormatRupiah(value int64) string {
	digits := strconv.FormatInt(value, 10)
	groups := make([]string, 0, (len(digits)+2)/3)
	for len(digits) > 3 {
		groups = append([]string{digits[len(digits)-3:]}, groups...)
		digits = digits[:len(digits)-3]
	}
	groups = append([]string{digits}, groups...)
	return "Rp" + strings.Join(groups, ".")
}

func validCreativeTextRole(role string) bool {
	switch role {
	case "headline", "subheadline", "benefit", "supporting", "cta", "legal", "plan_field":
		return true
	default:
		return false
	}
}

func validCreativeNumericLayoutKind(value string) bool {
	return validCreativeCanonicalNumericLayoutKind(strings.TrimSpace(value))
}

func validCreativeNumericColumn(value string) bool {
	switch value {
	case "principal", "tenor", "monthly_installment", "total_interest", "total_repayment":
		return true
	default:
		return false
	}
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return 0, errors.New("value must be positive")
	}
	return value, nil
}
