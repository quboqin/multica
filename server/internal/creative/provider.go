package creative

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const (
	CreateJobToolName = "create_creative_job"
	GetJobToolName    = "get_creative_job"
)

type Candidate struct {
	ID          string
	Competitor  string
	Title       string
	AssetType   string
	PreviewURL  string
	ResourceURL string
	PosterURL   string
	ArchivedURL string
}

type Size struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Label  string `json:"label"`
}

type CreateJobRequest struct {
	LocalJobID string
	Prompt     string
	Rules      map[string]any
	Candidates []Candidate
	CreatedAt  time.Time
}

type CreateJobResult struct {
	ExternalJobID string
	Status        string
	Stage         string
	Progress      int
	PollAfter     time.Duration
	ProcessData   json.RawMessage
}

type GetJobRequest struct {
	ExternalJobID string
	Prompt        string
	Rules         map[string]any
	Candidates    []Candidate
	CreatedAt     time.Time
}

type Asset struct {
	Width       int
	Height      int
	Label       string
	URL         string
	ContentType string
	StorageKey  string
}

type Variant struct {
	CandidateID string
	Index       int
	Title       string
	Description string
	QCStatus    string
	Assets      []Asset
}

type GetJobResult struct {
	Status       string
	Stage        string
	Progress     int
	PollAfter    time.Duration
	ErrorMessage string
	Variants     []Variant
	ProcessData  json.RawMessage
}

type Provider interface {
	Name() string
	CreateJob(context.Context, CreateJobRequest) (CreateJobResult, error)
	GetJob(context.Context, GetJobRequest) (GetJobResult, error)
}

func AssetURL(candidate Candidate) string {
	for _, value := range []string{
		candidate.ArchivedURL,
		candidate.PreviewURL,
		candidate.PosterURL,
		candidate.ResourceURL,
	} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
