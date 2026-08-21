package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var creativeCmd = &cobra.Command{
	Use:   "creative",
	Short: "Work with creative material candidates",
}

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Process raster images with configured local capabilities",
}

var creativeMaterialsCmd = &cobra.Command{
	Use:   "materials <issue-id>",
	Short: "List creative materials attached to an issue",
	Args:  exactArgs(1),
	RunE:  runCreativeMaterials,
}

var creativeMaterialCmd = &cobra.Command{
	Use:   "material",
	Short: "Work with one creative material candidate",
}

var creativeContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage the fixed creative resource snapshot for an issue",
}

var creativeContextSetCmd = &cobra.Command{
	Use:   "set <issue-id>",
	Short: "Fix a published market pack and execution squad to an issue",
	Args:  exactArgs(1),
	RunE:  runCreativeContextSet,
}

var creativeDeliveryCmd = &cobra.Command{
	Use:   "delivery",
	Short: "Register final creative deliveries",
}

var creativeDeliveryRegisterCmd = &cobra.Command{
	Use:   "register <issue-id>",
	Short: "Register final attachments and their source mapping from a JSON manifest",
	Args:  exactArgs(1),
	RunE:  runCreativeDeliveryRegister,
}

var creativeMaterialDownloadCmd = &cobra.Command{
	Use:   "download <issue-id> <candidate-id>",
	Short: "Download a selected candidate from platform archive storage",
	Args:  exactArgs(2),
	RunE:  runCreativeMaterialDownload,
}

var creativeMaterialBriefCmd = &cobra.Command{
	Use:   "brief <issue-id> <candidate-id>",
	Short: "Save structured visual intent and benefit analysis for a selected candidate",
	Args:  exactArgs(2),
	RunE:  runCreativeMaterialBrief,
}

var imageEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit reference images with the configured GPT Image API",
	Args:  cobra.NoArgs,
	RunE:  runImageEdit,
}

type openAIImageEditResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

type imageEditHTTPError struct {
	StatusCode int
	RequestID  string
	RetryAfter string
	Body       string
}

const maxProviderImageAspectDeviation = 0.05
const maxProviderImageAspectRetries = 1

type imageOutputDimensions struct {
	Width       int
	Height      int
	AspectRatio float64
}

type providerImageAspectDeviationError struct {
	actualWidth      int
	actualHeight     int
	expectedWidth    int
	expectedHeight   int
	actualAspect     float64
	expectedAspect   float64
	aspectDeviation  float64
	maximumDeviation float64
}

func (e *providerImageAspectDeviationError) Error() string {
	return fmt.Sprintf(
		"provider returned %dx%d (aspect %.5f) for requested %dx%d (aspect %.5f); aspect deviation %.2f%% exceeds %.2f%%",
		e.actualWidth, e.actualHeight, e.actualAspect,
		e.expectedWidth, e.expectedHeight, e.expectedAspect,
		e.aspectDeviation*100, e.maximumDeviation*100,
	)
}

