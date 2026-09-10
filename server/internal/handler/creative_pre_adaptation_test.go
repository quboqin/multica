package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCreativePreAdaptationInstructionContainsApprovedValueIgnoresWhitespace(t *testing.T) {
	if !creativePreAdaptationInstructionContainsValue("展示 Jumlah Pinjaman Rp 4.000.000。", "Rp4.000.000") {
		t.Fatal("currency whitespace should not make an approved value disappear from the render instruction")
	}
	if creativePreAdaptationInstructionContainsValue("展示 Rp40.000.000。", "Rp4.000.000") {
		t.Fatal("a different numeric value must not match by whitespace normalization")
	}
}

func TestBuildAutomaticCreativePreAdaptationResultUsesFrozenPlanValues(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","semantic_kind":"copy"},
  {"id":"amount","location":"左侧金额卡","role":"plan_field","source_text":"Rp 15.000.000","semantic_kind":"principal"},
  {"id":"monthly","location":"左侧金额卡","role":"plan_field","source_text":"Rp 1.385.000","semantic_kind":"monthly_installment"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":40,"y":40,"width":800,"height":100}},
  {"id":"loan-card","location":"左侧金额卡","kind":"numeric","source_block_ids":["amount","monthly"],"visual_bounds":{"x":40,"y":220,"width":400,"height":180}}
]}`)
	copyLibrary := json.RawMessage(`{"fragments":[
  {"id":"headline","key":"plan-headline-1","role":"headline","text":"Pembiayaan Fleksibel","creative_types":["repayment_plan"],"status":"approved"}
],"repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
  {"id":"plan-4000000-3","key":"plan-4000000-3","principal":4000000,"tenor_months":3,"monthly_installment":1369333,"total_interest":107999,"total_repayment":4107999,"status":"approved"}
]}}`)
	taskContext := creativePreAdaptationTaskContext{MarketPackID: "market-1", MarketPackVersion: 2, CopyLibraryID: "library-1", CopyLibraryVersion: 3}
	result, err := buildAutomaticCreativePreAdaptationResult(source, taskContext, copyLibrary)
	if err != nil {
		t.Fatalf("automatic repair result: %v", err)
	}
	if len(result.RepaymentPlanSelections) != 1 || len(result.NumericLayouts) != 1 {
		t.Fatalf("automatic repair did not create one frozen plan/card: %#v", result)
	}
	if result.RepaymentPlanSelections[0].Values.Principal != "Rp4.000.000" {
		t.Fatalf("principal was not formatted from the frozen plan: %#v", result.RepaymentPlanSelections[0])
	}
	if result.TextReplacements[0].Status != "ready" || result.TextReplacements[0].SourceKeys[0] != "plan-headline-1" {
		t.Fatalf("ordinary copy was not bound to the frozen library: %#v", result.TextReplacements)
	}
}

func TestBuildAutomaticCreativePreAdaptationResultKeepsUnmatchedRowsEditable(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"principal-1","location":"还款表第1行","role":"plan_field","source_text":"Rp5.000.000","semantic_kind":"principal"},
  {"id":"tenor-1","location":"还款表第1行","role":"plan_field","source_text":"12 Bulan","semantic_kind":"tenor"},
  {"id":"monthly-1","location":"还款表第1行","role":"plan_field","source_text":"Rp500.000","semantic_kind":"monthly_installment"},
  {"id":"principal-2","location":"还款表第2行","role":"plan_field","source_text":"Rp10.000.000","semantic_kind":"principal"},
  {"id":"tenor-2","location":"还款表第2行","role":"plan_field","source_text":"12 Bulan","semantic_kind":"tenor"},
  {"id":"monthly-2","location":"还款表第2行","role":"plan_field","source_text":"Rp1.000.000","semantic_kind":"monthly_installment"},
  {"id":"principal-3","location":"还款表第3行","role":"plan_field","source_text":"Rp20.000.000","semantic_kind":"principal"},
  {"id":"tenor-3","location":"还款表第3行","role":"plan_field","source_text":"12 Bulan","semantic_kind":"tenor"},
  {"id":"monthly-3","location":"还款表第3行","role":"plan_field","source_text":"Rp2.000.000","semantic_kind":"monthly_installment"}
],"visual_regions":[
  {"id":"repayment-table","location":"还款表","kind":"numeric","source_block_ids":["principal-1","tenor-1","monthly-1","principal-2","tenor-2","monthly-2","principal-3","tenor-3","monthly-3"],"visual_bounds":{"x":40,"y":200,"width":800,"height":400}}
]}`)
	copyLibrary := json.RawMessage(`{"repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
  {"id":"plan-a","key":"plan-a","principal":4000000,"tenor_months":3,"monthly_installment":1369333,"total_interest":107999,"total_repayment":4107999,"status":"approved"},
  {"id":"plan-b","key":"plan-b","principal":8000000,"tenor_months":6,"monthly_installment":1405333,"total_interest":432000,"total_repayment":8431998,"status":"approved"}
]}}`)
	taskContext := creativePreAdaptationTaskContext{MarketPackID: "market-1", MarketPackVersion: 2, CopyLibraryID: "library-1", CopyLibraryVersion: 3}
	result, err := buildAutomaticCreativePreAdaptationResult(source, taskContext, copyLibrary)
	if err != nil {
		t.Fatalf("partial automatic recovery result: %v", err)
	}
	if len(result.NumericLayouts) != 2 || len(result.RepaymentPlanSelections) != 2 {
		t.Fatalf("recovery should render the two available rows: %#v", result)
	}
	missing := 0
	for _, replacement := range result.TextReplacements {
		if replacement.Status == "missing" {
			missing++
		}
	}
	if missing != 3 {
		t.Fatalf("unmatched row should remain editable, missing = %d, replacements = %#v", missing, result.TextReplacements)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := validateCompletedCreativePreAdaptation(source, encoded); err != nil {
		t.Fatalf("partial numeric result was rejected: %v", err)
	}
	if err := validateCreativePreAdaptationCopyBindings(source, encoded, copyLibrary); err != nil {
		t.Fatalf("partial numeric result failed frozen-resource validation: %v", err)
	}
}

