package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/previewdetect"
)

func newPreviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Manage interactive preview sessions",
	}
	cmd.AddCommand(newPreviewCreateCommand())
	cmd.AddCommand(newPreviewDeviceCommand())
	cmd.AddCommand(newPreviewListCommand())
	cmd.AddCommand(newPreviewStopCommand())
	cmd.AddCommand(newPreviewDetectCommand())
	return cmd
}

func newPreviewDetectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detect [path]",
		Short: "Detect previewable targets from the current development context",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runPreviewDetect,
	}
	cmd.Flags().Bool("write", false, "Write .multica/preview/targets.json under the scan root")
	cmd.Flags().String("output", "json", "Output format (json)")
	return cmd
}

func runPreviewDetect(cmd *cobra.Command, args []string) error {
	root := ""
	if len(args) == 1 {
		root = strings.TrimSpace(args[0])
	}
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}
	report, err := previewdetect.Detect(root)
	if err != nil {
		return fmt.Errorf("detect preview targets: %w", err)
	}
	if write, _ := cmd.Flags().GetBool("write"); write {
		if _, err := previewdetect.WriteReport(root, report); err != nil {
			return fmt.Errorf("write preview target report: %w", err)
		}
	}
	return cli.PrintJSON(os.Stdout, report)
}

func newPreviewDeviceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "device",
		Short: "Manage an Android local-device preview",
	}
	create := &cobra.Command{
		Use:   "create",
		Short: "Attach a local Android Device Runtime to an issue",
		Args:  cobra.NoArgs,
		RunE:  runPreviewDeviceCreate,
	}
	create.Flags().String("issue", "", "Issue identifier or ID (required)")
	create.Flags().String("url", "", "Loopback Device Runtime URL pinned with ?serial=<device> (required)")
	create.Flags().String("title", "Local Android device", "Preview title")
	create.Flags().String("expires-at", "", "Optional expiration timestamp (RFC3339)")
	create.Flags().String("output", "json", "Output format: table or json")
	cmd.AddCommand(create)
	cmd.AddCommand(newPreviewDeviceSyncCommand())
	cmd.AddCommand(newPreviewDeviceSnapshotCommand())
	cmd.AddCommand(newPreviewDeviceRunCommand())
	return cmd
}

func newPreviewCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Register an external Web preview for an issue",
		Args:  cobra.NoArgs,
		RunE:  runPreviewCreate,
	}
	cmd.Flags().String("issue", "", "Issue identifier or ID (required)")
	cmd.Flags().String("url", "", "Externally reachable preview URL (required)")
	cmd.Flags().String("title", "", "Optional preview title")
	cmd.Flags().String("expires-at", "", "Optional expiration timestamp (RFC3339)")
	cmd.Flags().String("output", "json", "Output format: table or json")
	return cmd
}

func newPreviewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List preview sessions for an issue",
		Args:  cobra.NoArgs,
		RunE:  runPreviewList,
	}
	cmd.Flags().String("issue", "", "Issue identifier or ID (required)")
	cmd.Flags().String("output", "table", "Output format: table or json")
	return cmd
}

func newPreviewStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <session-id>",
		Short: "Stop a preview session",
		Args:  exactArgs(1),
		RunE:  runPreviewStop,
	}
	cmd.Flags().String("output", "json", "Output format: table or json")
	return cmd
}

var previewCmd = newPreviewCommand()

type previewSessionListResponse struct {
	PreviewSessions []map[string]any `json:"preview_sessions"`
	Total           int              `json:"total"`
}

