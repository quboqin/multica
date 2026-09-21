package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// multica record {list|get|create|update|delete|restore|trash|link|unlink|backlinks}
// — the rows of a table — and multica issue records, the reverse lookup from an
// issue. See cmd_collection.go for tables and fields.
//
// A row is a title plus one value per field. Two things about the API shape the
// commands here:
//
//   - Every cell is written on its own, atomically. `record update --set` with
//     three fields is three writes, and a failure in the middle leaves the first
//     ones in place; the command says which landed.
//   - A relation cell is not a value. Its links are separate edges, written by
//     `record link` / `record unlink`, and the value path refuses them.
//
// Fields, options and members are addressed by name; rows by id or exact title.

type recordLinkDTO struct {
	ID           string `json:"id"`
	ToType       string `json:"to_type"`
	ToID         string `json:"to_id"`
	Title        string `json:"title"`
	Identifier   string `json:"identifier,omitempty"`
	Status       string `json:"status,omitempty"`
	CollectionID string `json:"collection_id,omitempty"`
	Missing      bool   `json:"missing"`
}

type recordDTO struct {
	ID           string                     `json:"id"`
	CollectionID string                     `json:"collection_id"`
	Title        string                     `json:"title"`
	Fields       map[string]any             `json:"fields"`
	Links        map[string][]recordLinkDTO `json:"links"`
	Revision     int64                      `json:"revision"`
	CreatedAt    string                     `json:"created_at"`
	UpdatedAt    string                     `json:"updated_at"`
	DeletedAt    string                     `json:"deleted_at,omitempty"`
}

type recordPageDTO struct {
	Records    []recordDTO `json:"records"`
	Total      int64       `json:"total"`
	NextCursor *string     `json:"next_cursor"`
}

// recordFieldRow is one filled cell of a row in --detail output, in the same
// shape `issue property list` prints: the stored value beside what it means. A
// relation cell is a row too — its value is the list of links, its display
// their labels.
type recordFieldRow struct {
	FieldID       string   `json:"field_id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Value         any      `json:"value"`
	Display       string   `json:"display"`
	DisplayValues []string `json:"display_values,omitempty"`
}

// recordOutput is how a row prints as JSON. By default the cells come as
// "values", keyed by field name and spelled the way `record update --set` takes
// them back — option and member names rather than ids — because that is all a
// caller needs to read a table and write to it, at a fraction of the size. With
// --detail they come as "fields" instead: ids, types and stored values, which
// is what identifies one link among several or an option that was renamed.
type recordOutput struct {
	ID           string `json:"id"`
	CollectionID string `json:"collection_id"`
	Title        string `json:"title"`
	Revision     int64  `json:"revision"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	DeletedAt    string `json:"deleted_at,omitempty"`
	// Exactly one of the two is set. They are pointers so the chosen one still
	// prints when empty — a row with no cells is "values": {}, not a missing key.
	Values *map[string]any   `json:"values,omitempty"`
	Fields *[]recordFieldRow `json:"fields,omitempty"`

	// rows backs the table renderers, which always need the display strings.
	rows []recordFieldRow
}

// recordListOutput is one page of `record list`. next_cursor is null on the
// last page and otherwise goes back in through --cursor.
type recordListOutput struct {
	Records    []recordOutput `json:"records"`
	Total      int64          `json:"total"`
	NextCursor *string        `json:"next_cursor"`
}

type recordBacklinkDTO struct {
	ID             string `json:"id"`
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name"`
	RecordID       string `json:"record_id"`
	RecordTitle    string `json:"record_title"`
	FieldID        string `json:"field_id"`
	FieldName      string `json:"field_name"`
}

const recordDetailHelp = `JSON output only: print cells as "fields" rows with field ids, types, stored values and link ids, instead of the default "values" keyed by field name`

const (
	recordListDefaultLimit = 50
	recordListMaxLimit     = 100
	// recordTableCellWidth keeps one long text value from pushing every other
	// column off the terminal. JSON output is never truncated.
	recordTableCellWidth = 32
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Work with the rows (records) of a table",
	Long: `A row has a title and one value per field of its table. It has no status, no
assignee and no comments, and writing one never starts a run.

Run "multica collection get <table>" first: it lists the field names, types and
select options these commands take.`,
}

var recordListCmd = &cobra.Command{
	Use:   "list <table>",
	Short: "List the rows of a table",
	Long: `List rows, newest first unless --sort is given. At most 100 rows come back per
call; when more match, the JSON output carries next_cursor — pass it to --cursor
with the SAME filters to read on.

--filter takes "Field=Value" and is repeatable: the same field twice matches
either value, different fields must all match, and "Field=__none__" matches
rows where the field is empty. Values are written the way "record update --set"
takes them. Relation fields cannot be filtered or sorted yet.`,
	Args: exactArgs(1),
	RunE: runRecordList,
}

var recordGetCmd = &cobra.Command{
	Use:   "get <table> <row>",
	Short: "Show one row",
	Args:  exactArgs(2),
	RunE:  runRecordGet,
}

var recordCreateCmd = &cobra.Command{
	Use:   "create <table>",
	Short: "Add a row",
	Long: `Add a row. --set takes "Field=Value" and is repeatable; value forms by type:
  select        --set "Stage=Won"               (option name or id)
  multi_select  --set "Tags=iOS,Android"        (comma-separated option names or ids)
  actor         --set "Owner=Bohan"             (member name, email, or id)
  multi_actor   --set "Reviewers=Bohan,Jiayuan"
  checkbox      --set "Signed=true"
  number        --set "Seats=25"
  date          --set "Renewal=2026-10-01"
  text / url    --set "Notes=any string"

A relation field is not set here: create the row, then "multica record link".`,
	Args: exactArgs(1),
	RunE: runRecordCreate,
}

