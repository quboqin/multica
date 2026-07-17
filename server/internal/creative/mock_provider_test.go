package creative

import (
	"context"
	"testing"
	"time"
)

func TestMockProviderUsesRuleVariantCountAndSizes(t *testing.T) {
	provider := NewMockProvider(time.Nanosecond)
	result, err := provider.GetJob(context.Background(), GetJobRequest{
		ExternalJobID: "mock-1",
		CreatedAt:     time.Now().Add(-time.Second),
		Rules: map[string]any{
			"variant_count": float64(2),
			"sizes": []any{
				map[string]any{"width": float64(1080), "height": float64(1080), "label": "square"},
			},
		},
		Candidates: []Candidate{{ID: "candidate-1", ArchivedURL: "/uploads/source.jpg"}},
	})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(result.Variants) != 2 {
		t.Fatalf("variants = %d, want 2", len(result.Variants))
	}
	for _, variant := range result.Variants {
		if len(variant.Assets) != 1 {
			t.Fatalf("assets = %d, want 1", len(variant.Assets))
		}
		if variant.Assets[0].URL != "/uploads/source.jpg" {
			t.Fatalf("asset URL = %q", variant.Assets[0].URL)
		}
	}
}

func TestMockProviderWaitsBeforeCompleting(t *testing.T) {
	provider := NewMockProvider(time.Hour)
	result, err := provider.GetJob(context.Background(), GetJobRequest{CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if result.Status != "running" || result.Progress != 60 {
		t.Fatalf("result = %+v", result)
	}
}
