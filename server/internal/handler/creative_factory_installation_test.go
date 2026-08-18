package handler

import (
	"encoding/json"
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
	if skill.Version != 25 {
		t.Fatalf("pre-adaptation Skill version = %d, want 25", skill.Version)
	}
	var agent creativeFactoryAgentSpec
	for _, candidate := range creativeFactoryAgentSpecs {
		if candidate.Role == "reference_analysis" {
			agent = candidate
			break
		}
	}
	for _, required := range []string{"我方可用方案数的较小值", "源图多出的数值块", "不得借用其他期限金额", "render_instruction", "完整冻结展示值"} {
		if !strings.Contains(agent.Instructions, required) {
			t.Fatalf("reference-analysis Agent instructions missing %q", required)
		}
	}
}