func TestBuildAutomaticCreativePreAdaptationResultDoesNotGuessNumericHeadlineCopy(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"rate","location":"顶部利率","role":"headline","source_text":"0,01%","semantic_kind":"interest_rate"}
],"visual_regions":[
  {"id":"rate-region","location":"顶部利率","kind":"copy","source_block_ids":["rate"],"visual_bounds":{"x":40,"y":40,"width":800,"height":100}}
]}`)
	copyLibrary := json.RawMessage(`{"fragments":[
  {"id":"headline","key":"plan-headline-1","role":"headline","text":"Pembiayaan Fleksibel","creative_types":["repayment_plan"],"status":"approved"}
],"repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[]}}`)
	result, err := buildAutomaticCreativePreAdaptationResult(source, creativePreAdaptationTaskContext{
		MarketPackID: "market-1", MarketPackVersion: 1, CopyLibraryID: "library-1", CopyLibraryVersion: 1,
	}, copyLibrary)
	if err != nil {
		t.Fatalf("numeric headline should remain editable: %v", err)
	}
	if len(result.TextReplacements) != 1 || result.TextReplacements[0].Status != "missing" {
		t.Fatalf("numeric headline was guessed from generic copy: %#v", result.TextReplacements)
	}
}

func TestBuildAutomaticCreativePreAdaptationGeneratesPendingRecommendationForOrdinaryCopy(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","semantic_kind":"copy"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":40,"y":40,"width":800,"height":100}}
]}`)
	copyLibrary := json.RawMessage(`{"locale":"id-ID","fragments":[],"repayment_plan":{"entries":[]}}`)
	result, err := buildAutomaticCreativePreAdaptationResult(source, creativePreAdaptationTaskContext{
		MarketPackID: "market-1", MarketPackVersion: 1, CopyLibraryID: "library-1", CopyLibraryVersion: 1,
	}, copyLibrary)
	if err != nil {
		t.Fatalf("ordinary copy recommendation should be recoverable: %v", err)
	}
	if len(result.TextReplacements) != 1 {
		t.Fatalf("expected one text replacement, got %#v", result.TextReplacements)
	}
	replacement := result.TextReplacements[0]
	if replacement.Status != "recommended" || replacement.ReplacementText == "" || len(replacement.SourceKeys) != 0 || len(replacement.RecommendationBasis) == 0 {
		t.Fatalf("ordinary copy did not become a traceable pending recommendation: %#v", replacement)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := validateCompletedCreativePreAdaptation(source, encoded); err != nil {
		t.Fatalf("pending recommendation failed structural validation: %v", err)
	}
	if err := validateCreativePreAdaptationCopyBindings(source, encoded, copyLibrary); err != nil {
		t.Fatalf("pending recommendation failed frozen-resource validation: %v", err)
	}
}

func TestPromoteCreativePreAdaptationRecommendationsNormalizesMissingOrdinaryCopy(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","semantic_kind":"copy"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":40,"y":40,"width":800,"height":100}}
]}`)
	adaptation := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":1,
  "copy_library_id":"library-1","copy_library_version":1,
  "analysis_highlights":["素材包含一个标题区块","标题没有匹配到冻结文案","候选需要用户确认"],
  "text_replacements":[{"block_id":"headline","visual_region_id":"headline-region","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","replacement_text":"","source_keys":[],"status":"missing"}],
  "repayment_plan_selections":[],
  "numeric_layouts":[]
}`)
	copyLibrary := json.RawMessage(`{"locale":"id-ID","fragments":[],"repayment_plan":{"entries":[]}}`)
	normalized, changed, err := promoteCreativePreAdaptationRecommendations(source, adaptation, copyLibrary)
	if err != nil {
		t.Fatalf("missing ordinary copy should be normalized: %v", err)
	}
	if !changed {
		t.Fatal("missing ordinary copy should produce a changed result")
	}
	var result creativePreAdaptationCompletedResult
	if err := json.Unmarshal(normalized, &result); err != nil {
		t.Fatal(err)
	}
	replacement := result.TextReplacements[0]
	if replacement.Status != "recommended" || replacement.ReplacementText == "" || len(replacement.SourceKeys) != 0 || len(replacement.RecommendationBasis) == 0 {
		t.Fatalf("missing ordinary copy did not become a pending recommendation: %#v", replacement)
	}
	if err := validateCompletedCreativePreAdaptation(source, normalized); err != nil {
		t.Fatalf("normalized recommendation failed structural validation: %v", err)
	}
	if err := validateCreativePreAdaptationCopyBindings(source, normalized, copyLibrary); err != nil {
		t.Fatalf("normalized recommendation failed frozen-resource validation: %v", err)
	}
}

func TestValidateCompletedCreativePreAdaptationRejectsSourceAnchorMismatch(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"benefit-principal-1","location":"中左气泡","role":"benefit","source_text":"Pinjaman hingga Rp100Juta","semantic_kind":"principal"}
],"visual_regions":[
  {"id":"copy-benefit-principal","location":"中左额度气泡","kind":"copy","source_block_ids":["benefit-principal-1"],"visual_bounds":{"x":40,"y":400,"width":400,"height":120}}
]}`)
	result := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "copy_library_id":"library-1","copy_library_version":3,
  "analysis_highlights":["中部气泡承载额度利益点","住宅场景保持不变","额度文案来自文案库"],
  "text_replacements":[{"block_id":"benefit-principal-1","visual_region_id":"copy-benefit-principal","location":"中左额度气泡","role":"benefit","source_text":"Pinjaman hingga Rp100Juta","replacement_text":"Limit hingga Rp80.000.000","source_keys":["limit"],"status":"ready"}],
  "repayment_plan_selections":[],
  "numeric_layouts":[]
}`)
	if err := validateCompletedCreativePreAdaptation(source, result); err == nil || !strings.Contains(err.Error(), "text replacement does not match source analysis") {
		t.Fatalf("location mismatch should be rejected without a server rewrite, got %v", err)
	}
}

func TestValidateCreativePreAdaptationCopyBindingsRejectsUnapprovedModelCopy(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","semantic_kind":"copy"},
  {"id":"limit-copy","location":"中部卖点","role":"benefit","source_text":"Pinjaman hingga Rp100Juta","semantic_kind":"principal"},
  {"id":"rate-copy","location":"底部利率","role":"benefit","source_text":"Bunga 1%","semantic_kind":"interest_rate"},
  {"id":"amount","location":"金额框","role":"plan_field","source_text":"Rp15.000.000","semantic_kind":"principal"},
  {"id":"tenor","location":"期限按钮","role":"plan_field","source_text":"6 Bulan","semantic_kind":"tenor"},
  {"id":"monthly","location":"月还表","role":"plan_field","source_text":"Rp8.345.000","semantic_kind":"monthly_installment"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":40,"y":40,"width":860,"height":90}},
  {"id":"limit-region","location":"中部卖点","kind":"copy","source_block_ids":["limit-copy"],"visual_bounds":{"x":40,"y":150,"width":860,"height":90}},
  {"id":"rate-region","location":"底部利率","kind":"copy","source_block_ids":["rate-copy"],"visual_bounds":{"x":40,"y":760,"width":860,"height":90}},
  {"id":"loan-region","location":"还款数值区","kind":"numeric","source_block_ids":["amount","tenor","monthly"],"visual_bounds":{"x":60,"y":300,"width":820,"height":320}}
]}`)
	copyLibrary := json.RawMessage(`{
  "fragments":[
    {"id":"headline","key":"headline","text":"Pembiayaan Fleksibel","semantic_group":"","status":"approved"},
    {"id":"limit","key":"limit_80m","text":"Limit hingga Rp80.000.000","semantic_group":"limit","status":"approved"}
  ],
  "repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
    {"id":"plan-row","key":"plan-8000000-6","principal":8000000,"tenor_months":6,"monthly_installment":1405333,"total_interest":432000,"total_repayment":8431998,"source":"approved sheet","status":"approved"}
  ]}
}`)
	result := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "copy_library_id":"library-1","copy_library_version":5,
  "analysis_highlights":["原图保留金融信息框架"],
  "text_replacements":[
    {"block_id":"headline","visual_region_id":"headline-region","location":"错误位置","role":"headline","source_text":"wrong","replacement_text":"WRONG","source_keys":["headline"],"status":"ready"},
    {"block_id":"limit-copy","visual_region_id":"limit-region","location":"错误位置","role":"benefit","source_text":"wrong","replacement_text":"WRONG","source_keys":["limit_80m"],"status":"ready"},
    {"block_id":"rate-copy","visual_region_id":"rate-region","location":"底部利率","role":"benefit","source_text":"Bunga 1%","replacement_text":"Bunga mulai dari 0,03%*","source_keys":["not_in_library"],"status":"ready"},
    {"block_id":"amount","visual_region_id":"loan-region","location":"金额框","role":"plan_field","source_text":"Rp15.000.000","replacement_text":"Limit hingga Rp80.000.000","source_keys":["limit_80m"],"status":"ready"}
  ],
  "repayment_plan_selections":[{"id":"row-six","plan_key":"plan-8000000-6","principal":1,"tenor_months":99,"values":{"principal":"stale","tenor":"stale","total_interest":"stale","total_repayment":"stale","monthly_installment":"stale"}}],
  "numeric_layouts":[{"id":"loan-values","visual_region_id":"wrong","source_block_ids":["amount","tenor","monthly"],"location":"","layout_kind":"loan_summary","scenario_ids":["plan-8000000-6"],"target_columns":["principal","tenor","monthly_installment","unknown"],"render_instruction":"WRONG"}]
}`)
	if err := validateCreativePreAdaptationCopyBindings(source, result, copyLibrary); err == nil {
		t.Fatal("unapproved model copy should be rejected instead of being replaced by the server")
	}
}

func TestValidateCompletedCreativePreAdaptationGroupsSourcePlanFieldsIntoApprovedRows(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[{"id":"top-amount","location":"上方还款表","role":"plan_field","source_text":"Rp50.000.000"},{"id":"top-tenor","location":"上方还款表","role":"plan_field","source_text":"3 / 6 / 9 Bulan"},{"id":"top-monthly","location":"上方还款表","role":"plan_field","source_text":"Rp1.234.567"},{"id":"banner","location":"中部横幅","role":"benefit","source_text":"Flexible"}]}`)
	instruction := "保留上方还款表，用 Jumlah Pinjaman Rp30.000.000、Periode Cicilan 3 Bulan、Cicilan per Bulan Rp10.270.000；Jumlah Pinjaman Rp60.000.000、Periode Cicilan 6 Bulan、Cicilan per Bulan Rp10.540.000 重排。"
	valid := json.RawMessage(`{
	  "market_pack_id":"market-1","market_pack_version":2,
	  "analysis_highlights":["上方是金额期限月供对照","中部横幅承接核心卖点","底部行动区独立"],
  "text_replacements":[{"block_id":"banner","location":"中部横幅","role":"benefit","source_text":"Flexible","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["benefit_flat"],"status":"ready"}],
  "repayment_plan_selections":[
    {"id":"target-30m-3","plan_key":"30m-3","principal":30000000,"tenor_months":3,"values":{"principal":"Rp30.000.000","tenor":"3 Bulan","total_interest":"Rp810.000","total_repayment":"Rp30.810.000","monthly_installment":"Rp10.270.000"}},
    {"id":"target-60m-6","plan_key":"60m-6","principal":60000000,"tenor_months":6,"values":{"principal":"Rp60.000.000","tenor":"6 Bulan","total_interest":"Rp3.240.000","total_repayment":"Rp63.240.000","monthly_installment":"Rp10.540.000"}}
  ],
  "numeric_layouts":[{"id":"top-table","source_block_ids":["top-amount","top-tenor","top-monthly"],"location":"上方还款表","layout_kind":"table","scenario_ids":["target-30m-3","target-60m-6"],"target_columns":["principal","tenor","monthly_installment"],"render_instruction":"` + instruction + `"}]
}`)

	if err := validateCompletedCreativePreAdaptation(source, valid); err != nil {
		t.Fatalf("first-party plan layout was rejected: %v", err)
	}
	missingText := json.RawMessage(strings.Replace(string(valid), `"text_replacements":[{"block_id":"banner","location":"中部横幅","role":"benefit","source_text":"Flexible","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["benefit_flat"],"status":"ready"}]`, `"text_replacements":[]`, 1))
	if err := validateCompletedCreativePreAdaptation(source, missingText); err == nil {
		t.Fatal("a non-numeric source block must still require a text replacement")
	}
	missingReplacement := json.RawMessage(strings.Replace(string(valid), `{"block_id":"banner","location":"中部横幅","role":"benefit","source_text":"Flexible","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["benefit_flat"],"status":"ready"}`, `{"block_id":"banner","location":"中部横幅","role":"benefit","source_text":"Flexible","replacement_text":"","source_keys":[],"status":"missing"}`, 1))
	if err := validateCompletedCreativePreAdaptation(source, missingReplacement); err != nil {
		t.Fatalf("an explicit unfilled text block should stay editable instead of failing the whole adaptation: %v", err)
	}
}

