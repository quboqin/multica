package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreativePlatformSkillsStayNativeAndMapped(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	skillRoot := filepath.Join(repoRoot, "scripts", "creative-platform-skills")

	sourceMaps := map[string]string{
		"appgrowing-material-collector": "appgrowing-collection-source-map.md",
		"ad-creative-analysis":          "ad-creative-analysis-source-map.md",
		"ad-creative-pre-adaptation":    "ad-creative-pre-adaptation-source-map.md",
		"ad-creative-plan":              "ad-creative-plan-source-map.md",
		"ad-creative-production":        "ad-creative-production-source-map.md",
		"ad-creative-prime-compose":     "source-map.md",
		"ad-creative-direct-edit":       "ad-creative-direct-edit-source-map.md",
		"ad-creative-qc":                "ad-creative-qc-source-map.md",
		"ad-creative-leadership":        "ad-creative-leadership-source-map.md",
		"creative-flow-diagnostician":   "creative-flow-diagnosis-source-map.md",
	}

	for skillName, sourceMapName := range sourceMaps {
		skillName, sourceMapName := skillName, sourceMapName
		t.Run(skillName, func(t *testing.T) {
			t.Parallel()

			content := readCreativePlatformContractFile(t, filepath.Join(skillRoot, skillName, "SKILL.md"))
			assertNoRepeatedLongSkillLines(t, content)
			for _, forbidden := range []string{
				"multica creative material get",
				"multica creative materials",
				"--help",
				"Rp80.000.000",
				"0,03%",
				"https://example.com",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("SKILL.md contains forbidden runtime probe or business example %q", forbidden)
				}
			}

			readCreativePlatformContractFile(t, filepath.Join(skillRoot, skillName, "references", sourceMapName))
		})
	}
}

