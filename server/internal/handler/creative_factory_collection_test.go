package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCreativeFactoryCollectionLimit(t *testing.T) {
	params := json.RawMessage(`{"limit":25,"max_materials":25,"selection_rules":{"new_materials":{"ratio_pct":40}}}`)
	root, ok := crawlParamsObject(params)
	if !ok {
		t.Fatal("expected object params")
	}
	root["limit"] = creativeFactoryCollectionLimit
	root["max_materials"] = creativeFactoryCollectionLimit
	encoded, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := creativeFactoryCollectionLimitFromParams(encoded); got != creativeFactoryCollectionLimit {
		t.Fatalf("limit = %d, want %d", got, creativeFactoryCollectionLimit)
	}
	materials := make([]creativeMaterialInput, 5)
	if got := len(capCreativeFactoryMaterials(materials)); got != creativeFactoryCollectionLimit {
		t.Fatalf("capped materials = %d, want %d", got, creativeFactoryCollectionLimit)
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
	for _, required := range []string{
		"AppGrowing",
		"新素材 40%",
		"asset_type=image",
		"最多输出：2 张图片",
		"task fanout",
		"action_required",
	} {
		if !strings.Contains(creativeFactoryAutopilotDescription, required) {
			t.Fatalf("collection autopilot prompt is missing %q", required)
		}
	}
	if len([]rune(creativeFactoryAutopilotDescription)) < 500 {
		t.Fatalf("collection autopilot prompt is unexpectedly short: %d runes", len([]rune(creativeFactoryAutopilotDescription)))
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
