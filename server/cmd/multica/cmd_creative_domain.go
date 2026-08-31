package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var creativeLibraryCmd = &cobra.Command{
	Use:   "library",
	Short: "Work with workspace creative materials",
}

var creativeLibraryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspace creative materials",
	RunE:  runCreativeLibrary,
}

var creativeLibraryDownloadCmd = &cobra.Command{
	Use:   "download <candidate-id>",
	Short: "Download one archived workspace creative material",
	Args:  exactArgs(1),
	RunE:  runCreativeLibraryDownload,
}

var creativeSourceAnalysisCmd = &cobra.Command{
	Use:   "source-analysis",
	Short: "Read and write structured source analyses",
}

var creativeSourceAnalysisPutCmd = &cobra.Command{
	Use:   "put",
	Short: "Create or update one source analysis from a JSON envelope",
	RunE:  runCreativeSourceAnalysisPut,
}

var creativeSourceAnalysisListCmd = &cobra.Command{
	Use:   "list",
	Short: "List source analyses",
	RunE:  runCreativeSourceAnalysisList,
}

var creativeSourceAnalysisPreAdaptationContextCmd = &cobra.Command{
	Use:   "pre-adaptation-context <source-analysis-id>",
	Short: "Read frozen source-analysis, market-pack, and copy-library inputs",
	Args:  exactArgs(1),
	RunE:  runCreativeSourceAnalysisPreAdaptationContext,
}

var creativeSourceAnalysisPreAdaptationPutCmd = &cobra.Command{
	Use:   "pre-adaptation-put <source-analysis-id>",
	Short: "Save one market-specific pre-adaptation result",
	Args:  exactArgs(1),
	RunE:  runCreativeSourceAnalysisPreAdaptationPut,
}

var creativeMarketPackCmd = &cobra.Command{
	Use:   "market-pack",
	Short: "Work with creative market packs",
}

var creativeOrderCmd = &cobra.Command{
	Use:   "order",
	Short: "Work with creative production orders",
}

var creativeOrderCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a creative order from a JSON envelope",
	RunE:  runCreativeOrderCreate,
}

var creativeOrderListCmd = &cobra.Command{
	Use:   "list",
	Short: "List creative orders",
	RunE:  runCreativeOrderList,
}

var creativeOrderGetCmd = &cobra.Command{
	Use:   "get <order-id>",
	Short: "Get a creative order with items, variants, assets, and QC",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderGet,
}

var creativeOrderQCContextCmd = &cobra.Command{
	Use:   "qc-context <order-id>",
	Short: "Get the current task-bound visual QC target and Prime assets",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderQCContext,
}

var creativeOrderVariantPutCmd = &cobra.Command{
	Use:   "variant-put <order-id>",
	Short: "Create or update one order variant from JSON",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderVariantPut,
}

var creativeOrderCandidateSelectCmd = &cobra.Command{
	Use:   "candidate-select <order-id> <item-id>",
	Short: "Atomically rank three selected creative candidates and ordered reserves",
	Args:  exactArgs(2),
	RunE:  runCreativeOrderCandidateSelect,
}

var creativeOrderImageOperationPutCmd = &cobra.Command{
	Use:   "image-operation-put <order-id>",
	Short: "Create or update one durable size-level image operation attempt",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderImageOperationPut,
}

var creativeOrderAssetPutCmd = &cobra.Command{
	Use:   "asset-put <order-id>",
	Short: "Create or update one generated, Prime, or delivered asset from JSON",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderAssetPut,
}

var creativeOrderPrimeComposeCmd = &cobra.Command{
	Use:   "prime-compose <order-id>",
	Short: "Run the backend-owned deterministic Prime composition for one variant",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderPrimeCompose,
}

var creativeOrderDiagnosticAssetPutCmd = &cobra.Command{
	Use:   "diagnostic-asset-put <order-id>",
	Short: "Create or update one process image from an uploaded attachment",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderDiagnosticAssetPut,
}

var creativeOrderQCPutCmd = &cobra.Command{
	Use:   "qc-put <order-id>",
	Short: "Create or update one visual QC report from JSON",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderQCPut,
}

var creativeOrderQCFinalizeCmd = &cobra.Command{
	Use:   "qc-finalize <order-id>",
	Short: "Atomically finalize one Creative Order Variant after visual QC reports",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderQCFinalize,
}

var creativeOrderWorkflowRetryCmd = &cobra.Command{
	Use:   "workflow-retry <order-id> <task-id>",
	Short: "Retry one recoverable creative workflow task",
	Args:  exactArgs(2),
	RunE:  runCreativeOrderWorkflowRetry,
}

