package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// multica collection {list|get|create|update|archive} and
// multica collection field {list|add|update|archive} — tables ("collections")
// and their field catalogs. Rows live under `multica record`.
//
// Who may do what is the server's call, not this file's. Reads, row writes and
// creating a table are open to members and agents alike; reshaping a table is
// for its creator and workspace owners/admins. A run's task token authenticates
// as its runtime's owner and is held to that same rule, so an agent meets the
// refusal exactly where that person would. It carries a stable code, which
// collectionSchemaRequestError turns into a sentence the caller can act on.
//
// Like properties, everything here is addressed BY NAME where a name exists:
// tables by name, id or id prefix; fields by name or id.

type collectionDTO struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   *string `json:"project_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	TitleName   string  `json:"title_name"`
	CreatedBy   string  `json:"created_by"`
	RecordCount *int64  `json:"record_count,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type collectionRelationDTO struct {
	ToType       string `json:"to_type"`
	CollectionID string `json:"collection_id,omitempty"`
}

type collectionFieldDTO struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Position float64 `json:"position"`
	Config   struct {
		Options  []propertyOptionDTO    `json:"options,omitempty"`
		Relation *collectionRelationDTO `json:"relation,omitempty"`
	} `json:"config"`
}

// asProperty views a field as the property definition the shared value
// encoders take. Tables reuse the nine issue property types, so option and
// member names resolve through exactly the same code.
func (f collectionFieldDTO) asProperty() propertyDTO {
	p := propertyDTO{ID: f.ID, Name: f.Name, Type: f.Type}
	p.Config.Options = f.Config.Options
	return p
}

func (f collectionFieldDTO) isRelation() bool { return f.Type == collectionRelationType }

// relatesToIssues reports whether a relation field points at issues rather
// than at the records of a table.
func (f collectionFieldDTO) relatesToIssues() bool {
	return f.isRelation() && f.Config.Relation != nil && f.Config.Relation.ToType == "issue"
}

type collectionDetailDTO struct {
	Collection collectionDTO        `json:"collection"`
	Fields     []collectionFieldDTO `json:"fields"`
}

const (
	collectionRelationType = "relation"
	// collectionSchemaForbiddenCode is the server's stable code for a table
	// structure change refused because the caller is neither the table's
	// creator nor a workspace owner/admin.
	collectionSchemaForbiddenCode = "collection_schema_forbidden"
)

var collectionFieldTypes = []string{"text", "number", "select", "multi_select", "date", "checkbox", "url", "actor", "multi_actor", collectionRelationType}

var collectionCmd = &cobra.Command{
	Use:   "collection",
	Short: "Work with tables (collections) and their fields",
	Long: `Tables hold rows of your own data, separate from issues: a row has a title and
one value per field, and never an assignee, a status or a run.

Rows are under "multica record". Anyone in the workspace can create a table;
renaming or archiving one, and adding or changing its fields, is for its creator
and for workspace owners/admins. An agent's run acts for the person who owns its
runtime: it may change what that person may change.`,
}

var collectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tables in the workspace",
	Args:  exactArgs(0),
	RunE:  runCollectionList,
}

var collectionGetCmd = &cobra.Command{
	Use:   "get <table>",
	Short: "Show a table and its fields",
	Long: `Show a table and its fields: the names, types, select options and relation
targets that "multica record" commands accept. <table> is a name, an id, or an
id prefix.`,
	Args: exactArgs(1),
	RunE: runCollectionGet,
}

var collectionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a table",
	Long: `Create a table. It starts with one column, the row title; add the rest with
"multica collection field add". The table belongs to whoever created it — for an
agent's run, the person who owns its runtime — and that person manages its
structure afterwards, as do workspace owners/admins.`,
	Args: exactArgs(0),
	RunE: runCollectionCreate,
}

var collectionUpdateCmd = &cobra.Command{
	Use:   "update <table>",
	Short: "Rename a table or its first column (its creator, or a workspace owner/admin)",
	Args:  exactArgs(1),
	RunE:  runCollectionUpdate,
}