var recordUpdateCmd = &cobra.Command{
	Use:   "update <table> <row>",
	Short: "Change a row's title or cells",
	Long: `Change a row. Each --set / --unset is its own atomic write, applied in order;
if one fails, the ones before it stay written and the error names them.

--expect "Field=Value" makes the write to that field conditional: it only
lands while the cell still holds that value ("Field=" means still empty).
Without it the write is unconditional. A refusal means someone else changed the
cell — read the row again before deciding what to write.`,
	Args: exactArgs(2),
	RunE: runRecordUpdate,
}

var recordDeleteCmd = &cobra.Command{
	Use:   "delete <table> <row>",
	Short: "Move a row to the table's trash (restorable for 30 days)",
	Args:  exactArgs(2),
	RunE:  makeRecordTrashRun(true),
}

var recordRestoreCmd = &cobra.Command{
	Use:   "restore <table> <row>",
	Short: "Bring a row back from the trash",
	Args:  exactArgs(2),
	RunE:  makeRecordTrashRun(false),
}

var recordTrashCmd = &cobra.Command{
	Use:   "trash <table>",
	Short: "List the rows in a table's trash",
	Args:  exactArgs(1),
	RunE:  runRecordTrash,
}

var recordLinkCmd = &cobra.Command{
	Use:   "link <table> <row>",
	Short: "Link a row to an issue or to another row through a relation field",
	Long: `Add links to a relation cell. What --to accepts follows the field: an issue key
or id when the field points at issues, a row id or exact title when it points at
a table. Linking twice is a no-op, and a cell holds at most 50 links.

  multica record link Requests "Embed live views" --field Implementation --to MUL-31 --to MUL-60`,
	Args: exactArgs(2),
	RunE: runRecordLink,
}

var recordUnlinkCmd = &cobra.Command{
	Use:   "unlink <table> <row>",
	Short: "Remove links from a relation cell",
	Long: `Remove links. --to takes what "record link" takes, or a link id — which is how
a link whose target was deleted is removed: "record get" prints it as
{"deleted": true, "link_id": "..."}.`,
	Args: exactArgs(2),
	RunE: runRecordUnlink,
}

var recordBacklinksCmd = &cobra.Command{
	Use:   "backlinks <table> <row>",
	Short: "List the rows that link to this row",
	Args:  exactArgs(2),
	RunE:  runRecordBacklinks,
}

var issueRecordsCmd = &cobra.Command{
	Use:   "records <issue-id>",
	Short: "List the table rows that link to an issue",
	Args:  exactArgs(1),
	RunE:  runIssueRecords,
}

func init() {
	recordCmd.AddCommand(recordListCmd)
	recordCmd.AddCommand(recordGetCmd)
	recordCmd.AddCommand(recordCreateCmd)
	recordCmd.AddCommand(recordUpdateCmd)
	recordCmd.AddCommand(recordDeleteCmd)
	recordCmd.AddCommand(recordRestoreCmd)
	recordCmd.AddCommand(recordTrashCmd)
	recordCmd.AddCommand(recordLinkCmd)
	recordCmd.AddCommand(recordUnlinkCmd)
	recordCmd.AddCommand(recordBacklinksCmd)
	issueCmd.AddCommand(issueRecordsCmd)

	recordListCmd.Flags().String("output", "table", "Output format: table or json")
	recordListCmd.Flags().String("search", "", "Only rows whose title contains this text")
	recordListCmd.Flags().StringArray("filter", nil, `Only rows where "Field=Value" holds (repeatable; "Field=__none__" matches an empty cell)`)
	recordListCmd.Flags().String("sort", "", `Sort by "title", "created_at", or a field name or id`)
	recordListCmd.Flags().Bool("desc", false, "Sort descending (with --sort)")
	recordListCmd.Flags().Int("limit", recordListDefaultLimit, "Rows per call, 1 to 100")
	recordListCmd.Flags().String("cursor", "", "Continue from a previous call's next_cursor (same filters and sort)")

	for _, cmd := range []*cobra.Command{
		recordGetCmd, recordCreateCmd, recordUpdateCmd, recordDeleteCmd, recordRestoreCmd, recordLinkCmd, recordUnlinkCmd,
	} {
		cmd.Flags().String("output", "json", "Output format: table or json")
	}
	recordTrashCmd.Flags().String("output", "table", "Output format: table or json")
	for _, cmd := range []*cobra.Command{
		recordListCmd, recordGetCmd, recordCreateCmd, recordUpdateCmd, recordDeleteCmd, recordRestoreCmd, recordTrashCmd, recordLinkCmd, recordUnlinkCmd,
	} {
		cmd.Flags().Bool("detail", false, recordDetailHelp)
	}
	recordBacklinksCmd.Flags().String("output", "table", "Output format: table or json")
	issueRecordsCmd.Flags().String("output", "table", "Output format: table or json")

	recordCreateCmd.Flags().String("title", "", "Row title (the table's first column)")
	recordCreateCmd.Flags().StringArray("set", nil, `Cell value as "Field=Value" (repeatable; see --help for per-type forms)`)

	recordUpdateCmd.Flags().String("title", "", "New row title")
	recordUpdateCmd.Flags().StringArray("set", nil, `Cell value as "Field=Value" (repeatable; see "record create --help" for per-type forms)`)
	recordUpdateCmd.Flags().StringArray("unset", nil, "Field to clear (repeatable)")
	recordUpdateCmd.Flags().StringArray("expect", nil, `Only write a field while it still holds "Field=Value" (repeatable; "Field=" means still empty)`)

	recordLinkCmd.Flags().String("field", "", "Relation field name or id (required)")
	recordLinkCmd.Flags().StringArray("to", nil, "Link target: an issue key or id, or a row id or exact title (repeatable; required)")
	recordUnlinkCmd.Flags().String("field", "", "Relation field name or id (required)")
	recordUnlinkCmd.Flags().StringArray("to", nil, "Link to remove: what `record link` takes, or a link id or target id (repeatable; required)")
}