var creativeOrderQCRetryCmd = &cobra.Command{
	Use:   "qc-retry <order-id> <variant-id>",
	Short: "Queue another visual QC attempt for one completed Prime package",
	Args:  exactArgs(2),
	RunE:  runCreativeOrderQCRetry,
}

var creativeOrderAdoptCmd = &cobra.Command{
	Use:   "adopt <order-id> <item-id>",
	Short: "Adopt one finalized creative order variant for delivery",
	Args:  exactArgs(2),
	RunE:  runCreativeOrderAdopt,
}

func init() {
	creativeCmd.AddCommand(creativeLibraryCmd)
	creativeLibraryCmd.AddCommand(creativeLibraryListCmd, creativeLibraryDownloadCmd)
	creativeLibraryListCmd.Flags().String("run-id", "", "Only return candidates from this Crawl Run UUID")
	creativeLibraryListCmd.Flags().String("output", "json", "Output format: json")
	creativeLibraryDownloadCmd.Flags().String("output-file", "", "Local destination path (required)")
	creativeLibraryDownloadCmd.Flags().String("output", "json", "Output format: json")

	creativeCmd.AddCommand(creativeSourceAnalysisCmd)
	creativeSourceAnalysisCmd.AddCommand(creativeSourceAnalysisPutCmd, creativeSourceAnalysisListCmd, creativeSourceAnalysisPreAdaptationContextCmd, creativeSourceAnalysisPreAdaptationPutCmd)
	creativeSourceAnalysisPutCmd.Flags().String("input-file", "", "UTF-8 JSON analysis envelope (required)")
	creativeSourceAnalysisPutCmd.Flags().String("output", "json", "Output format: json")
	creativeSourceAnalysisListCmd.Flags().String("candidate-id", "", "Only return analyses for one candidate UUID")
	creativeSourceAnalysisListCmd.Flags().String("output", "json", "Output format: json")
	creativeSourceAnalysisPreAdaptationContextCmd.Flags().String("market-pack-id", "", "Frozen market pack UUID (required)")
	creativeSourceAnalysisPreAdaptationContextCmd.Flags().Int("market-pack-version", 0, "Frozen market pack version (required)")
	creativeSourceAnalysisPreAdaptationContextCmd.Flags().String("copy-library-id", "", "Frozen copy library UUID (required)")
	creativeSourceAnalysisPreAdaptationContextCmd.Flags().Int("copy-library-version", 0, "Frozen copy library version (required)")
	creativeSourceAnalysisPreAdaptationContextCmd.Flags().String("output", "json", "Output format: json")
	creativeSourceAnalysisPreAdaptationPutCmd.Flags().String("input-file", "", "UTF-8 JSON pre-adaptation envelope (required)")
	creativeSourceAnalysisPreAdaptationPutCmd.Flags().String("output", "json", "Output format: json")

	creativeCmd.AddCommand(creativeMarketPackCmd)

	creativeCmd.AddCommand(creativeOrderCmd)
	creativeOrderCmd.AddCommand(
		creativeOrderCreateCmd,
		creativeOrderListCmd,
		creativeOrderGetCmd,
		creativeOrderQCContextCmd,
		creativeOrderVariantPutCmd,
		creativeOrderCandidateSelectCmd,
		creativeOrderImageOperationPutCmd,
		creativeOrderAssetPutCmd,
		creativeOrderPrimeComposeCmd,
		creativeOrderDiagnosticAssetPutCmd,
		creativeOrderQCPutCmd,
		creativeOrderQCFinalizeCmd,
		creativeOrderWorkflowRetryCmd,
		creativeOrderQCRetryCmd,
		creativeOrderAdoptCmd,
	)
	for _, command := range []*cobra.Command{
		creativeOrderCreateCmd,
		creativeOrderVariantPutCmd,
		creativeOrderCandidateSelectCmd,
		creativeOrderImageOperationPutCmd,
		creativeOrderAssetPutCmd,
		creativeOrderDiagnosticAssetPutCmd,
		creativeOrderQCPutCmd,
	} {
		command.Flags().String("input-file", "", "UTF-8 JSON envelope (required)")
		command.Flags().String("output", "json", "Output format: json")
	}
	creativeOrderAssetPutCmd.Flags().String("model-result-file", "", "UTF-8 JSON result from multica image edit or image edit-batch")
	creativeOrderAssetPutCmd.Flags().String("model-result-id", "", "Result ID when --model-result-file is an image edit-batch response")
	creativeOrderAssetPutCmd.Flags().String("prompt-contract-file", "", "UTF-8 JSON prompt compiler evidence")
	creativeOrderAssetPutCmd.Flags().String("copy-validation-file", "", "UTF-8 JSON copy validation evidence (legacy optional)")
	creativeOrderAssetPutCmd.Flags().String("normalization-evidence-file", "", "UTF-8 JSON normalized delivery evidence")
	creativeOrderListCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderGetCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderQCContextCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderPrimeComposeCmd.Flags().String("variant", "", "Creative Order Variant UUID (required)")
	creativeOrderPrimeComposeCmd.Flags().Bool("force", false, "Explicitly retry deterministic Prime composition for the current staging revision")
	creativeOrderPrimeComposeCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderQCFinalizeCmd.Flags().String("variant", "", "Creative Order Variant UUID (must match the task-bound target when provided)")
	creativeOrderQCFinalizeCmd.Flags().Int("revision", 0, "Variant revision (must match the task-bound target when provided)")
	creativeOrderQCFinalizeCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderWorkflowRetryCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderQCRetryCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderAdoptCmd.Flags().String("variant", "", "Creative Order Variant UUID (required)")
	creativeOrderAdoptCmd.Flags().Bool("qc-risk-acknowledged", false, "Explicitly accept a failed QC result for adoption")
	creativeOrderAdoptCmd.Flags().String("qc-risk-reason", "", "Reason for accepting a failed QC result")
	creativeOrderAdoptCmd.Flags().String("output", "json", "Output format: json")
}

