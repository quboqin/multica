package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	collectionListCmd.Flags().Bool("archived", false, "List archived tables")
	collectionFieldListCmd.Flags().Bool("archived", false, "List archived fields")
	for _, cmd := range []*cobra.Command{collectionCreateCmd, collectionUpdateCmd} {
		cmd.Flags().String("icon", "", "Table icon (emoji, up to 32 characters)")
		cmd.Flags().String("description", "", "Table description (up to 4000 characters)")
	}
	restore := &cobra.Command{Use: "restore <table>", Short: "Restore an archived table (name or ID)", Args: exactArgs(1), RunE: runCollectionRestore}
	restore.Flags().String("output", "table", "Output format: table or json")
	collectionCmd.AddCommand(restore)
	fieldRestore := &cobra.Command{Use: "restore <table> <field>", Short: "Restore an archived field and its retained values", Args: exactArgs(2), RunE: runCollectionFieldRestore}
	fieldRestore.Flags().String("output", "table", "Output format: table or json")
	collectionFieldCmd.AddCommand(fieldRestore)
	imp := &cobra.Command{Use: "import <table>", Short: "Import up to 10,000 CSV rows atomically", Long: `Import a UTF-8 CSV. Headers are title and existing field names or IDs.
Multiple values use JSON arrays; options accept names or IDs; members use
member:USER_ID. Empty cells stay unset. Relations use record link separately.
--dry-run validates the entire file without writing. Invalid data writes nothing.`, Args: exactArgs(1), RunE: runRecordImport}
	imp.Flags().String("file", "", "CSV file path (- reads stdin)")
	imp.Flags().Bool("dry-run", false, "Validate without writing")
	imp.Flags().String("output", "json", "Output format: table or json")
	recordCmd.AddCommand(imp)
	for _, action := range []string{"update", "delete", "restore"} {
		cmd := &cobra.Command{Use: "batch-" + action + " <table>", Short: "Atomically " + action + " up to 500 selected rows", Args: exactArgs(1), RunE: runRecordBatch(action)}
		cmd.Flags().StringArray("record", nil, "Row ID or exact title (repeatable)")
		cmd.Flags().StringArray("expect-revision", nil, "Expected row ID=revision (repeat for every selected row)")
		cmd.Flags().String("output", "json", "Output format: table or json")
		if action == "update" {
			cmd.Flags().StringArray("set", nil, "Field=Value (repeatable)")
			cmd.Flags().StringArray("unset", nil, "Field to clear (repeatable)")
		}
		if action == "delete" {
			cmd.Flags().Bool("yes", false, "Confirm moving all selected rows to trash")
		}
		recordCmd.AddCommand(cmd)
	}
}

