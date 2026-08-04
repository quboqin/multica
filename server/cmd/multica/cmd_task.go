package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/service"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Work with native agent tasks",
}

var taskFanoutCmd = &cobra.Command{
	Use:   "fanout",
	Short: "Enqueue multiple native direct tasks for one agent",
	Long:  "Enqueue multiple native direct tasks for one agent. The input JSON must contain trigger_evidence_kind, trigger_evidence_ref_id, and items. Each item requires item_key and a JSON-object context with a non-empty type, for example {\"item_key\":\"material-42\",\"context\":{\"type\":\"creative_analysis\",\"input\":{...}}}.",
	RunE:  runTaskFanout,
}

var taskBySourceCmd = &cobra.Command{
	Use:   "by-source",
	Short: "Manage direct tasks by trigger evidence",
}

var taskBySourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List direct tasks for one trigger evidence source",
	RunE:  runTaskBySourceList,
}

var taskBySourceCancelCmd = &cobra.Command{
	Use:   "cancel",
	Short: "Cancel active direct tasks for one trigger evidence source",
	RunE:  runTaskBySourceCancel,
}

var taskBySourceRetryFailedCmd = &cobra.Command{
	Use:   "retry-failed",
	Short: "Retry failed direct tasks for one trigger evidence source",
	RunE:  runTaskBySourceRetryFailed,
}

type taskFanoutManifest struct {
	TriggerEvidenceKind  string                         `json:"trigger_evidence_kind"`
	TriggerEvidenceRefID string                         `json:"trigger_evidence_ref_id"`
	Items                []service.DirectTaskFanoutItem `json:"items"`
}

func init() {
	taskCmd.AddCommand(taskFanoutCmd)
	taskCmd.AddCommand(taskBySourceCmd)
	taskFanoutCmd.Flags().String("agent", "", "Target agent UUID (required)")
	taskFanoutCmd.Flags().String("input-file", "", "UTF-8 JSON manifest containing trigger evidence and items (required)")
	taskFanoutCmd.Flags().String("output", "json", "Output format: json")
	for _, command := range []*cobra.Command{taskBySourceListCmd, taskBySourceCancelCmd, taskBySourceRetryFailedCmd} {
		taskBySourceCmd.AddCommand(command)
		command.Flags().String("agent", "", "Target agent UUID (required)")
		command.Flags().String("kind", "", "Trigger evidence kind (required)")
		command.Flags().String("ref", "", "Trigger evidence UUID (required)")
		command.Flags().String("output", "json", "Output format: json")
	}
	taskBySourceListCmd.Flags().String("status", "", "Filter: active or failed")
}

func runTaskBySourceList(cmd *cobra.Command, _ []string) error {
	path, err := taskBySourcePath(cmd)
	if err != nil {
		return err
	}
	status, _ := cmd.Flags().GetString("status")
	if status != "" {
		if status != "active" && status != "failed" {
			return fmt.Errorf("--status must be active or failed")
		}
		path += "&status=" + url.QueryEscape(status)
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("list tasks by source: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runTaskBySourceCancel(cmd *cobra.Command, _ []string) error {
	return runTaskBySourcePost(cmd, "/cancel", "cancel tasks by source")
}

func runTaskBySourceRetryFailed(cmd *cobra.Command, _ []string) error {
	return runTaskBySourcePost(cmd, "/retry-failed", "retry failed tasks by source")
}

func runTaskBySourcePost(cmd *cobra.Command, suffix, action string) error {
	path, err := taskBySourcePath(cmd)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, strings.Split(path, "?")[0]+suffix+"?"+strings.Split(path, "?")[1], nil, &result); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return cli.PrintJSON(os.Stdout, result)
}

func taskBySourcePath(cmd *cobra.Command) (string, error) {
	agentID, _ := cmd.Flags().GetString("agent")
	kind, _ := cmd.Flags().GetString("kind")
	ref, _ := cmd.Flags().GetString("ref")
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(kind) == "" || strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("--agent, --kind, and --ref are required")
	}
	return "/api/agents/" + url.PathEscape(agentID) + "/tasks/by-source?trigger_evidence_kind=" + url.QueryEscape(kind) + "&trigger_evidence_ref_id=" + url.QueryEscape(ref), nil
}

func runTaskFanout(cmd *cobra.Command, _ []string) error {
	agentID, _ := cmd.Flags().GetString("agent")
	inputFile, _ := cmd.Flags().GetString("input-file")
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("--agent is required")
	}
	if strings.TrimSpace(inputFile) == "" {
		return fmt.Errorf("--input-file is required")
	}
	raw, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("read task fanout manifest: %w", err)
	}
	var manifest taskFanoutManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("parse task fanout manifest: %w", err)
	}
	if strings.TrimSpace(manifest.TriggerEvidenceKind) == "" || strings.TrimSpace(manifest.TriggerEvidenceRefID) == "" || len(manifest.Items) == 0 {
		return fmt.Errorf("manifest requires trigger_evidence_kind, trigger_evidence_ref_id, and items; each item context must be an object with type")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/agents/"+agentID+"/tasks/fanout", manifest, &result); err != nil {
		return fmt.Errorf("fan out tasks: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}