func runCreativeLibrary(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	runID, _ := cmd.Flags().GetString("run-id")
	path := "/api/creative/materials"
	if strings.TrimSpace(runID) != "" {
		path += "?run_id=" + url.QueryEscape(strings.TrimSpace(runID))
	}
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return err
	}
	if strings.TrimSpace(runID) != "" {
		result = filterCreativeLibraryByRun(result, strings.TrimSpace(runID))
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runCreativeLibraryDownload(cmd *cobra.Command, args []string) error {
	outputFile, _ := cmd.Flags().GetString("output-file")
	if strings.TrimSpace(outputFile) == "" {
		return fmt.Errorf("--output-file is required")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var library struct {
		Candidates []creativeMaterialCandidateCLI `json:"candidates"`
	}
	if err := client.GetJSON(ctx, "/api/creative/materials", &library); err != nil {
		return err
	}
	var candidate *creativeMaterialCandidateCLI
	for index := range library.Candidates {
		if library.Candidates[index].ID == args[0] {
			candidate = &library.Candidates[index]
			break
		}
	}
	if candidate == nil {
		return fmt.Errorf("creative material candidate %s was not found", args[0])
	}
	data, source, err := downloadCreativeMaterialCandidate(ctx, client, *candidate)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("creative material candidate %s has no readable asset URL", args[0])
	}
	if directory := filepath.Dir(outputFile); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(outputFile, data, 0o644); err != nil {
		return fmt.Errorf("write creative material: %w", err)
	}
	absolute, err := filepath.Abs(outputFile)
	if err != nil {
		absolute = outputFile
	}
	return cli.PrintJSON(os.Stdout, map[string]any{
		"candidate_id": candidate.ID,
		"path":         absolute,
		"bytes":        len(data),
		"source":       source,
	})
}

func creativeLibraryDownloadSource(candidate creativeMaterialCandidateCLI) (string, string) {
	if attachmentID := creativeCandidateAttachmentID(candidate); attachmentID != "" {
		return "/api/attachments/" + url.PathEscape(attachmentID) + "/download", "attachment"
	}
	if candidate.ArchiveStatus == "completed" && strings.TrimSpace(candidate.ArchivedURL) != "" {
		return strings.TrimSpace(candidate.ArchivedURL), "archive"
	}
	return firstNonEmpty(candidate.OriginalURL, candidate.PreviewURL, candidate.ResourceURL, candidate.PosterURL), "origin"
}

// creativeCandidateAttachmentID finds the candidate's own platform attachment
// even when older material responses omitted source_attachment_id but retained
// an attachment download URL. Ambiguous URLs deliberately do not become a
// fallback: a candidate must resolve to exactly one attachment.
func creativeCandidateAttachmentID(candidate creativeMaterialCandidateCLI) string {
	if attachmentID := strings.TrimSpace(candidate.SourceAttachmentID); attachmentID != "" {
		return attachmentID
	}
	attachmentIDs := map[string]struct{}{}
	for _, rawURL := range []string{
		candidate.ArchivedURL,
		candidate.OriginalURL,
		candidate.PreviewURL,
		candidate.ResourceURL,
		candidate.PosterURL,
	} {
		if attachmentID := attachmentIDFromDownloadURL(rawURL); attachmentID != "" {
			attachmentIDs[attachmentID] = struct{}{}
		}
	}
	if len(attachmentIDs) != 1 {
		return ""
	}
	for attachmentID := range attachmentIDs {
		return attachmentID
	}
	return ""
}

func attachmentIDFromDownloadURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 4 || !strings.EqualFold(segments[0], "api") || !strings.EqualFold(segments[1], "attachments") || !strings.EqualFold(segments[3], "download") {
		return ""
	}
	attachmentID, err := url.PathUnescape(segments[2])
	if err != nil {
		return ""
	}
	return strings.TrimSpace(attachmentID)
}

