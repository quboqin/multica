package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// multica document {list|get|create|save|move|status} — pages of the workspace
// document tree.
//
// A document is an issue of kind "doc", so comments, attachments, subscribers
// and search stay under `multica issue`. What differs lives here:
//
//   - The body is versioned. Every save names the revision it was written
//     against, and the server refuses one that is no longer current. `save`
//     never adopts a newer revision on its own: a conflict comes back to the
//     caller, who re-reads and merges.
//   - The tree is its own structure: `list` prints it, `move` reparents and
//     reorders inside it.
//   - Documents start private. The human owner manages audiences with share.
//     Editors can read and restore history; sharing survives content edits.

const (
	documentKind = "doc"
	// documentTransitionRequiresHumanCode is the server's stable code for a
	// lifecycle change refused because the caller holds a machine credential.
	documentTransitionRequiresHumanCode = "document_transition_requires_human"
	// documentMoveToEnd and documentMoveToStart are positions past every sibling
	// in either direction. The move endpoint inserts before the first sibling at
	// or beyond the position it is given and then renumbers them all, so only
	// the ordering matters — and a new document's position can be negative,
	// which is why "first" is not simply zero.
	documentMoveToEnd   = 1e15
	documentMoveToStart = -1e15
)

// documentStatusActions maps the status a caller asks for onto the transition
// the API names. Callers think in states ("make it published"); the endpoint
// thinks in actions.
var documentStatusActions = map[string]string{
	"draft":     "draft",
	"reviewing": "review",
	"published": "publish",
}

var documentCmd = &cobra.Command{
	Use:   "document",
	Short: "Work with documents (pages of the document tree)",
	Long: `Documents are versioned pages organised in a tree, per workspace or per project.

Comments, attachments and search work as they do for issues: use
"multica issue comment", and "multica issue search --kind doc".`,
}

var documentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the document tree (titles and status, no bodies)",
	Args:  exactArgs(0),
	RunE:  runDocumentList,
}

var documentGetCmd = &cobra.Command{
	Use:   "get <document>",
	Short: "Show a document with its body and revision",
	Long: `Show a document. The JSON carries the Markdown body in "description" and the
revision to save against in "document_revision".

--output markdown prints the body alone, ready to redirect into a file; the
revision goes to stderr.`,
	Args: exactArgs(1),
	RunE: runDocumentGet,
}

var documentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a document (always starts as a draft)",
	Args:  exactArgs(0),
	RunE:  runDocumentCreate,
}

var documentSaveCmd = &cobra.Command{
	Use:   "save <document>",
	Short: "Save a document's body against the revision you read",
	Long: `Save the body. --expected-revision is the "document_revision" you read before
editing; the save lands only while that is still the current one, and the
revision then goes up by one.

When someone else saved first, nothing is written. Read the document again,
merge your change into the current body, and save against the new revision —
never retry with the new number alone, which would overwrite their work.

Saving a shared document preserves its audience. Use document versions to inspect
history, and document restore to restore title and body as a new version.`,
	Args: exactArgs(1),
	RunE: runDocumentSave,
}

var documentMoveCmd = &cobra.Command{
	Use:   "move <document>",
	Short: "Move a document within the tree",
	Long: `Move a document under another one (--parent), to the top level (--root), or
reorder it among its current siblings (neither flag). It lands last unless
--before or --first says otherwise. A parent must be a document of the same
project, and never one of the document's own descendants.`,
	Args: exactArgs(1),
	RunE: runDocumentMove,
}

var documentStatusCmd = &cobra.Command{
	Use:        "status <document> <status>",
	Short:      "Removed: use document share for explicit audience settings",
	Deprecated: "review was removed; use document share",
	Hidden:     true,
	Args:       exactArgs(2), RunE: runDocumentStatus,
}