var collectionArchiveCmd = &cobra.Command{
	Use:   "archive <table>",
	Short: "Archive a table (its creator, or a workspace owner/admin)",
	Long: `Archive a table. It leaves every list and its rows can no longer be opened;
the data is kept, but nothing restores a table yet. Relations pointing at its
rows read as deleted.`,
	Args: exactArgs(1),
	RunE: runCollectionArchive,
}

var collectionFieldCmd = &cobra.Command{
	Use:   "field",
	Short: "Manage a table's fields",
}

var collectionFieldListCmd = &cobra.Command{
	Use:   "list <table>",
	Short: "List a table's fields",
	Args:  exactArgs(1),
	RunE:  runCollectionFieldList,
}

var collectionFieldAddCmd = &cobra.Command{
	Use:   "add <table>",
	Short: "Add a field (the table's creator, or a workspace owner/admin)",
	Long: `Add a field. Types: text, number, select, multi_select, date, checkbox, url,
actor, multi_actor, relation. A table holds at most 50 fields.

Select types take repeatable --option flags (the ":#rrggbb" color is optional):
  multica collection field add Customers --name Stage --type select \
      --option "Lead:#6b7280" --option "Won:#22c55e"

A relation links rows to issues, or to the rows of one table. Its target is set
once and cannot change afterwards:
  multica collection field add Requests --name Implementation --type relation --relation-to issues
  multica collection field add Requests --name Customer --type relation --relation-to Customers`,
	Args: exactArgs(1),
	RunE: runCollectionFieldAdd,
}

var collectionFieldUpdateCmd = &cobra.Command{
	Use:   "update <table> <field>",
	Short: "Rename a field, change its options or convert its type",
	Long: `Update a field.

--add-option appends options and leaves the existing ones, and every row value,
untouched. Reach for it first:
  multica collection field update Customers Stage --add-option "Churned:#ef4444"

--option flags REPLACE the full option list; options are matched by name so
their ids (and the values rows hold) survive, and an option left out is cleared
from every row, for good. Use it to reorder, recolor, rename or remove.

--type only converts along safe paths (select ↔ multi_select, actor ↔
multi_actor, number/date/url/checkbox → text). Anything else is refused: add a
new field instead. A relation's target never changes.`,
	Args: exactArgs(2),
	RunE: runCollectionFieldUpdate,
}

var collectionFieldArchiveCmd = &cobra.Command{
	Use:   "archive <table> <field>",
	Short: "Archive a field (hidden everywhere; row values are kept)",
	Args:  exactArgs(2),
	RunE:  runCollectionFieldArchive,
}

