package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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

var creativeOrderVariantPutCmd = &cobra.Command{
	Use:   "variant-put <order-id>",
	Short: "Create or update one order variant from JSON",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderVariantPut,
}

var creativeOrderAssetPutCmd = &cobra.Command{
	Use:   "asset-put <order-id>",
	Short: "Create or update one generated, Prime, or delivered asset from JSON",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderAssetPut,
}

var creativeOrderQCPutCmd = &cobra.Command{
	Use:   "qc-put <order-id>",
	Short: "Create or update one technical or visual QC report from JSON",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderQCPut,
}

var creativeOrderQCFinalizeCmd = &cobra.Command{
	Use:   "qc-finalize <order-id>",
	Short: "Atomically finalize one Creative Order Variant after both QC lanes report",
	Args:  exactArgs(1),
	RunE:  runCreativeOrderQCFinalize,
}

func init() {
	creativeCmd.AddCommand(creativeLibraryCmd)
	creativeLibraryCmd.AddCommand(creativeLibraryListCmd, creativeLibraryDownloadCmd)
	creativeLibraryListCmd.Flags().String("run-id", "", "Only return candidates from this Crawl Run UUID")
	creativeLibraryListCmd.Flags().String("output", "json", "Output format: json")
	creativeLibraryDownloadCmd.Flags().String("output-file", "", "Local destination path (required)")
	creativeLibraryDownloadCmd.Flags().String("output", "json", "Output format: json")

	creativeCmd.AddCommand(creativeSourceAnalysisCmd)
	creativeSourceAnalysisCmd.AddCommand(creativeSourceAnalysisPutCmd, creativeSourceAnalysisListCmd)
	creativeSourceAnalysisPutCmd.Flags().String("input-file", "", "UTF-8 JSON analysis envelope (required)")
	creativeSourceAnalysisPutCmd.Flags().String("output", "json", "Output format: json")
	creativeSourceAnalysisListCmd.Flags().String("candidate-id", "", "Only return analyses for one candidate UUID")
	creativeSourceAnalysisListCmd.Flags().String("output", "json", "Output format: json")

	creativeCmd.AddCommand(creativeOrderCmd)
	creativeOrderCmd.AddCommand(
		creativeOrderCreateCmd,
		creativeOrderListCmd,
		creativeOrderGetCmd,
		creativeOrderVariantPutCmd,
		creativeOrderAssetPutCmd,
		creativeOrderQCPutCmd,
		creativeOrderQCFinalizeCmd,
	)
	for _, command := range []*cobra.Command{
		creativeOrderCreateCmd,
		creativeOrderVariantPutCmd,
		creativeOrderAssetPutCmd,
		creativeOrderQCPutCmd,
	} {
		command.Flags().String("input-file", "", "UTF-8 JSON envelope (required)")
		command.Flags().String("output", "json", "Output format: json")
	}
	creativeOrderListCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderGetCmd.Flags().String("output", "json", "Output format: json")
	creativeOrderQCFinalizeCmd.Flags().String("variant", "", "Creative Order Variant UUID (required)")
	creativeOrderQCFinalizeCmd.Flags().Int("revision", 1, "Variant revision to finalize")
	creativeOrderQCFinalizeCmd.Flags().String("output", "json", "Output format: json")
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
	downloadURL, source := creativeLibraryDownloadSource(*candidate)
	if downloadURL == "" {
		return fmt.Errorf("creative material candidate %s has no readable asset URL", args[0])
	}
	data, err := client.DownloadFile(ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("download creative material from %s: %w", source, err)
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
	if candidate.ArchiveStatus == "completed" && strings.TrimSpace(candidate.ArchivedURL) != "" {
		return strings.TrimSpace(candidate.ArchivedURL), "archive"
	}
	return firstNonEmpty(candidate.OriginalURL, candidate.PreviewURL, candidate.ResourceURL, candidate.PosterURL), "origin"
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

func runCreativeOrderCreate(cmd *cobra.Command, _ []string) error {
	return postCreativeDomainJSON(cmd, "/api/creative/orders")
}

func runCreativeOrderList(cmd *cobra.Command, _ []string) error {
	return getCreativeDomainJSON(cmd, "/api/creative/orders")
}

func runCreativeOrderGet(cmd *cobra.Command, args []string) error {
	return getCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0]))
}

func runCreativeOrderVariantPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/variants")
}

func runCreativeOrderAssetPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/assets")
}

func runCreativeOrderQCPut(cmd *cobra.Command, args []string) error {
	return putCreativeDomainJSON(cmd, "/api/creative/orders/"+url.PathEscape(args[0])+"/qc-reports")
}

func runCreativeOrderQCFinalize(cmd *cobra.Command, args []string) error {
	variantID, _ := cmd.Flags().GetString("variant")
	if strings.TrimSpace(variantID) == "" {
		return fmt.Errorf("--variant is required")
	}
	revision, _ := cmd.Flags().GetInt("revision")
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result any
	if err := client.PostJSON(ctx, "/api/creative/orders/"+url.PathEscape(args[0])+"/qc-finalize", map[string]any{
		"variant_id": strings.TrimSpace(variantID),
		"revision":   revision,
	}, &result); err != nil {
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

func putCreativeDomainJSON(cmd *cobra.Command, path string) error {
	return writeCreativeDomainJSON(cmd, path, "PUT")
}

func writeCreativeDomainJSON(cmd *cobra.Command, path, method string) error {
	payload, err := creativeDomainPayload(cmd)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
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
