package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreativeFactorySeedPublishAllowed(t *testing.T) {
	if !creativeFactorySeedPublishAllowed("copy_library", map[string]any{}) {
		t.Fatal("copy-library factory seed should be published for pre-adaptation")
	}
	if !creativeFactorySeedPublishAllowed("market_pack", map[string]any{"pre_adaptation_default": true}) {
		t.Fatal("default market-pack factory seed should be published for pre-adaptation")
	}
	if creativeFactorySeedPublishAllowed("market_pack", map[string]any{"pre_adaptation_default": false}) {
		t.Fatal("non-default market pack should not be auto-published")
	}
}

func TestCreativeFactoryMarketPackNeedsSetupOnlyForSeed(t *testing.T) {
	seed, _ := json.Marshal(map[string]any{"pre_adaptation_seed": true})
	if !creativeFactoryMarketPackNeedsSetup(creativeResourceResponse{Config: seed}) {
		t.Fatal("seed market pack must remain in setup state")
	}
	production, _ := json.Marshal(map[string]any{"prime_template_set": map[string]any{"version": 1}})
	if creativeFactoryMarketPackNeedsSetup(creativeResourceResponse{Config: production}) {
		t.Fatal("production market pack should not be marked setup-only")
	}
}

func TestCloneCreativeFactoryConfigDoesNotShareNestedValues(t *testing.T) {
	source := map[string]any{
		"prime_template_set": map[string]any{
			"families": []any{map[string]any{"id": "light_background"}},
		},
	}
	clone := cloneCreativeFactoryConfig(source)
	clone["copy_library_id"] = "workspace-copy-library"
	cloneFamily := clone["prime_template_set"].(map[string]any)["families"].([]any)[0].(map[string]any)
	cloneFamily["id"] = "dark_background"

	if _, ok := source["copy_library_id"]; ok {
		t.Fatal("config clone mutated the source map")
	}
	sourceFamily := source["prime_template_set"].(map[string]any)["families"].([]any)[0].(map[string]any)
	if sourceFamily["id"] != "light_background" {
		t.Fatalf("config clone shared nested map: %#v", sourceFamily)
	}
}

func TestCreativeFactoryResourceDefaultsAreEmbedded(t *testing.T) {
	defaults, err := creativeFactoryResourceDefaults()
	if err != nil {
		t.Fatalf("creativeFactoryResourceDefaults: %v", err)
	}
	copyConfig := defaults["copy_library"].Config
	if len(copyConfig["fragments"].([]any)) < 30 {
		t.Fatalf("embedded copy library is incomplete: %d fragments", len(copyConfig["fragments"].([]any)))
	}
	if len(copyConfig["repayment_plan"].(map[string]any)["entries"].([]any)) < 20 {
		t.Fatalf("embedded repayment plan is incomplete")
	}
	marketConfig := defaults["market_pack"].Config
	for _, key := range []string{"calculation_rules", "prime_template_set", "prime_layout_contract", "prime_template_set_validation"} {
		if _, ok := marketConfig[key]; !ok {
			t.Fatalf("embedded market pack is missing %q", key)
		}
	}
	if marketConfig["naming_rule"] != "{month}_P_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}" {
		t.Fatalf("embedded market pack has unexpected image naming rule: %#v", marketConfig["naming_rule"])
	}
	if marketConfig["video_naming_rule"] != "{month}_V_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}_{duration}" {
		t.Fatalf("embedded market pack has unexpected video naming rule: %#v", marketConfig["video_naming_rule"])
	}
	sizeAliases, ok := marketConfig["naming_size_abbreviations"].(map[string]any)
	if !ok || sizeAliases["1200x628"] != "191" {
		t.Fatalf("embedded market pack has unexpected size aliases: %#v", marketConfig["naming_size_abbreviations"])
	}
	encodedMarketConfig, err := json.Marshal(marketConfig)
	if err != nil {
		t.Fatalf("encode embedded market pack: %v", err)
	}
	if strings.Count(string(encodedMarketConfig), `"qr_payload"`) != 6 {
		t.Fatalf("embedded market pack should retain six optional template QR evidence entries")
	}
	for _, key := range []string{"qr_payload", "qr_canonical_payload", "qr_allowed_domains", "qr_approval_status", "qr_approval_note"} {
		if _, ok := marketConfig[key]; ok {
			t.Fatalf("embedded market pack must not contain legacy QR configuration %q", key)
		}
	}
	if got := len(defaults["market_pack"].Files); got != 7 {
		t.Fatalf("embedded market pack files = %d, want 7", got)
	}
}