func init() {
	collectionCmd.AddCommand(collectionListCmd)
	collectionCmd.AddCommand(collectionGetCmd)
	collectionCmd.AddCommand(collectionCreateCmd)
	collectionCmd.AddCommand(collectionUpdateCmd)
	collectionCmd.AddCommand(collectionArchiveCmd)
	collectionCmd.AddCommand(collectionFieldCmd)

	collectionFieldCmd.AddCommand(collectionFieldListCmd)
	collectionFieldCmd.AddCommand(collectionFieldAddCmd)
	collectionFieldCmd.AddCommand(collectionFieldUpdateCmd)
	collectionFieldCmd.AddCommand(collectionFieldArchiveCmd)

	collectionListCmd.Flags().String("output", "table", "Output format: table or json")
	collectionListCmd.Flags().String("project", "", "Only tables of this project (id or id prefix)")
	collectionListCmd.Flags().Bool("full-id", false, "Show full UUIDs in table output")

	collectionGetCmd.Flags().String("output", "json", "Output format: table or json")

	collectionCreateCmd.Flags().String("output", "json", "Output format: table or json")
	collectionCreateCmd.Flags().String("name", "", "Table name, 1 to 80 characters (required)")
	collectionCreateCmd.Flags().String("project", "", "Project the table belongs to (id or id prefix); omit for a workspace table")

	collectionUpdateCmd.Flags().String("output", "json", "Output format: table or json")
	collectionUpdateCmd.Flags().String("name", "", "New table name")
	collectionUpdateCmd.Flags().String("title-name", "", "New label for the first column, which holds each row's title; pass an empty value for the default")

	collectionArchiveCmd.Flags().String("output", "table", "Output format: table or json")

	collectionFieldListCmd.Flags().String("output", "table", "Output format: table or json")

	collectionFieldAddCmd.Flags().String("output", "table", "Output format: table or json")
	collectionFieldAddCmd.Flags().String("name", "", "Field name (required)")
	collectionFieldAddCmd.Flags().String("type", "", "Field type: "+strings.Join(collectionFieldTypes, ", ")+" (required)")
	collectionFieldAddCmd.Flags().StringArray("option", nil, `Select option as "Name" or "Name:#rrggbb" (repeatable; select types only)`)
	collectionFieldAddCmd.Flags().String("relation-to", "", `Relation target: "issues", or a table name or id (relation type only)`)

	collectionFieldUpdateCmd.Flags().String("output", "table", "Output format: table or json")
	collectionFieldUpdateCmd.Flags().String("name", "", "New field name")
	collectionFieldUpdateCmd.Flags().String("type", "", "Convert to this type (safe conversions only)")
	collectionFieldUpdateCmd.Flags().StringArray("add-option", nil, `Option to append as "Name" or "Name:#rrggbb" (repeatable); existing options and row values are kept`)
	collectionFieldUpdateCmd.Flags().StringArray("option", nil, `Replacement option list as "Name" or "Name:#rrggbb" (repeatable); an option left out is cleared from every row`)
	collectionFieldUpdateCmd.Flags().Float64("position", 0, "New position among the table's fields")

	collectionFieldArchiveCmd.Flags().String("output", "table", "Output format: table or json")
}

// collectionSchemaRequestError turns a refused structure change into what the
// caller should read.
//
// FormatError collapses every 403 into generic "no access" copy so a refusal
// cannot confirm that a resource exists. That hides the one refusal here a
// caller can act on: they CAN see the table and write its rows, and only its
// structure is closed to them. The branch is on the server's stable code, never
// on the English sentence.
func collectionSchemaRequestError(action string, err error) error {
	if cli.ServerErrorCode(err) == collectionSchemaForbiddenCode {
		return cli.WithUserMessage("only a table's creator or a workspace owner/admin can change its structure, and you are neither here — an agent's run counts as the person who owns its runtime. The rows stay open to you through `multica record`; for the structure, ask one of those people, or create a table of your own.", err)
	}
	return fmt.Errorf("%s: %w", action, err)
}

// appendedFieldOptions is the option list for --add-option: every existing
// option as it is, then the new ones. The API only takes whole lists, and an
// option missing from one is cleared from every row, so the existing options
// are carried over by id rather than left to the caller to retype.
func appendedFieldOptions(field collectionFieldDTO, flags []string) ([]map[string]string, error) {
	if field.Type != "select" && field.Type != "multi_select" {
		return nil, fmt.Errorf("--add-option only applies to select and multi_select fields; %q is %s", field.Name, field.Type)
	}
	options := make([]map[string]string, 0, len(field.Config.Options)+len(flags))
	taken := make(map[string]bool, len(field.Config.Options)+len(flags))
	for _, opt := range field.Config.Options {
		color := opt.Color
		if color == "" {
			color = defaultOptionColor
		}
		options = append(options, map[string]string{"id": opt.ID, "name": opt.Name, "color": color})
		taken[strings.ToLower(opt.Name)] = true
	}
	for _, added := range parseOptionFlags(flags, nil) {
		key := strings.ToLower(added["name"])
		if key == "" {
			return nil, fmt.Errorf("--add-option needs a name")
		}
		if taken[key] {
			return nil, fmt.Errorf("field %q already has an option named %q; nothing was changed", field.Name, added["name"])
		}
		taken[key] = true
		options = append(options, added)
	}
	return options, nil
}