func TestCreativePreAdaptationManualRequiredMatchesFrozenResources(t *testing.T) {
	source := json.RawMessage(`{"adaptation":{"status":"unavailable","error_code":"manual_confirmation_required","result":{"market_pack_id":"market-1","market_pack_version":2,"copy_library_id":"library-1","copy_library_version":3}}}`)
	marketPack := creativeResourceResponse{ID: "market-1", PublishedVersion: 2}
	copyLibrary := creativeResourceResponse{ID: "library-1", PublishedVersion: 3}
	if !creativePreAdaptationManualRequiredForResources(source, marketPack, copyLibrary) {
		t.Fatal("matching manual-required adaptation was not recognized")
	}
	if creativePreAdaptationManualRequiredForResources(source, marketPack, creativeResourceResponse{ID: "library-1", PublishedVersion: 4}) {
		t.Fatal("manual-required adaptation from another frozen copy library version matched")
	}
}

func TestCreativePreAdaptationArtifactAcceptanceRequiresCompletedOrExplicitManualState(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		errorCode  string
		frozen     bool
		wantAccept bool
	}{
		{name: "completed", status: "completed", frozen: true, wantAccept: true},
		{name: "completed with another resource", status: "completed", frozen: false},
		{name: "manual confirmation", status: "unavailable", errorCode: "manual_confirmation_required", frozen: true, wantAccept: true},
		{name: "source recovery queued", status: "unavailable", errorCode: "SOURCE_ANALYSIS_MIXED_NUMERIC_REGION", frozen: true, wantAccept: true},
		{name: "recoverable unavailable", status: "unavailable", errorCode: "source_analysis_invalid", frozen: true},
		{name: "failed", status: "failed", errorCode: "agent_error", frozen: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := creativePreAdaptationArtifactAccepted(test.status, test.errorCode, test.frozen); got != test.wantAccept {
				t.Fatalf("accepted = %v, want %v", got, test.wantAccept)
			}
		})
	}
}

