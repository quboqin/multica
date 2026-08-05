package handler

import "testing"

func TestNormalizeCreativeMarketPackExtractionResultVariableCandidates(t *testing.T) {
	result, err := normalizeCreativeMarketPackExtractionResult("found two", []creativeMarketPackExtractionCandidate{
		{ID: "logo", Label: "Brand logo", Kind: "image", SuggestedComponentID: "logo", Rect: [4]int{10, 20, 200, 100}, Confidence: 0.98},
		{ID: "terms", Label: "Terms", Kind: "text", Content: "Terms apply", Rect: [4]int{300, 20, 600, 80}, Confidence: 0.8},
	}, 1080, 1080)
	if err != nil {
		t.Fatalf("normalize result: %v", err)
	}
	if len(result.Candidates) != 2 || result.Candidates[1].Content != "Terms apply" {
		t.Fatalf("unexpected result: %+v", result)
	}

	empty, err := normalizeCreativeMarketPackExtractionResult("nothing reusable", nil, 1080, 1080)
	if err != nil || len(empty.Candidates) != 0 {
		t.Fatalf("zero candidates should be valid: result=%+v err=%v", empty, err)
	}
}

func TestNormalizeCreativeMarketPackExtractionResultRejectsInvalidCandidates(t *testing.T) {
	tests := []struct {
		name       string
		candidates []creativeMarketPackExtractionCandidate
	}{
		{name: "outside source", candidates: []creativeMarketPackExtractionCandidate{{ID: "one", Label: "Logo", Kind: "image", Rect: [4]int{0, 0, 1081, 100}, Confidence: 1}}},
		{name: "duplicate ids", candidates: []creativeMarketPackExtractionCandidate{
			{ID: "one", Label: "Logo", Kind: "image", Rect: [4]int{0, 0, 100, 100}, Confidence: 1},
			{ID: "one", Label: "Seal", Kind: "image", Rect: [4]int{100, 100, 200, 200}, Confidence: 1},
		}},
		{name: "invalid kind", candidates: []creativeMarketPackExtractionCandidate{{ID: "one", Label: "Logo", Kind: "creative", Rect: [4]int{0, 0, 100, 100}, Confidence: 1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeCreativeMarketPackExtractionResult("", test.candidates, 1080, 1080); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDerivedMarketPackExtractionStatus(t *testing.T) {
	tests := []struct{ stored, task, want string }{
		{stored: "pending", task: "queued", want: "pending"},
		{stored: "pending", task: "running", want: "running"},
		{stored: "pending", task: "failed", want: "failed"},
		{stored: "completed", task: "running", want: "completed"},
		{stored: "applied", task: "completed", want: "applied"},
	}
	for _, test := range tests {
		if got := derivedMarketPackExtractionStatus(test.stored, test.task); got != test.want {
			t.Fatalf("derived status (%s, %s) = %s, want %s", test.stored, test.task, got, test.want)
		}
	}
}
