package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Run credential-backed crawls",
}

var crawlRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a crawl through the credential broker",
	RunE:  runCrawlRun,
}

func init() {
	crawlCmd.AddCommand(crawlRunCmd)

	crawlRunCmd.Flags().String("profile-id", "", "Credential profile ID")
	crawlRunCmd.Flags().String("connector", "", "Credential connector ID; uses the workspace's active profile")
	crawlRunCmd.Flags().String("issue-id", "", "Issue UUID or identifier to receive imported creative material candidates")
	crawlRunCmd.Flags().String("capability", "material_search", "Connector capability to run")
	crawlRunCmd.Flags().String("params-json", "{}", "Crawl params as JSON")
	crawlRunCmd.Flags().String("intent", "", "Natural-language crawl intent; merged into params as intent")
	crawlRunCmd.Flags().String("analysis-agent-id", "", "Reference-analysis agent UUID to record on this Crawl Run")
	crawlRunCmd.Flags().String("intent-file", "", "Read natural-language crawl intent from a file")
	crawlRunCmd.Flags().Bool("intent-stdin", false, "Read natural-language crawl intent from stdin")
	crawlRunCmd.Flags().String("output", "json", "Output format: json or table")
	crawlRunCmd.Flags().Duration("timeout", crawlRunHTTPTimeout, "Maximum time to wait for the crawl response")
}

func runCrawlRun(cmd *cobra.Command, args []string) error {
	profileID, _ := cmd.Flags().GetString("profile-id")
	connector, _ := cmd.Flags().GetString("connector")
	issueID, _ := cmd.Flags().GetString("issue-id")
	if profileID == "" && connector == "" {
		return fmt.Errorf("--profile-id or --connector is required")
	}
	capability, _ := cmd.Flags().GetString("capability")
	paramsJSON, _ := cmd.Flags().GetString("params-json")
	intent, err := resolveCrawlIntent(cmd)
	if err != nil {
		return err
	}
	params, err := crawlParamsWithIntent(paramsJSON, intent)
	if err != nil {
		return fmt.Errorf("--params-json must be valid JSON: %w", err)
	}
	analysisAgentID, _ := cmd.Flags().GetString("analysis-agent-id")
	params, err = crawlParamsWithAnalysisAgent(params, analysisAgentID)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	timeout, _ := cmd.Flags().GetDuration("timeout")
	if timeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if client.HTTPClient != nil {
		if cmd.Flags().Changed("timeout") || client.HTTPClient.Timeout < timeout {
			client.HTTPClient.Timeout = timeout
		}
	}
	var resp struct {
		Status            string          `json:"status"`
		Downloaded        int             `json:"downloaded"`
		OutputPrefix      string          `json:"output_prefix"`
		Message           string          `json:"message"`
		Raw               json.RawMessage `json:"raw,omitempty"`
		CreativeMaterials any             `json:"creative_materials,omitempty"`
		CrawlRunID        string          `json:"crawl_run_id,omitempty"`
		AnalysisAgentID   string          `json:"analysis_agent_id,omitempty"`
	}
	if err := client.PostJSON(context.Background(), "/api/credential-crawl", map[string]any{
		"profile_id":   profileID,
		"issue_id":     issueID,
		"connector_id": connector,
		"capability":   capability,
		"params":       params,
	}, &resp); err != nil {
		return err
	}
	if summary, ok := resp.CreativeMaterials.(map[string]any); ok {
		resp.CrawlRunID = strVal(summary, "run_id")
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, resp)
	}
	cli.PrintTable(os.Stdout, []string{"STATUS", "DOWNLOADED", "OUTPUT", "MESSAGE"}, [][]string{{
		resp.Status,
		fmt.Sprintf("%d", resp.Downloaded),
		resp.OutputPrefix,
		resp.Message,
	}})
	return nil
}

const crawlRunHTTPTimeout = 15 * time.Minute

func resolveCrawlIntent(cmd *cobra.Command) (string, error) {
	intent, _ := cmd.Flags().GetString("intent")
	intentFile, _ := cmd.Flags().GetString("intent-file")
	intentStdin, _ := cmd.Flags().GetBool("intent-stdin")
	sources := 0
	if strings.TrimSpace(intent) != "" {
		sources++
	}
	if strings.TrimSpace(intentFile) != "" {
		sources++
	}
	if intentStdin {
		sources++
	}
	if sources > 1 {
		return "", fmt.Errorf("use only one of --intent, --intent-file, or --intent-stdin")
	}
	if strings.TrimSpace(intentFile) != "" {
		data, err := os.ReadFile(intentFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	if intentStdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(intent), nil
}

func crawlParamsWithIntent(paramsJSON string, intent string) (json.RawMessage, error) {
	raw := []byte(strings.TrimSpace(paramsJSON))
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if strings.TrimSpace(intent) == "" {
		var params json.RawMessage
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, err
		}
		return params, nil
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{}
	}
	params["intent"] = strings.TrimSpace(intent)
	merged, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(merged), nil
}

func crawlParamsWithAnalysisAgent(params json.RawMessage, analysisAgentID string) (json.RawMessage, error) {
	analysisAgentID = strings.TrimSpace(analysisAgentID)
	if analysisAgentID == "" {
		return params, nil
	}
	var values map[string]any
	if err := json.Unmarshal(params, &values); err != nil {
		return nil, fmt.Errorf("crawl params must be a JSON object: %w", err)
	}
	if values == nil {
		values = map[string]any{}
	}
	values["analysis_agent_id"] = analysisAgentID
	merged, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(merged), nil
}
