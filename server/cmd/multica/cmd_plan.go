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

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Work with plans",
}

var planListCmd = &cobra.Command{
	Use:   "list",
	Short: "List plans in the workspace",
	RunE:  runPlanList,
}

var planGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get plan details",
	Args:  exactArgs(1),
	RunE:  runPlanGet,
}

var planCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new plan",
	RunE:  runPlanCreate,
}

var planUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a plan",
	Args:  exactArgs(1),
	RunE:  runPlanUpdate,
}

var planDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a plan",
	Args:  exactArgs(1),
	RunE:  runPlanDelete,
}

var planStatusCmd = &cobra.Command{
	Use:   "status <id> <status>",
	Short: "Change plan status",
	Args:  exactArgs(2),
	RunE:  runPlanStatus,
}

var validPlanStatuses = []string{
	"planned", "in_progress", "paused", "completed", "cancelled",
}

func validatePlanStatus(status string) error {
	for _, s := range validPlanStatuses {
		if s == status {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q; valid values: %s", status, strings.Join(validPlanStatuses, ", "))
}

func init() {
	planCmd.AddCommand(planListCmd)
	planCmd.AddCommand(planGetCmd)
	planCmd.AddCommand(planCreateCmd)
	planCmd.AddCommand(planUpdateCmd)
	planCmd.AddCommand(planDeleteCmd)
	planCmd.AddCommand(planStatusCmd)

	planListCmd.Flags().String("output", "table", "Output format: table or json")
	planListCmd.Flags().Bool("full-id", false, "Show full UUIDs in table output")
	planListCmd.Flags().String("status", "", "Filter by status")

	planGetCmd.Flags().String("output", "json", "Output format: table or json")

	planCreateCmd.Flags().String("title", "", "Plan title (required)")
	planCreateCmd.Flags().String("description", "", "Plan description")
	planCreateCmd.Flags().String("start-date", "", "Start date (YYYY-MM-DD)")
	planCreateCmd.Flags().String("end-date", "", "End date (YYYY-MM-DD)")
	planCreateCmd.Flags().String("status", "", "Plan status")
	planCreateCmd.Flags().Int32("position", 0, "Display position")
	planCreateCmd.Flags().String("output", "json", "Output format: table or json")

	planUpdateCmd.Flags().String("title", "", "New title")
	planUpdateCmd.Flags().String("description", "", "New description")
	planUpdateCmd.Flags().String("start-date", "", "New start date (YYYY-MM-DD; pass empty string to clear)")
	planUpdateCmd.Flags().String("end-date", "", "New end date (YYYY-MM-DD; pass empty string to clear)")
	planUpdateCmd.Flags().String("status", "", "New status")
	planUpdateCmd.Flags().Int32("position", 0, "New display position")
	planUpdateCmd.Flags().String("output", "json", "Output format: table or json")

	planDeleteCmd.Flags().String("output", "json", "Output format: table or json")
	planStatusCmd.Flags().String("output", "table", "Output format: table or json")
}

func runPlanList(cmd *cobra.Command, _ []string) error {
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
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		params.Set("status", v)
	}

	path := "/api/milestones"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result map[string]any
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("list plans: %w", err)
	}

	plansRaw, _ := result["milestones"].([]any)
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, plansRaw)
	}

	fullID, _ := cmd.Flags().GetBool("full-id")
	headers := []string{"ID", "TITLE", "STATUS", "START", "END"}
	rows := make([][]string, 0, len(plansRaw))
	for _, raw := range plansRaw {
		plan, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			displayID(strVal(plan, "id"), fullID),
			strVal(plan, "title"),
			strVal(plan, "status"),
			strVal(plan, "start_date"),
			strVal(plan, "end_date"),
		})
	}
	cli.PrintTable(os.Stdout, headers, rows)
	return nil
}

func runPlanGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	planRef, err := resolvePlanID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}

	var plan map[string]any
	if err := client.GetJSON(ctx, "/api/milestones/"+planRef.ID, &plan); err != nil {
		return fmt.Errorf("get plan: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		headers := []string{"ID", "TITLE", "STATUS", "START", "END", "DESCRIPTION"}
		rows := [][]string{{
			strVal(plan, "id"),
			strVal(plan, "title"),
			strVal(plan, "status"),
			strVal(plan, "start_date"),
			strVal(plan, "end_date"),
			strVal(plan, "description"),
		}}
		cli.PrintTable(os.Stdout, headers, rows)
		return nil
	}
	return cli.PrintJSON(os.Stdout, plan)
}

func runPlanCreate(cmd *cobra.Command, _ []string) error {
	title, _ := cmd.Flags().GetString("title")
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("--title is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{"title": strings.TrimSpace(title)}
	if v, _ := cmd.Flags().GetString("description"); v != "" {
		body["description"] = v
	}
	if v, _ := cmd.Flags().GetString("start-date"); cmd.Flags().Changed("start-date") {
		body["start_date"] = strings.TrimSpace(v)
	}
	if v, _ := cmd.Flags().GetString("end-date"); cmd.Flags().Changed("end-date") {
		body["end_date"] = strings.TrimSpace(v)
	}
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		if err := validatePlanStatus(v); err != nil {
			return err
		}
		body["status"] = v
	}
	if cmd.Flags().Changed("position") {
		v, _ := cmd.Flags().GetInt32("position")
		body["position"] = v
	}

	var result map[string]any
	if err := client.PostJSON(ctx, "/api/milestones", body, &result); err != nil {
		return fmt.Errorf("create plan: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		headers := []string{"ID", "TITLE", "STATUS"}
		rows := [][]string{{strVal(result, "id"), strVal(result, "title"), strVal(result, "status")}}
		cli.PrintTable(os.Stdout, headers, rows)
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runPlanUpdate(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	planRef, err := resolvePlanID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}

	body := map[string]any{}
	if cmd.Flags().Changed("title") {
		v, _ := cmd.Flags().GetString("title")
		body["title"] = v
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		body["description"] = v
	}
	if cmd.Flags().Changed("start-date") {
		v, _ := cmd.Flags().GetString("start-date")
		body["start_date"] = strings.TrimSpace(v)
	}
	if cmd.Flags().Changed("end-date") {
		v, _ := cmd.Flags().GetString("end-date")
		body["end_date"] = strings.TrimSpace(v)
	}
	if cmd.Flags().Changed("status") {
		v, _ := cmd.Flags().GetString("status")
		if err := validatePlanStatus(v); err != nil {
			return err
		}
		body["status"] = v
	}
	if cmd.Flags().Changed("position") {
		v, _ := cmd.Flags().GetInt32("position")
		body["position"] = v
	}
	if len(body) == 0 {
		return fmt.Errorf("no fields to update; use flags like --title, --status, --description, --start-date, --end-date, --position")
	}

	var result map[string]any
	if err := client.PutJSON(ctx, "/api/milestones/"+planRef.ID, body, &result); err != nil {
		return fmt.Errorf("update plan: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		headers := []string{"ID", "TITLE", "STATUS"}
		rows := [][]string{{strVal(result, "id"), strVal(result, "title"), strVal(result, "status")}}
		cli.PrintTable(os.Stdout, headers, rows)
		return nil
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runPlanDelete(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	planRef, err := resolvePlanID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}
	if err := client.DeleteJSON(ctx, "/api/milestones/"+planRef.ID); err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Plan %s deleted.\n", planRef.Display)
	return nil
}

func runPlanStatus(cmd *cobra.Command, args []string) error {
	status := args[1]
	if err := validatePlanStatus(status); err != nil {
		return err
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	planRef, err := resolvePlanID(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}

	body := map[string]any{"status": status}
	var result map[string]any
	if err := client.PutJSON(ctx, "/api/milestones/"+planRef.ID, body, &result); err != nil {
		return fmt.Errorf("update plan status: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Plan %s status changed to %s.\n", strVal(result, "title"), status)
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	return nil
}
