package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	Short: "Work with creative material jobs",
}

var creativeCreateJobCmd = &cobra.Command{
	Use:   "create-job <issue-id>",
	Short: "Create a creative job from selected issue materials",
	Args:  exactArgs(1),
	RunE:  runCreativeCreateJob,
}

var creativeGetJobCmd = &cobra.Command{
	Use:   "get-job <issue-id> <job-id>",
	Short: "Get and optionally wait for a creative job",
	Args:  exactArgs(2),
	RunE:  runCreativeGetJob,
}

var creativeDownloadCmd = &cobra.Command{
	Use:   "download <issue-id> <job-id>",
	Short: "Download a completed creative job as a ZIP package",
	Args:  exactArgs(2),
	RunE:  runCreativeDownload,
}

type creativeJobCLI struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"`
	Stage         string   `json:"stage"`
	Progress      int      `json:"progress"`
	ExternalJobID string   `json:"external_job_id"`
	ErrorMessage  string   `json:"error_message"`
	CandidateIDs  []string `json:"candidate_ids"`
	Variants      []any    `json:"variants"`
}

type creativeMaterialsCLIResponse struct {
	EditJobs []creativeJobCLI `json:"edit_jobs"`
}

func init() {
	creativeCmd.AddCommand(creativeCreateJobCmd, creativeGetJobCmd, creativeDownloadCmd)

	creativeCreateJobCmd.Flags().StringSlice("candidate-ids", nil, "Candidate IDs; defaults to all selected materials on the issue")
	creativeCreateJobCmd.Flags().String("prompt", "", "Additional creative direction")
	creativeCreateJobCmd.Flags().Bool("prompt-stdin", false, "Read the prompt from stdin")
	creativeCreateJobCmd.Flags().String("prompt-file", "", "Read the prompt from a UTF-8 file")
	creativeCreateJobCmd.Flags().String("rules-json", "", "Dynamic creative rules as a JSON object")
	creativeCreateJobCmd.Flags().Int("variant-count", 0, "Number of variants (1-6; default 3)")
	creativeCreateJobCmd.Flags().StringSlice("sizes", nil, "Output sizes, e.g. 1080x1080,800x1000")
	creativeCreateJobCmd.Flags().String("market", "", "Creative market profile, e.g. idn-adakami")
	creativeCreateJobCmd.Flags().String("strategy", "", "Creative strategy, e.g. instruct or resize")
	creativeCreateJobCmd.Flags().String("output", "json", "Output format: json or table")

	creativeGetJobCmd.Flags().Bool("wait", false, "Wait until the job is completed or failed")
	creativeGetJobCmd.Flags().Duration("timeout", 10*time.Minute, "Maximum wait time")
	creativeGetJobCmd.Flags().Duration("interval", 2*time.Second, "Status refresh interval while waiting")
	creativeGetJobCmd.Flags().String("output", "json", "Output format: json or table")

	creativeDownloadCmd.Flags().StringP("output-dir", "o", ".", "Directory for the ZIP package")
}

func runCreativeCreateJob(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	prompt, _, err := resolveTextFlag(cmd, "prompt")
	if err != nil {
		return err
	}
	rules, err := creativeRulesFromFlags(cmd)
	if err != nil {
		return err
	}
	candidateIDs, _ := cmd.Flags().GetStringSlice("candidate-ids")
	body := map[string]any{
		"candidate_ids": uniqueCLIStrings(candidateIDs),
		"prompt":        prompt,
	}
	if len(rules) > 0 {
		body["rules"] = rules
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()
	var response creativeMaterialsCLIResponse
	if err := client.PostJSON(ctx, "/api/issues/"+args[0]+"/creative-edit-jobs", body, &response); err != nil {
		return err
	}
	if len(response.EditJobs) == 0 {
		return fmt.Errorf("creative job response did not include a job")
	}
	return printCreativeJob(cmd, response.EditJobs[0])
}

func runCreativeGetJob(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	waitForCompletion, _ := cmd.Flags().GetBool("wait")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	interval, _ := cmd.Flags().GetDuration("interval")
	if timeout <= 0 || interval <= 0 {
		return fmt.Errorf("--timeout and --interval must be greater than zero")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		job, err := fetchCreativeJob(ctx, client, args[0], args[1])
		if err != nil {
			return err
		}
		if !waitForCompletion || job.Status == "completed" || job.Status == "failed" {
			return printCreativeJob(cmd, job)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for creative job: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func fetchCreativeJob(ctx context.Context, client *cli.APIClient, issueID, jobID string) (creativeJobCLI, error) {
	var response creativeMaterialsCLIResponse
	path := "/api/issues/" + issueID + "/creative-edit-jobs/" + jobID + "/sync"
	if err := client.PostJSON(ctx, path, map[string]any{}, &response); err != nil {
		return creativeJobCLI{}, err
	}
	for _, job := range response.EditJobs {
		if job.ID == jobID {
			return job, nil
		}
	}
	return creativeJobCLI{}, fmt.Errorf("creative job %s was not returned", jobID)
}

func runCreativeDownload(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(5*time.Minute))
	defer cancel()
	path := fmt.Sprintf("/api/issues/%s/creative-edit-jobs/%s/download", args[0], args[1])
	data, err := client.DownloadFile(ctx, path)
	if err != nil {
		return err
	}
	outputDir, _ := cmd.Flags().GetString("output-dir")
	filename := "creative-job-" + shortCLIIdentifier(args[1]) + ".zip"
	destination := filepath.Join(outputDir, filename)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		return err
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		abs = destination
	}
	return cli.PrintJSON(os.Stdout, map[string]any{
		"job_id": args[1],
		"path":   abs,
		"size":   len(data),
	})
}

func creativeRulesFromFlags(cmd *cobra.Command) (map[string]any, error) {
	rules := map[string]any{}
	raw, _ := cmd.Flags().GetString("rules-json")
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			return nil, fmt.Errorf("--rules-json must be a JSON object: %w", err)
		}
	}
	variantCount, _ := cmd.Flags().GetInt("variant-count")
	if variantCount != 0 {
		rules["variant_count"] = variantCount
	}
	sizes, _ := cmd.Flags().GetStringSlice("sizes")
	if len(sizes) > 0 {
		parsed, err := parseCreativeSizes(sizes)
		if err != nil {
			return nil, err
		}
		rules["sizes"] = parsed
	}
	market, _ := cmd.Flags().GetString("market")
	if strings.TrimSpace(market) != "" {
		rules["market"] = strings.TrimSpace(market)
	}
	strategy, _ := cmd.Flags().GetString("strategy")
	if strings.TrimSpace(strategy) != "" {
		rules["strategy"] = strings.TrimSpace(strategy)
	}
	return rules, nil
}

func parseCreativeSizes(values []string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		parts := strings.Split(value, "x")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid creative size %q; expected WIDTHxHEIGHT", value)
		}
		width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid creative size %q", value)
		}
		height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || width < 100 || height < 100 || width > 4096 || height > 4096 {
			return nil, fmt.Errorf("invalid creative size %q", value)
		}
		out = append(out, map[string]any{
			"width":  width,
			"height": height,
			"label":  fmt.Sprintf("%dx%d", width, height),
		})
	}
	return out, nil
}

func printCreativeJob(cmd *cobra.Command, job creativeJobCLI) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, job)
	}
	cli.PrintTable(os.Stdout, []string{"ID", "STATUS", "STAGE", "PROGRESS", "EXTERNAL JOB"}, [][]string{{
		job.ID,
		job.Status,
		job.Stage,
		fmt.Sprintf("%d%%", job.Progress),
		job.ExternalJobID,
	}})
	return nil
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