func TestCreativePreAdaptationTaskRetryableHonorsCancellationAndAttemptBudget(t *testing.T) {
	task := db.AgentTaskQueue{
		Status:               "failed",
		Attempt:              1,
		MaxAttempts:          2,
		TriggerEvidenceKind:  pgtype.Text{String: creativePreAdaptationEvidenceKind, Valid: true},
		TriggerEvidenceRefID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
	}
	if !creativePreAdaptationTaskRetryable(task) {
		t.Fatal("a failed pre-adaptation task with budget should be retryable")
	}
	for _, reason := range []string{"cancelled", "user_cancelled", "manual"} {
		task.FailureReason = pgtype.Text{String: reason, Valid: true}
		if creativePreAdaptationTaskRetryable(task) {
			t.Fatalf("failure reason %q should not be retried", reason)
		}
	}
	task.FailureReason = pgtype.Text{}
	task.Attempt = 2
	if creativePreAdaptationTaskRetryable(task) {
		t.Fatal("an exhausted pre-adaptation task should not be retried")
	}
}

func TestValidateCompletedCreativePreAdaptationRequiresOneVisualRegionPerSourceBlock(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Flexible"},
  {"id":"amount-label","location":"左侧金额卡","role":"supporting","source_text":"Jumlah Pinjaman"},
  {"id":"amount","location":"左侧金额卡","role":"plan_field","source_text":"Rp10.000.000"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":50,"y":50,"width":600,"height":100}},
  {"id":"amount-region","location":"左侧金额卡","kind":"numeric","source_block_ids":["amount-label","amount"],"visual_bounds":{"x":50,"y":250,"width":350,"height":180}}
]}`)
	instruction := "在左侧金额卡中使用 Jumlah Pinjaman Rp30.000.000。"
	valid := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "analysis_highlights":["顶部是标题","左侧是金额卡","金额来自同一还款行"],
  "text_replacements":[{"block_id":"headline","visual_region_id":"headline-region","location":"顶部标题","role":"headline","source_text":"Flexible","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["benefit_flat"],"status":"ready"}],
  "repayment_plan_selections":[{"id":"row-1","plan_key":"30m-3","principal":30000000,"tenor_months":3,"values":{"principal":"Rp30.000.000","tenor":"3 Bulan","total_interest":"Rp810.000","total_repayment":"Rp30.810.000","monthly_installment":"Rp10.270.000"}}],
  "numeric_layouts":[{"id":"amount-card","visual_region_id":"amount-region","source_block_ids":["amount-label","amount"],"location":"左侧金额卡","layout_kind":"single_card","scenario_ids":["row-1"],"target_columns":["principal"],"render_instruction":"` + instruction + `"}]
}`)
	if err := validateCompletedCreativePreAdaptation(source, valid); err != nil {
		t.Fatalf("visual-region plan was rejected: %v", err)
	}
	wrongRegion := json.RawMessage(strings.Replace(string(valid), `"visual_region_id":"amount-region","source_block_ids":["amount-label","amount"]`, `"visual_region_id":"headline-region","source_block_ids":["amount-label","amount"]`, 1))
	if err := validateCompletedCreativePreAdaptation(source, wrongRegion); err == nil {
		t.Fatal("numeric layout was accepted for a different visual region")
	}
}

