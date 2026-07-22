package creative

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const MockProviderName = "mock_creative_mcp"

type MockProvider struct {
	Delay time.Duration
}

func NewMockProvider(delay time.Duration) *MockProvider {
	if delay <= 0 {
		delay = 4 * time.Second
	}
	return &MockProvider{Delay: delay}
}

func (p *MockProvider) Name() string {
	return MockProviderName
}

func (p *MockProvider) CreateJob(_ context.Context, request CreateJobRequest) (CreateJobResult, error) {
	id := strings.ReplaceAll(request.LocalJobID, "-", "")
	if len(id) > 16 {
		id = id[:16]
	}
	return CreateJobResult{
		ExternalJobID: "mock_creative_job_" + id,
		Status:        "running",
		Stage:         CreateJobToolName,
		Progress:      15,
		PollAfter:     time.Second,
	}, nil
}

func (p *MockProvider) GetJob(_ context.Context, request GetJobRequest) (GetJobResult, error) {
	if time.Since(request.CreatedAt) < p.Delay {
		return GetJobResult{
			Status:    "running",
			Stage:     GetJobToolName,
			Progress:  60,
			PollAfter: time.Second,
		}, nil
	}

	variantCount := intFromRules(request.Rules, "variant_count", 3)
	sizes := sizesFromRules(request.Rules)
	variants := make([]Variant, 0, len(request.Candidates)*variantCount)
	for _, candidate := range request.Candidates {
		assetURL := AssetURL(candidate)
		for variantIndex := 1; variantIndex <= variantCount; variantIndex++ {
			assets := make([]Asset, 0, len(sizes))
			for _, size := range sizes {
				assets = append(assets, Asset{
					Width:       size.Width,
					Height:      size.Height,
					Label:       size.Label,
					URL:         assetURL,
					ContentType: "image/jpeg",
				})
			}
			name := firstNonEmpty(candidate.Title, candidate.Competitor, "素材")
			variants = append(variants, Variant{
				CandidateID: candidate.ID,
				Index:       variantIndex,
				Title:       fmt.Sprintf("变体 %d", variantIndex),
				Description: fmt.Sprintf("基于 %s 的 mock 修图结果，真实 MCP 上线后由创意服务返回。", name),
				QCStatus:    "mock",
				Assets:      assets,
			})
		}
	}
	return GetJobResult{
		Status:    "completed",
		Stage:     GetJobToolName,
		Progress:  100,
		Variants:  variants,
		PollAfter: 0,
	}, nil
}

func intFromRules(rules map[string]any, key string, fallback int) int {
	value, ok := rules[key]
	if !ok {
		return fallback
	}
	var parsed int
	switch typed := value.(type) {
	case int:
		parsed = typed
	case int32:
		parsed = int(typed)
	case int64:
		parsed = int(typed)
	case float64:
		parsed = int(typed)
	}
	if parsed < 1 || parsed > 6 {
		return fallback
	}
	return parsed
}

func sizesFromRules(rules map[string]any) []Size {
	defaults := []Size{
		{Width: 1080, Height: 1080, Label: "1080x1080"},
		{Width: 800, Height: 1000, Label: "800x1000"},
		{Width: 1200, Height: 628, Label: "1200x628"},
	}
	raw, ok := rules["sizes"].([]any)
	if !ok || len(raw) == 0 || len(raw) > 6 {
		return defaults
	}
	out := make([]Size, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			return defaults
		}
		width := numberAsInt(entry["width"])
		height := numberAsInt(entry["height"])
		if width < 100 || height < 100 || width > 4096 || height > 4096 {
			return defaults
		}
		label, _ := entry["label"].(string)
		if strings.TrimSpace(label) == "" {
			label = fmt.Sprintf("%dx%d", width, height)
		}
		out = append(out, Size{Width: width, Height: height, Label: strings.TrimSpace(label)})
	}
	return out
}

func numberAsInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