func TestCreativePlatformSkillsUseFrozenBusinessInputs(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	skillRoot := filepath.Join(root, "scripts", "creative-platform-skills")
	required := map[string][]string{
		"ad-creative-analysis":       {"app_ui_replacement_needed", "app_ui_bounds", "不得读取、选择或引用 `app_ui_reference`", "不得输出 attachment ID"},
		"ad-creative-pre-adaptation": {"source_analysis", "text_replacements", "visual_direction", "recommendation_basis", "repayment_plan_selections", "numeric_layouts", "render_instruction", "完整展示字符串", "pre-adaptation-put", "min(", "我方优先、数量取小", "只有明显的还款结构才生成 repayment 选择和 numeric layout", "Rp100Juta", "保留后续可手动改写空间"},
		"ad-creative-plan":           {"copy_snapshot", "input_snapshot", "source_analysis", "app_ui_replacement", "app_ui_reference", "resource_file_id", "attachment_id", "只替换手机屏幕内容"},
		"ad-creative-production":     {"copy_snapshot", "prompt_sha256", "request_id", "Input 3 是选中的 AdaKami App UI reference", "app_ui_reference_attachment_id", "multica attachment download", "attachment_snapshot.candidate_sources", "auth_expired", "attachment_not_found", "storage_timeout", "cli_contract_mismatch", "只替换手机屏幕内容", "--result-file", "--operation-id", "--operation-attempt", "late_receipt_recovery", "late_receipt_recoveries", "同一个 Bash/exec", "operation_id", "reconcile_confirmed=true", "input_snapshot", "input_asset_fingerprints", "validate_image_operation.py", "validate_task_scope.py", "不能因为读到了同订单的另一个候选", "run_image_edit_job.py", "独立会话", "约 30 秒无 stdout"},
		"ad-creative-direct-edit":    {"copy_snapshot", "prompt_sha256", "delivery_mode", "即使先前识别错了，也要保留后续手动改写空间", "不要把流程卡死在还款计划选择", "不得同时要求“保持原始 scale”", "protected_content_envelope", "prompt 不得写入坐标、百分比", "official_prime_reflow_window_context", "--content-envelope-output", "阻塞屏障", "任务工作目录根部", "image-edit-result-<size>.json", "intent-plan.json", "final_visual_validation=true", "--operation-id", "--operation-attempt", "late_receipt_recovery", "late_receipt_recoveries", "operation_id", "reconcile_confirmed=true"},
		"ad-creative-qc":             {"copy_snapshot", "compose_result", "qc-finalize", "qc_finalize_transient_failure", "auth_expired", "attachment_not_found", "storage_timeout", "cli_contract_mismatch", "multica attachment download", "view_image", "base64/stdout", "stage=primed", "revision=<context.revision>", "已批准模板家族"},
		"ad-creative-prime-compose":  {"prime-compose", "creative_prime_backend.go", "does not edit model prompts", "each delivery size", "publishes no partial package"},
	}

	for skillName, terms := range required {
		content := readCreativePlatformContractFile(t, filepath.Join(skillRoot, skillName, "SKILL.md"))
		for _, term := range terms {
			if !strings.Contains(content, term) {
				t.Errorf("%s/SKILL.md must reference frozen/runtime contract %q", skillName, term)
			}
		}
	}

	bootstrap := readCreativePlatformContractFile(t, filepath.Join(root, "scripts", "bootstrap-creative-platform-demo.ps1"))
	for _, required := range []string{
		"capability = 'reference_analysis'; version = 19",
		"capability = 'generation_plan'; version = 40",
		"capability = 'image_edit'; version = 111",
		"capability = 'prime_compose'; version = 6",
		"capability = 'direct_image_edit'; version = 30",
		"capability = 'quality_control'; version = 43",
		"capability = 'creative_leadership'; version = 51",
		"初始 direct_edit 由平台原子创建 revision 和 task，Leader 只恢复领域状态确认缺失的当前 revision task",
		"creative_candidate_selection",
		"Skill 及其 references 是提示词、证据、归一化、Prime 和恢复规则的唯一执行真值",
	} {
		if !strings.Contains(bootstrap, required) {
			t.Errorf("bootstrap QC setup must contain %q", required)
		}
	}

	installation := readCreativePlatformContractFile(t, filepath.Join(root, "server", "internal", "handler", "creative_factory_installation.go"))
	for _, required := range []string{
		`Capability: "reference_analysis", Version: 19`,
		`Capability: "generation_plan", Version: 40`,
		`Capability: "image_edit", Version: 111`,
		`Capability: "prime_compose", Version: 6`,
		`Capability: "direct_image_edit", Version: 30`,
		`Capability: "quality_control", Version: 43`,
		`Capability: "creative_leadership", Version: 51`,
		"初始 direct_edit 由平台原子创建 revision 和 task，Leader 只恢复领域状态确认缺失的当前 revision task",
		"creative_candidate_selection",
		"Skill 及其 references 是提示词、证据、归一化、Prime 和恢复规则的唯一执行真值",
	} {
		if !strings.Contains(installation, required) {
			t.Errorf("creative factory QC setup must contain %q", required)
		}
	}
	for _, required := range []string{
		"只有用户明确要求维护文案库时",
		"add-fragment、update-fragment、upsert-repayment-plan 保存草稿",
		"只有用户明确要求发布时才加 --publish",
	} {
		if !strings.Contains(bootstrap, required) {
			t.Errorf("bootstrap diagnostician policy must contain %q", required)
		}
		if !strings.Contains(installation, required) {
			t.Errorf("creative factory diagnostician policy must contain %q", required)
		}
	}

	router := readCreativePlatformContractFile(t, filepath.Join(root, "server", "cmd", "server", "router.go"))
	if strings.Contains(router, `r.With(handler.RequireHumanActor).Put("/", h.UpdateCreativeResource)`) {
		t.Fatal("creative resource draft updates must remain callable by an authorized task actor")
	}
	if !strings.Contains(router, `r.Put("/", h.UpdateCreativeResource)`) {
		t.Fatal("creative resource draft update route is missing")
	}
}

