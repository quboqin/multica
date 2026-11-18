package migrations

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestMigrationNumericPrefixesAreUnique(t *testing.T) {
	files := migrationFilesForLint(t, "*.up.sql")

	// Migrations through 128 contain historical duplicate numeric prefixes.
	// From 129 onward, keep the numeric sequence unique so release tooling and
	// operators can identify one schema change unambiguously by its number.
	const firstUniqueMigrationNumber = 129

	// These fork migrations shipped before the upstream merge. The runner
	// records full filename stems, so renumbering would apply deployed DDL twice.
	// Keep this exact allowlist frozen; all new prefixes remain unique.
	historicalForkMigrations := map[string]bool{
		"564_cortex_documents":               true,
		"565_document_tree_index":            true,
		"566_document_publication_index":     true,
		"567_cortex_collections":             true,
		"568_collection_id_index":            true,
		"569_collection_field_id_index":      true,
		"570_record_id_index":                true,
		"571_record_page_index":              true,
		"572_collection_field_name_index":    true,
		"573_document_tree_guard":            true,
		"574_collection_title_name":          true,
		"575_record_link":                    true,
		"576_record_link_id_index":           true,
		"577_record_link_edge_index":         true,
		"578_record_link_target_index":       true,
		"579_collection_field_relation_type": true,
		"580_collection_formula":             true,
		"581_document_sharing_versions":      true,
		"582_document_access_index":          true,
		"583_document_collaborator_index":    true,
		"584_document_version_index":         true,
		"585_document_private_lifecycle":     true,
		"586_collection_sharing":             true,
		"587_collection_access_index":        true,
		"588_collection_collaborator_index":  true,
		"589_comment_agent_grant":            true,
		"590_collection_record_parent":       true,
		"591_collection_record_parent_index": true,
		"592_collection_email_field":         true,
	}
	stemByNumber := make(map[int]string)
	for _, file := range files {
		stem, _, ok := splitMigrationFilename(filepath.Base(file))
		if !ok {
			continue
		}
		if historicalForkMigrations[stem] {
			continue
		}
		prefix, _, ok := strings.Cut(stem, "_")
		if !ok {
			continue
		}
		number, err := strconv.Atoi(prefix)
		if err != nil || number < firstUniqueMigrationNumber {
			continue
		}
		if previous, exists := stemByNumber[number]; exists {
			t.Errorf("migrations %s and %s share numeric prefix %s", previous, stem, prefix)
			continue
		}
		stemByNumber[number] = stem
	}
}

func TestMigrationFilesHaveMatchingDirections(t *testing.T) {
	files := migrationFilesForLint(t, "*.sql")

	directionsByStem := make(map[string]map[string]bool)
	for _, file := range files {
		stem, direction, ok := splitMigrationFilename(filepath.Base(file))
		if !ok {
			continue
		}
		if directionsByStem[stem] == nil {
			directionsByStem[stem] = make(map[string]bool)
		}
		directionsByStem[stem][direction] = true
	}

	for stem, directions := range directionsByStem {
		if !directions["up"] || !directions["down"] {
			t.Errorf("migration %s must have both .up.sql and .down.sql files", stem)
		}
	}
}

func migrationFilesForLint(t *testing.T, pattern string) []string {
	t.Helper()

	dir := realMigrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no migration files matched %s in %s", pattern, dir)
	}
	sort.Strings(files)
	return files
}

func realMigrationsDir(t *testing.T) string {
	t.Helper()

	// Relative to the package directory go test runs in; runtime.Caller
	// would return a module path under -trimpath.
	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	return dir
}

func splitMigrationFilename(name string) (stem, direction string, ok bool) {
	for _, candidateDirection := range []string{"up", "down"} {
		suffix := fmt.Sprintf(".%s.sql", candidateDirection)
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), candidateDirection, true
		}
	}
	return "", "", false
}