func TestValidateCompletedCreativePreAdaptationAcceptsFineGrainedNumericLayouts(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","semantic_kind":"copy"},
  {"id":"amount","location":"中上金额框","role":"plan_field","source_text":"Rp 15.000.000","semantic_kind":"principal"},
  {"id":"tenor-3","location":"期限按钮左","role":"plan_field","source_text":"[3 Bulan]","semantic_kind":"tenor"},
  {"id":"tenor-6","location":"期限按钮中","role":"plan_field","source_text":"[6 Bulan]","semantic_kind":"tenor"},
  {"id":"row-value","location":"还款表右侧","role":"plan_field","source_text":"Rp 8.345.000","semantic_kind":"monthly_installment"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":50,"y":50,"width":800,"height":80}},
  {"id":"amount-region","location":"中上金额框","kind":"numeric","source_block_ids":["amount"],"visual_bounds":{"x":50,"y":180,"width":800,"height":120}},
  {"id":"tenor-region","location":"期限按钮组","kind":"numeric","source_block_ids":["tenor-3","tenor-6"],"visual_bounds":{"x":50,"y":320,"width":800,"height":80}},
  {"id":"row-region","location":"还款表右侧","kind":"numeric","source_block_ids":["row-value"],"visual_bounds":{"x":520,"y":520,"width":360,"height":70}}
]}`)
	amountInstruction := "在中上金额框展示 Rp8.000.000。"
	tenorInstruction := "期限按钮依次展示 3 Bulan、6 Bulan。"
	rowInstruction := "在还款表右侧展示 Rp1.405.333。"
	result := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "copy_library_id":"library-1","copy_library_version":3,
  "analysis_highlights":["顶部保留主标题","金额框为单值区域","期限为按钮组","表格右侧为单行数值"],
  "text_replacements":[{"block_id":"headline","visual_region_id":"headline-region","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","replacement_text":"Pembiayaan Fleksibel","source_keys":["headline"],"status":"ready"}],
  "repayment_plan_selections":[
    {"id":"row-3","plan_key":"8m-3","principal":8000000,"tenor_months":3,"values":{"principal":"Rp8.000.000","tenor":"3 Bulan","total_interest":"Rp216.000","total_repayment":"Rp8.216.000","monthly_installment":"Rp2.738.667"}},
    {"id":"row-6","plan_key":"8m-6","principal":8000000,"tenor_months":6,"values":{"principal":"Rp8.000.000","tenor":"6 Bulan","total_interest":"Rp432.000","total_repayment":"Rp8.432.000","monthly_installment":"Rp1.405.333"}}
  ],
  "numeric_layouts":[
    {"id":"amount-layout","visual_region_id":"amount-region","source_block_ids":["amount"],"location":"中上金额框","layout_kind":"single_value","scenario_ids":["row-6"],"target_columns":["principal"],"render_instruction":"` + amountInstruction + `"},
    {"id":"tenor-layout","visual_region_id":"tenor-region","source_block_ids":["tenor-3","tenor-6"],"location":"期限按钮组","layout_kind":"option_buttons","scenario_ids":["row-3","row-6"],"target_columns":["tenor"],"render_instruction":"` + tenorInstruction + `"},
    {"id":"row-layout","visual_region_id":"row-region","source_block_ids":["row-value"],"location":"还款表右侧","layout_kind":"table_row","scenario_ids":["row-6"],"target_columns":["monthly_installment"],"render_instruction":"` + rowInstruction + `"}
  ]
}`)
	if err := validateCompletedCreativePreAdaptation(source, result); err != nil {
		t.Fatalf("fine-grained numeric layouts were rejected: %v", err)
	}
	copyLibrary := json.RawMessage(`{
  "fragments":[{"id":"headline","key":"headline","text":"Pembiayaan Fleksibel","status":"approved"}],
  "repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
    {"id":"plan-8m-3","key":"8m-3","principal":8000000,"tenor_months":3,"monthly_installment":2738667,"total_interest":216000,"total_repayment":8216000,"source":"approved sheet","status":"approved"},
    {"id":"plan-8m-6","key":"8m-6","principal":8000000,"tenor_months":6,"monthly_installment":1405333,"total_interest":432000,"total_repayment":8432000,"source":"approved sheet","status":"approved"}
  ]}
}`)
	if err := validateCreativePreAdaptationCopyBindings(source, result, copyLibrary); err != nil {
		t.Fatalf("fine-grained numeric layouts failed frozen copy binding: %v", err)
	}
}

