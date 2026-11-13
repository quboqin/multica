package main

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectionUpgradePreservesLegacyDataAndRollbackValues(t *testing.T) {
	admin := openTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schema := createScratchSchema(t, ctx, admin, "migrate_collection_compat_")
	pool := openTestPoolWithSearchPath(t, schema)
	if _, err := pool.Exec(ctx, "CREATE TABLE issue_view (id UUID)"); err != nil {
		t.Fatal(err)
	}
	options := runOptions{
		Direction:             "up",
		Files:                 realMigrationFiles(t, []string{"567_cortex_collections", "579_collection_field_relation_type", "580_collection_formula"}, "up"),
		SchemaMigrationsTable: schema + ".schema_migrations",
		AdvisoryLockKey:       int64(rand.Uint64()&0x7fffffffffffffff) | 1,
		Hooks:                 hooksForDirection("up"),
	}
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	const legacyFields = `{"text":"客户, Oracle","number":0,"checkbox":false,"url":"https://example.com/a?b=c","select":"option-1","multi_select":["option-1","option-2"],"date":"2026-11-13","actor":"member:11111111-1111-4111-8111-111111111111","multi_actor":["member:11111111-1111-4111-8111-111111111111"]}`
	if _, err := pool.Exec(ctx, `INSERT INTO record(id,workspace_id,collection_id,title,fields,revision)
 VALUES(md5('legacy')::uuid,md5('workspace')::uuid,md5('collection')::uuid,'Oracle',$1::jsonb,7)`, legacyFields); err != nil {
		t.Fatal(err)
	}
	versions := []string{"590_collection_record_parent", "591_collection_record_parent_index", "592_collection_email_field"}
	options.Files = realMigrationFiles(t, versions, "up")
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	assertIndexValidity(t, pool, schema, "record_parent_index", true)
	var unchanged bool
	if err := pool.QueryRow(ctx, `SELECT fields=$1::jsonb AND title='Oracle' AND revision=7 AND parent_id IS NULL FROM record WHERE id=md5('legacy')::uuid`, legacyFields).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("upgrade changed a legacy record: unchanged=%v err=%v", unchanged, err)
	}
	// Old backend writes omit the new nullable column and keep their existing
	// explicit projections. Exercise those shapes against the upgraded schema.
	var title string
	if err := pool.QueryRow(ctx, `UPDATE record SET title='Legacy writer',revision=revision+1 WHERE id=md5('legacy')::uuid RETURNING title`).Scan(&title); err != nil || title != "Legacy writer" {
		t.Fatalf("legacy update: %q, %v", title, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO record(workspace_id,collection_id,title,fields) VALUES(md5('workspace')::uuid,md5('collection')::uuid,'Legacy new row','{}')`); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	for _, statement := range []string{
		`INSERT INTO collection_field(id,workspace_id,collection_id,name,type) VALUES(md5('email')::uuid,md5('workspace')::uuid,md5('collection')::uuid,'Email','email')`,
		`INSERT INTO record(id,workspace_id,collection_id,parent_id,title,fields) VALUES(md5('child')::uuid,md5('workspace')::uuid,md5('collection')::uuid,md5('legacy')::uuid,'Child',jsonb_build_object(md5('email')::uuid::text,'person@example.com'))`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	checkSQL, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "check-collection-db-compat.sql"))
	if err != nil {
		t.Fatal(err)
	}
	assertCompatible := func(want bool) {
		t.Helper()
		var raw []byte
		if err := pool.QueryRow(ctx, string(checkSQL)).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Compatible     bool `json:"compatible"`
			InvalidParents int  `json:"invalid_parents"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.Compatible != want {
			t.Fatalf("deployment compatibility check: %s, want compatible=%v, err=%v", raw, want, err)
		}
	}
	assertCompatible(true)
	if _, err := pool.Exec(ctx, `UPDATE record SET parent_id=md5('missing')::uuid WHERE id=md5('child')::uuid`); err != nil {
		t.Fatal(err)
	}
	assertCompatible(false)
	if _, err := pool.Exec(ctx, `UPDATE record SET parent_id=md5('legacy')::uuid WHERE id=md5('child')::uuid`); err != nil {
		t.Fatal(err)
	}
	// Re-running an already recorded upgrade must not rewrite user data.
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT fields=$1::jsonb AND revision=8 FROM record WHERE id=md5('legacy')::uuid`, legacyFields).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("retry changed values: %v, %v", unchanged, err)
	}
	options.Direction = "down"
	options.Hooks = hooksForDirection("down")
	options.Files = realMigrationFiles(t, []string{versions[2], versions[1], versions[0]}, "down")
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	assertIndexExists(t, pool, schema, "record_parent_index", false)
	var fieldType, email string
	var count int
	if err := pool.QueryRow(ctx, `SELECT type FROM collection_field WHERE id=md5('email')::uuid`).Scan(&fieldType); err != nil || fieldType != "text" {
		t.Fatalf("rollback email type: %q, %v", fieldType, err)
	}
	if err := pool.QueryRow(ctx, `SELECT fields->>(md5('email')::uuid::text) FROM record WHERE id=md5('child')::uuid`).Scan(&email); err != nil || email != "person@example.com" {
		t.Fatalf("rollback email value: %q, %v", email, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM record`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("rollback lost records: %d, %v", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT fields=$1::jsonb FROM record WHERE id=md5('legacy')::uuid`, legacyFields).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("rollback lost legacy values: %v, %v", unchanged, err)
	}
	options.Direction, options.Hooks, options.Files = "up", hooksForDirection("up"), realMigrationFiles(t, versions, "up")
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	assertIndexValidity(t, pool, schema, "record_parent_index", true)
	assertCompatible(true)
}
