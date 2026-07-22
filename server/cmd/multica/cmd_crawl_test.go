package main

import (
	"encoding/json"
	"testing"
)

func TestCrawlParamsWithIntentMergesIntoObjectParams(t *testing.T) {
	raw, err := crawlParamsWithIntent(`{"limit":10}`, "竞品：Easycash")
	if err != nil {
		t.Fatalf("crawlParamsWithIntent: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal merged params: %v", err)
	}
	if got["limit"] != float64(10) {
		t.Fatalf("limit = %#v, want 10", got["limit"])
	}
	if got["intent"] != "竞品：Easycash" {
		t.Fatalf("intent = %#v", got["intent"])
	}
}

func TestCrawlParamsWithIntentRequiresObjectParams(t *testing.T) {
	if _, err := crawlParamsWithIntent(`[]`, "竞品：Easycash"); err == nil {
		t.Fatal("expected non-object params-json to fail when intent is provided")
	}
}

func TestCrawlParamsWithIntentLeavesRawJSONWithoutIntent(t *testing.T) {
	raw, err := crawlParamsWithIntent(`[{"x":1}]`, "")
	if err != nil {
		t.Fatalf("crawlParamsWithIntent: %v", err)
	}
	if string(raw) != `[{"x":1}]` {
		t.Fatalf("raw = %s", raw)
	}
}