// ---------------------------------------------------------------------------
// Reading rows
// ---------------------------------------------------------------------------

func recordsPath(collectionID string) string {
	return "/api/collections/" + collectionID + "/records"
}

// fetchRecord reads one live row. Rows have no GET of their own: the list
// endpoint narrows to one id, which also keeps it inside the table given.
func fetchRecord(ctx context.Context, client *cli.APIClient, collectionID, recordID string) (recordDTO, error) {
	var page recordPageDTO
	params := url.Values{"record_id": {recordID}, "limit": {"1"}}
	if err := client.GetJSON(ctx, recordsPath(collectionID)+"?"+params.Encode(), &page); err != nil {
		return recordDTO{}, fmt.Errorf("get row: %w", err)
	}
	if len(page.Records) == 0 {
		return recordDTO{}, fmt.Errorf("row %s not found in this table; it may be in the trash — see `multica record trash`", recordID)
	}
	return page.Records[0], nil
}

// resolveRecordRef turns a row reference into an id: a UUID as-is, anything
// else as an exact (case-insensitive) title. Titles are not unique, so two rows
// sharing one is an error that lists both rather than a guess.
func resolveRecordRef(ctx context.Context, client *cli.APIClient, collection collectionDTO, ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", fmt.Errorf("a row id or title is required")
	}
	if uuidRegexp.MatchString(trimmed) {
		return strings.ToLower(trimmed), nil
	}
	var page recordPageDTO
	params := url.Values{"search": {trimmed}, "limit": {strconv.Itoa(recordListMaxLimit)}}
	if err := client.GetJSON(ctx, recordsPath(collection.ID)+"?"+params.Encode(), &page); err != nil {
		return "", fmt.Errorf("find row: %w", err)
	}
	return matchRecordTitle(page.Records, collection.Name, trimmed, page.NextCursor != nil)
}

func matchRecordTitle(records []recordDTO, table, title string, truncated bool) (string, error) {
	var matches []recordDTO
	for _, r := range records {
		if strings.EqualFold(strings.TrimSpace(r.Title), title) {
			matches = append(matches, r)
		}
	}
	switch {
	case len(matches) == 1 && !truncated:
		return matches[0].ID, nil
	case len(matches) == 0 && !truncated:
		return "", fmt.Errorf("no row titled %q in table %q; pass the row id from `multica record list`", title, table)
	case len(matches) <= 1:
		// More rows contain this text than one page holds, so an unseen page
		// could hold another exact match. Refuse rather than pick.
		return "", fmt.Errorf("too many rows contain %q to match a title safely; pass the row id from `multica record list --search`", title)
	}
	lines := make([]string, len(matches))
	for i, r := range matches {
		lines[i] = fmt.Sprintf("  %s  %s", r.ID, r.Title)
	}
	return "", fmt.Errorf("%d rows in table %q are titled %q; pass the id instead:\n%s", len(matches), table, title, strings.Join(lines, "\n"))
}

func recordLinkLabel(link recordLinkDTO) string {
	switch {
	case link.Missing:
		return "(deleted)"
	case link.ToType == "issue":
		return strings.TrimSpace(link.Identifier + " " + link.Title)
	case link.Title == "":
		return "(untitled)"
	default:
		return link.Title
	}
}

// buildRecordRows lists a row's filled cells in the table's field order. A key
// in the value bag with no field behind it belongs to an archived field, which
// the table no longer shows; it is left out here the same way.
func buildRecordRows(fields []collectionFieldDTO, record recordDTO, actorNames map[string]string) []recordFieldRow {
	rows := make([]recordFieldRow, 0, len(fields))
	for _, field := range fields {
		if field.isRelation() {
			links := record.Links[field.ID]
			if len(links) == 0 {
				continue
			}
			labels := make([]string, len(links))
			for i, link := range links {
				labels[i] = recordLinkLabel(link)
			}
			rows = append(rows, recordFieldRow{
				FieldID: field.ID, Name: field.Name, Type: field.Type,
				Value: links, Display: strings.Join(labels, ", "), DisplayValues: labels,
			})
			continue
		}
		value, present := record.Fields[field.ID]
		if !present || value == nil {
			continue
		}
		property := field.asProperty()
		rows = append(rows, recordFieldRow{
			FieldID: field.ID, Name: field.Name, Type: field.Type, Value: value,
			Display:       formatIssuePropertyValue(property, value, actorNames),
			DisplayValues: issuePropertyDisplayValues(property, value, actorNames),
		})
	}
	return rows
}

// recordCellValue spells a cell the way the write commands take it back: names
// in place of ids, and the stored scalar for everything that has no id to hide.
func recordCellValue(row recordFieldRow) any {
	switch row.Type {
	case "select", "actor":
		return row.Display
	case "multi_select", "multi_actor":
		return row.DisplayValues
	case collectionRelationType:
		links, _ := row.Value.([]recordLinkDTO)
		targets := make([]map[string]any, len(links))
		for i, link := range links {
			targets[i] = recordLinkTarget(link)
		}
		return targets
	default:
		return row.Value
	}
}