func TestCreativeFactoryPreAdaptationTemplateUsesFirstPartyRowCapacity(t *testing.T) {
	var skill creativeFactorySkillSpec
	for _, candidate := range creativeFactorySkillSpecs {
		if candidate.Role == "pre_adaptation" {
			skill = candidate
			break
		}
	}
	if skill.Version != 26 {
		t.Fatalf("pre-adaptation Skill version = %d, want 26", skill.Version)
	}
	var agent creativeFactoryAgentSpec
	for _, candidate := range creativeFactoryAgentSpecs {
		if candidate.Role == "reference_analysis" {
			agent = candidate
			break
		}
	}
	for _, required := range []string{"我方可用方案数的较小值", "源图多出的数值块", "不得借用其他期限金额", "render_instruction", "完整冻结展示值", "只有明显的还款结构才锁定"} {
		if !strings.Contains(agent.Instructions, required) {
			t.Fatalf("reference-analysis Agent instructions missing %q", required)
		}
	}
}

func TestCreativeFactoryImageEditingUsesOneAgentWithWorkflowSkills(t *testing.T) {
	var imageEditor *creativeFactoryAgentSpec
	var directEditSkill creativeFactorySkillSpec
	var directEditors int
	for index := range creativeFactorySkillSpecs {
		if creativeFactorySkillSpecs[index].Role == "direct_image_edit" {
			directEditSkill = creativeFactorySkillSpecs[index]
			break
		}
	}
	if directEditSkill.Version != 22 {
		t.Fatalf("direct-edit Skill version = %d, want 22", directEditSkill.Version)
	}
	for index := range creativeFactoryAgentSpecs {
		spec := &creativeFactoryAgentSpecs[index]
		switch spec.Role {
		case "image_edit":
			imageEditor = spec
		case "direct_image_edit":
			directEditors++
		}
	}
	if imageEditor == nil {
		t.Fatal("creative factory must define an image-edit Agent")
	}
	if directEditors != 0 {
		t.Fatalf("creative factory must not define a separate direct-edit Agent; found %d", directEditors)
	}

	wantSkills := map[string]bool{
		"image_edit":        false,
		"direct_image_edit": false,
		"prime_compose":     false,
	}
	for _, role := range imageEditor.SkillRoles {
		if _, ok := wantSkills[role]; ok {
			wantSkills[role] = true
		}
	}
	for role, found := range wantSkills {
		if !found {
			t.Errorf("merged image-edit Agent is missing %q Skill", role)
		}
	}
	for _, workflow := range []string{"creative_production", "creative_direct_edit"} {
		if !strings.Contains(imageEditor.Instructions, workflow) {
			t.Errorf("merged image-edit Agent instructions missing %q branch", workflow)
		}
	}
	for _, required := range []string{"creative_production", "creative_direct_edit", "candidate_state", "DesignDNA", "多目标同时验收", "绑定的素材_技能_出图"} {
		if !strings.Contains(imageEditor.Instructions, required) {
			t.Errorf("merged image-edit Agent instructions missing %q", required)
		}
	}
	for _, duplicated := range []string{"COMPOSITION GATE", "--result-file", "diagnostic-asset-put", "/app/creative-platform-skills"} {
		if strings.Contains(imageEditor.Instructions, duplicated) {
			t.Errorf("merged image-edit Agent instructions duplicate Skill contract %q", duplicated)
		}
	}
}