func fetchCollections(ctx context.Context, client *cli.APIClient) ([]collectionDTO, error) {
	var collections []collectionDTO
	if err := client.GetJSON(ctx, "/api/collections", &collections); err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	return collections, nil
}

func fetchCollectionDetail(ctx context.Context, client *cli.APIClient, id string) (collectionDetailDTO, error) {
	var detail collectionDetailDTO
	if err := client.GetJSON(ctx, "/api/collections/"+id, &detail); err != nil {
		return collectionDetailDTO{}, fmt.Errorf("get table: %w", err)
	}
	return detail, nil
}

// matchCollectionRef picks a table by id, then exact name, then id prefix.
// Names come before prefixes so a table called "cafe" is never shadowed by an
// id that happens to start with those four hex characters.
func matchCollectionRef(collections []collectionDTO, ref string) (collectionDTO, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return collectionDTO{}, fmt.Errorf("a table name or id is required")
	}
	for _, c := range collections {
		if strings.EqualFold(c.ID, trimmed) {
			return c, nil
		}
	}
	var named []collectionDTO
	for _, c := range collections {
		if strings.EqualFold(c.Name, trimmed) {
			named = append(named, c)
		}
	}
	if len(named) == 1 {
		return named[0], nil
	}
	if len(named) > 1 {
		return collectionDTO{}, ambiguousCollectionError(trimmed, named)
	}
	if prefix, err := normalizeUUIDPrefix(trimmed); err == nil {
		var prefixed []collectionDTO
		for _, c := range collections {
			if strings.HasPrefix(compactUUID(c.ID), prefix) {
				prefixed = append(prefixed, c)
			}
		}
		if len(prefixed) == 1 {
			return prefixed[0], nil
		}
		if len(prefixed) > 1 {
			return collectionDTO{}, ambiguousCollectionError(trimmed, prefixed)
		}
	}
	names := make([]string, len(collections))
	for i, c := range collections {
		names[i] = c.Name
	}
	if len(names) == 0 {
		return collectionDTO{}, fmt.Errorf("table %q not found; this workspace has no tables", trimmed)
	}
	return collectionDTO{}, fmt.Errorf("table %q not found; available: %s", trimmed, strings.Join(names, ", "))
}

func ambiguousCollectionError(ref string, matches []collectionDTO) error {
	lines := make([]string, len(matches))
	for i, c := range matches {
		lines[i] = fmt.Sprintf("  %s  %s", c.ID, c.Name)
	}
	return fmt.Errorf("%q matches %d tables; pass the id instead:\n%s", ref, len(matches), strings.Join(lines, "\n"))
}

// resolveCollectionRef turns a CLI table reference into an id. A full UUID is
// self-identifying and costs no request.
func resolveCollectionRef(ctx context.Context, client *cli.APIClient, ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if uuidRegexp.MatchString(trimmed) {
		return strings.ToLower(trimmed), nil
	}
	collections, err := fetchCollections(ctx, client)
	if err != nil {
		return "", err
	}
	collection, err := matchCollectionRef(collections, trimmed)
	if err != nil {
		return "", err
	}
	return collection.ID, nil
}

// loadCollection resolves a table reference and reads the table with its
// fields — what nearly every record command needs before it can do anything.
func loadCollection(ctx context.Context, client *cli.APIClient, ref string) (collectionDetailDTO, error) {
	id, err := resolveCollectionRef(ctx, client, ref)
	if err != nil {
		return collectionDetailDTO{}, err
	}
	return fetchCollectionDetail(ctx, client, id)
}

