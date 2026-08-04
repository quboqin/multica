package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFirstCreativeCandidateSourcePrefersArchivedAsset(t *testing.T) {
	candidate := creativeMaterialCandidateCLI{
		PreviewURL:  "https://cdn.example.test/preview.png",
		ResourceURL: "https://cdn.example.test/original.png",
		ArchivedURL: "https://multica.example.test/uploads/source.png",
	}
	if got := firstCreativeCandidateSource(candidate); got != candidate.ArchivedURL {
		t.Fatalf("firstCreativeCandidateSource() = %q, want %q", got, candidate.ArchivedURL)
	}
}

func TestCreativeLibraryDownloadSourceUsesOriginWhileArchiveIsPending(t *testing.T) {
	candidate := creativeMaterialCandidateCLI{
		ArchiveStatus: "pending",
		ArchivedURL:   "https://multica.example.test/incomplete.png",
		OriginalURL:   "https://cdn.example.test/original.png",
		PreviewURL:    "https://cdn.example.test/preview.png",
	}
	gotURL, gotSource := creativeLibraryDownloadSource(candidate)
	if gotURL != candidate.OriginalURL || gotSource != "origin" {
		t.Fatalf("creativeLibraryDownloadSource() = (%q, %q), want (%q, origin)", gotURL, gotSource, candidate.OriginalURL)
	}
	candidate.ArchiveStatus = "completed"
	gotURL, gotSource = creativeLibraryDownloadSource(candidate)
	if gotURL != candidate.ArchivedURL || gotSource != "archive" {
		t.Fatalf("completed creativeLibraryDownloadSource() = (%q, %q), want (%q, archive)", gotURL, gotSource, candidate.ArchivedURL)
	}
}

func TestValidateGPTImageSize(t *testing.T) {
	for _, size := range []string{"auto", "1024x1536", "1088x1360"} {
		if err := validateGPTImageSize(size); err != nil {
			t.Fatalf("validateGPTImageSize(%q): %v", size, err)
		}
	}
	for _, size := range []string{"1080x1350", "1025x1536", "4000x1024"} {
		if err := validateGPTImageSize(size); err == nil {
			t.Fatalf("validateGPTImageSize(%q) unexpectedly passed", size)
		}
	}
}

func TestNormalizedOpenAIImageBaseURL(t *testing.T) {
	got, err := normalizedOpenAIImageBaseURL("https://example.test/v1/")
	if err != nil || got != "https://example.test/v1/images/edits" {
		t.Fatalf("normalizedOpenAIImageBaseURL() = %q, %v", got, err)
	}
	if got, err := openAIImageEditEndpoint("http://one-ai.adakamicorp.id/", "/images/edits"); err != nil || got != "http://one-ai.adakamicorp.id/images/edits" {
		t.Fatalf("internal one-ai endpoint = %q, %v", got, err)
	}
	if got, err := openAIImageEditEndpoint("http://one-ai.adakamicorp.id", "custom/edits"); err != nil || got != "http://one-ai.adakamicorp.id/custom/edits" {
		t.Fatalf("custom edit path = %q, %v", got, err)
	}
	if _, err := normalizedOpenAIImageBaseURL("one-ai"); err == nil {
		t.Fatal("expected a bare host name to be rejected")
	}
}

func TestRequestGPTImageEdit(t *testing.T) {
	input := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(input, []byte("source-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "gpt-image-2" {
			t.Fatalf("model = %q", got)
		}
		if got := r.FormValue("size"); got != "1088x1360" {
			t.Fatalf("size = %q", got)
		}
		files := r.MultipartForm.File["image"]
		if len(files) != 1 || files[0].Filename != "reference.png" {
			t.Fatalf("image files = %#v", files)
		}
		if got := files[0].Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("image content type = %q", got)
		}
		opened, err := files[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		body, err := io.ReadAll(opened)
		if err != nil || string(body) != "source-bytes" {
			t.Fatalf("input body = %q, %v", body, err)
		}
		w.Header().Set("x-request-id", "req_image_test")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png-bytes")) + `"}]}`))
	}))
	defer server.Close()
	image, requestID, err := requestGPTImageEdit(context.Background(), server.Client(), server.URL+"/v1/images/edits", "test-key", "gpt-image-2", "image", []string{input}, "", "change the layout", "1088x1360", "medium")
	if err != nil || string(image) != "png-bytes" || requestID != "req_image_test" {
		t.Fatalf("requestGPTImageEdit() = %q, %q, %v", image, requestID, err)
	}
}

func TestRequestGPTImageEditWithRetryRecoversFromRateLimit(t *testing.T) {
	input := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(input, []byte("source-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.Header().Set("x-request-id", "req_rate_limited")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		w.Header().Set("x-request-id", "req_recovered")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png-bytes")) + `"}]}`))
	}))
	defer server.Close()

	image, requestID, usedAttempts, err := requestGPTImageEditWithRetry(
		context.Background(), server.Client(), server.URL, "test-key", "gpt-image-2", "image",
		[]string{input}, "", "change the layout", "1088x1088", "medium", 3,
	)
	if err != nil || string(image) != "png-bytes" || requestID != "req_recovered" || usedAttempts != 3 {
		t.Fatalf("requestGPTImageEditWithRetry() = %q, %q, %d, %v", image, requestID, usedAttempts, err)
	}
}

