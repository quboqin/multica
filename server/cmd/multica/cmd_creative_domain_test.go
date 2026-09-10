package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBindCreativeOrderQCReportPayloadUsesTaskTarget(t *testing.T) {
	target := creativeOrderQCTargetResponse{
		VariantID: "0fdac4b9-3478-433c-90b2-489f17c06256",
		Workflow:  "creative_qc_visual",
		Revision:  1,
		Attempt:   2,
	}
	payload, err := bindCreativeOrderQCReportPayload(json.RawMessage(`{"status":"passed","findings":{}}`), target)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report["variant_id"] != target.VariantID || report["lane"] != "visual" || report["revision"] != float64(1) || report["attempt"] != float64(2) {
		t.Fatalf("bound report = %#v", report)
	}
}

func TestBindCreativeOrderQCReportPayloadRejectsSiblingVariant(t *testing.T) {
	target := creativeOrderQCTargetResponse{
		VariantID: "0fdac4b9-3478-433c-90b2-489f17c06256",
		Workflow:  "creative_qc_visual",
		Revision:  1,
		Attempt:   1,
	}
	_, err := bindCreativeOrderQCReportPayload(json.RawMessage(`{"variant_id":"4845a89e-ea12-4765-9f16-328a6cb8acea","lane":"visual","revision":1,"attempt":1}`), target)
	if err == nil || !strings.Contains(err.Error(), "target mismatch") {
		t.Fatalf("error = %v, want sibling target mismatch", err)
	}
}