// resolveCollectionFieldRef matches a field by id first, then by
// case-insensitive name. Names are unique within a table.
func resolveCollectionFieldRef(detail collectionDetailDTO, ref string) (collectionFieldDTO, error) {
	trimmed := strings.TrimSpace(ref)
	for _, f := range detail.Fields {
		if strings.EqualFold(f.ID, trimmed) {
			return f, nil
		}
	}
	for _, f := range detail.Fields {
		if strings.EqualFold(f.Name, trimmed) {
			return f, nil
		}
	}
	names := make([]string, len(detail.Fields))
	for i, f := range detail.Fields {
		names[i] = f.Name
	}
	if len(names) == 0 {
		return collectionFieldDTO{}, fmt.Errorf("field %q not found; table %q has no fields yet", trimmed, detail.Collection.Name)
	}
	return collectionFieldDTO{}, fmt.Errorf("field %q not found on table %q; available: %s", trimmed, detail.Collection.Name, strings.Join(names, ", "))
}

// collectionFieldTarget describes what a relation field points at, for people.
// names maps table ids to names; an id it does not hold prints as the id.
func collectionFieldTarget(field collectionFieldDTO, names map[string]string) string {
	if !field.isRelation() || field.Config.Relation == nil {
		return ""
	}
	if field.relatesToIssues() {
		return "issues"
	}
	if name, ok := names[field.Config.Relation.CollectionID]; ok {
		return "table " + name
	}
	return "table " + field.Config.Relation.CollectionID
}

func printCollectionFieldTable(fields []collectionFieldDTO, names map[string]string) {
	headers := []string{"NAME", "TYPE", "OPTIONS / TARGET", "ID"}
	rows := make([][]string, 0, len(fields))
	for _, f := range fields {
		detail := collectionFieldTarget(f, names)
		if detail == "" {
			options := make([]string, len(f.Config.Options))
			for i, opt := range f.Config.Options {
				options[i] = opt.Name
			}
			detail = strings.Join(options, ", ")
		}
		rows = append(rows, []string{f.Name, f.Type, detail, f.ID})
	}
	cli.PrintTable(os.Stdout, headers, rows)
}

// collectionNames reads the workspace's table names so a relation target can
// print as a name. It is display only: on failure targets print as ids.
func collectionNames(ctx context.Context, client *cli.APIClient, fields []collectionFieldDTO) map[string]string {
	needed := false
	for _, f := range fields {
		if f.isRelation() && !f.relatesToIssues() {
			needed = true
		}
	}
	if !needed {
		return nil
	}
	collections, err := fetchCollections(ctx, client)
	if err != nil {
		return nil
	}
	names := make(map[string]string, len(collections))
	for _, c := range collections {
		names[c.ID] = c.Name
	}
	return names
}

func runCollectionList(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var collections []collectionDTO
	archived, _ := cmd.Flags().GetBool("archived")
	path := "/api/collections"
	if archived {
		path += "?archived=true"
	}
	err = client.GetJSON(ctx, path, &collections)
	if err != nil {
		return err
	}
	if ref, _ := cmd.Flags().GetString("project"); ref != "" {
		project, err := resolveProjectID(ctx, client, ref)
		if err != nil {
			return fmt.Errorf("resolve project: %w", err)
		}
		kept := collections[:0]
		for _, c := range collections {
			if c.ProjectID != nil && strings.EqualFold(*c.ProjectID, project.ID) {
				kept = append(kept, c)
			}
		}
		collections = kept
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		if collections == nil {
			collections = []collectionDTO{}
		}
		return cli.PrintJSON(os.Stdout, collections)
	}
	fullID, _ := cmd.Flags().GetBool("full-id")
	headers := []string{"ID", "NAME", "ROWS", "SCOPE", "UPDATED"}
	rows := make([][]string, 0, len(collections))
	for _, c := range collections {
		count := ""
		if c.RecordCount != nil {
			count = strconv.FormatInt(*c.RecordCount, 10)
		}
		scope := "workspace"
		if c.ProjectID != nil && *c.ProjectID != "" {
			scope = "project " + displayID(*c.ProjectID, fullID)
		}
		updated := c.UpdatedAt
		if len(updated) >= 10 {
			updated = updated[:10]
		}
		rows = append(rows, []string{displayID(c.ID, fullID), c.Name, count, scope, updated})
	}
	cli.PrintTable(os.Stdout, headers, rows)
	return nil
}

