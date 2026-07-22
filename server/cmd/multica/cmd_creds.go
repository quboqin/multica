package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var credsCmd = &cobra.Command{
	Use:   "creds",
	Short: "Manage credential broker profiles",
}

var credsConnectorsCmd = &cobra.Command{
	Use:   "connectors",
	Short: "List credential connectors",
	RunE:  runCredsConnectors,
}

var credsBindCmd = &cobra.Command{
	Use:   "bind",
	Short: "Start a credential binding session",
	RunE:  runCredsBind,
}

var credsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List credential profiles",
	RunE:  runCredsList,
}

var credsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a credential profile",
	RunE:  runCredsStatus,
}

var credsRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke a credential profile",
	RunE:  runCredsRevoke,
}

type credentialProfileCLI struct {
	ID          string  `json:"id"`
	ConnectorID string  `json:"connector_id"`
	Label       string  `json:"label"`
	Status      string  `json:"status"`
	LastUsedAt  *string `json:"last_used_at"`
	ExpiresHint *string `json:"expires_hint"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type credentialSessionCLI struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profile_id"`
	ConnectorID string `json:"connector_id"`
	BrowserURL  string `json:"browser_url"`
	Status      string `json:"status"`
	ExpiresAt   string `json:"expires_at"`
	CreatedAt   string `json:"created_at"`
}

type credentialConnectorCLI struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	LoginURL     string   `json:"login_url"`
	Capabilities []string `json:"capabilities"`
}

func init() {
	credsCmd.AddCommand(credsConnectorsCmd)
	credsCmd.AddCommand(credsBindCmd)
	credsCmd.AddCommand(credsListCmd)
	credsCmd.AddCommand(credsStatusCmd)
	credsCmd.AddCommand(credsRevokeCmd)

	credsConnectorsCmd.Flags().String("output", "table", "Output format: table or json")

	credsBindCmd.Flags().String("connector", "", "Connector ID, e.g. appgrowing (required)")
	credsBindCmd.Flags().String("label", "", "Profile label")
	credsBindCmd.Flags().String("output", "json", "Output format: json or table")

	credsListCmd.Flags().String("output", "table", "Output format: table or json")
	credsListCmd.Flags().Bool("full-id", false, "Show full UUIDs in table output")

	credsStatusCmd.Flags().String("profile-id", "", "Credential profile ID (required)")
	credsStatusCmd.Flags().String("output", "json", "Output format: json or table")

	credsRevokeCmd.Flags().String("profile-id", "", "Credential profile ID (required)")
	credsRevokeCmd.Flags().String("output", "json", "Output format: json or table")
}

func runCredsConnectors(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	var resp struct {
		Connectors []credentialConnectorCLI `json:"connectors"`
	}
	if err := client.GetJSON(context.Background(), "/api/credential-connectors", &resp); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, resp.Connectors)
	}
	rows := make([][]string, 0, len(resp.Connectors))
	for _, c := range resp.Connectors {
		rows = append(rows, []string{c.ID, c.DisplayName, c.LoginURL, fmt.Sprintf("%v", c.Capabilities)})
	}
	cli.PrintTable(os.Stdout, []string{"ID", "NAME", "LOGIN URL", "CAPABILITIES"}, rows)
	return nil
}

func runCredsBind(cmd *cobra.Command, args []string) error {
	connector, _ := cmd.Flags().GetString("connector")
	if connector == "" {
		return fmt.Errorf("--connector is required")
	}
	label, _ := cmd.Flags().GetString("label")
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	var resp struct {
		Profile credentialProfileCLI `json:"profile"`
		Session credentialSessionCLI `json:"session"`
	}
	if err := client.PostJSON(context.Background(), "/api/credential-login-sessions", map[string]any{
		"connector_id": connector,
		"label":        label,
	}, &resp); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, resp)
	}
	cli.PrintTable(os.Stdout, []string{"PROFILE", "CONNECTOR", "STATUS", "LOGIN URL", "EXPIRES"}, [][]string{{
		resp.Profile.ID,
		resp.Profile.ConnectorID,
		resp.Profile.Status,
		resp.Session.BrowserURL,
		resp.Session.ExpiresAt,
	}})
	return nil
}

func runCredsList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	var resp struct {
		Profiles []credentialProfileCLI `json:"profiles"`
	}
	if err := client.GetJSON(context.Background(), "/api/credential-profiles", &resp); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, resp.Profiles)
	}
	fullID, _ := cmd.Flags().GetBool("full-id")
	rows := make([][]string, 0, len(resp.Profiles))
	for _, p := range resp.Profiles {
		id := p.ID
		if !fullID {
			id = truncateID(id)
		}
		lastUsed := ""
		if p.LastUsedAt != nil {
			lastUsed = *p.LastUsedAt
		}
		rows = append(rows, []string{id, p.ConnectorID, p.Label, p.Status, lastUsed, p.UpdatedAt})
	}
	cli.PrintTable(os.Stdout, []string{"ID", "CONNECTOR", "LABEL", "STATUS", "LAST USED", "UPDATED"}, rows)
	return nil
}

func runCredsStatus(cmd *cobra.Command, args []string) error {
	profileID, _ := cmd.Flags().GetString("profile-id")
	if profileID == "" {
		return fmt.Errorf("--profile-id is required")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	var profile credentialProfileCLI
	if err := client.GetJSON(context.Background(), "/api/credential-profiles/"+url.PathEscape(profileID), &profile); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, profile)
	}
	cli.PrintTable(os.Stdout, []string{"FIELD", "VALUE"}, [][]string{
		{"id", profile.ID},
		{"connector", profile.ConnectorID},
		{"label", profile.Label},
		{"status", profile.Status},
		{"created_at", profile.CreatedAt},
		{"updated_at", profile.UpdatedAt},
	})
	return nil
}

func runCredsRevoke(cmd *cobra.Command, args []string) error {
	profileID, _ := cmd.Flags().GetString("profile-id")
	if profileID == "" {
		return fmt.Errorf("--profile-id is required")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	if err := client.DeleteJSON(context.Background(), "/api/credential-profiles/"+url.PathEscape(profileID)); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, map[string]any{"id": profileID, "revoked": true})
	}
	cli.PrintTable(os.Stdout, []string{"ID", "REVOKED"}, [][]string{{profileID, "true"}})
	return nil
}