func downloadCreativeMaterialCandidate(ctx context.Context, client *cli.APIClient, candidate creativeMaterialCandidateCLI) ([]byte, string, error) {
	downloadURL, source := creativeLibraryDownloadSource(candidate)
	if downloadURL == "" {
		return nil, "", fmt.Errorf("creative material candidate %s has no readable asset URL", candidate.ID)
	}
	data, err := client.DownloadFile(ctx, downloadURL)
	if err == nil {
		return data, source, nil
	}

	// The current source might be a stale archive URL. If this candidate also
	// exposes its own attachment identity, retry through the authenticated
	// platform attachment endpoint before reporting a source failure.
	attachmentID := creativeCandidateAttachmentID(candidate)
	attachmentURL := ""
	if attachmentID != "" {
		attachmentURL = "/api/attachments/" + url.PathEscape(attachmentID) + "/download"
	}
	if source != "attachment" && attachmentURL != "" && attachmentURL != downloadURL {
		fallbackData, fallbackErr := client.DownloadFile(ctx, attachmentURL)
		if fallbackErr == nil {
			return fallbackData, "attachment_fallback", nil
		}
		return nil, "", fmt.Errorf("download creative material from %s: %w; attachment fallback failed: %v", source, err, fallbackErr)
	}
	return nil, "", fmt.Errorf("download creative material from %s: %w", source, err)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func runCreativeSourceAnalysisPut(cmd *cobra.Command, _ []string) error {
	return postCreativeDomainJSON(cmd, "/api/creative/source-analyses")
}

func runCreativeSourceAnalysisList(cmd *cobra.Command, _ []string) error {
	path := "/api/creative/source-analyses"
	candidateID, _ := cmd.Flags().GetString("candidate-id")
	if strings.TrimSpace(candidateID) != "" {
		path += "?candidate_id=" + url.QueryEscape(strings.TrimSpace(candidateID))
	}
	return getCreativeDomainJSON(cmd, path)
}

func runCreativeSourceAnalysisPreAdaptationContext(cmd *cobra.Command, args []string) error {
	marketPackID, _ := cmd.Flags().GetString("market-pack-id")
	marketPackVersion, _ := cmd.Flags().GetInt("market-pack-version")
	copyLibraryID, _ := cmd.Flags().GetString("copy-library-id")
	copyLibraryVersion, _ := cmd.Flags().GetInt("copy-library-version")
	if strings.TrimSpace(marketPackID) == "" || marketPackVersion < 1 || strings.TrimSpace(copyLibraryID) == "" || copyLibraryVersion < 1 {
		return fmt.Errorf("--market-pack-id, --market-pack-version, --copy-library-id, and --copy-library-version are required")
	}
	query := url.Values{}
	query.Set("market_pack_id", strings.TrimSpace(marketPackID))
	query.Set("market_pack_version", strconv.Itoa(marketPackVersion))
	query.Set("copy_library_id", strings.TrimSpace(copyLibraryID))
	query.Set("copy_library_version", strconv.Itoa(copyLibraryVersion))
	return getCreativeDomainJSON(cmd, "/api/creative/source-analyses/"+url.PathEscape(args[0])+"/pre-adaptation-context?"+query.Encode())
}

func runCreativeSourceAnalysisPreAdaptationPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/source-analyses/"+url.PathEscape(args[0])+"/pre-adaptation")
}

func runCreativeOrderCreate(cmd *cobra.Command, _ []string) error {
	return postCreativeDomainJSON(cmd, "/api/creative/orders")
}

func runCreativeOrderList(cmd *cobra.Command, _ []string) error {
	return getCreativeDomainJSON(cmd, "/api/creative/orders")
}

func runCreativeOrderGet(cmd *cobra.Command, args []string) error {
	return getCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0]))
}

func runCreativeOrderQCContext(cmd *cobra.Command, args []string) error {
	return getCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/qc-context")
}

func runCreativeOrderVariantPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/variants")
}

