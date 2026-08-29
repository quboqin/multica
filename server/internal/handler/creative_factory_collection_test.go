package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCreativeFactoryCollectionTargetComesFromParams(t *testing.T) {
	params := json.RawMessage(`{"limit":25,"max_materials":25,"selection_rules":{"new_materials":{"ratio_pct":40}}}`)
	root, ok := crawlParamsObject(params)
	if !ok {
		t.Fatal("expected object params")
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := creativeFactoryCollectionTargetFromParams(encoded); got != 25 {
		t.Fatalf("target = %d, want 25", got)
	}
	materials := make([]creativeMaterialInput, 30)
	if got := len(capCreativeFactoryMaterials(materials, encoded)); got != 25 {
		t.Fatalf("capped materials = %d, want 25", got)
	}
	if got := len(capCreativeFactoryMaterials(materials, json.RawMessage(`{}`))); got != 30 {
		t.Fatalf("uncapped materials = %d, want 30", got)
	}
}

func TestCreativeFactoryCollectionTargetAcceptsOutputAliases(t *testing.T) {
	params := json.RawMessage(`{"max_outputs":"5"}`)
	if got := creativeFactoryCollectionTargetFromParams(params); got != 5 {
		t.Fatalf("target = %d, want 5", got)
	}
	materials := make([]creativeMaterialInput, 25)
	if got := len(capCreativeFactoryMaterials(materials, params)); got != 5 {
		t.Fatalf("capped materials = %d, want 5", got)
	}
}

func TestCreativeFactoryMaterialSearchBudgetLeavesRequestCompletionHeadroom(t *testing.T) {
	if got := boundedCreativeFactoryMaterialSearchBudget(map[string]any{}); got != creativeFactoryMaterialSearchBudgetMS {
		t.Fatalf("default budget = %d, want %d", got, creativeFactoryMaterialSearchBudgetMS)
	}
	if got := boundedCreativeFactoryMaterialSearchBudget(map[string]any{"crawl_budget_ms": 90_000}); got != 90_000 {
		t.Fatalf("short caller budget = %d, want 90000", got)
	}
	if got := boundedCreativeFactoryMaterialSearchBudget(map[string]any{"material_search_budget_ms": 12 * 60 * 1000}); got != creativeFactoryMaterialSearchBudgetMS {
		t.Fatalf("long caller budget = %d, want %d", got, creativeFactoryMaterialSearchBudgetMS)
	}
}

func TestCreativeFactoryCollectionParamsHasAgent(t *testing.T) {
	params := json.RawMessage(`{"analysis_agent_id":"agent-1"}`)
	if !creativeFactoryCollectionParamsHasAgent(params, "agent-1") {
		t.Fatal("expected injected agent to match")
	}
	if creativeFactoryCollectionParamsHasAgent(params, "agent-2") {
		t.Fatal("unexpected agent match")
	}
}

func TestCreativeFactoryAutopilotUsesCollectionPrompt(t *testing.T) {
	description := creativeFactoryAutopilotDescriptionForProfile(creativeFactoryIndonesiaProfile())
	for _, required := range []string{
		"AppGrowing",
		"新素材 40%",
		"asset_type=image",
		"不占用名额",
		"task fanout",
		"action_required",
	} {
		if !strings.Contains(description, required) {
			t.Fatalf("collection autopilot prompt is missing %q", required)
		}
	}
	if len([]rune(description)) < 500 {
		t.Fatalf("collection autopilot prompt is unexpectedly short: %d runes", len([]rune(description)))
	}
}

func TestCreativeFactoryAutopilotNeedsMigrationOnlyForInstallerDefault(t *testing.T) {
	base := db.Autopilot{
		AssigneeType:  "squad",
		AssigneeID:    pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		Status:        "active",
		ExecutionMode: "run_only",
		Description: pgtype.Text{
			String: "由创意工厂事件驱动，从素材分析、文案适配到出图和验收自动推进。",
			Valid:  true,
		},
	}
	if !creativeFactoryAutopilotNeedsMigration(base, base.AssigneeID) {
		t.Fatal("installer-created squad autopilot should be migrated")
	}
	base.Description.String = "用户已经编辑过的自动化"
	if creativeFactoryAutopilotNeedsMigration(base, base.AssigneeID) {
		t.Fatal("user-edited autopilot should not be migrated")
	}
	base.Description.String = "由创意工厂事件驱动，从素材分析、文案适配到出图和验收自动推进。"
	if creativeFactoryAutopilotNeedsMigration(base, pgtype.UUID{Bytes: [16]byte{2}, Valid: true}) {
		t.Fatal("autopilot with a changed assignee should not be migrated")
	}
}

func TestCreativeFactoryAutopilotUsesInstallerDefaultWithLegacyDescription(t *testing.T) {
	legacyDescription := strings.Replace(
		creativeFactoryDefaultAutopilotDescription,
		"不占用名额",
		"不占用 2 条配额",
		1,
	)
	legacyDescription = strings.Replace(
		legacyDescription,
		creativeFactoryDefaultCollectionTargetLine,
		"最多输出：2 张图片",
		1,
	)
	autopilot := db.Autopilot{
		Title:         creativeFactoryDefaultAutopilotTitle,
		AssigneeType:  "agent",
		Status:        "active",
		ExecutionMode: "run_only",
		Description: pgtype.Text{
			String: legacyDescription,
			Valid:  true,
		},
	}
	if !creativeFactoryAutopilotUsesInstallerDefault(autopilot) {
		t.Fatal("installer-created collection autopilot with legacy prompt wording should be recognized")
	}
	autopilot.Description.String = "用户自定义采集规则"
	if creativeFactoryAutopilotUsesInstallerDefault(autopilot) {
		t.Fatal("user-authored collection autopilot should not be treated as installer default")
	}
	autopilot.Description.String = legacyDescription
	autopilot.Title = "我的竞品素材采集"
	if creativeFactoryAutopilotUsesInstallerDefault(autopilot) {
		t.Fatal("custom title should not be treated as installer default")
	}
}
