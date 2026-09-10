package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

const creativePreAdaptationOutputMismatchError = "pre-adaptation task completed without a matching frozen adaptation result"

func creativePreAdaptationFragmentSupportsType(types []string, creativeType string) bool {
	for _, value := range types {
		if strings.TrimSpace(value) == creativeType {
			return true
		}
	}
	return false
}

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
}

// enqueueCreativePreAdaptation intentionally runs after the source analysis
// completes. The latter is market-neutral; this task freezes one explicit
// market pack and its bound copy library before making any recommendation.
func (h *Handler) enqueueCreativePreAdaptation(ctx context.Context, sourceTask db.AgentTaskQueue, workspaceRaw string) error {
	if h.TaskService == nil || !sourceTask.TriggerEvidenceKind.Valid || sourceTask.TriggerEvidenceKind.String != "creative_crawl_run_analysis" {
		return nil
	}
	var sourceContext creativeReferenceAnalysisTaskContext
	if json.Unmarshal(sourceTask.Context, &sourceContext) != nil || sourceContext.Workflow != "creative_reference_analysis" {
		return nil
	}
	workspaceID, err := parseUUIDString(strings.TrimSpace(workspaceRaw))
	if err != nil {
		return fmt.Errorf("parse creative pre-adaptation workspace: %w", err)
	}
	candidateID, err := parseUUIDString(strings.TrimSpace(sourceContext.CandidateID))
	if err != nil {
		return fmt.Errorf("parse creative pre-adaptation candidate: %w", err)
	}
	var analysisID pgtype.UUID
	err = h.DB.QueryRow(ctx, `
SELECT id FROM creative_source_analysis
WHERE workspace_id = $1 AND candidate_id = $2 AND analysis_version = $3 AND status = 'completed'
  AND NOT EXISTS (
    SELECT 1 FROM creative_source_analysis newer
    WHERE newer.workspace_id = $1 AND newer.candidate_id = $2 AND newer.analysis_version > $3
  )
ORDER BY completed_at DESC LIMIT 1
`, workspaceID, candidateID, sourceContext.AnalysisVersion).Scan(&analysisID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("read completed source analysis for pre-adaptation: %w", err)
	}
	marketPack, copyLibrary, err := h.defaultCreativePreAdaptationResources(ctx, workspaceID)
	if err != nil {
		// An explicit default is a prerequisite, not a reason to fail the
		// already-valid source analysis.
		return fmt.Errorf("resolve default pre-adaptation resources: %w", err)
	}
	agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: sourceTask.AgentID, WorkspaceID: workspaceID})
	if err != nil {
		return fmt.Errorf("resolve reference-analysis agent for pre-adaptation: %w", err)
	}
	if agent.ArchivedAt.Valid {
		return errors.New("reference-analysis agent for pre-adaptation is archived")
	}
	if _, err := h.enqueueCreativePreAdaptationTask(ctx, agent, sourceTask.RequestingUserID, analysisID, candidateID, marketPack, copyLibrary); err != nil {
		return fmt.Errorf("enqueue creative pre-adaptation task: %w", err)
	}
	return nil
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

// retryCreativePreAdaptationOutput creates one bounded child attempt after the
// daemon's completed output cannot be matched back to the frozen source
// analysis. The failed task is already terminal when this helper runs, so the
// direct-task retry service can safely clone it without exposing a manual
// recovery step to the order user.
func (h *Handler) retryCreativePreAdaptationOutput(ctx context.Context, task db.AgentTaskQueue) (string, error) {
	if h.TaskService == nil || !creativePreAdaptationTaskRetryable(task) {
		return "", nil
	}
	retried, err := h.TaskService.RetryFailedDirectTasksByEvidence(ctx, task.AgentID, creativePreAdaptationEvidenceKind, task.TriggerEvidenceRefID)
	if err != nil {
		return "", err
	}
	if len(retried) == 0 {
		return "", nil
	}
	return uuidToString(retried[0].ID), nil
}

func creativePreAdaptationTaskRetryable(task db.AgentTaskQueue) bool {
	if task.Status != "failed" || task.Attempt < 1 ||
		(task.MaxAttempts > 0 && task.Attempt >= task.MaxAttempts) ||
		!task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != creativePreAdaptationEvidenceKind ||
		!task.TriggerEvidenceRefID.Valid {
		return false
	}
	if task.FailureReason.Valid {
		switch strings.TrimSpace(task.FailureReason.String) {
		case "cancelled", "user_cancelled", "manual":
			return false
		}
	}
	return true
}