func init() {
	documentCmd.AddCommand(documentListCmd)
	documentCmd.AddCommand(documentGetCmd)
	documentCmd.AddCommand(documentCreateCmd)
	documentCmd.AddCommand(documentSaveCmd)
	documentCmd.AddCommand(documentMoveCmd)
	documentCmd.AddCommand(documentStatusCmd)

	documentListCmd.Flags().String("output", "table", "Output format: table or json")
	documentListCmd.Flags().String("project", "", "Only this project's tree (id or id prefix)")

	documentGetCmd.Flags().String("output", "json", "Output format: json, table, or markdown (the body alone)")

	documentCreateCmd.Flags().String("output", "json", "Output format: table or json")
	documentCreateCmd.Flags().String("title", "", "Document title (required)")
	registerDocumentContentFlags(documentCreateCmd, "Document body as Markdown")
	documentCreateCmd.Flags().String("parent", "", "Parent document (key or id); its project is used unless --project is given")
	documentCreateCmd.Flags().String("project", "", "Project the document belongs to (id or id prefix); omit for a workspace document")
	documentCreateCmd.Flags().Bool("allow-duplicate", false, "Allow creating a document even when an active one with the same title exists in the same place")

	documentSaveCmd.Flags().String("output", "json", "Output format: table or json")
	documentSaveCmd.Flags().Int64("expected-revision", 0, "The document_revision you read before editing (required)")
	documentSaveCmd.Flags().String("title", "", "New title")
	registerDocumentContentFlags(documentSaveCmd, "New body as Markdown")

	documentMoveCmd.Flags().String("output", "json", "Output format: table or json")
	documentMoveCmd.Flags().String("parent", "", "New parent document (key or id)")
	documentMoveCmd.Flags().Bool("root", false, "Move to the top level")
	documentMoveCmd.Flags().String("before", "", "Place it before this sibling (key or id)")
	documentMoveCmd.Flags().Bool("first", false, "Place it first among its siblings")

	documentStatusCmd.Flags().String("output", "json", "Output format: table or json")
	documentStatusCmd.Flags().Int64("expected-revision", 0, "The document_revision you read; the change applies to exactly that revision (required)")
}

func registerDocumentContentFlags(cmd *cobra.Command, what string) {
	cmd.Flags().String("content", "", what+" (decodes \\n, \\r, \\t, \\\\; pipe via --content-stdin for multi-line bodies or to preserve literal backslashes)")
	cmd.Flags().Bool("content-stdin", false, "Read the body from stdin (preserves multi-line content verbatim)")
	cmd.Flags().String("content-file", "", "Read the body from a UTF-8 file (preserves multi-line content verbatim; use this on Windows when stdin piping mangles non-ASCII bytes). The path must be inside the current working directory unless --allow-external-file is set.")
	cmd.Flags().Bool("allow-external-file", false, "Allow --content-file to read a path outside the current working directory. Off by default so a stale file from another run/environment can't be picked up (MUL-4252).")
}

// documentStatusRequestError turns a refused lifecycle change into what the
// caller should read. See collectionSchemaRequestError for why a 403 needs the
// opt-in: the generic copy would tell an agent to go and ask for access it can
// never be given.
func documentStatusRequestError(err error) error {
	if cli.ServerErrorCode(err) == documentTransitionRequiresHumanCode {
		return cli.WithUserMessage("submitting, publishing and withdrawing a document are approvals signed by a person: a run's task token cannot change a document's status. Save your draft, then leave a comment on the document asking a workspace member to review it.", err)
	}
	return fmt.Errorf("change document status: %w", err)
}