func (e *imageEditHTTPError) Error() string {
	if e.RetryAfter != "" {
		return fmt.Sprintf("GPT Image edit failed with status %d (request_id=%s, retry_after=%s): %s", e.StatusCode, e.RequestID, e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("GPT Image edit failed with status %d (request_id=%s): %s", e.StatusCode, e.RequestID, e.Body)
}

type imageEditTransportError struct {
	err error
}

func (e *imageEditTransportError) Error() string {
	return fmt.Sprintf("request GPT Image edit: %v", e.err)
}
func (e *imageEditTransportError) Unwrap() error { return e.err }

type creativeMaterialsCLIResponse struct {
	Candidates  []creativeMaterialCandidateCLI `json:"candidates"`
	CrawlRuns   []creativeMaterialCrawlRunCLI  `json:"crawl_runs"`
	Items       []creativeIssueItemCLI         `json:"items"`
	Deliveries  []creativeDeliveryCLI          `json:"deliveries"`
	Adjustments []json.RawMessage              `json:"adjustments"`
	Context     json.RawMessage                `json:"context"`
}

type creativeMaterialCrawlRunCLI struct {
	ID            string `json:"id"`
	IssueID       string `json:"issue_id"`
	ConnectorID   string `json:"connector_id"`
	QuerySummary  string `json:"query_summary"`
	Status        string `json:"status"`
	ImportedCount int    `json:"imported_count"`
	ExistingCount int    `json:"existing_count"`
	TotalCount    int    `json:"total_count"`
	CreatedAt     string `json:"created_at"`
}

type creativeDeliveryCLI struct {
	ID                        string `json:"id"`
	IssueID                   string `json:"issue_id"`
	CandidateID               string `json:"candidate_id"`
	WorkIssueID               string `json:"work_issue_id"`
	Variant                   int    `json:"variant"`
	Size                      string `json:"size"`
	Revision                  int    `json:"revision"`
	BaseAttachmentID          string `json:"base_attachment_id"`
	FinalAttachmentID         string `json:"final_attachment_id"`
	PrimeEvidenceAttachmentID string `json:"prime_evidence_attachment_id"`
	QCIssueID                 string `json:"qc_issue_id"`
	CreatedAt                 string `json:"created_at"`
	UpdatedAt                 string `json:"updated_at"`
}

type creativeIssueItemCLI struct {
	CandidateID   string          `json:"candidate_id"`
	CreativeBrief json.RawMessage `json:"creative_brief"`
	WorkIssueID   string          `json:"work_issue_id"`
	Revision      int             `json:"revision"`
	Status        string          `json:"status"`
}

type creativeMaterialCandidateCLI struct {
	ID                 string `json:"id"`
	Competitor         string `json:"competitor"`
	Title              string `json:"title"`
	AssetType          string `json:"asset_type"`
	PreviewURL         string `json:"preview_url"`
	ResourceURL        string `json:"resource_url"`
	PosterURL          string `json:"poster_url"`
	OriginalURL        string `json:"original_url"`
	ArchivedURL        string `json:"archived_url"`
	ArchiveStatus      string `json:"archive_status"`
	Status             string `json:"status"`
	SourceAttachmentID string `json:"source_attachment_id"`
	SourceIssueID      string `json:"source_issue_id"`
	SourceRunID        string `json:"source_run_id"`
	IsNewInRun         bool   `json:"is_new_in_run"`
}

func init() {
	creativeCmd.AddCommand(creativeMaterialsCmd)
	creativeCmd.AddCommand(creativeMaterialCmd)
	creativeCmd.AddCommand(creativeContextCmd)
	creativeCmd.AddCommand(creativeDeliveryCmd)
	creativeContextCmd.AddCommand(creativeContextSetCmd)
	creativeDeliveryCmd.AddCommand(creativeDeliveryRegisterCmd)
	creativeMaterialCmd.AddCommand(creativeMaterialDownloadCmd)
	creativeMaterialCmd.AddCommand(creativeMaterialBriefCmd)
	imageCmd.AddCommand(imageEditCmd)

	creativeMaterialsCmd.Flags().Bool("selected", false, "Only return materials selected by a person")
	creativeMaterialsCmd.Flags().String("output", "json", "Output format: json or table")
	creativeMaterialDownloadCmd.Flags().String("output-file", "", "Local file path for the archived candidate")
	creativeMaterialDownloadCmd.Flags().String("output", "json", "Output format: json or table")
	creativeMaterialBriefCmd.Flags().String("input-file", "", "UTF-8 JSON file containing the creative brief")
	creativeMaterialBriefCmd.Flags().String("output", "json", "Output format: json or table")
	creativeContextSetCmd.Flags().String("market-pack-id", "", "Published creative market pack UUID")
	creativeContextSetCmd.Flags().String("squad-id", "", "Execution squad UUID")
	creativeContextSetCmd.Flags().String("output", "json", "Output format: json")
	creativeDeliveryRegisterCmd.Flags().String("input-file", "", "UTF-8 JSON manifest containing a deliveries array")
	creativeDeliveryRegisterCmd.Flags().String("output", "json", "Output format: json")

	imageEditCmd.Flags().StringSlice("input", nil, "Reference image files (1-16 files)")
	imageEditCmd.Flags().String("mask", "", "Optional PNG mask with alpha channel")
	imageEditCmd.Flags().String("prompt", "", "Edit prompt")
	imageEditCmd.Flags().Bool("prompt-stdin", false, "Read the prompt from stdin")
	imageEditCmd.Flags().String("prompt-file", "", "Read the prompt from a UTF-8 file")
	imageEditCmd.Flags().String("model", "gpt-image-2", "GPT Image model")
	imageEditCmd.Flags().String("size", "auto", "Canvas size, e.g. 1088x1360 or auto")
	imageEditCmd.Flags().String("quality", "", "Optional image quality: low, medium, high, or auto")
	imageEditCmd.Flags().Int("max-attempts", 3, "Maximum attempts for transient image API failures (1-5)")
	imageEditCmd.Flags().String("output-file", "", "Output PNG file")
	imageEditCmd.Flags().String("output", "json", "Output format: json or table")
}

func runCreativeContextSet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	marketPackID, _ := cmd.Flags().GetString("market-pack-id")
	squadID, _ := cmd.Flags().GetString("squad-id")
	if strings.TrimSpace(marketPackID) == "" || strings.TrimSpace(squadID) == "" {
		return fmt.Errorf("--market-pack-id and --squad-id are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()
	var result map[string]any
	path := "/api/issues/" + url.PathEscape(args[0]) + "/creative-context"
	body := map[string]string{"market_pack_id": marketPackID, "squad_id": squadID}
	if err := client.PutJSON(ctx, path, body, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runCreativeDeliveryRegister(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	inputFile, _ := cmd.Flags().GetString("input-file")
	if strings.TrimSpace(inputFile) == "" {
		return fmt.Errorf("--input-file is required")
	}
	payload, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("read creative delivery manifest: %w", err)
	}
	var manifest struct {
		Deliveries []json.RawMessage `json:"deliveries"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return fmt.Errorf("decode creative delivery manifest: %w", err)
	}
	if len(manifest.Deliveries) == 0 || len(manifest.Deliveries) > 9 {
		return fmt.Errorf("creative delivery manifest must contain between 1 and 9 deliveries")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()
	var result map[string]any
	path := "/api/issues/" + url.PathEscape(args[0]) + "/creative-deliveries/register"
	if err := client.PostJSON(ctx, path, json.RawMessage(payload), &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runCreativeMaterialBrief(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	inputFile, _ := cmd.Flags().GetString("input-file")
	if strings.TrimSpace(inputFile) == "" {
		return fmt.Errorf("--input-file is required")
	}
	payload, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("read creative brief: %w", err)
	}
	var brief map[string]any
	if err := json.Unmarshal(payload, &brief); err != nil {
		return fmt.Errorf("decode creative brief JSON: %w", err)
	}
	if len(brief) == 0 {
		return fmt.Errorf("creative brief JSON must be a non-empty object")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()
	var result creativeIssueItemCLI
	path := "/api/issues/" + url.PathEscape(args[0]) + "/creative-materials/" + url.PathEscape(args[1]) + "/brief"
	if err := client.PutJSON(ctx, path, brief, &result); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		cli.PrintTable(os.Stdout, []string{"CANDIDATE", "REVISION", "STATUS"}, [][]string{{result.CandidateID, strconv.Itoa(result.Revision), result.Status}})
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runCreativeMaterialDownload(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	outputFile, _ := cmd.Flags().GetString("output-file")
	if strings.TrimSpace(outputFile) == "" {
		return fmt.Errorf("--output-file is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(2*time.Minute))
	defer cancel()
	var response creativeMaterialsCLIResponse
	if err := client.GetJSON(ctx, "/api/issues/"+url.PathEscape(args[0])+"/creative-materials", &response); err != nil {
		return err
	}
	var selected *creativeMaterialCandidateCLI
	for index := range response.Candidates {
		candidate := &response.Candidates[index]
		if candidate.ID == args[1] {
			selected = candidate
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("creative material candidate %s was not found on issue %s", args[1], args[0])
	}
	downloadURL, source := creativeLibraryDownloadSource(*selected)
	if downloadURL == "" {
		return fmt.Errorf("creative material candidate %s has no readable asset URL", args[1])
	}
	if source != "attachment" && (selected.ArchiveStatus != "completed" || strings.TrimSpace(selected.ArchivedURL) == "") {
		return fmt.Errorf("creative material candidate %s is not available in platform archive storage", args[1])
	}
	data, err := client.DownloadFile(ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("download creative material from %s: %w", source, err)
	}
	directory := filepath.Dir(outputFile)
	if directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(outputFile, data, 0o644); err != nil {
		return fmt.Errorf("write creative material: %w", err)
	}
	abs, err := filepath.Abs(outputFile)
	if err != nil {
		abs = outputFile
	}
	result := map[string]any{
		"issue_id": args[0], "candidate_id": selected.ID, "archive_status": selected.ArchiveStatus,
		"path": abs, "bytes": len(data), "source": source,
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		cli.PrintTable(os.Stdout, []string{"ISSUE", "CANDIDATE", "BYTES", "PATH"}, [][]string{{args[0], selected.ID, strconv.Itoa(len(data)), abs}})
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runImageEdit(cmd *cobra.Command, _ []string) error {
	inputs, _ := cmd.Flags().GetStringSlice("input")
	inputs = uniqueCLIStrings(inputs)
	if len(inputs) == 0 || len(inputs) > 16 {
		return fmt.Errorf("--input requires between 1 and 16 reference images")
	}
	prompt, _, err := resolveTextFlag(cmd, "prompt")
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("--prompt, --prompt-file, or --prompt-stdin is required")
	}
	outputFile, _ := cmd.Flags().GetString("output-file")
	if strings.TrimSpace(outputFile) == "" {
		return fmt.Errorf("--output-file is required")
	}
	model, _ := cmd.Flags().GetString("model")
	if strings.TrimSpace(model) != "gpt-image-2" {
		return fmt.Errorf("only gpt-image-2 is supported by this direct image-edit capability")
	}
	size, _ := cmd.Flags().GetString("size")
	size = strings.TrimSpace(size)
	providerSize, err := providerGPTImageSize(size)
	if err != nil {
		return err
	}
	quality, _ := cmd.Flags().GetString("quality")
	quality = strings.TrimSpace(strings.ToLower(quality))
	if quality != "" && !map[string]bool{"low": true, "medium": true, "high": true, "auto": true}[quality] {
		return fmt.Errorf("--quality must be low, medium, high, or auto")
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required for image edit")
	}
	endpoint, err := openAIImageEditEndpoint(os.Getenv("OPENAI_BASE_URL"), os.Getenv("OPENAI_IMAGE_EDIT_PATH"))
	if err != nil {
		return err
	}
	imageField := strings.TrimSpace(os.Getenv("OPENAI_IMAGE_FILE_FIELD"))
	if imageField == "" {
		imageField = "image"
	}
	providerSlotLimit, err := configuredImageConcurrency()
	if err != nil {
		return err
	}
	mask, _ := cmd.Flags().GetString("mask")
	maxAttempts, _ := cmd.Flags().GetInt("max-attempts")
	if maxAttempts < 1 || maxAttempts > 5 {
		return fmt.Errorf("--max-attempts must be between 1 and 5")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(50*time.Minute))
	defer cancel()
	image, requestID, attempts, aspectRetries, dimensions, aspectFallback, timing, err := requestGPTImageEditWithValidAspect(ctx, http.DefaultClient, endpoint, apiKey, model, imageField, inputs, mask, prompt, providerSize, quality, maxAttempts, acquireGlobalImageSlot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil && filepath.Dir(outputFile) != "." {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outputFile, image, 0o644); err != nil {
		return fmt.Errorf("write image output: %w", err)
	}
	abs, err := filepath.Abs(outputFile)
	if err != nil {
		abs = outputFile
	}
	output, _ := cmd.Flags().GetString("output")
	result := map[string]any{
		"model": model, "input_count": len(inputs), "size": size, "provider_size": providerSize, "quality": quality,
		"path": abs, "bytes": len(image), "request_id": requestID, "attempts": attempts, "aspect_retries": aspectRetries, "aspect_fallback": aspectFallback,
		"actual_width": dimensions.Width, "actual_height": dimensions.Height, "actual_aspect_ratio": dimensions.AspectRatio,
		"provider_slot_limit": providerSlotLimit, "prompt": prompt, "prompt_sha256": imagePromptSHA256(prompt),
		"queue_wait_seconds": timing.QueueWait.Seconds(), "provider_elapsed_seconds": timing.ProviderElapsed.Seconds(), "timeout_stage": timing.TimeoutStage,
		"generated_asset": map[string]any{
			"completed": true, "path": abs, "size": size, "width": dimensions.Width,
			"height": dimensions.Height, "aspect_fallback": aspectFallback,
		},
	}
	if output == "table" {
		cli.PrintTable(os.Stdout, []string{"MODEL", "INPUTS", "SIZE", "QUALITY", "BYTES", "REQUEST ID", "PATH"}, [][]string{{model, strconv.Itoa(len(inputs)), size, quality, strconv.Itoa(len(image)), requestID, abs}})
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

// providerGPTImageSize keeps the public delivery contract independent from the
// provider's 16px canvas constraint. The normalized asset is still written at
// the requested delivery size and the original request remains in the trace.
func providerGPTImageSize(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if err := validateGPTImageSize(requested); err == nil {
		return requested, nil
	} else {
		canonical := strings.ToLower(requested)
		switch canonical {
		case "1080x1080":
			return "1088x1088", nil
		case "1200x628":
			return "1200x624", nil
		case "800x1000":
			return "800x992", nil
		default:
			return "", err
		}
	}
}

func imagePromptSHA256(prompt string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))
}

func requestGPTImageEditWithRetry(ctx context.Context, client *http.Client, endpoint, apiKey, model, imageField string, inputs []string, mask, prompt, size, quality string, maxAttempts int) ([]byte, string, int, error) {
	return requestGPTImageEditWithRetryUsingSlots(ctx, client, endpoint, apiKey, model, imageField, inputs, mask, prompt, size, quality, maxAttempts, nil)
}

func requestGPTImageEditWithRetryAndSlots(ctx context.Context, client *http.Client, endpoint, apiKey, model, imageField string, inputs []string, mask, prompt, size, quality string, maxAttempts int) ([]byte, string, int, error) {
	return requestGPTImageEditWithRetryUsingSlots(ctx, client, endpoint, apiKey, model, imageField, inputs, mask, prompt, size, quality, maxAttempts, acquireGlobalImageSlot)
}

type imageSlotAcquirer func(context.Context, string, string) (func() error, error)

type imageEditTiming struct {
	QueueWait       time.Duration
	ProviderElapsed time.Duration
	TimeoutStage    string
}

func requestGPTImageEditWithRetryUsingSlots(ctx context.Context, client *http.Client, endpoint, apiKey, model, imageField string, inputs []string, mask, prompt, size, quality string, maxAttempts int, acquire imageSlotAcquirer) ([]byte, string, int, error) {
	image, requestID, attempts, _, err := requestGPTImageEditWithRetryUsingSlotsTimed(ctx, client, endpoint, apiKey, model, imageField, inputs, mask, prompt, size, quality, maxAttempts, acquire)
	return image, requestID, attempts, err
}

func requestGPTImageEditWithRetryUsingSlotsTimed(ctx context.Context, client *http.Client, endpoint, apiKey, model, imageField string, inputs []string, mask, prompt, size, quality string, maxAttempts int, acquire imageSlotAcquirer) ([]byte, string, int, imageEditTiming, error) {
	var lastRequestID string
	var lastErr error
	var timing imageEditTiming
	usedAttempts := 0
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		usedAttempts = attempt
		var release func() error
		if acquire != nil {
			queuedAt := time.Now()
			var err error
			release, err = acquire(ctx, endpoint, apiKey)
			timing.QueueWait += time.Since(queuedAt)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					timing.TimeoutStage = "queue"
				}
				return nil, lastRequestID, attempt, timing, fmt.Errorf("acquire image concurrency slot: %w", err)
			}
		}
		providerCtx, cancelProvider := context.WithTimeout(context.WithoutCancel(ctx), cli.AtLeastAPITimeout(20*time.Minute))
		providerStartedAt := time.Now()
		image, requestID, err := requestGPTImageEdit(providerCtx, client, endpoint, apiKey, model, imageField, inputs, mask, prompt, size, quality)
		timing.ProviderElapsed += time.Since(providerStartedAt)
		if errors.Is(providerCtx.Err(), context.DeadlineExceeded) {
			timing.TimeoutStage = "provider"
		}
		cancelProvider()
		if release != nil {
			if releaseErr := release(); err == nil && releaseErr != nil {
				err = fmt.Errorf("release image concurrency slot: %w", releaseErr)
			}
		}
		if err == nil {
			return image, requestID, attempt, timing, nil
		}
		lastRequestID, lastErr = requestID, err
		delay, retry := imageEditRetryDelay(err, attempt)
		if !retry || attempt == maxAttempts {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				timing.TimeoutStage = "queue"
			}
			return nil, lastRequestID, attempt, timing, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastRequestID, usedAttempts, timing, fmt.Errorf("GPT Image edit failed after %d attempt(s): %w", usedAttempts, lastErr)
}

func imageEditRetryDelay(err error, attempt int) (time.Duration, bool) {
	var httpErr *imageEditHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode != http.StatusRequestTimeout && httpErr.StatusCode != http.StatusTooManyRequests && httpErr.StatusCode < 500 {
			return 0, false
		}
		if seconds, parseErr := strconv.Atoi(strings.TrimSpace(httpErr.RetryAfter)); parseErr == nil && seconds >= 0 {
			return minDuration(time.Duration(seconds)*time.Second, 30*time.Second), true
		}
		return minDuration(time.Duration(1<<maxInt(attempt-1, 0))*2*time.Second, 30*time.Second), true
	}
	var transportErr *imageEditTransportError
	if errors.As(err, &transportErr) {
		return minDuration(time.Duration(1<<maxInt(attempt-1, 0))*2*time.Second, 30*time.Second), true
	}
	return 0, false
}

func openAIImageEditEndpoint(rawBaseURL, rawPath string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = "/images/edits"
	}
	path = "/" + strings.TrimLeft(path, "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("OPENAI_BASE_URL must be an absolute OpenAI-compatible URL")
	}
	return base + path, nil
}

func normalizedOpenAIImageBaseURL(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return openAIImageEditEndpoint(base, "/images/edits")
}

func validateGPTImageSize(size string) error {
	if strings.EqualFold(strings.TrimSpace(size), "auto") {
		return nil
	}
	width, height, err := parseGPTImageSize(size)
	if err != nil {
		return err
	}
	if width <= 0 || height <= 0 || width > 3840 || height > 3840 || width%16 != 0 || height%16 != 0 || width*height < 655360 || width*height > 8294400 || maxInt(width, height) > 3*minInt(width, height) {
		return fmt.Errorf("--size must use 16px multiples, 655360-8294400 pixels, maximum edge 3840px, and at most 3:1 ratio")
	}
	return nil
}

func parseGPTImageSize(size string) (int, int, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--size must be auto or WIDTHxHEIGHT")
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("--size must be auto or WIDTHxHEIGHT")
	}
	return width, height, nil
}

func validateProviderImageOutput(data []byte, requestedSize string) (imageOutputDimensions, error) {
	dimensions, err := decodeImageDimensions(data)
	if err != nil {
		return imageOutputDimensions{}, err
	}
	if strings.EqualFold(strings.TrimSpace(requestedSize), "auto") {
		return dimensions, nil
	}
	expectedWidth, expectedHeight, err := parseGPTImageSize(requestedSize)
	if err != nil {
		return imageOutputDimensions{}, err
	}
	expectedAspectRatio := float64(expectedWidth) / float64(expectedHeight)
	deviation := math.Abs(dimensions.AspectRatio/expectedAspectRatio - 1)
	if deviation > maxProviderImageAspectDeviation {
		return imageOutputDimensions{}, &providerImageAspectDeviationError{
			actualWidth: dimensions.Width, actualHeight: dimensions.Height, expectedWidth: expectedWidth, expectedHeight: expectedHeight,
			actualAspect: dimensions.AspectRatio, expectedAspect: expectedAspectRatio,
			aspectDeviation: deviation, maximumDeviation: maxProviderImageAspectDeviation,
		}
	}
	return dimensions, nil
}

func decodeImageDimensions(data []byte) (imageOutputDimensions, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return imageOutputDimensions{}, fmt.Errorf("provider returned an unreadable image: %w", err)
	}
	return imageOutputDimensions{
		Width:       config.Width,
		Height:      config.Height,
		AspectRatio: float64(config.Width) / float64(config.Height),
	}, nil
}

func requestGPTImageEditWithValidAspect(
	ctx context.Context,
	client *http.Client,
	endpoint, apiKey, model, imageField string,
	inputs []string,
	mask, prompt, size, quality string,
	maxAttempts int,
	acquire imageSlotAcquirer,
) ([]byte, string, int, int, imageOutputDimensions, bool, imageEditTiming, error) {
	totalAttempts := 0
	lastRequestID := ""
	var totalTiming imageEditTiming
	for aspectAttempt := 0; aspectAttempt <= maxProviderImageAspectRetries; aspectAttempt++ {
		image, requestID, attempts, timing, err := requestGPTImageEditWithRetryUsingSlotsTimed(ctx, client, endpoint, apiKey, model, imageField, inputs, mask, prompt, size, quality, maxAttempts, acquire)
		totalAttempts += attempts
		totalTiming.QueueWait += timing.QueueWait
		totalTiming.ProviderElapsed += timing.ProviderElapsed
		if timing.TimeoutStage != "" {
			totalTiming.TimeoutStage = timing.TimeoutStage
		}
		if requestID != "" {
			lastRequestID = requestID
		}
		if err != nil {
			return nil, lastRequestID, totalAttempts, aspectAttempt, imageOutputDimensions{}, false, totalTiming, err
		}
		dimensions, err := validateProviderImageOutput(image, size)
		if err == nil {
			return image, lastRequestID, totalAttempts, aspectAttempt, dimensions, false, totalTiming, nil
		}
		var aspectErr *providerImageAspectDeviationError
		if !errors.As(err, &aspectErr) {
			return nil, lastRequestID, totalAttempts, aspectAttempt, imageOutputDimensions{}, false, totalTiming, err
		}
		if aspectAttempt == maxProviderImageAspectRetries {
			fallbackDimensions, dimensionErr := decodeImageDimensions(image)
			if dimensionErr != nil {
				return nil, lastRequestID, totalAttempts, aspectAttempt, imageOutputDimensions{}, false, totalTiming, dimensionErr
			}
			return image, lastRequestID, totalAttempts, aspectAttempt, fallbackDimensions, true, totalTiming, nil
		}
	}
	panic("unreachable image aspect retry state")
}

func requestGPTImageEdit(ctx context.Context, client *http.Client, endpoint, apiKey, model, imageField string, inputs []string, mask, prompt, size, quality string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := []struct{ key, value string }{{"model", model}, {"prompt", prompt}, {"size", size}, {"output_format", "png"}}
	if quality != "" {
		fields = append(fields, struct{ key, value string }{"quality", quality})
	}
	for _, field := range fields {
		if err := writer.WriteField(field.key, field.value); err != nil {
			return nil, "", err
		}
	}
	for _, input := range inputs {
		if err := addImageEditFile(writer, imageField, input); err != nil {
			return nil, "", err
		}
	}
	if strings.TrimSpace(mask) != "" {
		if err := addImageEditFile(writer, "mask", mask); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", &imageEditTransportError{err: err}
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if readErr != nil {
		return nil, "", fmt.Errorf("read GPT Image response: %w", readErr)
	}
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, requestID, &imageEditHTTPError{
			StatusCode: resp.StatusCode,
			RequestID:  requestID,
			RetryAfter: resp.Header.Get("Retry-After"),
			Body:       truncateCLIError(payload, 1000),
		}
	}
	var response openAIImageEditResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, requestID, fmt.Errorf("decode GPT Image response: %w", err)
	}
	if len(response.Data) == 0 || strings.TrimSpace(response.Data[0].B64JSON) == "" {
		return nil, requestID, fmt.Errorf("GPT Image response did not include data[0].b64_json")
	}
	image, err := base64.StdEncoding.DecodeString(response.Data[0].B64JSON)
	if err != nil {
		return nil, requestID, fmt.Errorf("decode GPT Image output: %w", err)
	}
	return image, requestID, nil
}

func addImageEditFile(writer *multipart.Writer, field, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	contentType := imageEditFileContentType(path)
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": filepath.Base(path)})},
		"Content-Type":        {contentType},
	})
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("attach %s: %w", path, err)
	}
	return nil
}

func imageEditFileContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	}
	return "application/octet-stream"
}

func truncateCLIError(value []byte, limit int) string {
	text := strings.TrimSpace(string(value))
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func runCreativeMaterials(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()
	var response creativeMaterialsCLIResponse
	if err := client.GetJSON(ctx, "/api/issues/"+args[0]+"/creative-materials", &response); err != nil {
		return err
	}
	onlySelected, _ := cmd.Flags().GetBool("selected")
	if onlySelected {
		filtered := make([]creativeMaterialCandidateCLI, 0, len(response.Candidates))
		selectedIDs := map[string]struct{}{}
		for _, candidate := range response.Candidates {
			if candidate.Status == "selected" {
				filtered = append(filtered, candidate)
				selectedIDs[candidate.ID] = struct{}{}
			}
		}
		response.Candidates = filtered
		filteredItems := make([]creativeIssueItemCLI, 0, len(response.Items))
		for _, item := range response.Items {
			if _, selected := selectedIDs[item.CandidateID]; selected {
				filteredItems = append(filteredItems, item)
			}
		}
		response.Items = filteredItems
		filteredDeliveries := make([]creativeDeliveryCLI, 0, len(response.Deliveries))
		for _, delivery := range response.Deliveries {
			if _, selected := selectedIDs[delivery.CandidateID]; selected {
				filteredDeliveries = append(filteredDeliveries, delivery)
			}
		}
		response.Deliveries = filteredDeliveries
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, response)
	}
	rows := make([][]string, 0, len(response.Candidates))
	for _, candidate := range response.Candidates {
		rows = append(rows, []string{
			candidate.ID,
			candidate.Status,
			candidate.Competitor,
			candidate.Title,
			candidate.AssetType,
			firstCreativeCandidateSource(candidate),
		})
	}
	cli.PrintTable(os.Stdout, []string{"ID", "STATUS", "COMPETITOR", "TITLE", "TYPE", "SOURCE"}, rows)
	return nil
}

func firstCreativeCandidateSource(candidate creativeMaterialCandidateCLI) string {
	for _, value := range []string{candidate.ArchivedURL, candidate.PreviewURL, candidate.ResourceURL, candidate.PosterURL} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueCLIStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				out = append(out, value)
			}
		}
	}
	return out
}

func shortCLIIdentifier(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