func runCollectionGet(cmd *cobra.Command, args []string) error {
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
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		if detail.Fields == nil {
			detail.Fields = []collectionFieldDTO{}
		}
		return cli.PrintJSON(os.Stdout, detail)
	}
	fmt.Fprintf(os.Stdout, "%s  %s\n\n", detail.Collection.ID, detail.Collection.Name)
	printCollectionFieldTable(detail.Fields, collectionNames(ctx, client, detail.Fields))
	return nil
}

func printCollectionResult(cmd *cobra.Command, collection collectionDTO, message string) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, collection)
	}
	fmt.Fprintln(os.Stdout, message)
	cli.PrintTable(os.Stdout, []string{"ID", "NAME"}, [][]string{{collection.ID, collection.Name}})
	return nil
}

func runCollectionCreate(cmd *cobra.Command, _ []string) error {
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

	body := map[string]any{"name": name}
	for _, key := range []string{"icon", "description"} {
		if cmd.Flags().Changed(key) {
			value, _ := cmd.Flags().GetString(key)
			body[key] = value
		}
	}
	if ref, _ := cmd.Flags().GetString("project"); ref != "" {
		project, err := resolveProjectID(ctx, client, ref)
		if err != nil {
			return fmt.Errorf("resolve project: %w", err)
		}
		body["project_id"] = project.ID
	}
	var created collectionDTO
	if err := client.PostJSON(ctx, "/api/collections", body, &created); err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	return printCollectionResult(cmd, created, fmt.Sprintf("Table %q created.", created.Name))
}

func runCollectionUpdate(cmd *cobra.Command, args []string) error {
	body := map[string]any{}
	for _, key := range []string{"icon", "description"} {
		if cmd.Flags().Changed(key) {
			value, _ := cmd.Flags().GetString(key)
			body[key] = value
		}
	}
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		body["name"] = name
	}
	if cmd.Flags().Changed("title-name") {
		titleName, _ := cmd.Flags().GetString("title-name")
		body["title_name"] = titleName
	}
	if len(body) == 0 {
		return fmt.Errorf("nothing to update; pass --name, --title-name, --icon or --description")
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
	var updated collectionDTO
	if err := client.PatchJSON(ctx, "/api/collections/"+id, body, &updated); err != nil {
		return collectionSchemaRequestError("update table", err)
	}
	return printCollectionResult(cmd, updated, fmt.Sprintf("Table %q updated.", updated.Name))
}

func runCollectionArchive(cmd *cobra.Command, args []string) error {
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
	var archived collectionDTO
	if err := client.PatchJSON(ctx, "/api/collections/"+id, map[string]any{"archived": true}, &archived); err != nil {
		return collectionSchemaRequestError("archive table", err)
	}
	return printCollectionResult(cmd, archived, fmt.Sprintf("Table %q archived.", archived.Name))
}

func runCollectionFieldList(cmd *cobra.Command, args []string) error {
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
	if archived, _ := cmd.Flags().GetBool("archived"); archived {
		if err = client.GetJSON(ctx, "/api/collections/"+detail.Collection.ID+"/fields/archived", &detail.Fields); err != nil {
			return err
		}
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		if detail.Fields == nil {
			detail.Fields = []collectionFieldDTO{}
		}
		return cli.PrintJSON(os.Stdout, detail.Fields)
	}
	printCollectionFieldTable(detail.Fields, collectionNames(ctx, client, detail.Fields))
	return nil
}

func printCollectionFieldResult(ctx context.Context, client *cli.APIClient, cmd *cobra.Command, field collectionFieldDTO, message string) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, field)
	}
	fmt.Fprintln(os.Stdout, message)
	fields := []collectionFieldDTO{field}
	printCollectionFieldTable(fields, collectionNames(ctx, client, fields))
	return nil
}