func runCreativeOrderCandidateSelect(cmd *cobra.Command, args []string) error {
	return postCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/items/"+url.PathEscape(args[1])+"/candidate-selection")
}

func runCreativeOrderImageOperationPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/image-operations")
}

func runCreativeOrderAssetPut(cmd *cobra.Command, args []string) error {
	payload, err := creativeOrderAssetPayload(cmd)
	if err != nil {
		return err
	}
	return writeCreativeDomainPayload(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/assets", "PUT", payload)
}

func runCreativeOrderPrimeCompose(cmd *cobra.Command, args []string) error {
	variantID, _ := cmd.Flags().GetString("variant")
	if strings.TrimSpace(variantID) == "" {
		return fmt.Errorf("--variant is required")
	}
	force, _ := cmd.Flags().GetBool("force")
	payload, err := json.Marshal(map[string]any{"variant_id": strings.TrimSpace(variantID), "force": force})
	if err != nil {
		return fmt.Errorf("encode prime composition request: %w", err)
	}
	return writeCreativeDomainPayloadWithTimeout(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/prime-compose", "POST", payload, 5*time.Minute)
}

func runCreativeOrderDiagnosticAssetPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/diagnostic-assets")
}

func runCreativeOrderQCPut(cmd *cobra.Command, args []string) error {
	payload, err := creativeDomainPayload(cmd)
	if err != nil {
		return err
	}
	target, err := creativeOrderQCTarget(cmd, args[0])
	if err != nil {
		return err
	}
	payload, err = bindCreativeOrderQCReportPayload(payload, target)
	if err != nil {
		return err
	}
	return writeCreativeDomainPayload(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/qc-reports", "PUT", payload)
}

func runCreativeOrderQCFinalize(cmd *cobra.Command, args []string) error {
	target, err := creativeOrderQCTarget(cmd, args[0])
	if err != nil {
		return err
	}
	variantID, _ := cmd.Flags().GetString("variant")
	if cmd.Flags().Changed("variant") && strings.TrimSpace(variantID) != target.VariantID {
		return fmt.Errorf("creative QC target mismatch: --variant %q does not match task variant_id %q", strings.TrimSpace(variantID), target.VariantID)
	}
	revision, _ := cmd.Flags().GetInt("revision")
	if cmd.Flags().Changed("revision") && revision != target.Revision {
		return fmt.Errorf("creative QC target mismatch: --revision %d does not match task revision %d", revision, target.Revision)
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result any
	if err := client.PostJSON(ctx, "/api/creative/orders/"+url.PathEscape(args[0])+"/qc-finalize", map[string]any{
		"variant_id": target.VariantID,
		"revision":   target.Revision,
	}, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runCreativeOrderWorkflowRetry(cmd *cobra.Command, args []string) error {
	return postCreativeDomainEmpty(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/workflow-failures/"+url.PathEscape(args[1])+"/retry")
}

func runCreativeOrderQCRetry(cmd *cobra.Command, args []string) error {
	return postCreativeDomainEmpty(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/variants/"+url.PathEscape(args[1])+"/qc/retry")
}

type creativeOrderQCTargetResponse struct {
	VariantID string `json:"variant_id"`
	Workflow  string `json:"workflow"`
	Revision  int    `json:"revision"`
	Attempt   int    `json:"attempt"`
}

func creativeOrderQCTarget(cmd *cobra.Command, orderID string) (creativeOrderQCTargetResponse, error) {
	client, err := newAPIClient(cmd)
	if err != nil {
		return creativeOrderQCTargetResponse{}, err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var target creativeOrderQCTargetResponse
	if err := client.GetJSON(ctx, "/api/creative/orders/"+url.PathEscape(orderID)+"/qc-context", &target); err != nil {
		return creativeOrderQCTargetResponse{}, err
	}
	if target.Workflow != "creative_qc_visual" || strings.TrimSpace(target.VariantID) == "" || target.Revision < 1 || target.Attempt < 1 {
		return creativeOrderQCTargetResponse{}, fmt.Errorf("invalid task-bound creative QC target")
	}
	return target, nil
}

func bindCreativeOrderQCReportPayload(payload json.RawMessage, target creativeOrderQCTargetResponse) (json.RawMessage, error) {
	var report map[string]json.RawMessage
	if json.Unmarshal(payload, &report) != nil || report == nil {
		return nil, fmt.Errorf("creative QC report must be a JSON object")
	}
	var variantID, lane string
	var revision, attempt int
	if raw := report["variant_id"]; raw != nil && json.Unmarshal(raw, &variantID) != nil {
		return nil, fmt.Errorf("creative QC report variant_id must be a string")
	}
	if raw := report["lane"]; raw != nil && json.Unmarshal(raw, &lane) != nil {
		return nil, fmt.Errorf("creative QC report lane must be a string")
	}
	if raw := report["revision"]; raw != nil && json.Unmarshal(raw, &revision) != nil {
		return nil, fmt.Errorf("creative QC report revision must be an integer")
	}
	if raw := report["attempt"]; raw != nil && json.Unmarshal(raw, &attempt) != nil {
		return nil, fmt.Errorf("creative QC report attempt must be an integer")
	}
	if strings.TrimSpace(variantID) != "" && strings.TrimSpace(variantID) != target.VariantID {
		return nil, fmt.Errorf("creative QC target mismatch: report variant_id %q does not match task variant_id %q", strings.TrimSpace(variantID), target.VariantID)
	}
	if strings.TrimSpace(lane) != "" && strings.TrimSpace(lane) != "visual" {
		return nil, fmt.Errorf("creative QC target mismatch: report lane %q does not match task lane %q", strings.TrimSpace(lane), "visual")
	}
	if revision != 0 && revision != target.Revision {
		return nil, fmt.Errorf("creative QC target mismatch: report revision %d does not match task revision %d", revision, target.Revision)
	}
	if attempt != 0 && attempt != target.Attempt {
		return nil, fmt.Errorf("creative QC target mismatch: report attempt %d does not match task attempt %d", attempt, target.Attempt)
	}
	report["variant_id"], _ = json.Marshal(target.VariantID)
	report["lane"], _ = json.Marshal("visual")
	report["revision"], _ = json.Marshal(target.Revision)
	report["attempt"], _ = json.Marshal(target.Attempt)
	return json.Marshal(report)
}

func runCreativeOrderAdopt(cmd *cobra.Command, args []string) error {
	variantID, _ := cmd.Flags().GetString("variant")
	if strings.TrimSpace(variantID) == "" {
		return fmt.Errorf("--variant is required")
	}
	qcRiskAcknowledged, _ := cmd.Flags().GetBool("qc-risk-acknowledged")
	qcRiskReason, _ := cmd.Flags().GetString("qc-risk-reason")
	qcRiskReason = strings.TrimSpace(qcRiskReason)
	if qcRiskAcknowledged != (qcRiskReason != "") {
		return fmt.Errorf("--qc-risk-acknowledged and --qc-risk-reason must be provided together")
	}
	payload := map[string]any{"variant_id": strings.TrimSpace(variantID)}
	if qcRiskAcknowledged {
		payload["qc_risk_acknowledged"] = true
		payload["qc_risk_reason"] = qcRiskReason
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result any
	if err := client.PostJSON(ctx, "/api/creative/orders/"+url.PathEscape(args[0])+"/items/"+url.PathEscape(args[1])+"/adoption", payload, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func getCreativeDomainJSON(cmd *cobra.Command, path string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result any
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func postCreativeDomainJSON(cmd *cobra.Command, path string) error {
	return writeCreativeDomainJSON(cmd, path, "POST")
}

func postCreativeDomainEmpty(cmd *cobra.Command, path string) error {
	return writeCreativeDomainPayload(cmd, path, "POST", nil)
}

func putCreativeDomainJSON(cmd *cobra.Command, path string) error {
	return writeCreativeDomainJSON(cmd, path, "PUT")
}

func writeCreativeDomainJSON(cmd *cobra.Command, path, method string) error {
	payload, err := creativeDomainPayload(cmd)
	if err != nil {
		return err
	}
	return writeCreativeDomainPayload(cmd, path, method, payload)
}

func writeCreativeDomainPayload(cmd *cobra.Command, path, method string, payload json.RawMessage) error {
	return writeCreativeDomainPayloadWithTimeout(cmd, path, method, payload, 0)
}

func writeCreativeDomainPayloadWithTimeout(cmd *cobra.Command, path, method string, payload json.RawMessage, minTimeout time.Duration) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	timeout := cli.APITimeout()
	if minTimeout > 0 {
		if client.HTTPClient != nil && client.HTTPClient.Timeout < minTimeout {
			client.HTTPClient.Timeout = minTimeout
		}
		if client.HTTPClient != nil && client.HTTPClient.Timeout+5*time.Second > timeout {
			timeout = client.HTTPClient.Timeout + 5*time.Second
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var result any
	switch method {
	case "POST":
		err = client.PostJSON(ctx, path, payload, &result)
	case "PUT":
		err = client.PutJSON(ctx, path, payload, &result)
	default:
		return fmt.Errorf("unsupported creative domain method %s", method)
	}
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func creativeDomainPayload(cmd *cobra.Command) (json.RawMessage, error) {
	inputFile, _ := cmd.Flags().GetString("input-file")
	if strings.TrimSpace(inputFile) == "" {
		return nil, fmt.Errorf("--input-file is required")
	}
	raw, err := os.ReadFile(inputFile)
	if err != nil {
		return nil, fmt.Errorf("read creative domain input: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("creative domain input must be a JSON object")
	}
	return json.RawMessage(raw), nil
}

func creativeOrderAssetPayload(cmd *cobra.Command) (json.RawMessage, error) {
	payload, err := creativeDomainPayload(cmd)
	if err != nil {
		return nil, err
	}
	modelResultFile, _ := cmd.Flags().GetString("model-result-file")
	promptContractFile, _ := cmd.Flags().GetString("prompt-contract-file")
	copyValidationFile, _ := cmd.Flags().GetString("copy-validation-file")
	normalizationFile, _ := cmd.Flags().GetString("normalization-evidence-file")
	resultID, _ := cmd.Flags().GetString("model-result-id")
	requiredEvidenceFiles := []string{modelResultFile, promptContractFile, normalizationFile}
	provided := 0
	for _, file := range requiredEvidenceFiles {
		if strings.TrimSpace(file) != "" {
			provided++
		}
	}
	if provided == 0 {
		if strings.TrimSpace(resultID) != "" {
			return nil, fmt.Errorf("--model-result-id requires --model-result-file")
		}
		if strings.TrimSpace(copyValidationFile) != "" {
			return nil, fmt.Errorf("--copy-validation-file requires --model-result-file, --prompt-contract-file, and --normalization-evidence-file")
		}
		return payload, nil
	}
	if provided != len(requiredEvidenceFiles) {
		return nil, fmt.Errorf("--model-result-file, --prompt-contract-file, and --normalization-evidence-file must be provided together")
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope == nil {
		return nil, fmt.Errorf("creative order asset input must be a JSON object")
	}
	asset := envelope
	if nested, ok := envelope["asset"]; ok {
		if err := json.Unmarshal(nested, &asset); err != nil || asset == nil {
			return nil, fmt.Errorf("creative order asset must be a JSON object")
		}
	}
	if _, exists := asset["metadata"]; exists {
		return nil, fmt.Errorf("asset metadata is generated from the evidence files")
	}
	if _, exists := asset["evidence"]; exists {
		return nil, fmt.Errorf("asset evidence is generated from the evidence files")
	}

	modelResult, err := creativeAssetModelResult(modelResultFile, resultID)
	if err != nil {
		return nil, err
	}
	promptContract, err := creativeAssetEvidenceFile(promptContractFile, "prompt contract")
	if err != nil {
		return nil, err
	}
	normalization, err := creativeAssetEvidenceFile(normalizationFile, "normalization evidence")
	if err != nil {
		return nil, err
	}
	var copyValidation []byte
	if strings.TrimSpace(copyValidationFile) != "" {
		copyValidation, err = creativeAssetEvidenceFile(copyValidationFile, "copy validation")
		if err != nil {
			return nil, err
		}
	}

	var trace struct {
		Model             string  `json:"model"`
		Prompt            string  `json:"prompt"`
		PromptSHA256      string  `json:"prompt_sha256"`
		RequestID         string  `json:"request_id"`
		Attempts          int     `json:"attempts"`
		ActualWidth       int     `json:"actual_width"`
		ActualHeight      int     `json:"actual_height"`
		ActualAspectRatio float64 `json:"actual_aspect_ratio"`
		ProviderSlotLimit int     `json:"provider_slot_limit"`
	}
	if err := json.Unmarshal(modelResult, &trace); err != nil {
		return nil, fmt.Errorf("decode model result: %w", err)
	}
	if strings.TrimSpace(trace.Model) != "gpt-image-2" || strings.TrimSpace(trace.Prompt) == "" || strings.TrimSpace(trace.RequestID) == "" || trace.Attempts < 1 || trace.ActualWidth < 1 || trace.ActualHeight < 1 || trace.ActualAspectRatio <= 0 {
		return nil, fmt.Errorf("model result is missing the completed generated asset trace")
	}
	if trace.PromptSHA256 != imagePromptSHA256(trace.Prompt) {
		return nil, fmt.Errorf("model result prompt_sha256 does not match prompt")
	}
	var contract struct {
		PromptSHA256 string `json:"prompt_sha256"`
	}
	if err := json.Unmarshal(promptContract, &contract); err != nil || contract.PromptSHA256 != trace.PromptSHA256 {
		return nil, fmt.Errorf("prompt contract prompt_sha256 does not match model result")
	}
	var normalized struct {
		TargetSize struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"target_size"`
	}
	if err := json.Unmarshal(normalization, &normalized); err != nil || normalized.TargetSize.Width < 1 || normalized.TargetSize.Height < 1 {
		return nil, fmt.Errorf("normalization evidence must contain target_size.width and target_size.height")
	}
	var sizeKey string
	if rawSizeKey, ok := asset["size_key"]; !ok || json.Unmarshal(rawSizeKey, &sizeKey) != nil {
		return nil, fmt.Errorf("asset size_key is required when normalization evidence is provided")
	}
	wantWidth, wantHeight, ok := creativeOrderAssetDimensions(sizeKey)
	if !ok || normalized.TargetSize.Width != wantWidth || normalized.TargetSize.Height != wantHeight {
		return nil, fmt.Errorf("normalization target_size %dx%d does not match asset size_key %s", normalized.TargetSize.Width, normalized.TargetSize.Height, sizeKey)
	}

	metadata, err := json.Marshal(map[string]any{
		"model": trace.Model, "prompt": trace.Prompt,
		"actual_width": trace.ActualWidth, "actual_height": trace.ActualHeight,
		"actual_aspect_ratio": trace.ActualAspectRatio,
	})
	if err != nil {
		return nil, err
	}
	evidence := map[string]any{
		"request_id": trace.RequestID, "attempts": trace.Attempts, "prompt_sha256": trace.PromptSHA256,
		"model_result": json.RawMessage(modelResult), "prompt_contract": json.RawMessage(promptContract),
		"normalization": json.RawMessage(normalization),
		"prime_status":  "pending",
	}
	if len(copyValidation) > 0 {
		evidence["copy_validation"] = json.RawMessage(copyValidation)
	}
	if trace.ProviderSlotLimit > 0 {
		evidence["provider_slot_limit"] = trace.ProviderSlotLimit
	}
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	asset["metadata"] = metadata
	asset["evidence"] = encodedEvidence
	return marshalCreativeAssetJSONObject(asset)
}

func creativeOrderAssetDimensions(sizeKey string) (int, int, bool) {
	switch strings.TrimSpace(sizeKey) {
	case "1080x1080":
		return 1080, 1080, true
	case "1200x628":
		return 1200, 628, true
	case "800x1000":
		return 800, 1000, true
	default:
		return 0, 0, false
	}
}

func creativeAssetModelResult(path, resultID string) (json.RawMessage, error) {
	modelResult, err := creativeAssetEvidenceFile(path, "model result")
	if err != nil {
		return nil, err
	}
	var batch struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(modelResult, &batch); err != nil || batch.Results == nil {
		if strings.TrimSpace(resultID) != "" {
			return nil, fmt.Errorf("--model-result-id is only valid for an image edit-batch result")
		}
		return stripCreativeAssetLocalPath(modelResult)
	}
	if strings.TrimSpace(resultID) == "" {
		return nil, fmt.Errorf("--model-result-id is required for an image edit-batch result")
	}
	for _, result := range batch.Results {
		var item struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(result, &item); err == nil && item.ID == resultID {
			return stripCreativeAssetLocalPath(result)
		}
	}
	return nil, fmt.Errorf("model result %q was not found", resultID)
}

func creativeAssetEvidenceFile(path, label string) (json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return json.RawMessage(raw), nil
}

func stripCreativeAssetLocalPath(raw json.RawMessage) (json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("model result must be a JSON object")
	}
	delete(object, "path")
	for _, key := range []string{"generated_asset", "completed_generated_asset"} {
		nested, ok := object[key]
		if !ok {
			continue
		}
		var nestedObject map[string]json.RawMessage
		if err := json.Unmarshal(nested, &nestedObject); err != nil || nestedObject == nil {
			continue
		}
		delete(nestedObject, "path")
		encoded, err := json.Marshal(nestedObject)
		if err != nil {
			return nil, err
		}
		object[key] = encoded
	}
	return marshalCreativeAssetJSONObject(object)
}

func marshalCreativeAssetJSONObject(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func filterCreativeLibraryByRun(result map[string]any, runID string) map[string]any {
	filtered := make([]any, 0)
	for _, candidate := range anySlice(result["candidates"]) {
		record, ok := candidate.(map[string]any)
		if ok && fmt.Sprint(record["source_run_id"]) == runID {
			filtered = append(filtered, record)
		}
	}
	copy := make(map[string]any, len(result)+1)
	for key, value := range result {
		copy[key] = value
	}
	copy["candidates"] = filtered
	copy["crawl_run_id"] = runID
	return copy
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}