func TestImageEditFileContentType(t *testing.T) {
	cases := map[string]string{
		"reference.png":  "image/png",
		"reference.JPEG": "image/jpeg",
		"reference.webp": "image/webp",
		"reference.bin":  "application/octet-stream",
	}
	for path, want := range cases {
		if got := imageEditFileContentType(path); got != want {
			t.Errorf("imageEditFileContentType(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestTruncateCLIError(t *testing.T) {
	if got := truncateCLIError([]byte("  hello  "), 20); got != "hello" {
		t.Fatalf("truncateCLIError = %q", got)
	}
	if got := truncateCLIError([]byte(strings.Repeat("x", 10)), 4); got != "xxxx..." {
		t.Fatalf("truncateCLIError = %q", got)
	}
}

func TestCrawlParamsWithAnalysisAgentPreservesExistingIntent(t *testing.T) {
	params, err := crawlParamsWithAnalysisAgent(json.RawMessage(`{"intent":"cash loan"}`), "agent-123")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(params, &got); err != nil {
		t.Fatal(err)
	}
	if got["intent"] != "cash loan" || got["analysis_agent_id"] != "agent-123" {
		t.Fatalf("params = %#v", got)
	}
}

func TestCreativeMaterialsCLIResponsePreservesDeliveries(t *testing.T) {
	var response creativeMaterialsCLIResponse
	err := json.Unmarshal([]byte(`{
		"candidates":[{
			"id":"candidate-1","source_issue_id":"issue-1","source_run_id":"run-1","is_new_in_run":true
		}],
		"crawl_runs":[{
			"id":"run-1","issue_id":"issue-1","status":"completed","imported_count":25
		}],
		"items":[],"context":{},"adjustment_requests":[],
		"deliveries":[{
			"id":"delivery-1","issue_id":"issue-1","candidate_id":"candidate-1",
			"work_issue_id":"work-1","variant":3,"size":"1200x628","revision":2,
			"base_attachment_id":"base-1","final_attachment_id":"final-1",
			"prime_evidence_attachment_id":"prime-1","qc_issue_id":"qc-1"
		}]
	}`), &response)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(response.Deliveries))
	}
	if len(response.CrawlRuns) != 1 || response.CrawlRuns[0].ID != "run-1" || response.CrawlRuns[0].ImportedCount != 25 {
		t.Fatalf("crawl runs = %#v", response.CrawlRuns)
	}
	if len(response.Candidates) != 1 || response.Candidates[0].SourceRunID != "run-1" || !response.Candidates[0].IsNewInRun {
		t.Fatalf("candidate crawl identity = %#v", response.Candidates)
	}
	delivery := response.Deliveries[0]
	if delivery.CandidateID != "candidate-1" || delivery.Variant != 3 || delivery.Size != "1200x628" || delivery.FinalAttachmentID != "final-1" {
		t.Fatalf("delivery = %#v", delivery)
	}
}

func TestFilterCreativeLibraryByRunKeepsOnlyMatchingCandidates(t *testing.T) {
	input := map[string]any{
		"candidates": []any{
			map[string]any{"id": "one", "source_run_id": "run-a"},
			map[string]any{"id": "two", "source_run_id": "run-b"},
		},
		"crawl_runs": []any{map[string]any{"id": "run-a"}},
	}
	filtered := filterCreativeLibraryByRun(input, "run-a")
	candidates := anySlice(filtered["candidates"])
	if len(candidates) != 1 || candidates[0].(map[string]any)["id"] != "one" {
		t.Fatalf("filtered candidates = %#v", candidates)
	}
	if filtered["crawl_run_id"] != "run-a" {
		t.Fatalf("crawl_run_id = %#v", filtered["crawl_run_id"])
	}
	if len(anySlice(input["candidates"])) != 2 {
		t.Fatal("filter mutated the source response")
	}
}

func TestCreativeLibraryListCommandExposesRunFilter(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"creative", "library", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if command != creativeLibraryListCmd || !command.Runnable() {
		t.Fatalf("creative library list command = %#v", command)
	}
	if command.Flags().Lookup("run-id") == nil || command.Flags().Lookup("output") == nil {
		t.Fatal("creative library list must expose run-id and output flags")
	}
}

func TestExecuteImageEditBatchRunsDependentJobsTogether(t *testing.T) {
	t.Setenv("MULTICA_IMAGE_SLOT_DIR", t.TempDir())
	t.Setenv("MULTICA_IMAGE_MAX_CONCURRENT", "5")
	workDir := t.TempDir()
	reference := filepath.Join(workDir, "reference.png")
	if err := os.WriteFile(reference, []byte("reference"), 0o600); err != nil {
		t.Fatal(err)
	}

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	var derivativeCount atomic.Int32
	releaseDerivatives := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			previous := maxInFlight.Load()
			if current <= previous || maxInFlight.CompareAndSwap(previous, current) {
				break
			}
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		size := r.FormValue("size")
		payload := "square"
		if size != "1088x1088" {
			files := r.MultipartForm.File["image"]
			if len(files) != 2 || files[0].Filename != "square.png" {
				t.Errorf("derivative inputs = %#v", files)
			}
			opened, err := files[0].Open()
			if err != nil {
				t.Error(err)
			} else {
				body, readErr := io.ReadAll(opened)
				_ = opened.Close()
				if readErr != nil || string(body) != "square" {
					t.Errorf("derivative source = %q, %v", body, readErr)
				}
			}
			payload = size
			if derivativeCount.Add(1) == 2 {
				releaseOnce.Do(func() { close(releaseDerivatives) })
			}
			select {
			case <-releaseDerivatives:
			case <-time.After(2 * time.Second):
				t.Error("derivative jobs did not overlap")
			}
		}
		w.Header().Set("x-request-id", "req-"+size)
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString([]byte(payload)))
	}))
	defer server.Close()

	batch := preparedImageEditBatch{
		MaxConcurrency: 2,
		Jobs: []preparedImageEditJob{
			{ID: "square", Inputs: []imageEditBatchInput{{Path: reference}}, Prompt: "square", Model: "gpt-image-2", Size: "1088x1088", MaxAttempts: 1, OutputFile: filepath.Join(workDir, "square.png")},
			{ID: "landscape", Inputs: []imageEditBatchInput{{Job: "square"}, {Path: reference}}, Prompt: "landscape", Model: "gpt-image-2", Size: "1680x880", MaxAttempts: 1, OutputFile: filepath.Join(workDir, "landscape.png"), DependsOn: []string{"square"}},
			{ID: "portrait", Inputs: []imageEditBatchInput{{Job: "square"}, {Path: reference}}, Prompt: "portrait", Model: "gpt-image-2", Size: "832x1040", MaxAttempts: 1, OutputFile: filepath.Join(workDir, "portrait.png"), DependsOn: []string{"square"}},
		},
	}
	summary := executeImageEditBatch(context.Background(), server.Client(), server.URL, "test-key", "image", batch)
	if summary.Succeeded != 3 || summary.Failed != 0 || summary.Skipped != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if maxInFlight.Load() != 2 {
		t.Fatalf("max in-flight requests = %d, want 2", maxInFlight.Load())
	}
}

func TestGlobalImageSlotsBoundConcurrency(t *testing.T) {
	t.Setenv("MULTICA_IMAGE_SLOT_DIR", t.TempDir())
	t.Setenv("MULTICA_IMAGE_MAX_CONCURRENT", "2")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type acquisition struct {
		release func() error
		err     error
	}
	acquired := make(chan acquisition, 3)
	for range 3 {
		go func() {
			release, err := acquireGlobalImageSlot(ctx, "https://provider.example/images", "account-key")
			acquired <- acquisition{release: release, err: err}
		}()
	}
	first := <-acquired
	second := <-acquired
	if first.err != nil || second.err != nil {
		t.Fatalf("initial acquisitions = %v, %v", first.err, second.err)
	}
	select {
	case third := <-acquired:
		if third.release != nil {
			_ = third.release()
		}
		t.Fatal("third acquisition bypassed the two-slot limit")
	case <-time.After(150 * time.Millisecond):
	}
	if err := first.release(); err != nil {
		t.Fatal(err)
	}
	third := <-acquired
	if third.err != nil {
		t.Fatal(third.err)
	}
	if err := second.release(); err != nil {
		t.Fatal(err)
	}
	if err := third.release(); err != nil {
		t.Fatal(err)
	}
}