func runCollectionFieldAdd(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	fieldType, _ := cmd.Flags().GetString("type")
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	if fieldType == "" {
		return fmt.Errorf("--type is required; one of %s", strings.Join(collectionFieldTypes, ", "))
	}
	optionFlags, _ := cmd.Flags().GetStringArray("option")
	relationTo, _ := cmd.Flags().GetString("relation-to")
	if fieldType == collectionRelationType && relationTo == "" {
		return fmt.Errorf(`--relation-to is required for a relation field: "issues", or a table name or id`)
	}
	if fieldType != collectionRelationType && relationTo != "" {
		return fmt.Errorf("--relation-to only applies to --type relation")
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
	body := map[string]any{"name": name, "type": fieldType}
	switch {
	case fieldType == collectionRelationType:
		relation := map[string]any{"to_type": "issue"}
		if !strings.EqualFold(relationTo, "issues") && !strings.EqualFold(relationTo, "issue") {
			target, err := resolveCollectionRef(ctx, client, relationTo)
			if err != nil {
				return fmt.Errorf("resolve --relation-to: %w", err)
			}
			relation = map[string]any{"to_type": "record", "collection_id": target}
		}
		body["config"] = map[string]any{"relation": relation}
	case len(optionFlags) > 0:
		body["config"] = map[string]any{"options": parseOptionFlags(optionFlags, nil)}
	}

	var created collectionFieldDTO
	if err := client.PostJSON(ctx, "/api/collections/"+id+"/fields", body, &created); err != nil {
		return collectionSchemaRequestError("add field", err)
	}
	return printCollectionFieldResult(ctx, client, cmd, created, fmt.Sprintf("Field %q added.", created.Name))
}

func runCollectionFieldUpdate(cmd *cobra.Command, args []string) error {
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
	field, err := resolveCollectionFieldRef(detail, args[1])
	if err != nil {
		return err
	}
	body := map[string]any{}
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		body["name"] = name
	}
	if cmd.Flags().Changed("type") {
		fieldType, _ := cmd.Flags().GetString("type")
		body["type"] = fieldType
	}
	if cmd.Flags().Changed("option") && cmd.Flags().Changed("add-option") {
		return fmt.Errorf("--option replaces the whole option list and --add-option extends it; pass one of them")
	}
	if cmd.Flags().Changed("option") {
		optionFlags, _ := cmd.Flags().GetStringArray("option")
		body["config"] = map[string]any{"options": parseOptionFlags(optionFlags, field.Config.Options)}
	}
	if cmd.Flags().Changed("add-option") {
		added, _ := cmd.Flags().GetStringArray("add-option")
		options, err := appendedFieldOptions(field, added)
		if err != nil {
			return err
		}
		body["config"] = map[string]any{"options": options}
	}
	if cmd.Flags().Changed("position") {
		position, _ := cmd.Flags().GetFloat64("position")
		body["position"] = position
	}
	if len(body) == 0 {
		return fmt.Errorf("nothing to update; pass --name, --type, --add-option, --option, or --position")
	}
	var updated collectionFieldDTO
	path := "/api/collections/" + detail.Collection.ID + "/fields/" + field.ID
	if err := client.PatchJSON(ctx, path, body, &updated); err != nil {
		return collectionSchemaRequestError("update field", err)
	}
	return printCollectionFieldResult(ctx, client, cmd, updated, fmt.Sprintf("Field %q updated.", updated.Name))
}

func runCollectionFieldArchive(cmd *cobra.Command, args []string) error {
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
	field, err := resolveCollectionFieldRef(detail, args[1])
	if err != nil {
		return err
	}
	var archived collectionFieldDTO
	path := "/api/collections/" + detail.Collection.ID + "/fields/" + field.ID
	if err := client.PatchJSON(ctx, path, map[string]any{"archived": true}, &archived); err != nil {
		return collectionSchemaRequestError("archive field", err)
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, archived)
	}
	fmt.Fprintf(os.Stdout, "Field %q archived.\n", archived.Name)
	return nil
}