func TestCreativeFactoryCreativeContractTemplatesStayInSync(t *testing.T) {
	wantVersions := map[string]int{
		"generation_plan":     39,
		"image_edit":          99,
		"prime_compose":       4,
		"direct_image_edit":   22,
		"quality_control":     37,
		"creative_leadership": 51,
	}
	for _, spec := range creativeFactorySkillSpecs {
		if want, ok := wantVersions[spec.Role]; ok && spec.Version != want {
			t.Errorf("%s Skill version = %d, want %d", spec.Role, spec.Version, want)
		}
	}
	if !strings.Contains(creativeFactoryImageEditAgentInstructions(), "写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续") {
		t.Fatal("factory producer instructions lost the specialist failure handoff contract")
	}
	leaderInstructions := ""
	for _, spec := range creativeFactoryAgentSpecs {
		if spec.Role == "leadership" {
			leaderInstructions = spec.Instructions
			break
		}
	}
	if !strings.Contains(leaderInstructions, "初始 direct_edit 由平台原子创建 revision 和 task，Leader 只恢复领域状态确认缺失的当前 revision task") {
		t.Fatal("factory Leader instructions lost the atomic direct-edit initialization contract")
	}

	templates, err := loadCreativeFactoryTemplates()
	if err != nil {
		t.Fatalf("loadCreativeFactoryTemplates: %v", err)
	}
	checks := map[string][]string{
		"ad-creative-plan":          {"4-5 个候选", "primary_size", "CreativeIntent", "DesignDNA", "LayoutPlan", "pipeline_version", "candidate_v1"},
		"ad-creative-production":    {"candidate_primary", "selected", "model-prompt-contract.md", "不把方图 raster 当作不可替代输入", "pipeline_version", "candidate_v1"},
		"ad-creative-prime-compose": {"each delivery size", "publishes no partial package", "fail closed"},
		"ad-creative-direct-edit":   {"target_masks", "Transform ONLY", "official Prime visual context", "<当前 Skill 目录>/../ad-creative-production/references/normalize_image.py"},
		"ad-creative-qc":            {"creative_candidate_selection", "candidate-select", "selected_ids", "foreground_polarity", "cross_size_design_dna_mismatch"},
		"ad-creative-leadership":    {"4-5 个候选", "原子晋级 3 个", "最终视觉 QC", "初始 direct-edit revision 和 task 由平台建单事务原子创建", "source_revision = revision - 1"},
	}
	for directory, required := range checks {
		template := templates[directory]
		for _, value := range required {
			if !strings.Contains(template.Content, value) {
				t.Errorf("%s template missing %q", directory, value)
			}
		}
		for _, file := range template.Files {
			if strings.Contains(file.Path, "/.pytest_cache/") || strings.Contains(file.Path, "/__pycache__/") || strings.HasSuffix(file.Path, ".pyc") {
				t.Errorf("%s template includes test cache file %q", directory, file.Path)
			}
		}
	}

	productionTemplate := templates["ad-creative-production"]
	var promptContract string
	for _, file := range productionTemplate.Files {
		if file.Path == "references/model-prompt-contract.md" {
			promptContract = file.Content
			break
		}
	}
	for _, value := range []string{"TASK", "INPUT ROLES", "LOCKED DESIGN DNA", "EDITABLE LAYOUT", "APPROVED COPY", "PRIME SUPPORT", "ACCEPTANCE", "platform to typeset"} {
		if !strings.Contains(promptContract, value) {
			t.Errorf("model prompt contract missing %q", value)
		}
	}

	root, err := creativeFactoryTemplateRoot()
	if err != nil {
		t.Fatalf("creativeFactoryTemplateRoot: %v", err)
	}
	bootstrap, err := os.ReadFile(filepath.Join(root, "..", "bootstrap-creative-platform-demo.ps1"))
	if err != nil {
		t.Fatalf("read bootstrap script: %v", err)
	}
	for _, value := range []string{
		"capability = 'generation_plan'; version = 39",
		"capability = 'image_edit'; version = 99",
		"capability = 'prime_compose'; version = 4",
		"capability = 'direct_image_edit'; version = 22",
		"capability = 'quality_control'; version = 37",
		"capability = 'creative_leadership'; version = 51",
		"初始 direct_edit 由平台原子创建 revision 和 task，Leader 只恢复领域状态确认缺失的当前 revision task",
		"creative_candidate_selection",
		"写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续",
	} {
		if !strings.Contains(string(bootstrap), value) {
			t.Errorf("bootstrap contract missing %q", value)
		}
	}
	if strings.Contains(string(bootstrap), "/app/creative-platform-skills/ad-creative-production/references/normalize_image.py") {
		t.Fatal("bootstrap still embeds the old absolute normalization path")
	}
}

