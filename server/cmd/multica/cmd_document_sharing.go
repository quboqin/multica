package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
)

var documentShareCmd = &cobra.Command{Use: "share <document>", Short: "Read or change document sharing (human owner only)", Long: `Without flags, show the current audience and permissions. --collaborator adds or updates a workspace member; --remove-collaborator revokes that direct grant. Permissions are additive: a broader edit grant still allows editing. Project sharing follows the project's current workspace audience.`, Args: exactArgs(1), RunE: runDocumentShare}
var documentVersionsCmd = &cobra.Command{Use: "versions <document>", Short: "List saved versions (requires edit access)", Args: exactArgs(1), RunE: runDocumentVersions}
var documentVersionCmd = &cobra.Command{Use: "version <document> <version>", Short: "Read a saved title and body", Args: exactArgs(2), RunE: runDocumentVersion}
var documentRestoreCmd = &cobra.Command{Use: "restore <document> <version>", Short: "Restore title and body as a new version, preserving sharing", Args: exactArgs(2), RunE: runDocumentRestore}

func init() {
	documentCmd.AddCommand(documentShareCmd, documentVersionsCmd, documentVersionCmd, documentRestoreCmd)
	documentShareCmd.Flags().String("scope", "", "private, project, or workspace")
	documentShareCmd.Flags().String("permission", "", "view or edit for the shared scope (default view)")
	documentShareCmd.Flags().String("project", "", "Project ID or prefix for project sharing")
	documentShareCmd.Flags().StringArray("collaborator", nil, "Add/update USER_UUID:view or USER_UUID:edit (repeatable)")
	documentShareCmd.Flags().StringArray("remove-collaborator", nil, "Remove a direct grant by USER_UUID (repeatable)")
	documentVersionsCmd.Flags().Int64("before", 0, "List versions older than this version")
	documentRestoreCmd.Flags().Int64("expected-revision", 0, "Current issue revision from document get (required; not document_revision)")
}
func documentResource(cmd *cobra.Command, ref string, fn func(context.Context, *cli.APIClient, map[string]any) error) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	doc, err := fetchDocument(ctx, client, ref)
	if err != nil {
		return err
	}
	return fn(ctx, client, doc)
}
func runDocumentShare(cmd *cobra.Command, args []string) error {
	return documentResource(cmd, args[0], func(ctx context.Context, client *cli.APIClient, doc map[string]any) error {
		path := "/api/documents/" + strVal(doc, "id") + "/access"
		var current struct {
			Scope         string              `json:"scope"`
			ScopeRole     string              `json:"scope_role"`
			ProjectID     *string             `json:"project_id"`
			Revision      int64               `json:"revision"`
			Collaborators []map[string]string `json:"collaborators"`
			CanManage     bool                `json:"can_manage"`
		}
		if err := client.GetJSON(ctx, path, &current); err != nil {
			return err
		}
		changed := false
		for _, flag := range []string{"scope", "permission", "project", "collaborator", "remove-collaborator"} {
			changed = changed || cmd.Flags().Changed(flag)
		}
		if !changed {
			return cli.PrintJSON(os.Stdout, current)
		}
		if !current.CanManage {
			return fmt.Errorf("only the human document owner can change sharing")
		}
		if cmd.Flags().Changed("scope") {
			current.Scope, _ = cmd.Flags().GetString("scope")
		}
		if current.Scope != "private" && current.Scope != "project" && current.Scope != "workspace" {
			return fmt.Errorf("scope must be private, project, or workspace")
		}
		if cmd.Flags().Changed("permission") {
			current.ScopeRole, _ = cmd.Flags().GetString("permission")
		}
		if current.ScopeRole != "view" && current.ScopeRole != "edit" {
			return fmt.Errorf("permission must be view or edit")
		}
		if cmd.Flags().Changed("project") {
			ref, _ := cmd.Flags().GetString("project")
			id, err := resolveProjectID(ctx, client, ref)
			if err != nil {
				return err
			}
			current.ProjectID = &id.ID
		}
		if current.Scope != "project" {
			current.ProjectID = nil
		} else if current.ProjectID == nil {
			return fmt.Errorf("--project is required for project sharing")
		}
		remove, _ := cmd.Flags().GetStringArray("remove-collaborator")
		people := []map[string]string{}
		for _, person := range current.Collaborators {
			keep := true
			for _, id := range remove {
				if person["user_id"] == id {
					keep = false
				}
			}
			if keep {
				people = append(people, person)
			}
		}
		add, _ := cmd.Flags().GetStringArray("collaborator")
		for _, input := range add {
			id, role, ok := strings.Cut(input, ":")
			if !ok || id == "" || (role != "view" && role != "edit") {
				return fmt.Errorf("collaborator must be USER_UUID:view or USER_UUID:edit")
			}
			found := false
			for _, person := range people {
				if person["user_id"] == id {
					person["role"] = role
					found = true
				}
			}
			if !found {
				people = append(people, map[string]string{"user_id": id, "role": role})
			}
		}
		body := map[string]any{"scope": current.Scope, "scope_role": current.ScopeRole, "project_id": current.ProjectID, "collaborators": people, "expected_revision": current.Revision}
		var out map[string]any
		if err := client.PutJSON(ctx, path, body, &out); err != nil {
			return err
		}
		return cli.PrintJSON(os.Stdout, out)
	})
}
func runDocumentVersions(cmd *cobra.Command, args []string) error {
	return documentResource(cmd, args[0], func(ctx context.Context, client *cli.APIClient, doc map[string]any) error {
		before, _ := cmd.Flags().GetInt64("before")
		path := "/api/documents/" + strVal(doc, "id") + "/versions"
		if before > 0 {
			path += "?before=" + strconv.FormatInt(before, 10)
		}
		var out map[string]any
		if err := client.GetJSON(ctx, path, &out); err != nil {
			return err
		}
		return cli.PrintJSON(os.Stdout, out)
	})
}
func documentVersionNumber(raw string) (int64, error) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 1 {
		return 0, fmt.Errorf("version must be a positive integer")
	}
	return v, nil
}
func runDocumentVersion(cmd *cobra.Command, args []string) error {
	v, err := documentVersionNumber(args[1])
	if err != nil {
		return err
	}
	return documentResource(cmd, args[0], func(ctx context.Context, client *cli.APIClient, doc map[string]any) error {
		var out map[string]any
		if err := client.GetJSON(ctx, fmt.Sprintf("/api/documents/%s/versions/%d", strVal(doc, "id"), v), &out); err != nil {
			return err
		}
		return cli.PrintJSON(os.Stdout, out)
	})
}
func runDocumentRestore(cmd *cobra.Command, args []string) error {
	v, err := documentVersionNumber(args[1])
	if err != nil {
		return err
	}
	revision, _ := cmd.Flags().GetInt64("expected-revision")
	if revision < 1 {
		return fmt.Errorf("--expected-revision is required: pass revision (not document_revision) from document get")
	}
	return documentResource(cmd, args[0], func(ctx context.Context, client *cli.APIClient, doc map[string]any) error {
		var out map[string]any
		if err := client.PostJSON(ctx, fmt.Sprintf("/api/documents/%s/versions/%d/restore", strVal(doc, "id"), v), map[string]any{"expected_revision": revision}, &out); err != nil {
			return err
		}
		return cli.PrintJSON(os.Stdout, out)
	})
}