// recordLinkTarget names a link by what `record link --to` / `record unlink
// --to` accept for it: an issue key, a row id, or — once the target is gone and
// nothing else can name it — the link's own id.
func recordLinkTarget(link recordLinkDTO) map[string]any {
	switch {
	case link.Missing:
		return map[string]any{"deleted": true, "link_id": link.ID}
	case link.ToType == "issue":
		return map[string]any{"issue": link.Identifier, "title": link.Title, "status": link.Status}
	default:
		return map[string]any{"row": link.ToID, "title": link.Title}
	}
}

func buildRecordOutput(fields []collectionFieldDTO, record recordDTO, actorNames map[string]string, detail bool) recordOutput {
	rows := buildRecordRows(fields, record, actorNames)
	out := recordOutput{
		ID: record.ID, CollectionID: record.CollectionID, Title: record.Title, Revision: record.Revision,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, DeletedAt: record.DeletedAt,
		rows: rows,
	}
	if detail {
		out.Fields = &rows
		return out
	}
	values := make(map[string]any, len(rows))
	for _, row := range rows {
		values[row.Name] = recordCellValue(row)
	}
	out.Values = &values
	return out
}

// recordActorNames resolves member references for display. A failed lookup
// leaves the raw "member:<uuid>" reference in place rather than failing a
// command whose real work already succeeded.
func recordActorNames(ctx context.Context, client *cli.APIClient, directory *memberDirectory, fields []collectionFieldDTO, records []recordDTO) map[string]string {
	properties := make([]propertyDTO, len(fields))
	for i, f := range fields {
		properties[i] = f.asProperty()
	}
	bags := make([]map[string]any, len(records))
	for i, r := range records {
		bags[i] = r.Fields
	}
	names, _ := fetchActorPropertyNames(ctx, client, directory, properties, bags...)
	return names
}

func collectionTitleHeader(collection collectionDTO) string {
	if name := strings.TrimSpace(collection.TitleName); name != "" {
		return strings.ToUpper(name)
	}
	return "TITLE"
}

func truncateCell(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= recordTableCellWidth {
		return value
	}
	runes := []rune(value)
	return string(runes[:recordTableCellWidth-1]) + "…"
}

func printRecordGrid(detail collectionDetailDTO, records []recordOutput, extra ...string) {
	headers := []string{"ID", collectionTitleHeader(detail.Collection)}
	for _, f := range detail.Fields {
		headers = append(headers, strings.ToUpper(f.Name))
	}
	headers = append(headers, extra...)
	rows := make([][]string, 0, len(records))
	for _, r := range records {
		cells := make(map[string]string, len(r.rows))
		for _, cell := range r.rows {
			cells[cell.FieldID] = cell.Display
		}
		row := []string{r.ID, truncateCell(r.Title)}
		for _, f := range detail.Fields {
			row = append(row, truncateCell(cells[f.ID]))
		}
		if len(extra) > 0 {
			row = append(row, r.DeletedAt)
		}
		rows = append(rows, row)
	}
	cli.PrintTable(os.Stdout, headers, rows)
}

// printRecordResult prints one row after a read or a write.
func printRecordResult(ctx context.Context, client *cli.APIClient, cmd *cobra.Command, directory *memberDirectory, detail collectionDetailDTO, record recordDTO) error {
	names := recordActorNames(ctx, client, directory, detail.Fields, []recordDTO{record})
	withIDs, _ := cmd.Flags().GetBool("detail")
	result := buildRecordOutput(detail.Fields, record, names, withIDs)
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	fmt.Fprintf(os.Stdout, "%s  %s\n\n", result.ID, result.Title)
	rows := make([][]string, len(result.rows))
	for i, cell := range result.rows {
		rows[i] = []string{cell.Name, cell.Display, cell.Type}
	}
	cli.PrintTable(os.Stdout, []string{"FIELD", "VALUE", "TYPE"}, rows)
	return nil
}

// ---------------------------------------------------------------------------
// Filters, sort and cell values
// ---------------------------------------------------------------------------

// splitFieldPair splits one "Field=Value" flag at its first "=".
func splitFieldPair(flag, pair string) (string, string, error) {
	name, value, found := strings.Cut(pair, "=")
	if !found || strings.TrimSpace(name) == "" {
		return "", "", fmt.Errorf(`--%s %q must be in "Field=Value" form`, flag, pair)
	}
	return strings.TrimSpace(name), value, nil
}

// buildRecordFilterParam converts repeated --filter flags into the JSON object
// the records endpoint takes as `properties`: OR within a field, AND across
// fields, keyed by field id so a name and an id address the same entry.
func buildRecordFilterParam(ctx context.Context, client *cli.APIClient, directory *memberDirectory, detail collectionDetailDTO, pairs []string) (string, error) {
	filter := make(map[string][]string, len(pairs))
	for _, pair := range pairs {
		name, rawValue, err := splitFieldPair("filter", pair)
		if err != nil {
			return "", err
		}
		// Reserved so scripts never come to depend on "Seats>" resolving as a
		// field name once >=, <= and != mean comparison filters.
		if strings.HasSuffix(name, "<") || strings.HasSuffix(name, ">") || strings.HasSuffix(name, "!") {
			return "", fmt.Errorf(`--filter %q: comparison operators are not supported yet; only "Field=Value" is accepted`, pair)
		}
		if strings.TrimSpace(rawValue) == "" {
			return "", fmt.Errorf("--filter %s: value cannot be empty (use %s to match rows where the field is empty)", name, propertyNoValueSentinel)
		}
		field, err := resolveCollectionFieldRef(detail, name)
		if err != nil {
			return "", err
		}
		value, err := resolveRecordFilterValue(ctx, client, directory, field, rawValue)
		if err != nil {
			return "", err
		}
		duplicate := false
		for _, existing := range filter[field.ID] {
			if existing == value {
				duplicate = true
			}
		}
		if !duplicate {
			filter[field.ID] = append(filter[field.ID], value)
		}
	}
	buf, err := json.Marshal(filter)
	if err != nil {
		return "", fmt.Errorf("encode filter: %w", err)
	}
	return string(buf), nil
}