func runPreviewCreate(cmd *cobra.Command, _ []string) error {
	issueInput, _ := cmd.Flags().GetString("issue")
	issueInput = strings.TrimSpace(issueInput)
	if issueInput == "" {
		return fmt.Errorf("--issue is required")
	}
	previewURL, _ := cmd.Flags().GetString("url")
	previewURL = strings.TrimSpace(previewURL)
	if previewURL == "" {
		return fmt.Errorf("--url is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	issueRef, err := resolveIssueRef(ctx, client, issueInput)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}

	body := map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": previewURL,
	}
	if title, _ := cmd.Flags().GetString("title"); strings.TrimSpace(title) != "" {
		body["title"] = strings.TrimSpace(title)
	}
	if expiresAt, _ := cmd.Flags().GetString("expires-at"); strings.TrimSpace(expiresAt) != "" {
		body["expires_at"] = strings.TrimSpace(expiresAt)
	}

	var session map[string]any
	path := "/api/issues/" + url.PathEscape(issueRef.ID) + "/preview-sessions"
	if err := client.PostJSON(ctx, path, body, &session); err != nil {
		return fmt.Errorf("create preview session: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, session)
	}
	printPreviewSessions([]map[string]any{session})
	return nil
}

func runPreviewDeviceCreate(cmd *cobra.Command, _ []string) error {
	issueInput, _ := cmd.Flags().GetString("issue")
	issueInput = strings.TrimSpace(issueInput)
	if issueInput == "" {
		return fmt.Errorf("--issue is required")
	}
	previewURL, _ := cmd.Flags().GetString("url")
	previewURL = strings.TrimSpace(previewURL)
	if previewURL == "" {
		return fmt.Errorf("--url is required")
	}
	parsedPreviewURL, err := url.Parse(previewURL)
	if err != nil || strings.TrimSpace(parsedPreviewURL.Query().Get("serial")) == "" {
		return fmt.Errorf("--url must pin a device with ?serial=<device>; use 'multica preview device sync' for normal deployment")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issueRef, err := resolveIssueRef(ctx, client, issueInput)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	title, _ := cmd.Flags().GetString("title")
	body := map[string]any{
		"platform":    "android",
		"provider":    "local_device",
		"preview_url": previewURL,
		"title":       strings.TrimSpace(title),
	}
	if expiresAt, _ := cmd.Flags().GetString("expires-at"); strings.TrimSpace(expiresAt) != "" {
		body["expires_at"] = strings.TrimSpace(expiresAt)
	}
	var session map[string]any
	path := "/api/issues/" + url.PathEscape(issueRef.ID) + "/preview-sessions"
	if err := client.PostJSON(ctx, path, body, &session); err != nil {
		return fmt.Errorf("create device preview session: %w", err)
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, session)
	}
	printPreviewSessions([]map[string]any{session})
	return nil
}

func runPreviewList(cmd *cobra.Command, _ []string) error {
	issueInput, _ := cmd.Flags().GetString("issue")
	issueInput = strings.TrimSpace(issueInput)
	if issueInput == "" {
		return fmt.Errorf("--issue is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	issueRef, err := resolveIssueRef(ctx, client, issueInput)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}

	var result previewSessionListResponse
	path := "/api/issues/" + url.PathEscape(issueRef.ID) + "/preview-sessions"
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("list preview sessions: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	printPreviewSessions(result.PreviewSessions)
	return nil
}

func runPreviewStop(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("preview session id is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var session map[string]any
	path := "/api/preview-sessions/" + url.PathEscape(strings.TrimSpace(args[0])) + "/stop"
	if err := client.PostJSON(ctx, path, nil, &session); err != nil {
		return fmt.Errorf("stop preview session: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, session)
	}
	printPreviewSessions([]map[string]any{session})
	return nil
}

func printPreviewSessions(sessions []map[string]any) {
	headers := []string{"ID", "TITLE", "PLATFORM", "STATUS", "URL", "EXPIRES_AT"}
	rows := make([][]string, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, []string{
			strVal(session, "id"),
			strVal(session, "title"),
			strVal(session, "platform"),
			strVal(session, "status"),
			strVal(session, "preview_url"),
			strVal(session, "expires_at"),
		})
	}
	cli.PrintTable(os.Stdout, headers, rows)
}