func creativePreAdaptationArtifactAccepted(status, errorCode string, frozenResourcesMatch bool) bool {
	if !frozenResourcesMatch {
		return false
	}
	switch strings.TrimSpace(status) {
	case "completed":
		return true
	case "unavailable":
		return strings.TrimSpace(errorCode) == "manual_confirmation_required" || creativePreAdaptationSourceStructureRecoverable(errorCode)
	default:
		return false
	}
}

// compactCreativePreAdaptationValue makes the human-readable render
// instruction tolerant of harmless whitespace differences such as `Rp 4.000.000`
// versus the frozen value `Rp4.000.000`. The underlying repayment values remain
// exact; only the descriptive instruction is normalized for containment checks.
func compactCreativePreAdaptationValue(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), "")
}

func creativePreAdaptationInstructionContainsValue(instruction, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(instruction, value) {
		return true
	}
	return strings.Contains(compactCreativePreAdaptationValue(instruction), compactCreativePreAdaptationValue(value))
}

func creativePreAdaptationSafeCopyFallback(config composableCopyLibraryConfig, role, sourceText, semanticKind string, used map[string]bool, creativeType string) (struct{ key, text string }, bool) {
	keywords := strings.Fields(strings.ToLower(strings.NewReplacer(".", " ", ",", " ", "?", " ", "!", " ", "-", " ", "_", " ").Replace(sourceText)))
	best := struct {
		key, text string
		score     int
	}{}
	for _, fragment := range config.Fragments {
		key := strings.TrimSpace(fragment.Key)
		if fragment.Status != "approved" || key == "" || used[key] || strings.TrimSpace(fragment.Role) != strings.TrimSpace(role) || !creativePreAdaptationFragmentSupportsType(fragment.CreativeTypes, creativeType) {
			continue
		}
		candidateText := strings.ToLower(fragment.Text)
		matched := 0
		for _, keyword := range keywords {
			if len(keyword) >= 3 && strings.Contains(candidateText, keyword) {
				matched++
			}
		}
		if role != "headline" && len(keywords) > 1 && matched < len(keywords) {
			continue
		}
		// A generic headline fallback is only safe for an unclassified copy
		// block. Numeric or financial semantic kinds must remain missing unless
		// the approved fragment is actually supported by the source text.
		if matched == 0 && (role != "headline" || (semanticKind != "" && semanticKind != "copy")) {
			continue
		}
		score := matched * 3
		if role == "headline" {
			score++
		}
		if score == 0 {
			continue
		}
		if best.key == "" || score > best.score {
			best = struct {
				key, text string
				score     int
			}{key: key, text: strings.TrimSpace(fragment.Text), score: score}
		}
	}
	if best.key == "" || best.text == "" {
		return struct{ key, text string }{}, false
	}
	return struct{ key, text string }{key: best.key, text: best.text}, true
}

func creativePreAdaptationGeneratedRecommendation(config composableCopyLibraryConfig, role, semanticKind string) (string, []string, bool) {
	if semanticKind != "" && semanticKind != "copy" {
		return "", nil, false
	}
	var text string
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(config.Locale)), "id"):
		switch role {
		case "headline":
			text = "Solusi finansial untuk kebutuhanmu"
		case "subheadline":
			text = "Ajukan dengan proses yang mudah"
		case "benefit":
			text = "Bantu wujudkan kebutuhanmu"
		case "supporting":
			text = "Pilih sesuai kebutuhanmu"
		case "cta":
			text = "Ajukan Sekarang"
		}
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(config.Locale)), "ms"):
		switch role {
		case "headline":
			text = "Penyelesaian kewangan untuk keperluan anda"
		case "subheadline":
			text = "Mohon dengan proses yang mudah"
		case "benefit":
			text = "Bantu realisasikan keperluan anda"
		case "supporting":
			text = "Pilih mengikut keperluan anda"
		case "cta":
			text = "Mohon Sekarang"
		}
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(config.Locale)), "zh"):
		switch role {
		case "headline":
			text = "满足你的资金需求"
		case "subheadline":
			text = "申请流程简单便捷"
		case "benefit":
			text = "灵活应对日常需求"
		case "supporting":
			text = "按你的需求选择"
		case "cta":
			text = "立即申请"
		}
	default:
		switch role {
		case "headline":
			text = "A financial solution for your needs"
		case "subheadline":
			text = "Apply with a simple process"
		case "benefit":
			text = "Support your everyday needs"
		case "supporting":
			text = "Choose what fits your needs"
		case "cta":
			text = "Apply Now"
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", nil, false
	}
	return text, []string{
		"当前冻结文案库没有兼容的已审核片段",
		"根据当前市场语言和区块职责生成安全候选",
		"候选不包含竞品品牌、金融数值、法律文字或二维码，需用户确认",
	}, true
}

