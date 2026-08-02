package service

import (
	"encoding/json"
	"testing"
)

func TestCompactCreativeTaskResourceContextStripsDuplicatedSkillBodies(t *testing.T) {
	raw := []byte(`{
		"parent_issue_id":"parent",
		"pinned_resources":{
			"market_pack":{"files":[{"id":"prime","url":"/asset.png"}]},
			"squad":{
				"leader":{"skills":[{"id":"leader-skill","name":"lead","description":"delegate","content":"long body","references":[{"path":"ref.md","content":"long ref"}]}]},
				"members":[{"role":"image","skills":[{"id":"image-skill","name":"image","description":"generate","content":"long body","references":[]}]}]
			}
		},
		"selected_item":{"candidate_id":"candidate"}
	}`)

	compacted := compactCreativeTaskResourceContext(raw)
	var root map[string]any
	if err := json.Unmarshal(compacted, &root); err != nil {
		t.Fatalf("unmarshal compacted context: %v", err)
	}
	pinned := root["pinned_resources"].(map[string]any)
	squad := pinned["squad"].(map[string]any)
	leader := squad["leader"].(map[string]any)
	leaderSkill := leader["skills"].([]any)[0].(map[string]any)
	if _, exists := leaderSkill["content"]; exists {
		t.Fatal("leader skill content should be removed")
	}
	if _, exists := leaderSkill["references"]; exists {
		t.Fatal("leader skill references should be removed")
	}
	if leaderSkill["description"] != "delegate" {
		t.Fatalf("skill metadata was not preserved: %#v", leaderSkill)
	}
	member := squad["members"].([]any)[0].(map[string]any)
	memberSkill := member["skills"].([]any)[0].(map[string]any)
	if _, exists := memberSkill["content"]; exists {
		t.Fatal("member skill content should be removed")
	}
	market := pinned["market_pack"].(map[string]any)
	if len(market["files"].([]any)) != 1 {
		t.Fatal("market resources should be preserved")
	}
	if root["selected_item"].(map[string]any)["candidate_id"] != "candidate" {
		t.Fatal("selected item should be preserved")
	}
}

func TestCompactCreativeTaskResourceContextKeepsMalformedInput(t *testing.T) {
	raw := []byte(`{"broken"`)
	if got := string(compactCreativeTaskResourceContext(raw)); got != string(raw) {
		t.Fatalf("malformed input changed: %q", got)
	}
}
