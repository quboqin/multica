package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var kpiCmd = &cobra.Command{
	Use:   "kpi",
	Short: "Work with KPI metrics",
}

var kpiListCmd = &cobra.Command{
	Use:   "list",
	Short: "List KPI metrics in the workspace",
	RunE:  runKpiList,
}

var kpiGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get KPI details",
	Args:  exactArgs(1),
	RunE:  runKpiGet,
}

var kpiCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a KPI metric",
	RunE:  runKpiCreate,
}

var kpiUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a KPI metric",
	Args:  exactArgs(1),
	RunE:  runKpiUpdate,
}

var kpiDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a KPI metric",
	Args:  exactArgs(1),
	RunE:  runKpiDelete,
}

var validKpiStatuses = []string{"on_track", "at_risk", "missed", "pending"}

func validateKpiStatusCLI(status string) error {
	for _, s := range validKpiStatuses {
		if s == status {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q; valid values: %s", status, strings.Join(validKpiStatuses, ", "))
}

func init() {
	kpiCmd.AddCommand(kpiListCmd)
	kpiCmd.AddCommand(kpiGetCmd)
	kpiCmd.AddCommand(kpiCreateCmd)
	kpiCmd.AddCommand(kpiUpdateCmd)
	kpiCmd.AddCommand(kpiDeleteCmd)

	kpiListCmd.Flags().String("output", "table", "Output format: table or json")
	kpiListCmd.Flags().Bool("full-id", false, "Show full UUIDs in table output")

	kpiGetCmd.Flags().String("output", "json", "Output format: table or json")

	kpiCreateCmd.Flags().String("name", "", "KPI name (required)")
	kpiCreateCmd.Flags().String("owner", "", "KPI owner")
	kpiCreateCmd.Flags().String("target", "", "Target value")
	kpiCreateCmd.Flags().String("current", "", "Current value")
	kpiCreateCmd.Flags().String("status", "", "KPI status")
	kpiCreateCmd.Flags().String("note", "", "KPI note")
	kpiCreateCmd.Flags().String("link-type", "", "Link type: none, plan, project, or issue")
	kpiCreateCmd.Flags().String("link-id", "", "Link target ID")
	kpiCreateCmd.Flags().String("plan", "", "Convenience alias for --link-type plan --link-id <id>")
	kpiCreateCmd.Flags().String("project", "", "Convenience alias for --link-type project --link-id <id>")
	kpiCreateCmd.Flags().String("issue", "", "Convenience alias for --link-type issue --link-id <id>")
	kpiCreateCmd.Flags().Int32("completion-rate", 0, "Completion rate (0-100)")
	kpiCreateCmd.Flags().Int32("position", 0, "Display position")
	kpiCreateCmd.Flags().String("output", "json", "Output format: table or json")

	kpiUpdateCmd.Flags().String("name", "", "New KPI name")
	kpiUpdateCmd.Flags().String("owner", "", "New KPI owner")
	kpiUpdateCmd.Flags().String("target", "", "New target value")
	kpiUpdateCmd.Flags().String("current", "", "New current value")
	kpiUpdateCmd.Flags().String("status", "", "New KPI status")
	kpiUpdateCmd.Flags().String("note", "", "New KPI note")
	kpiUpdateCmd.Flags().String("link-type", "", "New link type: none, plan, project, or issue")
	kpiUpdateCmd.Flags().String("link-id", "", "New link target ID")
	kpiUpdateCmd.Flags().String("plan", "", "Convenience alias for --link-type plan --link-id <id>")
	kpiUpdateCmd.Flags().String("project", "", "Convenience alias for --link-type project --link-id <id>")
	kpiUpdateCmd.Flags().String("issue", "", "Convenience alias for --link-type issue --link-id <id>")
	kpiUpdateCmd.Flags().Int32("completion-rate", 0, "New completion rate (0-100)")
	kpiUpdateCmd.Flags().Int32("position", 0, "New display position")
	kpiUpdateCmd.Flags().String("output", "json", "Output format: table or json")

	kpiDeleteCmd.Flags().String("output", "json", "Output format: table or json")
}

func runKpiList(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	params := url.Values{}
	if client.WorkspaceID != "" {
		params.Set("workspace_id", client.WorkspaceID)
	}
	path := "/api/kpi-metrics"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result map[string]any
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("list kpis: %w", err)
	}

	items, _ := result["metrics"].([]any)
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, items)
	}

	fullID, _ := cmd.Flags().GetBool("full-id")
	headers := []string{"ID", "NAME", "STATUS", "OWNER", "TARGET", "CURRENT", "LINK"}
	rows := make([][]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			displayID(strVal(item, "id"), fullID),
			strVal(item, "name"),
			strVal(item, "status"),
			strVal(item, "owner"),
			strVal(item, "target"),
			strVal(item, "current"),
			formatKpiLink(item),
		})
	}
	cli.PrintTable(os.Stdout, headers, rows)
	return nil
}

func runKpiGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	kpiRef, err := resolveKpiID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve kpi: %w", err)
	}

	var item map[string]any
	if err := client.GetJSON(ctx, "/api/kpi-metrics/"+kpiRef.ID, &item); err != nil {
		return fmt.Errorf("get kpi: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		headers := []string{"ID", "NAME", "STATUS", "OWNER", "TARGET", "CURRENT", "NOTE", "LINK"}
		rows := [][]string{{
			strVal(item, "id"),
			strVal(item, "name"),
			strVal(item, "status"),
			strVal(item, "owner"),
			strVal(item, "target"),
			strVal(item, "current"),
			strVal(item, "note"),
			formatKpiLink(item),
		}}
		cli.PrintTable(os.Stdout, headers, rows)
		return nil
	}
	return cli.PrintJSON(os.Stdout, item)
}

func runKpiCreate(cmd *cobra.Command, _ []string) error {
	name, _ := cmd.Flags().GetString("name")
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{"name": strings.TrimSpace(name)}
	if v, _ := cmd.Flags().GetString("owner"); cmd.Flags().Changed("owner") {
		body["owner"] = v
	}
	if v, _ := cmd.Flags().GetString("target"); cmd.Flags().Changed("target") {
		body["target"] = v
	}
	if v, _ := cmd.Flags().GetString("current"); cmd.Flags().Changed("current") {
		body["current"] = v
	}
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		if err := validateKpiStatusCLI(v); err != nil {
			return err
		}
		body["status"] = v
	}
	if v, _ := cmd.Flags().GetString("note"); cmd.Flags().Changed("note") {
		body["note"] = v
	}
	if err := applyKpiLinkFlags(ctx, client, cmd, body, nil); err != nil {
		return err
	}
	if cmd.Flags().Changed("completion-rate") {
		v, _ := cmd.Flags().GetInt32("completion-rate")
		if v < 0 || v > 100 {
			return fmt.Errorf("--completion-rate must be between 0 and 100")
		}
		body["completion_rate"] = v
	}
	if cmd.Flags().Changed("position") {
		v, _ := cmd.Flags().GetInt32("position")
		body["position"] = v
	}

	var result map[string]any
	if err := client.PostJSON(ctx, "/api/kpi-metrics", body, &result); err != nil {
		return fmt.Errorf("create kpi: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		headers := []string{"ID", "NAME", "STATUS"}
		rows := [][]string{{strVal(result, "id"), strVal(result, "name"), strVal(result, "status")}}
		cli.PrintTable(os.Stdout, headers, rows)
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runKpiUpdate(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	kpiRef, err := resolveKpiID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve kpi: %w", err)
	}

	var existing map[string]any
	if err := client.GetJSON(ctx, "/api/kpi-metrics/"+kpiRef.ID, &existing); err != nil {
		return fmt.Errorf("get kpi: %w", err)
	}

	body := map[string]any{}
	for _, flag := range []string{"name", "owner", "target", "current", "note"} {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetString(flag)
			body[strings.ReplaceAll(flag, "-", "_")] = v
		}
	}
	if cmd.Flags().Changed("status") {
		v, _ := cmd.Flags().GetString("status")
		if err := validateKpiStatusCLI(v); err != nil {
			return err
		}
		body["status"] = v
	}
	if err := applyKpiLinkFlags(ctx, client, cmd, body, existing); err != nil {
		return err
	}
	if cmd.Flags().Changed("completion-rate") {
		v, _ := cmd.Flags().GetInt32("completion-rate")
		if v < 0 || v > 100 {
			return fmt.Errorf("--completion-rate must be between 0 and 100")
		}
		body["completion_rate"] = v
	}
	if cmd.Flags().Changed("position") {
		v, _ := cmd.Flags().GetInt32("position")
		body["position"] = v
	}
	if len(body) == 0 {
		return fmt.Errorf("no fields to update; use flags like --name, --status, --owner, --target, --current, --note, --plan, --project, --issue, --completion-rate, --position")
	}

	var result map[string]any
	if err := client.PutJSON(ctx, "/api/kpi-metrics/"+kpiRef.ID, body, &result); err != nil {
		return fmt.Errorf("update kpi: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		headers := []string{"ID", "NAME", "STATUS"}
		rows := [][]string{{strVal(result, "id"), strVal(result, "name"), strVal(result, "status")}}
		cli.PrintTable(os.Stdout, headers, rows)
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runKpiDelete(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	kpiRef, err := resolveKpiID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve kpi: %w", err)
	}
	if err := client.DeleteJSON(ctx, "/api/kpi-metrics/"+kpiRef.ID); err != nil {
		return fmt.Errorf("delete kpi: %w", err)
	}
	fmt.Fprintf(os.Stderr, "KPI %s deleted.\n", kpiRef.Display)
	return nil
}

func applyKpiLinkFlags(ctx context.Context, client *cli.APIClient, cmd *cobra.Command, body map[string]any, existing map[string]any) error {
	explicit := 0
	if cmd.Flags().Changed("plan") {
		explicit++
	}
	if cmd.Flags().Changed("project") {
		explicit++
	}
	if cmd.Flags().Changed("issue") {
		explicit++
	}
	if (cmd.Flags().Changed("link-type") || cmd.Flags().Changed("link-id")) && explicit > 0 {
		return fmt.Errorf("--link-type/--link-id cannot be combined with --plan, --project, or --issue")
	}
	if explicit > 1 {
		return fmt.Errorf("only one of --plan, --project, or --issue may be set")
	}

	if cmd.Flags().Changed("plan") {
		v, _ := cmd.Flags().GetString("plan")
		planRef, err := resolvePlanID(ctx, client, v)
		if err != nil {
			return fmt.Errorf("resolve plan: %w", err)
		}
		body["link_type"] = "milestone"
		body["link_id"] = planRef.ID
		return nil
	}
	if cmd.Flags().Changed("project") {
		v, _ := cmd.Flags().GetString("project")
		projectRef, err := resolveProjectID(ctx, client, v)
		if err != nil {
			return fmt.Errorf("resolve project: %w", err)
		}
		body["link_type"] = "project"
		body["link_id"] = projectRef.ID
		return nil
	}
	if cmd.Flags().Changed("issue") {
		v, _ := cmd.Flags().GetString("issue")
		issueRef, err := resolveIssueRef(ctx, client, v)
		if err != nil {
			return fmt.Errorf("resolve issue: %w", err)
		}
		body["link_type"] = "issue"
		body["link_id"] = issueRef.ID
		return nil
	}

	if cmd.Flags().Changed("link-type") {
		v, _ := cmd.Flags().GetString("link-type")
		switch v {
		case "", "none":
			body["link_type"] = "none"
		case "plan":
			body["link_type"] = "milestone"
		case "project", "issue", "milestone":
			body["link_type"] = v
		default:
			return fmt.Errorf("invalid link type %q; valid values: none, plan, project, issue", v)
		}
	}
	if cmd.Flags().Changed("link-id") {
		v, _ := cmd.Flags().GetString("link-id")
		linkType, _ := body["link_type"].(string)
		if linkType == "" && existing != nil {
			linkType = strVal(existing, "link_type")
		}
		resolvedID, err := resolveKpiLinkTarget(ctx, client, linkType, strings.TrimSpace(v))
		if err != nil {
			return err
		}
		body["link_id"] = resolvedID
	}
	return nil
}

func formatKpiLink(item map[string]any) string {
	linkType := strVal(item, "link_type")
	linkID := strVal(item, "link_id")
	if linkType == "" || linkType == "none" {
		return ""
	}
	return linkType + ":" + linkID
}

func resolveKpiLinkTarget(ctx context.Context, client *cli.APIClient, linkType, linkID string) (string, error) {
	if linkID == "" {
		return "", nil
	}
	switch linkType {
	case "", "none":
		return linkID, nil
	case "milestone":
		ref, err := resolvePlanID(ctx, client, linkID)
		if err != nil {
			return "", fmt.Errorf("resolve plan: %w", err)
		}
		return ref.ID, nil
	case "project":
		ref, err := resolveProjectID(ctx, client, linkID)
		if err != nil {
			return "", fmt.Errorf("resolve project: %w", err)
		}
		return ref.ID, nil
	case "issue":
		ref, err := resolveIssueRef(ctx, client, linkID)
		if err != nil {
			return "", fmt.Errorf("resolve issue: %w", err)
		}
		return ref.ID, nil
	default:
		return linkID, nil
	}
}