// resolveRecordFilterValue spells one filter value the way the cell stores it.
// A value that could never match is rejected here rather than sent, because an
// empty result reads like a real answer.
func resolveRecordFilterValue(ctx context.Context, client *cli.APIClient, directory *memberDirectory, field collectionFieldDTO, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if field.isRelation() {
		return "", fmt.Errorf("--filter %s: relation fields cannot be filtered yet; list the rows and read their links instead", field.Name)
	}
	if trimmed == propertyNoValueSentinel {
		return propertyNoValueSentinel, nil
	}
	switch field.Type {
	case "select", "multi_select":
		return resolveSelectOptionRef(field.asProperty(), trimmed)
	case "actor", "multi_actor":
		return resolveActorPropertyRef(ctx, client, directory, trimmed)
	case "checkbox":
		if trimmed != "true" && trimmed != "false" {
			return "", fmt.Errorf("--filter %s: value %q is not a valid bool (expected true or false)", field.Name, trimmed)
		}
		return trimmed, nil
	case "number":
		num, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(num) || math.IsInf(num, 0) {
			return "", fmt.Errorf("--filter %s: value %q is not a finite number", field.Name, trimmed)
		}
		return trimmed, nil
	case "date":
		if _, err := time.Parse("2006-01-02", trimmed); err != nil {
			return "", fmt.Errorf("--filter %s: value %q is not a date in YYYY-MM-DD form", field.Name, trimmed)
		}
		return trimmed, nil
	case "url":
		return trimmed, nil
	case "text":
		// Text is stored exactly as written, so trimming would miss a value
		// that really has spaces around it.
		return raw, nil
	default: // a field type only a newer backend knows about
		return "", fmt.Errorf("--filter %s: this CLI does not know how to filter %s fields; update with `multica update`", field.Name, field.Type)
	}
}

// resolveRecordSort maps --sort onto the endpoint's sort_by. A relation has no
// stored value to order by, and the server would quietly return creation order.
func resolveRecordSort(detail collectionDetailDTO, ref string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(ref)) {
	case "title":
		return "title", nil
	case "created_at", "created":
		return "created_at", nil
	}
	field, err := resolveCollectionFieldRef(detail, ref)
	if err != nil {
		return "", fmt.Errorf(`--sort takes "title", "created_at", or a field: %w`, err)
	}
	if field.isRelation() {
		return "", fmt.Errorf("--sort %s: relation fields have no sort order yet", field.Name)
	}
	return field.ID, nil
}

// encodeRecordCell turns one "Field=Value" into the field and the typed JSON
// the API stores, translating option and member names to ids on the way.
func encodeRecordCell(ctx context.Context, client *cli.APIClient, directory *memberDirectory, detail collectionDetailDTO, flag, pair string) (collectionFieldDTO, json.RawMessage, error) {
	name, rawValue, err := splitFieldPair(flag, pair)
	if err != nil {
		return collectionFieldDTO{}, nil, err
	}
	field, err := resolveCollectionFieldRef(detail, name)
	if err != nil {
		return collectionFieldDTO{}, nil, err
	}
	if field.isRelation() {
		return collectionFieldDTO{}, nil, fmt.Errorf("--%s %s: a relation cell holds links, not a value; use `multica record link` / `multica record unlink`", flag, field.Name)
	}
	if strings.TrimSpace(rawValue) == "" {
		return collectionFieldDTO{}, nil, fmt.Errorf("--%s %s: value cannot be empty; clear a cell with `multica record update --unset %q`", flag, field.Name, field.Name)
	}
	value, err := encodeIssuePropertyValue(ctx, client, directory, field.asProperty(), rawValue)
	if err != nil {
		return collectionFieldDTO{}, nil, fmt.Errorf("--%s %s: %w", flag, field.Name, err)
	}
	return field, value, nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func runRecordList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > recordListMaxLimit {
		return fmt.Errorf("--limit must be 1 to %d", recordListMaxLimit)
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
	var members memberDirectory
	params := url.Values{"limit": {strconv.Itoa(limit)}}
	if search, _ := cmd.Flags().GetString("search"); search != "" {
		params.Set("search", search)
	}
	if pairs, _ := cmd.Flags().GetStringArray("filter"); len(pairs) > 0 {
		filter, err := buildRecordFilterParam(ctx, client, &members, detail, pairs)
		if err != nil {
			return err
		}
		params.Set("properties", filter)
	}
	desc, _ := cmd.Flags().GetBool("desc")
	if sortRef, _ := cmd.Flags().GetString("sort"); sortRef != "" {
		sortBy, err := resolveRecordSort(detail, sortRef)
		if err != nil {
			return err
		}
		params.Set("sort_by", sortBy)
		if desc {
			params.Set("sort_dir", "desc")
		}
	} else if desc {
		return fmt.Errorf("--desc needs --sort; without it rows are already newest first")
	}
	if cursor, _ := cmd.Flags().GetString("cursor"); cursor != "" {
		params.Set("cursor", cursor)
	}

	var page recordPageDTO
	if err := client.GetJSON(ctx, recordsPath(detail.Collection.ID)+"?"+params.Encode(), &page); err != nil {
		return fmt.Errorf("list rows: %w", err)
	}
	names := recordActorNames(ctx, client, &members, detail.Fields, page.Records)
	withIDs, _ := cmd.Flags().GetBool("detail")
	records := make([]recordOutput, len(page.Records))
	for i, r := range page.Records {
		records[i] = buildRecordOutput(detail.Fields, r, names, withIDs)
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, recordListOutput{Records: records, Total: page.Total, NextCursor: page.NextCursor})
	}
	printRecordGrid(detail, records)
	if page.NextCursor != nil {
		fmt.Fprintf(os.Stderr, "Showing %d of %d rows. Read on with the same flags plus --cursor %s\n", len(records), page.Total, *page.NextCursor)
	}
	return nil
}