func TestValidateCompletedCreativePreAdaptationAcceptsLoanSemanticNumericLayouts(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","semantic_kind":"copy"},
  {"id":"amount","location":"中部本金卡","role":"plan_field","source_text":"Rp 15.000.000","semantic_kind":"principal"},
  {"id":"tenor-3","location":"期限按钮左","role":"plan_field","source_text":"[3 Bulan]","semantic_kind":"tenor"},
  {"id":"tenor-6","location":"期限按钮中","role":"plan_field","source_text":"[6 Bulan]","semantic_kind":"tenor"},
  {"id":"summary-principal","location":"摘要表右列第一行","role":"plan_field","source_text":"Rp 8.000.000","semantic_kind":"principal"},
  {"id":"summary-tenor","location":"摘要表右列第二行","role":"plan_field","source_text":"6 Bulan","semantic_kind":"tenor"},
  {"id":"summary-total","location":"摘要表右列第四行","role":"plan_field","source_text":"Rp 8.345.000","semantic_kind":"total_repayment"}
],"visual_regions":[
  {"id":"headline-region","location":"顶部标题","kind":"copy","source_block_ids":["headline"],"visual_bounds":{"x":50,"y":50,"width":800,"height":80}},
  {"id":"amount-region","location":"中部本金卡","kind":"numeric","source_block_ids":["amount"],"visual_bounds":{"x":50,"y":180,"width":800,"height":120}},
  {"id":"tenor-region","location":"期限按钮组","kind":"numeric","source_block_ids":["tenor-3","tenor-6"],"visual_bounds":{"x":50,"y":320,"width":800,"height":80}},
  {"id":"summary-region","location":"摘要表右列","kind":"numeric","source_block_ids":["summary-principal","summary-tenor","summary-total"],"visual_bounds":{"x":520,"y":520,"width":360,"height":260}}
]}`)
	amountInstruction := "在中部本金卡展示 Rp8.000.000。"
	tenorInstruction := "期限按钮展示 3 Bulan、6 Bulan。"
	summaryInstruction := "摘要表右列展示本金 Rp8.000.000、期限 6 Bulan、总还款 Rp8.431.998。"
	result := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "copy_library_id":"library-1","copy_library_version":3,
  "analysis_highlights":["顶部保留主标题","本金卡展示单值","期限为按钮组","摘要表右列展示还款值"],
  "text_replacements":[{"block_id":"headline","visual_region_id":"headline-region","location":"顶部标题","role":"headline","source_text":"Pinjaman Cepat","replacement_text":"Pembiayaan Fleksibel","source_keys":["headline"],"status":"ready"}],
  "repayment_plan_selections":[
    {"id":"row-3","plan_key":"8m-3","principal":8000000,"tenor_months":3,"values":{"principal":"Rp8.000.000","tenor":"3 Bulan","total_interest":"Rp216.001","total_repayment":"Rp8.216.001","monthly_installment":"Rp2.738.667"}},
    {"id":"row-6","plan_key":"8m-6","principal":8000000,"tenor_months":6,"values":{"principal":"Rp8.000.000","tenor":"6 Bulan","total_interest":"Rp431.998","total_repayment":"Rp8.431.998","monthly_installment":"Rp1.405.333"}}
  ],
  "numeric_layouts":[
    {"id":"amount-layout","visual_region_id":"amount-region","source_block_ids":["amount"],"location":"中部本金卡","layout_kind":"single_value","scenario_ids":["row-6"],"target_columns":["principal"],"render_instruction":"` + amountInstruction + `"},
    {"id":"tenor-layout","visual_region_id":"tenor-region","source_block_ids":["tenor-3","tenor-6"],"location":"期限按钮组","layout_kind":"option_buttons","scenario_ids":["row-3","row-6"],"target_columns":["tenor"],"render_instruction":"` + tenorInstruction + `"},
    {"id":"summary-layout","visual_region_id":"summary-region","source_block_ids":["summary-principal","summary-tenor","summary-total"],"location":"摘要表右列","layout_kind":"table","scenario_ids":["row-6"],"target_columns":["principal","tenor","total_repayment"],"render_instruction":"` + summaryInstruction + `"}
  ]
}`)
	if err := validateCompletedCreativePreAdaptation(source, result); err != nil {
		t.Fatalf("loan-semantic numeric layouts were rejected: %v", err)
	}
	copyLibrary := json.RawMessage(`{
  "fragments":[{"id":"headline","key":"headline","text":"Pembiayaan Fleksibel","status":"approved"}],
  "repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
    {"id":"plan-8m-3","key":"8m-3","principal":8000000,"tenor_months":3,"monthly_installment":2738667,"total_interest":216001,"total_repayment":8216001,"source":"approved sheet","status":"approved"},
    {"id":"plan-8m-6","key":"8m-6","principal":8000000,"tenor_months":6,"monthly_installment":1405333,"total_interest":431998,"total_repayment":8431998,"source":"approved sheet","status":"approved"}
  ]}
}`)
	if err := validateCreativePreAdaptationCopyBindings(source, result, copyLibrary); err != nil {
		t.Fatalf("loan-semantic numeric layouts failed frozen copy binding: %v", err)
	}
}

