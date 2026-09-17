package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestIssueKindValidationAndRollback(t *testing.T) {
	admin := openTestPool(t)
	ctx := context.Background()
	schema := fmt.Sprintf("migrate_issue_kind_%d", rand.Uint64())
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+ident+" CASCADE") })
	pool := openTestPoolWithSearchPath(t, schema)
	if _, err := pool.Exec(ctx, `CREATE TABLE issue (id int, workspace_id int, created_at timestamptz); INSERT INTO issue VALUES (1, 1, now())`); err != nil {
		t.Fatal(err)
	}
	options := runOptions{Direction: "up", SchemaMigrationsTable: schema + ".schema_migrations", AdvisoryLockKey: int64(rand.Uint64()&0x7fffffffffffffff) | 1}
	apply := func(direction string, versions ...string) error {
		options.Direction = direction
		options.Files = realMigrationFiles(t, versions, direction)
		return runMigrations(ctx, pool, options)
	}
	if err := apply("up", "478_issue_kind"); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err := pool.QueryRow(ctx, "SELECT convalidated FROM pg_constraint WHERE conrelid='issue'::regclass AND conname='issue_kind_check'").Scan(&valid); err != nil || valid {
		t.Fatalf("constraint should defer old-row scan: valid=%v err=%v", valid, err)
	}
	var kind string
	if err := pool.QueryRow(ctx, "SELECT kind FROM issue WHERE id=1").Scan(&kind); err != nil || kind != "task" {
		t.Fatalf("legacy row kind=%q err=%v", kind, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO issue (kind) VALUES ('invalid')"); err == nil {
		t.Fatal("NOT VALID must still reject invalid new writes")
	}
	if err := apply("up", "479_issue_doc_page_index", "480_validate_issue_kind"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT convalidated FROM pg_constraint WHERE conrelid='issue'::regclass AND conname='issue_kind_check'").Scan(&valid); err != nil || !valid {
		t.Fatalf("constraint not validated: %v %v", valid, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO issue (kind) VALUES ('doc')"); err != nil {
		t.Fatal(err)
	}
	if err := apply("down", "480_validate_issue_kind", "479_issue_doc_page_index", "478_issue_kind"); err == nil || !strings.Contains(err.Error(), "non-task data exists") {
		t.Fatalf("rollback must protect documents: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM issue WHERE kind='doc'"); err != nil {
		t.Fatal(err)
	}
	if err := apply("down", "478_issue_kind"); err != nil {
		t.Fatal(err)
	}
	if err := apply("up", "478_issue_kind", "479_issue_doc_page_index", "480_validate_issue_kind"); err != nil {
		t.Fatal(err)
	}
}