func TestAppGrowingCollectorContractKeepsBusinessSemanticsAndSingleSubmission(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	skillRoot := filepath.Join(root, "scripts", "creative-platform-skills", "appgrowing-material-collector")
	skill := readCreativePlatformContractFile(t, filepath.Join(skillRoot, "SKILL.md"))
	contract := readCreativePlatformContractFile(t, filepath.Join(skillRoot, "references", "appgrowing-collection-contract.md"))
	combined := skill + "\n" + contract

	for _, required := range []string{
		"selection_rules.new_materials",
		"selection_rules.volume_materials",
		"asset_type=image",
		"crawl_run_id",
		"delegate_preanalysis.py",
		"multica task fanout unavailable; upgrade CLI",
		"真实阶段、error code/message",
		"同一 task 最多创建一个 Crawl Run",
		"timeout_ms",
		"crawl_command_wait_incomplete",
		"worker_busy",
		"credential broker worker is busy",
		"crawler worker is busy",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("collector contract must contain %q", required)
		}
	}

	bootstrap := readCreativePlatformContractFile(t, filepath.Join(root, "scripts", "bootstrap-creative-platform-demo.ps1"))
	for _, required := range []string{
		"capability = 'material_collection'; version = 17",
		"Set-AgentDefinition -Name '素材_采集'",
		"-MaxConcurrentTasks 1",
	} {
		if !strings.Contains(bootstrap, required) {
			t.Errorf("bootstrap collector setup must contain %q", required)
		}
	}

	installation := readCreativePlatformContractFile(t, filepath.Join(root, "server", "internal", "handler", "creative_factory_installation.go"))
	for _, required := range []string{
		`Capability: "material_collection", Version: 17`,
		`Role: "collection", Name: "素材_采集"`,
		`MaxConcurrent: 1`,
	} {
		if !strings.Contains(installation, required) {
			t.Errorf("creative factory collector setup must contain %q", required)
		}
	}
}

func TestCreativeProductionPromptContractDefinesInputRolesAndNativeReflow(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	skill := readCreativePlatformContractFile(t, filepath.Join(root, "scripts", "creative-platform-skills", "ad-creative-production", "SKILL.md"))
	promptContract := readCreativePlatformContractFile(t, filepath.Join(root, "scripts", "creative-platform-skills", "ad-creative-production", "references", "model-prompt-contract.md"))
	for _, required := range []string{
		"TASK",
		"INPUT ROLES",
		"LOCKED DESIGN DNA",
		"EDITABLE LAYOUT",
		"APPROVED COPY",
		"PRIME SUPPORT",
		"ACCEPTANCE",
		"Input 1 is always the downloaded candidate source",
		"Input 2 is always the current-size, current-revision official Prime visual context",
		"Input 4 is optional",
		"It must not donate layout",
		"1080x1080 locked 1:1 square",
		"1200x628 locked 1.91:1 landscape",
		"800x1000 locked 4:5 portrait",
	} {
		if !strings.Contains(promptContract, required) {
			t.Errorf("production model prompt contract must contain %q", required)
		}
	}
	for _, required := range []string{
		"不把方图 raster 当作不可替代输入",
		"横版按 LayoutPlan 原生横向重排",
		"最后才小幅降低字号，不删冻结文案",
		"比例偏差 `<=10%`",
		"10%-25%",
		"自适应恢复",
	} {
		if !strings.Contains(skill, required) {
			t.Errorf("production Skill must contain %q", required)
		}
	}
}

func readCreativePlatformContractFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func assertNoRepeatedLongSkillLines(t *testing.T, content string) {
	t.Helper()
	seen := map[string]int{}
	for _, line := range strings.Split(content, "\n") {
		normalized := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if len(normalized) < 80 || strings.HasPrefix(normalized, "```") || strings.HasPrefix(normalized, "multica ") {
			continue
		}
		seen[normalized]++
		if seen[normalized] > 1 {
			t.Fatalf("SKILL.md repeats long instruction line: %q", normalized)
		}
	}
}