func TestValidateCompletedCreativePreAdaptationRequiresNonRepaymentPlanFieldsToBeFilled(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"name-label","location":"中部左上","role":"plan_field","source_text":"Nama"},
  {"id":"name-value","location":"中部右上","role":"plan_field","source_text":"Nabila"},
  {"id":"amount-label","location":"中部左下","role":"plan_field","source_text":"Pinjaman"},
  {"id":"amount-value","location":"中部右下","role":"plan_field","source_text":"Rp10.000.000"},
  {"id":"tenor-label","location":"中部左下","role":"plan_field","source_text":"Tenor"},
  {"id":"tenor-value","location":"中部右下","role":"plan_field","source_text":"12 Bulan"},
  {"id":"monthly-label","location":"中部左下","role":"plan_field","source_text":"Cicilan per Bulan"},
  {"id":"monthly-value","location":"中部右下","role":"plan_field","source_text":"Rp923.333"}
]}`)
	instruction := "使用 Jumlah Pinjaman Rp30.000.000、Periode Cicilan 3 Bulan、Cicilan per Bulan Rp10.270.000。"
	valid := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "analysis_highlights":["中部为左右标签和值","金额期限月供必须同一行","顶部保留标题层级"],
  "text_replacements":[
    {"block_id":"name-label","location":"中部左上","role":"plan_field","source_text":"Nama","replacement_text":"Tanpa Jaminan","source_keys":["no_collateral"],"status":"ready"},
    {"block_id":"name-value","location":"中部右上","role":"plan_field","source_text":"Nabila","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["flat"],"status":"ready"}
  ],
  "repayment_plan_selections":[{"id":"row-1","plan_key":"30m-3","principal":30000000,"tenor_months":3,"values":{"principal":"Rp30.000.000","tenor":"3 Bulan","total_interest":"Rp810.000","total_repayment":"Rp30.810.000","monthly_installment":"Rp10.270.000"}}],
  "numeric_layouts":[{"id":"loan-summary","source_block_ids":["amount-label","amount-value","tenor-label","tenor-value","monthly-label","monthly-value"],"location":"中部借款摘要","layout_kind":"table","scenario_ids":["row-1"],"target_columns":["principal","tenor","monthly_installment"],"render_instruction":"` + instruction + `"}]
}`)
	if err := validateCompletedCreativePreAdaptation(source, valid); err != nil {
		t.Fatalf("approved replacements for non-repayment plan fields were rejected: %v", err)
	}

	invalid := json.RawMessage(strings.Replace(string(valid), `"source_block_ids":["amount-label","amount-value","tenor-label","tenor-value","monthly-label","monthly-value"]`, `"source_block_ids":["name-label","name-value","amount-label","amount-value","tenor-label","tenor-value","monthly-label","monthly-value"]`, 1))
	if err := validateCompletedCreativePreAdaptation(source, invalid); err == nil {
		t.Fatal("unrelated plan-field text was incorrectly accepted as part of a three-column repayment layout")
	}
}

func TestValidateCreativePreAdaptationCopyBindingsRequiresAnExactApprovedPlanRow(t *testing.T) {
	copyLibrary := json.RawMessage(`{
  "fragments":[{"id":"benefit-flat","key":"benefit_flat","text":"Cicilan FLAT sepanjang Tenor","status":"approved"}],
  "repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
    {"id":"plan-30m-3","key":"30m-3","principal":30000000,"tenor_months":3,"monthly_installment":10270000,"total_interest":810000,"total_repayment":30810000,"source":"approved sheet","status":"approved"},
    {"id":"plan-60m-6","key":"60m-6","principal":60000000,"tenor_months":6,"monthly_installment":10540000,"total_interest":3240000,"total_repayment":63240000,"source":"approved sheet","status":"approved"}
  ]}
}`)
	instruction := "使用 Jumlah Pinjaman Rp30.000.000、Periode Cicilan 3 Bulan、Cicilan per Bulan Rp10.270.000；Jumlah Pinjaman Rp60.000.000、Periode Cicilan 6 Bulan、Cicilan per Bulan Rp10.540.000。"
	adaptation := json.RawMessage(`{
	  "market_pack_id":"market-1","market_pack_version":2,
	  "copy_library_id":"library-1","copy_library_version":4,
  "text_replacements":[{"block_id":"banner","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["benefit_flat"]}],
  "repayment_plan_selections":[
    {"id":"target-30m-3","plan_key":"30m-3","principal":30000000,"tenor_months":3,"values":{"principal":"Rp30.000.000","tenor":"3 Bulan","total_interest":"Rp810.000","total_repayment":"Rp30.810.000","monthly_installment":"Rp10.270.000"}},
    {"id":"target-60m-6","plan_key":"60m-6","principal":60000000,"tenor_months":6,"values":{"principal":"Rp60.000.000","tenor":"6 Bulan","total_interest":"Rp3.240.000","total_repayment":"Rp63.240.000","monthly_installment":"Rp10.540.000"}}
  ],
  "numeric_layouts":[{"id":"top-table","source_block_ids":["top-amount","top-tenor","top-monthly"],"location":"上方还款表","layout_kind":"table","scenario_ids":["target-30m-3","target-60m-6"],"target_columns":["principal","tenor","monthly_installment"],"render_instruction":"` + instruction + `"}]
}`)
	source := json.RawMessage(`{"text_blocks":[{"id":"banner","location":"中部横幅","role":"benefit","source_text":"Flexible"}]}`)
	if err := validateCreativePreAdaptationCopyBindings(source, adaptation, copyLibrary); err != nil {
		t.Fatalf("valid approved plan layout rejected: %v", err)
	}
	incorrectMonthly := json.RawMessage(strings.Replace(string(adaptation), "Rp10.540.000", "Rp9.999.999", 2))
	if err := validateCreativePreAdaptationCopyBindings(source, incorrectMonthly, copyLibrary); err == nil {
		t.Fatal("a monthly payment outside the approved plan row was accepted")
	}
}

