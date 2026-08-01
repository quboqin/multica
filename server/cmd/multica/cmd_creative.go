package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

type creativeMaterialsCLIResponse struct {
	Candidates []creativeMaterialCandidateCLI `json:"candidates"`
	Items      []creativeIssueItemCLI         `json:"items"`
	Context    json.RawMessage                `json:"context"`
}

type creativeIssueItemCLI struct {
	CandidateID   string          `json:"candidate_id"`
	CopyEntryID   string          `json:"copy_entry_id"`
	CopySnapshot  json.RawMessage `json:"copy_snapshot"`
	CreativeBrief json.RawMessage `json:"creative_brief"`
	WorkIssueID   string          `json:"work_issue_id"`
	Revision      int             `json:"revision"`
	Status        string          `json:"status"`
}

type creativeMaterialCandidateCLI struct {
	ID            string `json:"id"`
	Competitor    string `json:"competitor"`
	Title         string `json:"title"`
	AssetType     string `json:"asset_type"`
	PreviewURL    string `json:"preview_url"`
	ResourceURL   string `json:"resource_url"`
	PosterURL     string `json:"poster_url"`
	ArchivedURL   string `json:"archived_url"`
	ArchiveStatus string `json:"archive_status"`
	Status        string `json:"status"`
}

func init() {
	creativeCmd.AddCommand(creativeMaterialsCmd)
	creativeCmd.AddCommand(creativeMaterialCmd)
	creativeMaterialCmd.AddCommand(creativeMaterialDownloadCmd)
	creativeMaterialCmd.AddCommand(creativeMaterialBriefCmd)
	imageCmd.AddCommand(imageEditCmd)

	creativeMaterialsCmd.Flags().Bool("selected", false, "Only return materials selected by a person")
	creativeMaterialsCmd.Flags().String("output", "json", "Output format: json or table")
	creativeMaterialDownloadCmd.Flags().String("output-file", "", "Local file path for the archived candidate")
	creativeMaterialDownloadCmd.Flags().String("output", "json", "Output format: json or table")
	creativeMaterialBriefCmd.Flags().String("input-file", "", "UTF-8 JSON file containing the creative brief")
	creativeMaterialBriefCmd.Flags().String("output", "json", "Output format: json or table")

	imageEditCmd.Flags().StringSlice("input", nil, "Reference image files (1-16 files)")
	imageEditCmd.Flags().String("mask", "", "Optional PNG mask with alpha channel")
	imageEditCmd.Flags().String("prompt", "", "Edit prompt")
	imageEditCmd.Flags().Bool("prompt-stdin", false, "Read the prompt from stdin")
	imageEditCmd.Flags().String("prompt-file", "", "Read the prompt from a UTF-8 file")
	imageEditCmd.Flags().String("model", "gpt-image-2", "GPT Image model")
	imageEditCmd.Flags().String("size", "auto", "Canvas size, e.g. 1088x1360 or auto")
	imageEditCmd.Flags().String("quality", "", "Optional image quality: low, medium, high, or auto")
	imageEditCmd.Flags().String("output-file", "", "Output PNG file")
	imageEditCmd.Flags().String("output", "json", "Output format: json or table")
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
	if selected.Status != "selected" {
		return fmt.Errorf("creative material candidate %s is not selected", args[1])
	}
	if selected.ArchiveStatus != "completed" || strings.TrimSpace(selected.ArchivedURL) == "" {
		return fmt.Errorf("creative material candidate %s is not available in platform archive storage", args[1])
	}
	data, err := client.DownloadFile(ctx, selected.ArchivedURL)
	if err != nil {
		return fmt.Errorf("download archived creative material: %w", err)
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
		"path": abs, "bytes": len(data),
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
	if err := validateGPTImageSize(size); err != nil {
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
	mask, _ := cmd.Flags().GetString("mask")
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(10*time.Minute))
	defer cancel()
	image, requestID, err := requestGPTImageEdit(ctx, http.DefaultClient, endpoint, apiKey, model, imageField, inputs, mask, prompt, size, quality)
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
	result := map[string]any{"model": model, "input_count": len(inputs), "size": size, "quality": quality, "path": abs, "bytes": len(image), "request_id": requestID}
	if output == "table" {
		cli.PrintTable(os.Stdout, []string{"MODEL", "INPUTS", "SIZE", "QUALITY", "BYTES", "REQUEST ID", "PATH"}, [][]string{{model, strconv.Itoa(len(inputs)), size, quality, strconv.Itoa(len(image)), requestID, abs}})
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
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
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "auto" {
		return nil
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return fmt.Errorf("--size must be auto or WIDTHxHEIGHT")
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 || width > 3840 || height > 3840 || width%16 != 0 || height%16 != 0 || width*height < 655360 || width*height > 8294400 || maxInt(width, height) > 3*minInt(width, height) {
		return fmt.Errorf("--size must use 16px multiples, 655360-8294400 pixels, maximum edge 3840px, and at most 3:1 ratio")
	}
	return nil
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
		return nil, "", fmt.Errorf("request GPT Image edit: %w", err)
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if readErr != nil {
		return nil, "", fmt.Errorf("read GPT Image response: %w", readErr)
	}
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, requestID, fmt.Errorf("GPT Image edit failed with status %d (request_id=%s): %s", resp.StatusCode, requestID, truncateCLIError(payload, 1000))
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