func runRecordGet(cmd *cobra.Command, args []string) error {
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
	recordID, err := resolveRecordRef(ctx, client, detail.Collection, args[1])
	if err != nil {
		return err
	}
	record, err := fetchRecord(ctx, client, detail.Collection.ID, recordID)
	if err != nil {
		return err
	}
	return printRecordResult(ctx, client, cmd, &memberDirectory{}, detail, record)
}

func runRecordCreate(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	pairs, _ := cmd.Flags().GetStringArray("set")
	if strings.TrimSpace(title) == "" && len(pairs) == 0 {
		return fmt.Errorf("an empty row says nothing; pass --title and/or --set")
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
	var members memberDirectory
	values := make(map[string]json.RawMessage, len(pairs))
	for _, pair := range pairs {
		field, value, err := encodeRecordCell(ctx, client, &members, detail, "set", pair)
		if err != nil {
			return err
		}
		values[field.ID] = value
	}
	body := map[string]any{"title": title}
	if len(values) > 0 {
		body["fields"] = values
	}
	var created recordDTO
	if err := client.PostJSON(ctx, recordsPath(detail.Collection.ID), body, &created); err != nil {
		return fmt.Errorf("create row: %w", err)
	}
	return printRecordResult(ctx, client, cmd, &members, detail, created)
}

// recordCellWrite is one queued write of `record update`.
type recordCellWrite struct {
	field    collectionFieldDTO
	value    json.RawMessage // nil clears the cell
	expected json.RawMessage // nil writes unconditionally
}

func runRecordUpdate(cmd *cobra.Command, args []string) error {
	setPairs, _ := cmd.Flags().GetStringArray("set")
	unsetRefs, _ := cmd.Flags().GetStringArray("unset")
	expectPairs, _ := cmd.Flags().GetStringArray("expect")
	titleChanged := cmd.Flags().Changed("title")
	if !titleChanged && len(setPairs) == 0 && len(unsetRefs) == 0 {
		return fmt.Errorf("nothing to update; pass --title, --set, or --unset")
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
	var members memberDirectory

	// Everything is resolved before the first write, so a typo in the third
	// flag cannot leave the first two applied.
	writes := make([]recordCellWrite, 0, len(setPairs)+len(unsetRefs))
	for _, pair := range setPairs {
		field, value, err := encodeRecordCell(ctx, client, &members, detail, "set", pair)
		if err != nil {
			return err
		}
		writes = append(writes, recordCellWrite{field: field, value: value})
	}
	for _, ref := range unsetRefs {
		field, err := resolveCollectionFieldRef(detail, ref)
		if err != nil {
			return err
		}
		if field.isRelation() {
			return fmt.Errorf("--unset %s: a relation cell holds links; remove them with `multica record unlink`", field.Name)
		}
		writes = append(writes, recordCellWrite{field: field})
	}
	for _, pair := range expectPairs {
		name, rawValue, err := splitFieldPair("expect", pair)
		if err != nil {
			return err
		}
		field, err := resolveCollectionFieldRef(detail, name)
		if err != nil {
			return err
		}
		expected := json.RawMessage("null")
		if strings.TrimSpace(rawValue) != "" {
			if _, expected, err = encodeRecordCell(ctx, client, &members, detail, "expect", pair); err != nil {
				return err
			}
		}
		matched := false
		for i := range writes {
			if writes[i].field.ID == field.ID {
				writes[i].expected = expected
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("--expect %s guards a write, but no --set or --unset targets that field", field.Name)
		}
	}

	recordID, err := resolveRecordRef(ctx, client, detail.Collection, args[1])
	if err != nil {
		return err
	}
	record, err := fetchRecord(ctx, client, detail.Collection.ID, recordID)
	if err != nil {
		return err
	}

	var applied []string
	fail := func(what string, err error) error {
		if len(applied) > 0 {
			fmt.Fprintf(os.Stderr, "Already written before this failure: %s\n", strings.Join(applied, ", "))
		}
		return fmt.Errorf("%s: %w", what, err)
	}
	recordPath := recordsPath(detail.Collection.ID) + "/" + record.ID
	if titleChanged {
		title, _ := cmd.Flags().GetString("title")
		// title_base is the title just read: the server refuses the rename if
		// someone else retitled the row in between.
		body := map[string]any{"title": title, "title_base": record.Title}
		var updated recordDTO
		if err := client.PutJSON(ctx, recordPath, body, &updated); err != nil {
			return fail("update title", err)
		}
		record = updated
		applied = append(applied, "title")
	}
	for _, write := range writes {
		body := map[string]any{"value": nil}
		if write.value != nil {
			body["value"] = write.value
		}
		if write.expected != nil {
			body["expected_value"] = write.expected
		}
		// A fresh value per response: decoding into a populated map would keep
		// the key of a cell this very write just cleared.
		var updated recordDTO
		if err := client.PutJSON(ctx, recordPath+"/fields/"+write.field.ID, body, &updated); err != nil {
			return fail("write "+write.field.Name, err)
		}
		record = updated
		applied = append(applied, write.field.Name)
	}
	return printRecordResult(ctx, client, cmd, &members, detail, record)
}

func makeRecordTrashRun(remove bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
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
		var (
			recordID string
			record   recordDTO
		)
		if remove {
			if recordID, err = resolveRecordRef(ctx, client, detail.Collection, args[1]); err != nil {
				return err
			}
			if err := client.DeleteJSONResponse(ctx, recordsPath(detail.Collection.ID)+"/"+recordID, &record); err != nil {
				return fmt.Errorf("delete row: %w", err)
			}
		} else {
			if recordID, err = resolveTrashedRecordRef(ctx, client, detail.Collection, args[1]); err != nil {
				return err
			}
			if err := client.PostJSON(ctx, recordsPath(detail.Collection.ID)+"/"+recordID+"/restore", map[string]any{}, &record); err != nil {
				return fmt.Errorf("restore row: %w", err)
			}
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			return printRecordResult(ctx, client, cmd, &memberDirectory{}, detail, record)
		}
		if remove {
			fmt.Fprintf(os.Stdout, "Row %q moved to the trash of %q. Restore it within 30 days with `multica record restore`.\n", record.Title, detail.Collection.Name)
		} else {
			fmt.Fprintf(os.Stdout, "Row %q restored to %q.\n", record.Title, detail.Collection.Name)
		}
		return nil
	}
}

type recordTrashDTO struct {
	Records       []recordDTO `json:"records"`
	Total         int64       `json:"total"`
	RetentionDays int         `json:"retention_days"`
}

func fetchRecordTrash(ctx context.Context, client *cli.APIClient, collectionID string) (recordTrashDTO, error) {
	var trash recordTrashDTO
	if err := client.GetJSON(ctx, "/api/collections/"+collectionID+"/trash", &trash); err != nil {
		return recordTrashDTO{}, fmt.Errorf("list trash: %w", err)
	}
	return trash, nil
}

// resolveTrashedRecordRef is resolveRecordRef for the trash, which the row list
// never includes.
func resolveTrashedRecordRef(ctx context.Context, client *cli.APIClient, collection collectionDTO, ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if uuidRegexp.MatchString(trimmed) {
		return strings.ToLower(trimmed), nil
	}
	trash, err := fetchRecordTrash(ctx, client, collection.ID)
	if err != nil {
		return "", err
	}
	id, err := matchRecordTitle(trash.Records, collection.Name, trimmed, false)
	if err != nil {
		return "", fmt.Errorf("%w (looked in the trash)", err)
	}
	return id, nil
}

func runRecordTrash(cmd *cobra.Command, args []string) error {
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
	trash, err := fetchRecordTrash(ctx, client, detail.Collection.ID)
	if err != nil {
		return err
	}
	names := recordActorNames(ctx, client, &memberDirectory{}, detail.Fields, trash.Records)
	withIDs, _ := cmd.Flags().GetBool("detail")
	records := make([]recordOutput, len(trash.Records))
	for i, r := range trash.Records {
		records[i] = buildRecordOutput(detail.Fields, r, names, withIDs)
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, map[string]any{"records": records, "total": trash.Total, "retention_days": trash.RetentionDays})
	}
	printRecordGrid(detail, records, "DELETED")
	return nil
}

// ---------------------------------------------------------------------------
// Relation links
// ---------------------------------------------------------------------------

// loadRelationTarget resolves the pieces every link command needs: the table,
// the row, and a field that really is a relation.
func loadRelationTarget(ctx context.Context, client *cli.APIClient, cmd *cobra.Command, args []string) (collectionDetailDTO, recordDTO, collectionFieldDTO, []string, error) {
	fieldRef, _ := cmd.Flags().GetString("field")
	targets, _ := cmd.Flags().GetStringArray("to")
	var none collectionFieldDTO
	if strings.TrimSpace(fieldRef) == "" {
		return collectionDetailDTO{}, recordDTO{}, none, nil, fmt.Errorf("--field is required: the relation field to change")
	}
	if len(targets) == 0 {
		return collectionDetailDTO{}, recordDTO{}, none, nil, fmt.Errorf("--to is required")
	}
	detail, err := loadCollection(ctx, client, args[0])
	if err != nil {
		return collectionDetailDTO{}, recordDTO{}, none, nil, err
	}
	field, err := resolveCollectionFieldRef(detail, fieldRef)
	if err != nil {
		return collectionDetailDTO{}, recordDTO{}, none, nil, err
	}
	if !field.isRelation() || field.Config.Relation == nil {
		return collectionDetailDTO{}, recordDTO{}, none, nil, fmt.Errorf("field %q is a %s field, not a relation; write it with `multica record update --set`", field.Name, field.Type)
	}
	recordID, err := resolveRecordRef(ctx, client, detail.Collection, args[1])
	if err != nil {
		return collectionDetailDTO{}, recordDTO{}, none, nil, err
	}
	record, err := fetchRecord(ctx, client, detail.Collection.ID, recordID)
	if err != nil {
		return collectionDetailDTO{}, recordDTO{}, none, nil, err
	}
	return detail, record, field, targets, nil
}

// resolveRelationTarget turns one --to value into the id a relation field
// links to: an issue for a field that points at issues, otherwise a row of the
// field's target table.
func resolveRelationTarget(ctx context.Context, client *cli.APIClient, field collectionFieldDTO, ref string) (string, error) {
	if field.relatesToIssues() {
		issue, err := resolveIssueRef(ctx, client, ref)
		if err != nil {
			return "", fmt.Errorf("resolve issue %q: %w", ref, err)
		}
		return issue.ID, nil
	}
	target, err := fetchCollectionDetail(ctx, client, field.Config.Relation.CollectionID)
	if err != nil {
		return "", fmt.Errorf("field %q points at a table that can no longer be opened: %w", field.Name, err)
	}
	return resolveRecordRef(ctx, client, target.Collection, ref)
}

func runRecordLink(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	detail, record, field, targets, err := loadRelationTarget(ctx, client, cmd, args)
	if err != nil {
		return err
	}
	ids := make([]string, len(targets))
	for i, ref := range targets {
		if ids[i], err = resolveRelationTarget(ctx, client, field, ref); err != nil {
			return err
		}
	}
	path := recordsPath(detail.Collection.ID) + "/" + record.ID + "/links"
	for i, id := range ids {
		var updated recordDTO
		if err := client.PostJSON(ctx, path, map[string]any{"field_id": field.ID, "to_id": id}, &updated); err != nil {
			if i > 0 {
				fmt.Fprintf(os.Stderr, "Already linked before this failure: %s\n", strings.Join(targets[:i], ", "))
			}
			return fmt.Errorf("link %s: %w", targets[i], err)
		}
		record = updated
	}
	return printRecordResult(ctx, client, cmd, &memberDirectory{}, detail, record)
}

// matchRecordLink finds the link a --to value names among a cell's links. An
// id is tried as the link's own id and as its target's id before anything is
// resolved by name, so a link whose target is gone can still be removed.
func matchRecordLink(ctx context.Context, client *cli.APIClient, field collectionFieldDTO, links []recordLinkDTO, ref string) (recordLinkDTO, error) {
	trimmed := strings.TrimSpace(ref)
	if uuidRegexp.MatchString(trimmed) {
		for _, link := range links {
			if strings.EqualFold(link.ID, trimmed) || strings.EqualFold(link.ToID, trimmed) {
				return link, nil
			}
		}
		return recordLinkDTO{}, fmt.Errorf("%s is not linked in %q", trimmed, field.Name)
	}
	targetID, err := resolveRelationTarget(ctx, client, field, trimmed)
	if err != nil {
		return recordLinkDTO{}, err
	}
	for _, link := range links {
		if strings.EqualFold(link.ToID, targetID) {
			return link, nil
		}
	}
	return recordLinkDTO{}, fmt.Errorf("%s is not linked in %q", trimmed, field.Name)
}

func runRecordUnlink(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	detail, record, field, targets, err := loadRelationTarget(ctx, client, cmd, args)
	if err != nil {
		return err
	}
	links := make([]recordLinkDTO, len(targets))
	for i, ref := range targets {
		if links[i], err = matchRecordLink(ctx, client, field, record.Links[field.ID], ref); err != nil {
			return err
		}
	}
	base := recordsPath(detail.Collection.ID) + "/" + record.ID + "/links/"
	for i, link := range links {
		var updated recordDTO
		if err := client.DeleteJSONResponse(ctx, base+link.ID, &updated); err != nil {
			if i > 0 {
				fmt.Fprintf(os.Stderr, "Already unlinked before this failure: %s\n", strings.Join(targets[:i], ", "))
			}
			return fmt.Errorf("unlink %s: %w", targets[i], err)
		}
		record = updated
	}
	return printRecordResult(ctx, client, cmd, &memberDirectory{}, detail, record)
}

func printRecordBacklinks(cmd *cobra.Command, links []recordBacklinkDTO) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		if links == nil {
			links = []recordBacklinkDTO{}
		}
		return cli.PrintJSON(os.Stdout, links)
	}
	rows := make([][]string, len(links))
	for i, link := range links {
		rows[i] = []string{link.CollectionName, truncateCell(link.RecordTitle), link.FieldName, link.RecordID}
	}
	cli.PrintTable(os.Stdout, []string{"TABLE", "ROW", "FIELD", "ROW ID"}, rows)
	return nil
}

func runRecordBacklinks(cmd *cobra.Command, args []string) error {
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
	recordID, err := resolveRecordRef(ctx, client, detail.Collection, args[1])
	if err != nil {
		return err
	}
	var result struct {
		Links []recordBacklinkDTO `json:"links"`
	}
	if err := client.GetJSON(ctx, recordsPath(detail.Collection.ID)+"/"+recordID+"/backlinks", &result); err != nil {
		return fmt.Errorf("list backlinks: %w", err)
	}
	return printRecordBacklinks(cmd, result.Links)
}

func runIssueRecords(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	issue, err := resolveIssueRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	var result struct {
		Links []recordBacklinkDTO `json:"links"`
	}
	if err := client.GetJSON(ctx, "/api/issues/"+issue.ID+"/record-links", &result); err != nil {
		return fmt.Errorf("list linked rows: %w", err)
	}
	return printRecordBacklinks(cmd, result.Links)
}