// fetchDocument reads one document by key or id and refuses anything that is
// not one, so a typo'd issue key cannot be saved over as if it were a page.
//
// Id prefixes are not offered here: they resolve through the issue list, which
// leaves documents out, so a prefix could only ever match the wrong thing.
func fetchDocument(ctx context.Context, client *cli.APIClient, ref string) (map[string]any, error) {
	trimmed := strings.TrimSpace(ref)
	if !looksLikeIssueIdentifier(trimmed) && !uuidRegexp.MatchString(trimmed) {
		return nil, fmt.Errorf("%q is not a document key (like MUL-12) or a full id; find one with `multica document list`", ref)
	}
	var doc map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+url.PathEscape(trimmed), &doc); err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if kind := strVal(doc, "kind"); kind != documentKind {
		if kind == "" {
			kind = "task"
		}
		return nil, fmt.Errorf("%s is a %s, not a document; use `multica issue get %s`", issueDisplayKey(doc), kind, trimmed)
	}
	return doc, nil
}

func documentRevision(doc map[string]any) int64 {
	if v, ok := doc["document_revision"].(float64); ok {
		return int64(v)
	}
	return 0
}

func printDocumentResult(cmd *cobra.Command, doc map[string]any) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "table" {
		cli.PrintTable(os.Stdout, []string{"KEY", "TITLE", "STATUS", "REVISION"}, [][]string{{
			issueDisplayKey(doc), strVal(doc, "title"), strVal(doc, "status"), strconv.FormatInt(documentRevision(doc), 10),
		}})
		return nil
	}
	return cli.PrintJSON(os.Stdout, doc)
}

// orderDocumentTree flattens the tree depth-first and stamps each node with
// its depth. The API returns documents ordered by position, so visiting them
// in the order given keeps siblings in place. A document whose parent is not in
// the listing (another project, when --project narrowed it) prints as a root
// rather than disappearing.
func orderDocumentTree(docs []map[string]any) []map[string]any {
	known := make(map[string]bool, len(docs))
	for _, doc := range docs {
		known[strVal(doc, "id")] = true
	}
	children := make(map[string][]map[string]any, len(docs))
	var roots []map[string]any
	for _, doc := range docs {
		parent := strVal(doc, "parent_issue_id")
		if parent == "" || !known[parent] {
			roots = append(roots, doc)
			continue
		}
		children[parent] = append(children[parent], doc)
	}
	ordered := make([]map[string]any, 0, len(docs))
	var visit func(doc map[string]any, depth int)
	visit = func(doc map[string]any, depth int) {
		doc["depth"] = depth
		ordered = append(ordered, doc)
		for _, child := range children[strVal(doc, "id")] {
			visit(child, depth+1)
		}
	}
	for _, root := range roots {
		visit(root, 0)
	}
	return ordered
}

func runDocumentList(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	path := "/api/documents"
	if ref, _ := cmd.Flags().GetString("project"); ref != "" {
		project, err := resolveProjectID(ctx, client, ref)
		if err != nil {
			return fmt.Errorf("resolve project: %w", err)
		}
		path += "?" + url.Values{"project_id": {project.ID}}.Encode()
	}
	var docs []map[string]any
	if err := client.GetJSON(ctx, path, &docs); err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	// A tree listing is for finding a page, and one call must stay cheap however
	// long the pages are: bodies are read one at a time with `document get`.
	for _, doc := range docs {
		delete(doc, "description")
	}
	ordered := orderDocumentTree(docs)

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, ordered)
	}
	rows := make([][]string, len(ordered))
	for i, doc := range ordered {
		depth, _ := doc["depth"].(int)
		updated := strVal(doc, "updated_at")
		if len(updated) >= 10 {
			updated = updated[:10]
		}
		rows[i] = []string{
			issueDisplayKey(doc),
			strings.Repeat("  ", depth) + strVal(doc, "title"),
			strVal(doc, "status"),
			strconv.FormatInt(documentRevision(doc), 10),
			updated,
		}
	}
	cli.PrintTable(os.Stdout, []string{"KEY", "TITLE", "STATUS", "REVISION", "UPDATED"}, rows)
	return nil
}

func runDocumentGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	doc, err := fetchDocument(ctx, client, args[0])
	if err != nil {
		return err
	}
	if output, _ := cmd.Flags().GetString("output"); output == "markdown" {
		fmt.Fprintf(os.Stderr, "%s · revision %d · %s\n", issueDisplayKey(doc), documentRevision(doc), strVal(doc, "status"))
		fmt.Fprintln(os.Stdout, strVal(doc, "description"))
		return nil
	}
	return printDocumentResult(cmd, doc)
}

func runDocumentCreate(cmd *cobra.Command, _ []string) error {
	title, _ := cmd.Flags().GetString("title")
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("--title is required")
	}
	content, hasContent, err := resolveTextFlag(cmd, "content")
	if err != nil {
		return err
	}
	if hasContent {
		if err := guardLocalPathLinks(content, "document body",
			"Attach the file to the document with `multica issue comment add <document> --attachment <path>` and drop the link."); err != nil {
			return err
		}
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{"kind": documentKind, "title": title}
	if hasContent {
		body["description"] = content
	}
	projectRef, _ := cmd.Flags().GetString("project")
	if ref, _ := cmd.Flags().GetString("parent"); ref != "" {
		parent, err := fetchDocument(ctx, client, ref)
		if err != nil {
			return fmt.Errorf("--parent: %w", err)
		}
		body["parent_issue_id"] = strVal(parent, "id")
		// A page lives in its parent's project; the server refuses anything
		// else, so follow the parent unless told otherwise.
		if projectRef == "" {
			if project := strVal(parent, "project_id"); project != "" {
				body["project_id"] = project
			}
		}
	}
	if projectRef != "" {
		project, err := resolveProjectID(ctx, client, projectRef)
		if err != nil {
			return fmt.Errorf("resolve project: %w", err)
		}
		body["project_id"] = project.ID
	}
	if allow, _ := cmd.Flags().GetBool("allow-duplicate"); allow {
		body["allow_duplicate"] = true
	}

	var created map[string]any
	if err := client.PostJSON(ctx, "/api/issues", body, &created); err != nil {
		if msg, ok := activeDuplicateIssueCreateMessage(err); ok {
			return errors.New(msg)
		}
		return fmt.Errorf("create document: %w", err)
	}
	return printDocumentResult(cmd, created)
}

func requireExpectedRevision(cmd *cobra.Command) (int64, error) {
	if !cmd.Flags().Changed("expected-revision") {
		return 0, fmt.Errorf("--expected-revision is required: pass the document_revision you read with `multica document get`, so nothing is written over a version you never saw")
	}
	revision, _ := cmd.Flags().GetInt64("expected-revision")
	if revision < 1 {
		return 0, fmt.Errorf("--expected-revision must be positive")
	}
	return revision, nil
}

// documentConflictError reports a refused save with the revision the server
// holds now. The conflict response carries the whole current body, which is
// routinely longer than the slice of an error body the client keeps, so the
// revision is read back instead of parsed out of it.
func documentConflictError(ctx context.Context, client *cli.APIClient, id string, expected int64, err error) error {
	var httpErr *cli.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusConflict {
		return nil
	}
	var current map[string]any
	if getErr := client.GetJSON(ctx, "/api/issues/"+id, &current); getErr != nil {
		return nil
	}
	now := documentRevision(current)
	if now == expected {
		// The revision still matches, so this 409 is about something else
		// (a concurrent title edit, a retired status): let the server's own
		// message through.
		return nil
	}
	key := issueDisplayKey(current)
	return cli.WithUserMessage(fmt.Sprintf(
		"nothing was written: %s is at revision %d, and this command named revision %d. Read it again with `multica document get %s`; when saving, merge your change into the current body first, then repeat with --expected-revision %d.",
		key, now, expected, key, now), err)
}