func TestInitializeCreativeFactoryRefreshesNeedsSetupManagedAssets(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test fixture unavailable")
	}
	cleanupCreativeFactoryManagedAssetsForTest(t)
	t.Cleanup(func() { cleanupCreativeFactoryManagedAssetsForTest(t) })

	ctx := t.Context()
	var imageSkillID string
	oldImageConfig := `{"kind":"creative_role","capability":"image_edit","version":70,"template_version":5,"origin":"creative_factory"}`
	if err := testPool.QueryRow(ctx, `
INSERT INTO skill (workspace_id, name, description, content, config, created_by)
VALUES ($1, '素材_技能_出图', 'old image edit skill', 'stale image skill body', $2::jsonb, $3)
RETURNING id::text
`, testWorkspaceID, oldImageConfig, testUserID).Scan(&imageSkillID); err != nil {
		t.Fatalf("seed image-edit skill: %v", err)
	}

	var directSkillID string
	oldConfig := `{"kind":"creative_role","capability":"direct_image_edit","version":12,"template_version":5,"origin":"creative_factory"}`
	if err := testPool.QueryRow(ctx, `
INSERT INTO skill (workspace_id, name, description, content, config, created_by)
VALUES ($1, '素材_技能_改图', 'old direct edit skill', 'stale skill body', $2::jsonb, $3)
RETURNING id::text
`, testWorkspaceID, oldConfig, testUserID).Scan(&directSkillID); err != nil {
		t.Fatalf("seed direct-edit skill: %v", err)
	}

	var imageAgentID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO agent (
  workspace_id, name, description, runtime_mode, runtime_config,
  runtime_id, visibility, max_concurrent_tasks, owner_id,
  instructions, custom_env, custom_args, model, thinking_level
) VALUES (
  $1, '素材_出图', 'old image agent', 'cloud', '{}'::jsonb,
  $2, 'workspace', 1, $3,
  'stale image instructions', '{}'::jsonb, '[]'::jsonb, 'old-model', 'low'
)
RETURNING id::text
	`, testWorkspaceID, testRuntimeID, testUserID).Scan(&imageAgentID); err != nil {
		t.Fatalf("seed image-edit agent: %v", err)
	}
	var poolAgentID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO agent (
  workspace_id, name, description, runtime_mode, runtime_config,
  runtime_id, visibility, max_concurrent_tasks, owner_id,
  instructions, custom_env, custom_args, model, thinking_level
) VALUES (
  $1, '图像编辑智能体', '旧版精准改图直接交付', 'cloud', '{}'::jsonb,
  $2, 'workspace', 6, $3,
  '旧版合同要求不创建 QC task', '{}'::jsonb, '[]'::jsonb, 'pool-model', 'medium'
)
RETURNING id::text
`, testWorkspaceID, testRuntimeID, testUserID).Scan(&poolAgentID); err != nil {
		t.Fatalf("seed image-edit pool agent: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO agent_skill (agent_id, skill_id)
VALUES ($1::uuid, $2::uuid), ($1::uuid, $3::uuid)
`, poolAgentID, imageSkillID, directSkillID); err != nil {
		t.Fatalf("bind image-edit pool skills: %v", err)
	}
	var squadID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO squad (workspace_id, name, description, leader_id, creator_id)
VALUES ($1, '素材流程小队', 'test creative factory pool', $2, $3)
RETURNING id::text
`, testWorkspaceID, imageAgentID, testUserID).Scan(&squadID); err != nil {
		t.Fatalf("seed creative factory squad: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO squad_member (squad_id, member_type, member_id, role)
VALUES ($1::uuid, 'agent', $2::uuid, '图像编辑'),
       ($1::uuid, 'agent', $3::uuid, '出图')
`, squadID, imageAgentID, poolAgentID); err != nil {
		t.Fatalf("seed image-edit pool members: %v", err)
	}
	var customPoolAgentID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO agent (
  workspace_id, name, description, runtime_mode, runtime_config,
  runtime_id, visibility, max_concurrent_tasks, owner_id,
  instructions, custom_env, custom_args, model, thinking_level
) VALUES (
  $1, '素材_出图池_自定义测试', 'keep custom pool description', 'cloud', '{}'::jsonb,
  $2, 'workspace', 4, $3,
  'keep custom pool instructions', '{}'::jsonb, '[]'::jsonb, 'custom-pool-model', 'high'
)
RETURNING id::text
`, testWorkspaceID, testRuntimeID, testUserID).Scan(&customPoolAgentID); err != nil {
		t.Fatalf("seed custom image-edit pool agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1::uuid`, customPoolAgentID)
	})
	if _, err := testPool.Exec(ctx, `
INSERT INTO agent_skill (agent_id, skill_id)
VALUES ($1::uuid, $2::uuid)
`, customPoolAgentID, imageSkillID); err != nil {
		t.Fatalf("seed custom image-edit pool skill: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO squad_member (squad_id, member_type, member_id, role)
VALUES ($1::uuid, 'agent', $2::uuid, '自定义出图')
`, squadID, customPoolAgentID); err != nil {
		t.Fatalf("seed custom image-edit pool binding: %v", err)
	}

	roleAgents, _ := json.Marshal(map[string]string{
		"image_edit":        imageAgentID,
		"direct_image_edit": imageAgentID,
	})
	roleSkills, _ := json.Marshal(map[string]string{
		"image_edit":        imageSkillID,
		"direct_image_edit": directSkillID,
	})
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_factory_installation (
  workspace_id, status, schema_version, template_version, runtime_id,
  squad_id, role_agents, role_skills, config, initialized_by, initialized_at
) VALUES ($1, 'needs_setup', 1, 5, $2, $3, $4::jsonb, $5::jsonb, '{"template_version":5}'::jsonb, $6, now())
`, testWorkspaceID, testRuntimeID, squadID, string(roleAgents), string(roleSkills), testUserID); err != nil {
		t.Fatalf("seed ready factory installation: %v", err)
	}

	if _, err := testHandler.initializeCreativeFactory(ctx, parseUUID(testWorkspaceID), parseUUID(testUserID), nil); err != nil {
		t.Fatalf("initializeCreativeFactory refresh: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
UPDATE agent_skill binding
SET enabled = false
FROM skill role_skill
WHERE binding.skill_id = role_skill.id
  AND binding.agent_id IN ($1::uuid, $2::uuid)
  AND role_skill.config->>'capability' = 'prime_compose'
`, poolAgentID, customPoolAgentID); err != nil {
		t.Fatalf("disable pool Prime skills before repeated refresh: %v", err)
	}
	if _, err := testHandler.initializeCreativeFactory(ctx, parseUUID(testWorkspaceID), parseUUID(testUserID), nil); err != nil {
		t.Fatalf("initializeCreativeFactory repeated refresh: %v", err)
	}

	var content, configRaw string
	if err := testPool.QueryRow(ctx, `
SELECT content, config::text
FROM skill
WHERE id = $1::uuid
`, directSkillID).Scan(&content, &configRaw); err != nil {
		t.Fatalf("query refreshed direct-edit skill: %v", err)
	}
	for _, required := range []string{
		"用户在最终交付图上的标注 brief",
		"official Prime visual context",
		"Transform ONLY",
		"official_prime_visual_context_missing",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("direct-edit skill was not refreshed with required contract %q", required)
		}
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(configRaw), &config); err != nil {
		t.Fatalf("decode refreshed skill config: %v", err)
	}
	if got := int(config["version"].(float64)); got != 25 {
		t.Fatalf("direct-edit Skill version = %d, want 25", got)
	}
	if got := int(config["template_version"].(float64)); got != creativeFactoryTemplateVersion {
		t.Fatalf("direct-edit template_version = %d, want %d", got, creativeFactoryTemplateVersion)
	}

	var instructions string
	if err := testPool.QueryRow(ctx, `
SELECT instructions
FROM agent
WHERE id = $1::uuid
`, imageAgentID).Scan(&instructions); err != nil {
		t.Fatalf("query refreshed image-edit agent: %v", err)
	}
	for _, required := range []string{"creative_direct_edit", "多目标同时验收", "附件血缘", "归一化", "最终视觉 QC"} {
		if !strings.Contains(instructions, required) {
			t.Fatalf("image-edit Agent instructions missing refreshed contract %q", required)
		}
	}

	var poolInstructions, poolDescription, poolModel string
	var poolMaxConcurrent int
	if err := testPool.QueryRow(ctx, `
SELECT instructions, description, model, max_concurrent_tasks
FROM agent
WHERE id = $1::uuid
`, poolAgentID).Scan(&poolInstructions, &poolDescription, &poolModel, &poolMaxConcurrent); err != nil {
		t.Fatalf("query refreshed image-edit pool agent: %v", err)
	}
	for _, required := range []string{"creative_production", "creative_direct_edit", "DesignDNA", "最终视觉 QC"} {
		if !strings.Contains(poolInstructions, required) {
			t.Fatalf("image-edit pool Agent instructions missing refreshed contract %q", required)
		}
	}
	if poolDescription != "按唯一提示词合同执行候选主视觉、selected 扩尺寸或用户标注精准改图；只编辑无品牌底图，由贴片 Skill 确定性合成。" {
		t.Fatalf("image-edit pool Agent description was not refreshed: %q", poolDescription)
	}
	if poolModel != "pool-model" || poolMaxConcurrent != 6 {
		t.Fatalf("image-edit pool runtime settings changed: model=%q max_concurrent=%d", poolModel, poolMaxConcurrent)
	}
	var poolCapabilities []string
	if err := testPool.QueryRow(ctx, `
SELECT array_agg(role_skill.config->>'capability' ORDER BY role_skill.config->>'capability')
FROM agent_skill binding
JOIN skill role_skill ON role_skill.id = binding.skill_id
WHERE binding.agent_id = $1::uuid AND binding.enabled
  AND role_skill.config->>'kind' = 'creative_role'
`, poolAgentID).Scan(&poolCapabilities); err != nil {
		t.Fatalf("query refreshed image-edit pool capabilities: %v", err)
	}
	if strings.Join(poolCapabilities, ",") != "direct_image_edit,image_edit,prime_compose" {
		t.Fatalf("image-edit pool capabilities = %v", poolCapabilities)
	}

	var customInstructions, customDescription, customModel string
	var customMaxConcurrent int
	if err := testPool.QueryRow(ctx, `
SELECT instructions, description, model, max_concurrent_tasks
FROM agent
WHERE id = $1::uuid
`, customPoolAgentID).Scan(&customInstructions, &customDescription, &customModel, &customMaxConcurrent); err != nil {
		t.Fatalf("query custom image-edit pool agent: %v", err)
	}
	if customInstructions != "keep custom pool instructions" || customDescription != "keep custom pool description" {
		t.Fatalf("custom image-edit pool contract was overwritten: description=%q instructions=%q", customDescription, customInstructions)
	}
	if customModel != "custom-pool-model" || customMaxConcurrent != 4 {
		t.Fatalf("custom image-edit pool runtime settings changed: model=%q max_concurrent=%d", customModel, customMaxConcurrent)
	}
	var customCapabilities []string
	if err := testPool.QueryRow(ctx, `
SELECT array_agg(role_skill.config->>'capability' ORDER BY role_skill.config->>'capability')
FROM agent_skill binding
JOIN skill role_skill ON role_skill.id = binding.skill_id
WHERE binding.agent_id = $1::uuid AND binding.enabled
  AND role_skill.config->>'kind' = 'creative_role'
`, customPoolAgentID).Scan(&customCapabilities); err != nil {
		t.Fatalf("query custom image-edit pool capabilities: %v", err)
	}
	if strings.Join(customCapabilities, ",") != "image_edit,prime_compose" {
		t.Fatalf("custom image-edit pool capabilities = %v", customCapabilities)
	}

	var directSkillBound bool
	if err := testPool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM agent_skill
  WHERE agent_id = $1::uuid AND skill_id = $2::uuid
)
`, imageAgentID, directSkillID).Scan(&directSkillBound); err != nil {
		t.Fatalf("query refreshed agent skills: %v", err)
	}
	if !directSkillBound {
		t.Fatal("image-edit Agent was not rebound to the direct-edit Skill")
	}

	var templateVersion int
	var storedRoleAgentsRaw, storedRoleSkillsRaw string
	if err := testPool.QueryRow(ctx, `
SELECT template_version, role_agents::text, role_skills::text
FROM creative_factory_installation
WHERE workspace_id = $1::uuid
`, testWorkspaceID).Scan(&templateVersion, &storedRoleAgentsRaw, &storedRoleSkillsRaw); err != nil {
		t.Fatalf("query refreshed installation: %v", err)
	}
	if templateVersion != creativeFactoryTemplateVersion {
		t.Fatalf("installation template_version = %d, want %d", templateVersion, creativeFactoryTemplateVersion)
	}
	if !strings.Contains(storedRoleAgentsRaw, "collection") || !strings.Contains(storedRoleSkillsRaw, "creative_leadership") {
		t.Fatalf("installation role maps were not reconciled: agents=%s skills=%s", storedRoleAgentsRaw, storedRoleSkillsRaw)
	}
}

func cleanupCreativeFactoryManagedAssetsForTest(t *testing.T) {
	t.Helper()
	if testPool == nil {
		return
	}
	ctx := context.Background()
	skillNames := make([]string, 0, len(creativeFactorySkillSpecs)*2)
	for _, spec := range creativeFactorySkillSpecs {
		skillNames = append(skillNames, spec.Name)
		skillNames = append(skillNames, spec.Aliases...)
	}
	agentNames := make([]string, 0, len(creativeFactoryAgentSpecs)*2)
	for _, spec := range creativeFactoryAgentSpecs {
		agentNames = append(agentNames, spec.Name)
		agentNames = append(agentNames, spec.Aliases...)
	}
	_, _ = testPool.Exec(ctx, `DELETE FROM creative_factory_installation WHERE workspace_id = $1::uuid`, testWorkspaceID)
	_, _ = testPool.Exec(ctx, `DELETE FROM autopilot WHERE workspace_id = $1::uuid AND title = ANY($2::text[])`, testWorkspaceID, []string{creativeFactoryDefaultAutopilotTitle, creativeFactoryLegacyAutopilotTitle, creativeFactoryAutopilotTitleForProfile(creativeFactoryMalaysiaProfile())})
	_, _ = testPool.Exec(ctx, `DELETE FROM squad WHERE workspace_id = $1::uuid AND name = ANY($2::text[])`, testWorkspaceID, []string{"素材流程小队", "AdaKami 素材小队"})
	_, _ = testPool.Exec(ctx, `DELETE FROM agent WHERE workspace_id = $1::uuid AND name = ANY($2::text[])`, testWorkspaceID, agentNames)
	_, _ = testPool.Exec(ctx, `DELETE FROM skill WHERE workspace_id = $1::uuid AND name = ANY($2::text[])`, testWorkspaceID, skillNames)
}