func runCollectionRestore(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var list []collectionDTO
	if err = client.GetJSON(ctx, "/api/collections?archived=true", &list); err != nil {
		return err
	}
	c, err := matchCollectionRef(list, args[0])
	if err != nil {
		return err
	}
	var out collectionDTO
	if err = client.PostJSON(ctx, "/api/collections/"+c.ID+"/restore", nil, &out); err != nil {
		return collectionSchemaRequestError("restore table", err)
	}
	return printCollectionResult(cmd, out, "Table restored.")
}
func runCollectionFieldRestore(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	detail, err := loadCollection(ctx, client, args[0])
	if err != nil {
		return err
	}
	if err = client.GetJSON(ctx, "/api/collections/"+detail.Collection.ID+"/fields/archived", &detail.Fields); err != nil {
		return err
	}
	f, err := resolveCollectionFieldRef(detail, args[1])
	if err != nil {
		return err
	}
	var out collectionFieldDTO
	if err = client.PostJSON(ctx, "/api/collections/"+detail.Collection.ID+"/fields/"+f.ID+"/restore", nil, &out); err != nil {
		return collectionSchemaRequestError("restore field", err)
	}
	return printCollectionFieldResult(ctx, client, cmd, out, "Field restored.")
}
func runRecordImport(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("file")
	if path == "" {
		return fmt.Errorf("--file is required")
	}
	var reader io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		reader = f
	}
	data, err := io.ReadAll(io.LimitReader(reader, 24*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 24*1024*1024 {
		return fmt.Errorf("CSV exceeds 24 MiB")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	id, err := resolveCollectionRef(ctx, client, args[0])
	if err != nil {
		return err
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	var out map[string]any
	if err = client.PostJSON(ctx, recordsPath(id)+"/import", map[string]any{"csv": string(data), "dry_run": dry}, &out); err != nil {
		return fmt.Errorf("CSV import: %w", err)
	}
	return printCollectionBatchResult(cmd, out)
}
func runRecordBatch(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		refs, _ := cmd.Flags().GetStringArray("record")
		if len(refs) < 1 || len(refs) > 500 {
			return fmt.Errorf("pass 1 to 500 --record values")
		}
		if action == "delete" {
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return fmt.Errorf("batch-delete requires --yes to confirm moving the selected rows to trash")
			}
		}
		client, err := newAPIClient(cmd)
		if err != nil {
			return err
		}
		ctx, cancel := cli.APIContext(context.Background())
		defer cancel()
		detail, err := loadCollection(ctx, client, args[0])
		if err != nil {
			return err
		}
		ids := []string{}
		for _, ref := range refs {
			var id string
			if action == "restore" {
				id, err = resolveTrashedRecordRef(ctx, client, detail.Collection, ref)
			} else {
				id, err = resolveRecordRef(ctx, client, detail.Collection, ref)
			}
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		body := map[string]any{"action": action, "record_ids": ids, "confirmed": action == "delete"}
		expected := map[string]int64{}
		pairs, _ := cmd.Flags().GetStringArray("expect-revision")
		for _, pair := range pairs {
			id, raw, ok := strings.Cut(pair, "=")
			rev, err := strconv.ParseInt(raw, 10, 64)
			if !ok || err != nil || rev < 1 {
				return fmt.Errorf("--expect-revision requires row ID=positive revision")
			}
			expected[id] = rev
		}
		if len(expected) > 0 {
			body["expected_revisions"] = expected
		}
		if action == "update" {
			var members memberDirectory
			values := map[string]any{}
			pairs, _ := cmd.Flags().GetStringArray("set")
			for _, pair := range pairs {
				f, value, err := encodeRecordCell(ctx, client, &members, detail, "set", pair)
				if err != nil {
					return err
				}
				if _, dup := values[f.ID]; dup {
					return fmt.Errorf("duplicate field %s", f.Name)
				}
				values[f.ID] = value
			}
			unset, _ := cmd.Flags().GetStringArray("unset")
			for _, ref := range unset {
				f, err := resolveCollectionFieldRef(detail, ref)
				if err != nil {
					return err
				}
				if _, dup := values[f.ID]; dup {
					return fmt.Errorf("duplicate field %s", f.Name)
				}
				values[f.ID] = nil
			}
			if len(values) == 0 {
				return fmt.Errorf("pass --set or --unset")
			}
			body["fields"] = values
		}
		var out map[string]any
		if err = client.PostJSON(ctx, recordsPath(detail.Collection.ID)+"/batch", body, &out); err != nil {
			return fmt.Errorf("batch %s: %w", action, err)
		}
		return printCollectionBatchResult(cmd, out)
	}
}

func printCollectionBatchResult(cmd *cobra.Command, out map[string]any) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, out)
	}
	headers := []string{"COUNT"}
	row := []string{fmt.Sprint(out["count"])}
	if dry, ok := out["dry_run"]; ok {
		headers = append(headers, "DRY RUN")
		row = append(row, fmt.Sprint(dry))
	}
	cli.PrintTable(os.Stdout, headers, [][]string{row})
	return nil
}