func runDocumentSave(cmd *cobra.Command, args []string) error {
	revision, err := requireExpectedRevision(cmd)
	if err != nil {
		return err
	}
	content, hasContent, err := resolveTextFlag(cmd, "content")
	if err != nil {
		return err
	}
	titleChanged := cmd.Flags().Changed("title")
	if !hasContent && !titleChanged {
		return fmt.Errorf("nothing to save; pass --content, --content-stdin, --content-file, or --title")
	}
	if hasContent {
		if err := guardLocalPathLinks(content, "document body",
			"Attach the file to the document with `multica issue comment add <document> --attachment <path>` and drop the link."); err != nil {
			return err
		}
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	doc, err := fetchDocument(ctx, client, args[0])
	if err != nil {
		return err
	}
	id := strVal(doc, "id")
	body := map[string]any{"expected_document_revision": revision}
	if hasContent {
		body["description"] = content
	}
	if titleChanged {
		title, _ := cmd.Flags().GetString("title")
		body["title"] = title
	}
	var saved map[string]any
	if err := client.PutJSON(ctx, "/api/issues/"+id, body, &saved); err != nil {
		if conflict := documentConflictError(ctx, client, id, revision, err); conflict != nil {
			return conflict
		}
		return fmt.Errorf("save document: %w", err)
	}
	return printDocumentResult(cmd, saved)
}

func runDocumentMove(cmd *cobra.Command, args []string) error {
	parentRef, _ := cmd.Flags().GetString("parent")
	toRoot, _ := cmd.Flags().GetBool("root")
	beforeRef, _ := cmd.Flags().GetString("before")
	first, _ := cmd.Flags().GetBool("first")
	if parentRef != "" && toRoot {
		return fmt.Errorf("--parent and --root are mutually exclusive")
	}
	if beforeRef != "" && first {
		return fmt.Errorf("--before and --first are mutually exclusive")
	}
	if parentRef == "" && !toRoot && beforeRef == "" && !first {
		return fmt.Errorf("nothing to move; pass --parent, --root, --before, or --first")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	doc, err := fetchDocument(ctx, client, args[0])
	if err != nil {
		return err
	}
	body := map[string]any{"position": documentMoveToEnd}
	switch {
	case toRoot:
		body["parent_issue_id"] = nil
	case parentRef != "":
		parent, err := fetchDocument(ctx, client, parentRef)
		if err != nil {
			return fmt.Errorf("--parent: %w", err)
		}
		body["parent_issue_id"] = strVal(parent, "id")
	default:
		// Reordering only: the move endpoint always writes the parent, so the
		// current one is sent back unchanged.
		if parent := strVal(doc, "parent_issue_id"); parent != "" {
			body["parent_issue_id"] = parent
		} else {
			body["parent_issue_id"] = nil
		}
	}
	switch {
	case first:
		body["position"] = documentMoveToStart
	case beforeRef != "":
		before, err := fetchDocument(ctx, client, beforeRef)
		if err != nil {
			return fmt.Errorf("--before: %w", err)
		}
		body["before_id"] = strVal(before, "id")
	}
	var moved map[string]any
	if err := client.PostJSON(ctx, "/api/documents/"+strVal(doc, "id")+"/move", body, &moved); err != nil {
		return fmt.Errorf("move document: %w", err)
	}
	return printDocumentResult(cmd, moved)
}

func runDocumentStatus(cmd *cobra.Command, args []string) error {
	action, ok := documentStatusActions[strings.ToLower(strings.TrimSpace(args[1]))]
	if !ok {
		return fmt.Errorf("status must be draft, reviewing, or published; got %q", args[1])
	}
	revision, err := requireExpectedRevision(cmd)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	doc, err := fetchDocument(ctx, client, args[0])
	if err != nil {
		return err
	}
	id := strVal(doc, "id")
	body := map[string]any{"action": action, "expected_document_revision": revision}
	var updated map[string]any
	if err := client.PostJSON(ctx, "/api/documents/"+id+"/transition", body, &updated); err != nil {
		if conflict := documentConflictError(ctx, client, id, revision, err); conflict != nil {
			return conflict
		}
		return documentStatusRequestError(err)
	}
	return printDocumentResult(cmd, updated)
}