// promoteCreativePreAdaptationRecommendations closes the contract at the
// persistence boundary. A model may correctly report that an ordinary copy
// block has no approved match without constructing the pending recommendation
// itself; the stored result must still expose the same user-confirmable state.
func promoteCreativePreAdaptationRecommendations(sourceRaw, adaptationRaw, copyLibraryRaw json.RawMessage) (json.RawMessage, bool, error) {
	var source struct {
		TextBlocks    []creativePreAdaptationSourceTextBlock    `json:"text_blocks"`
		VisualRegions []creativePreAdaptationSourceVisualRegion `json:"visual_regions"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return nil, false, errors.New("source analysis result is invalid")
	}
	blocks := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		block.ID = strings.TrimSpace(block.ID)
		if block.ID == "" {
			return nil, false, errors.New("source analysis contains an empty text block id")
		}
		blocks[block.ID] = block
	}
	regionsByBlockID, hasVisualRegions, err := validatedCreativePreAdaptationVisualRegions(source.VisualRegions, blocks)
	if err != nil {
		return nil, false, err
	}

	var library composableCopyLibraryConfig
	if err := json.Unmarshal(copyLibraryRaw, &library); err != nil {
		return nil, false, errors.New("frozen copy library config is invalid")
	}
	var adaptation creativePreAdaptationCompletedResult
	if err := json.Unmarshal(adaptationRaw, &adaptation); err != nil {
		return nil, false, fmt.Errorf("completed pre-adaptation result is invalid: %w", err)
	}

	numericBlockIDs := make(map[string]bool)
	for id, block := range blocks {
		if block.Role == "plan_field" {
			numericBlockIDs[id] = true
		}
		if hasVisualRegions {
			if region, exists := regionsByBlockID[id]; exists && region.Kind == "numeric" {
				numericBlockIDs[id] = true
			}
		}
	}

	changed := false
	for index := range adaptation.TextReplacements {
		replacement := &adaptation.TextReplacements[index]
		if normalizedCreativePreAdaptationReplacementStatus(*replacement) != "missing" || strings.TrimSpace(replacement.ReplacementText) != "" || len(replacement.SourceKeys) != 0 {
			continue
		}
		block, exists := blocks[strings.TrimSpace(replacement.BlockID)]
		if !exists || numericBlockIDs[block.ID] {
			continue
		}
		semanticKind := creativeSourceTextBlockSemanticKind(block)
		text, basis, ok := creativePreAdaptationGeneratedRecommendation(library, block.Role, semanticKind)
		if !ok {
			continue
		}
		replacement.ReplacementText = text
		replacement.Status = "recommended"
		replacement.SourceKeys = nil
		replacement.RecommendationBasis = basis
		replacement.Note = "冻结文案库没有兼容的已审核片段，平台生成候选供用户确认。"
		changed = true
	}
	if !changed {
		return adaptationRaw, false, nil
	}
	encoded, err := json.Marshal(adaptation)
	if err != nil {
		return nil, false, err
	}
	return encoded, true, nil
}

// buildAutomaticCreativePreAdaptationResult is a bounded last-mile recovery
// for a model result that could not satisfy the numeric contract after its
// retry. It emits frozen values, preserves model recommendations, and creates
// a clearly pending recommendation for an ordinary copy block with no
// compatible approved fragment. A numeric region is split into renderable
// rows; blocks that cannot be assigned to an approved row remain explicit
// missing replacements for the confirmation page.
func buildAutomaticCreativePreAdaptationResult(sourceRaw json.RawMessage, taskContext creativePreAdaptationTaskContext, copyLibraryRaw json.RawMessage) (creativePreAdaptationCompletedResult, error) {
	var source struct {
		TextBlocks    []creativePreAdaptationSourceTextBlock    `json:"text_blocks"`
		VisualRegions []creativePreAdaptationSourceVisualRegion `json:"visual_regions"`
	}
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return creativePreAdaptationCompletedResult{}, errors.New("source analysis result is invalid")
	}
	if len(source.TextBlocks) == 0 {
		return creativePreAdaptationCompletedResult{}, errors.New("automatic pre-adaptation recovery requires source text blocks")
	}
	blocks := make(map[string]creativePreAdaptationSourceTextBlock, len(source.TextBlocks))
	for _, block := range source.TextBlocks {
		block.ID = strings.TrimSpace(block.ID)
		if block.ID == "" {
			return creativePreAdaptationCompletedResult{}, errors.New("source analysis contains an empty text block id")
		}
		blocks[block.ID] = block
	}
	regionsByBlockID, hasVisualRegions, err := validatedCreativePreAdaptationVisualRegions(source.VisualRegions, blocks)
	if err != nil {
		return creativePreAdaptationCompletedResult{}, err
	}

	var library composableCopyLibraryConfig
	if err := json.Unmarshal(copyLibraryRaw, &library); err != nil {
		return creativePreAdaptationCompletedResult{}, errors.New("frozen copy library config is invalid")
	}
	approvedEntries := make([]composableCopyLibraryRepaymentPlanEntry, 0, len(library.RepaymentPlan.Entries))
	for _, entry := range library.RepaymentPlan.Entries {
		if strings.TrimSpace(entry.Key) != "" && entry.Status == "approved" {
			approvedEntries = append(approvedEntries, entry)
		}
	}
	result := creativePreAdaptationCompletedResult{
		MarketPackID:            taskContext.MarketPackID,
		MarketPackVersion:       taskContext.MarketPackVersion,
		CopyLibraryID:           taskContext.CopyLibraryID,
		CopyLibraryVersion:      taskContext.CopyLibraryVersion,
		TextReplacements:        make([]creativePreAdaptationTextReplacement, 0, len(source.TextBlocks)),
		RepaymentPlanSelections: make([]creativePreAdaptationRepaymentPlanSelection, 0, len(source.VisualRegions)),
		NumericLayouts:          make([]creativePreAdaptationNumericLayout, 0, len(source.VisualRegions)),
		AnalysisHighlights: []string{
			"平台按 Source Analysis 的视觉区域拆分可渲染的数值行。",
			"已映射的本金、期限和月供均来自当前冻结文案库的已审核还款计划。",
			"未匹配的区块保留为可编辑项，不引入竞品金额或临时计算值。",
		},
	}
	coveredBlocks := map[string]bool{}
	usedPlanKeys := map[string]bool{}
	planIndex := 0
	type automaticNumericGroup struct {
		sourceBlockIDs []string
		columns        []string
		seenColumns    map[string]bool
	}
	for _, region := range source.VisualRegions {
		if strings.TrimSpace(region.Kind) != "numeric" {
			continue
		}
		groups := make([]automaticNumericGroup, 0, len(region.SourceBlockIDs))
		current := automaticNumericGroup{seenColumns: map[string]bool{}}
		flushCurrent := func() {
			if len(current.sourceBlockIDs) > 0 && len(current.columns) > 0 {
				groups = append(groups, current)
			}
			current = automaticNumericGroup{seenColumns: map[string]bool{}}
		}
		for _, rawBlockID := range region.SourceBlockIDs {
			blockID := strings.TrimSpace(rawBlockID)
			block, exists := blocks[blockID]
			if !exists {
				return creativePreAdaptationCompletedResult{}, fmt.Errorf("numeric visual region %q references an unknown source block", region.ID)
			}
			semanticKind := creativeSourceTextBlockSemanticKind(block)
			if validCreativeNumericColumn(semanticKind) {
				if current.seenColumns[semanticKind] {
					flushCurrent()
				}
				current.sourceBlockIDs = append(current.sourceBlockIDs, blockID)
				current.columns = append(current.columns, semanticKind)
				current.seenColumns[semanticKind] = true
				continue
			}
			if block.Role == "supporting" {
				if len(current.sourceBlockIDs) == 0 && len(groups) > 0 {
					groups[len(groups)-1].sourceBlockIDs = append(groups[len(groups)-1].sourceBlockIDs, blockID)
				} else {
					current.sourceBlockIDs = append(current.sourceBlockIDs, blockID)
				}
			}
		}
		flushCurrent()
		for groupIndex, group := range groups {
			// A layout can carry at most one source label and one source value
			// per selected column. Extra labels stay editable instead of making
			// the entire region invalid.
			maxSourceBlocks := len(group.columns) * 2
			if len(group.sourceBlockIDs) > maxSourceBlocks {
				for _, blockID := range group.sourceBlockIDs[maxSourceBlocks:] {
					coveredBlocks[blockID] = false
				}
				group.sourceBlockIDs = group.sourceBlockIDs[:maxSourceBlocks]
			}
			for planIndex < len(approvedEntries) && usedPlanKeys[strings.TrimSpace(approvedEntries[planIndex].Key)] {
				planIndex++
			}
			if planIndex >= len(approvedEntries) {
				continue
			}
			entry := approvedEntries[planIndex]
			planIndex++
			selectionID := fmt.Sprintf("automatic-plan-%d", len(result.RepaymentPlanSelections)+1)
			selection := creativePreAdaptationRepaymentPlanSelection{
				ID: selectionID, PlanKey: strings.TrimSpace(entry.Key), Principal: entry.Principal, TenorMonths: entry.TenorMonths,
				Values: creativePreAdaptationRepaymentPlanValues{
					Principal: creativeFormatRupiah(entry.Principal), Tenor: fmt.Sprintf("%d Bulan", entry.TenorMonths),
					TotalInterest: creativeFormatRupiah(entry.TotalInterest), TotalRepayment: creativeFormatRupiah(entry.TotalRepayment),
					MonthlyInstallment: creativeFormatRupiah(entry.MonthlyInstallment),
				},
			}
			result.RepaymentPlanSelections = append(result.RepaymentPlanSelections, selection)
			usedPlanKeys[selection.PlanKey] = true
			instructionParts := make([]string, 0, len(group.columns))
			for _, column := range group.columns {
				label := ""
				value := ""
				switch column {
				case "principal":
					label, value = library.RepaymentPlan.Labels.Principal, selection.Values.Principal
				case "tenor":
					label, value = library.RepaymentPlan.Labels.Tenor, selection.Values.Tenor
				case "monthly_installment":
					label, value = library.RepaymentPlan.Labels.MonthlyInstallment, selection.Values.MonthlyInstallment
				case "total_interest":
					label, value = library.RepaymentPlan.Labels.TotalInterest, selection.Values.TotalInterest
				case "total_repayment":
					label, value = library.RepaymentPlan.Labels.TotalRepayment, selection.Values.TotalRepayment
				}
				instructionParts = append(instructionParts, strings.TrimSpace(label+" "+value))
			}
			for _, blockID := range group.sourceBlockIDs {
				coveredBlocks[blockID] = true
			}
			result.NumericLayouts = append(result.NumericLayouts, creativePreAdaptationNumericLayout{
				ID: fmt.Sprintf("automatic-%s-%d", strings.TrimSpace(region.ID), groupIndex+1), VisualRegionID: strings.TrimSpace(region.ID), SourceBlockIDs: append([]string(nil), group.sourceBlockIDs...),
				Location: strings.TrimSpace(region.Location), LayoutKind: "single_card", ScenarioIDs: []string{selectionID}, TargetColumns: append([]string(nil), group.columns...),
				RenderInstruction: strings.Join(instructionParts, "；"),
			})
		}
	}

	usedCopyKeys := map[string]bool{}
	for _, block := range source.TextBlocks {
		if coveredBlocks[block.ID] {
			continue
		}
		region, exists := regionsByBlockID[block.ID]
		if !exists {
			return creativePreAdaptationCompletedResult{}, fmt.Errorf("source block %q is not covered by a visual region", block.ID)
		}
		replacement := creativePreAdaptationTextReplacement{
			BlockID: block.ID, VisualRegionID: region.ID, Location: block.Location, Role: block.Role, SourceText: block.SourceText,
			Status: "missing", Note: "该区块未匹配到可渲染的冻结方案，确认页可选择内容或留空移除。",
		}
		if (!hasVisualRegions || region.Kind == "copy") && block.Role != "plan_field" {
			semanticKind := creativeSourceTextBlockSemanticKind(block)
			if fragment, ok := creativePreAdaptationSafeCopyFallback(library, block.Role, block.SourceText, semanticKind, usedCopyKeys, "repayment_plan"); ok {
				usedCopyKeys[fragment.key] = true
				replacement.ReplacementText = fragment.text
				replacement.SourceKeys = []string{fragment.key}
				replacement.Status = "ready"
				replacement.Note = "平台按冻结文案库自动完成普通文案映射。"
			} else if text, basis, ok := creativePreAdaptationGeneratedRecommendation(library, block.Role, semanticKind); ok {
				replacement.ReplacementText = text
				replacement.RecommendationBasis = basis
				replacement.Status = "recommended"
				replacement.Note = "冻结文案库没有兼容的已审核片段，平台生成候选供用户确认。"
			}
		}
		result.TextReplacements = append(result.TextReplacements, replacement)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return creativePreAdaptationCompletedResult{}, err
	}
	if err := validateCompletedCreativePreAdaptation(sourceRaw, encoded); err != nil {
		return creativePreAdaptationCompletedResult{}, err
	}
	if err := validateCreativePreAdaptationCopyBindings(sourceRaw, encoded, copyLibraryRaw); err != nil {
		return creativePreAdaptationCompletedResult{}, err
	}
	return result, nil
}

func (h *Handler) repairCreativePreAdaptationAutomatically(ctx context.Context, workspaceID, analysisID pgtype.UUID, taskContext creativePreAdaptationTaskContext) (bool, error) {
	analysis, err := h.loadCreativeSourceAnalysisByID(ctx, workspaceID, analysisID)
	if err != nil {
		return false, err
	}
	marketPackID, err := parseUUIDString(strings.TrimSpace(taskContext.MarketPackID))
	if err != nil {
		return false, err
	}
	copyLibraryID, err := parseUUIDString(strings.TrimSpace(taskContext.CopyLibraryID))
	if err != nil {
		return false, err
	}
	marketPack, err := h.loadCreativeResourceVersion(ctx, workspaceID, marketPackID, "market_pack", taskContext.MarketPackVersion)
	if err != nil {
		return false, err
	}
	copyLibrary, err := h.loadCreativeResourceVersion(ctx, workspaceID, copyLibraryID, "copy_library", taskContext.CopyLibraryVersion)
	if err != nil {
		return false, err
	}
	result, err := buildAutomaticCreativePreAdaptationResult(analysis.Result, taskContext, copyLibrary.Config)
	if err != nil {
		return false, err
	}
	encodedResult, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	encodedAdaptation, err := json.Marshal(map[string]any{
		"status": "completed", "summary": "平台已完成可渲染的部分映射，未匹配区块保留给确认页选择。", "result": json.RawMessage(encodedResult),
		"error_code": "", "error_message": "",
	})
	if err != nil {
		return false, err
	}
	updated, err := h.DB.Exec(ctx, `
UPDATE creative_source_analysis
SET result = jsonb_set(result, '{adaptation}', $3::jsonb, true)
WHERE id = $1 AND workspace_id = $2
  AND COALESCE(result->'adaptation'->>'status', '') <> 'completed'
`, analysisID, workspaceID, encodedAdaptation)
	if err != nil {
		return false, err
	}
	if updated.RowsAffected() == 0 {
		return false, nil
	}
	h.publishCreativeMaterialsUpdated(workspaceID, pgtype.UUID{}, "system", "")
	slog.Info("automatic pre-adaptation numeric repair completed", "workspace_id", uuidToString(workspaceID), "source_analysis_id", uuidToString(analysisID), "market_pack_version", marketPack.PublishedVersion, "copy_library_version", copyLibrary.PublishedVersion)
	return true, nil
}

func (h *Handler) markCreativePreAdaptationAutomaticRepairAttempted(ctx context.Context, workspaceID, analysisID pgtype.UUID) error {
	_, err := h.DB.Exec(ctx, `
UPDATE creative_source_analysis
SET result = jsonb_set(result, '{adaptation,automatic_repair_attempted}', 'true'::jsonb, true)
WHERE id = $1 AND workspace_id = $2
  AND result->'adaptation'->>'status' = 'unavailable'
`, analysisID, workspaceID)
	return err
}

func (h *Handler) recoverFailedCreativePreAdaptationTask(ctx context.Context, task db.AgentTaskQueue, workspaceRaw, errorMessage string) (string, error) {
	if !task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != creativePreAdaptationEvidenceKind ||
		task.Status != "failed" {
		return "", nil
	}
	if task.FailureReason.Valid {
		switch strings.TrimSpace(task.FailureReason.String) {
		case "cancelled", "user_cancelled", "manual":
			return "", nil
		}
	}
	if task.Attempt < 1 {
		return "", nil
	}
	recoveryTaskID, err := h.retryCreativePreAdaptationOutput(ctx, task)
	if err != nil || recoveryTaskID != "" {
		return recoveryTaskID, err
	}
	if task.MaxAttempts < 1 || task.Attempt < task.MaxAttempts {
		return "", nil
	}
	var taskContext creativePreAdaptationTaskContext
	if err := json.Unmarshal(task.Context, &taskContext); err != nil || taskContext.Workflow != "creative_pre_adaptation" {
		return "", nil
	}
	analysisID, err := parseUUIDString(strings.TrimSpace(taskContext.SourceAnalysisID))
	if err != nil {
		return "", err
	}
	workspaceID, err := parseUUIDString(strings.TrimSpace(workspaceRaw))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(errorMessage) == "" {
		errorMessage = "预适配任务在自动恢复后仍未产出可校验结果。"
	}
	return "", h.markCreativePreAdaptationManualRequired(ctx, workspaceID, analysisID, taskContext, errorMessage)
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
	library, err := loadPublishedCreativeResource(ctx, h.DB, workspaceID, libraryID, "copy_library")
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
	var adaptationEnvelope struct {
		Status string `json:"status"`
	}
	var analysisEnvelope struct {
		Adaptation *json.RawMessage `json:"adaptation"`
	}
	if err := json.Unmarshal(analysis.Result, &analysisEnvelope); err != nil {
		writeError(w, http.StatusConflict, "source analysis adaptation state is invalid")
		return
	}
	adaptationStatus := ""
	if analysisEnvelope.Adaptation != nil && json.Unmarshal(*analysisEnvelope.Adaptation, &adaptationEnvelope) == nil {
		adaptationStatus = strings.TrimSpace(adaptationEnvelope.Status)
	}
	if adaptationStatus == "completed" {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": "", "status": "completed"})
		return
	}
	var activeTaskID pgtype.UUID
	var activeTaskStatus string
	if err := h.DB.QueryRow(r.Context(), `
SELECT id, status
FROM agent_task_queue
WHERE trigger_evidence_kind = $2
  AND trigger_evidence_ref_id = $1
  AND context->>'workflow' = 'creative_pre_adaptation'
  AND context->>'market_pack_id' = $3
  AND status IN ('pending', 'claimed', 'running')
ORDER BY created_at DESC
LIMIT 1
`, analysisID, creativePreAdaptationEvidenceKind, uuidToString(marketPackID)).Scan(&activeTaskID, &activeTaskStatus); err == nil {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": uuidToString(activeTaskID), "status": activeTaskStatus})
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to check existing pre-adaptation task")
		return
	}
	if !creativePreAdaptationSourceHasVisualRegions(analysis.Result) {
		writeError(w, http.StatusConflict, "source analysis must be updated with visual regions before preparing copy")
		return
	}
	marketPack, err := loadPublishedCreativeResource(r.Context(), h.DB, workspaceID, marketPackID, "market_pack")
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
	copyLibrary, err := loadPublishedCreativeResource(r.Context(), h.DB, workspaceID, copyLibraryID, "copy_library")
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
		if err := validateCompletedCreativePreAdaptation(analysis.Result, input.Result); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		normalizedResult, _, normalizeErr := promoteCreativePreAdaptationRecommendations(analysis.Result, input.Result, copyLibrary.Config)
		if normalizeErr != nil {
			writeError(w, http.StatusBadRequest, "completed pre-adaptation recommendation is invalid: "+normalizeErr.Error())
			return
		}
		input.Result = normalizedResult
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
	response := map[string]any{"status": "ok"}
	if input.Status != "completed" && creativePreAdaptationSourceStructureRecoverable(input.ErrorCode) {
		if taskID, queued, recoverErr := h.queueCreativePreAdaptationSourceRecovery(r, workspaceID, userID, analysisID); recoverErr != nil {
			response["auto_recovery_error"] = recoverErr.Error()
		} else if queued {
			response["auto_recovery_task_id"] = taskID
		}
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "pre_adaptation", "source_analysis_id": uuidToString(analysisID)})
	writeJSON(w, http.StatusOK, response)
}

// Structural source-analysis errors are recoverable by producing a fresh
// market-neutral analysis. The pre-adaptation result remains persisted as
// evidence, while this bounded handoff prevents a user from having to click
// "重新分析" after a model misclassified visual regions.
func creativePreAdaptationSourceStructureRecoverable(errorCode string) bool {
	switch strings.TrimSpace(errorCode) {
	case "SOURCE_ANALYSIS_MIXED_NUMERIC_REGION", "SOURCE_ANALYSIS_REGION_CONTRACT_CONFLICT", "SOURCE_ANALYSIS_VISUAL_REGION_INVALID":
		return true
	default:
		return false
	}
}

func (h *Handler) queueCreativePreAdaptationSourceRecovery(r *http.Request, workspaceID, userID, analysisID pgtype.UUID) (string, bool, error) {
	ctx := r.Context()
	var candidateID pgtype.UUID
	var connectorID string
	if err := h.DB.QueryRow(ctx, `
SELECT analysis.candidate_id, candidate.connector_id
FROM creative_source_analysis analysis
JOIN creative_material_candidate candidate
  ON candidate.id = analysis.candidate_id AND candidate.workspace_id = analysis.workspace_id
WHERE analysis.id = $1 AND analysis.workspace_id = $2
`, analysisID, workspaceID).Scan(&candidateID, &connectorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	var previousRecoveries int
	if err := h.DB.QueryRow(ctx, `
SELECT COUNT(*)
FROM creative_source_analysis
WHERE workspace_id = $1
  AND candidate_id = $2
  AND result->'adaptation'->>'error_code' IN ('SOURCE_ANALYSIS_MIXED_NUMERIC_REGION', 'SOURCE_ANALYSIS_REGION_CONTRACT_CONFLICT', 'SOURCE_ANALYSIS_VISUAL_REGION_INVALID')
`, workspaceID, candidateID).Scan(&previousRecoveries); err != nil {
		return "", false, err
	}
	if previousRecoveries > 1 {
		return "", false, nil
	}
	queued := h.enqueueManualReferenceAnalysis(ctx, workspaceID, userID, candidateID, connectorID, true)
	if queued.TaskID == "" {
		if queued.Warning != "" {
			return "", false, errors.New(queued.Warning)
		}
		return "", false, errors.New("reference analysis recovery was not queued")
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope":              "pre_adaptation_auto_recovery_queued",
		"source_analysis_id": uuidToString(analysisID),
		"candidate_id":       uuidToString(candidateID),
		"task_id":            queued.TaskID,
	})
	return queued.TaskID, true, nil
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
		return fmt.Errorf("completed pre-adaptation result is invalid: %w", err)
	}
	if len(adaptation.AnalysisHighlights) < 3 || len(adaptation.AnalysisHighlights) > 5 {
		return errors.New("completed pre-adaptation requires 3-5 analysis highlights")
	}
	for _, highlight := range adaptation.AnalysisHighlights {
		if strings.TrimSpace(highlight) == "" {
			return errors.New("completed pre-adaptation contains an empty analysis highlight")
		}
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
		return fmt.Errorf("completed pre-adaptation result is invalid: %w", err)
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
		return fmt.Errorf("completed pre-adaptation result is invalid: %w", err)
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
				if !valueExists || !creativePreAdaptationInstructionContainsValue(layout.RenderInstruction, value) {
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

func validCreativeCanonicalNumericLayoutKind(value string) bool {
	switch value {
	case "table", "card_grid", "comparison", "single_card", "single_value", "option_buttons", "table_row":
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