func TestValidateCreativePreAdaptationCopyBindingsRejectsSemanticMismatchAndDuplicateCopy(t *testing.T) {
	copyLibrary := json.RawMessage(`{
  "fragments":[
    {"id":"rate","key":"rate","text":"Bunga mulai dari 0,03%*","semantic_group":"interest_rate","status":"approved"},
    {"id":"flat","key":"flat","text":"Cicilan FLAT sepanjang Tenor","status":"approved"}
  ]
}`)
	source := json.RawMessage(`{"text_blocks":[
  {"id":"daily-amount","location":"中心卡","role":"benefit","source_text":"Rp10.000","purpose":"展示日利息金额"},
  {"id":"rate-value","location":"利益条","role":"benefit","source_text":"0,1%","purpose":"展示固定利率数值"},
  {"id":"benefit-a","location":"横幅 A","role":"benefit","source_text":"A"},
  {"id":"benefit-b","location":"横幅 B","role":"benefit","source_text":"B"}
]}`)
	semanticMismatch := json.RawMessage(`{
  "copy_library_id":"library-1","copy_library_version":4,
  "text_replacements":[{"block_id":"daily-amount","replacement_text":"Bunga mulai dari 0,03%*","source_keys":["rate"],"status":"ready"}]
}`)
	if err := validateCreativePreAdaptationCopyBindings(source, semanticMismatch, copyLibrary); err == nil || !strings.Contains(err.Error(), "daily_interest_amount") {
		t.Fatalf("daily interest amount mapped to a rate was accepted: %v", err)
	}
	duplicate := json.RawMessage(`{
  "copy_library_id":"library-1","copy_library_version":4,
  "text_replacements":[
    {"block_id":"benefit-a","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["flat"],"status":"ready"},
    {"block_id":"benefit-b","replacement_text":"Cicilan FLAT sepanjang Tenor","source_keys":["flat"],"status":"ready"}
  ]
}`)
	if err := validateCreativePreAdaptationCopyBindings(source, duplicate, copyLibrary); err == nil || !strings.Contains(err.Error(), "reuse approved copy source") {
		t.Fatalf("one approved copy source was reused across two blocks: %v", err)
	}
}

func TestValidateCreativePreAdaptationAllowsTraceableRecommendationsAndCalculations(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[
  {"id":"daily-amount","location":"中心金额区","role":"benefit","source_text":"Rp10.000","semantic_kind":"daily_interest_amount"},
  {"id":"headline","location":"顶部标题","role":"headline","source_text":"Dana cepat"}
]}`)
	result := json.RawMessage(`{
  "market_pack_id":"market-1","market_pack_version":2,
  "copy_library_id":"library-1","copy_library_version":4,
  "analysis_highlights":["顶部是主标题","中心保留金额层级","日息金额需使用本次参数"],
  "text_replacements":[
    {"block_id":"daily-amount","location":"中心金额区","role":"benefit","source_text":"Rp10.000","replacement_text":"Rp1.500 per hari","source_keys":[],"status":"calculated","calculation":{"rule_key":"id_simple_daily_interest_v1","formula":"本金 × 日利率","inputs":["本金 Rp5.000.000","日利率 0,03%"],"result":"Rp1.500 per hari"}},
    {"block_id":"headline","location":"顶部标题","role":"headline","source_text":"Dana cepat","replacement_text":"Dana fleksibel untuk kebutuhanmu","source_keys":[],"status":"recommended","recommendation_basis":["当前订单的已发布通用利益点","原图标题区域不承载金融数值"]}
  ],
  "repayment_plan_selections":[],"numeric_layouts":[]
}`)
	if err := validateCompletedCreativePreAdaptation(source, result); err != nil {
		t.Fatalf("traceable calculated and recommended replacements were rejected: %v", err)
	}
	if err := validateCreativePreAdaptationCopyBindings(source, result, json.RawMessage(`{"fragments":[]}`)); err != nil {
		t.Fatalf("pending calculated and recommended replacements were treated as missing library bindings: %v", err)
	}

	invalidCalculation := json.RawMessage(strings.Replace(string(result), `"result":"Rp1.500 per hari"`, `"result":"Rp1.499 per hari"`, 1))
	if err := validateCompletedCreativePreAdaptation(source, invalidCalculation); err == nil {
		t.Fatal("a calculated replacement with a mismatched formula result was accepted")
	}
}

func TestValidateCreativePreAdaptationCalculationRulesUsesFrozenMarketRule(t *testing.T) {
	source := json.RawMessage(`{"text_blocks":[{"id":"daily","semantic_kind":"daily_interest_amount"}]}`)
	adaptation := json.RawMessage(`{"text_replacements":[{"block_id":"daily","status":"calculated","calculation":{"rule_key":"id_simple_daily_interest_v1","formula":"principal × daily_rate"}}]}`)
	marketPack := json.RawMessage(`{"calculation_rules":[{"key":"id_simple_daily_interest_v1","formulas":{"daily_interest_amount":"principal × daily_rate"}}]}`)
	if err := validateCreativePreAdaptationCalculationRules(source, adaptation, marketPack); err != nil {
		t.Fatalf("frozen calculation rule was rejected: %v", err)
	}

	wrongFormula := json.RawMessage(strings.Replace(string(adaptation), "principal × daily_rate", "principal + daily_rate", 1))
	if err := validateCreativePreAdaptationCalculationRules(source, wrongFormula, marketPack); err == nil {
		t.Fatal("calculation outside the frozen rule was accepted")
	}
}
