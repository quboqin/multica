package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCollectionRollbackFailsClosedForForcedRLSOwner(t *testing.T) {
	adminPool := openTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano(), rand.Uint32())
	schema := "migrate_collection_" + suffix
	role := "migrate_collection_owner_" + suffix
	schemaIdent := pgx.Identifier{schema}.Sanitize()
	roleIdent := pgx.Identifier{role}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schemaIdent); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := adminPool.Exec(ctx, "CREATE ROLE "+roleIdent+" NOLOGIN NOBYPASSRLS"); err != nil {
		t.Fatalf("create migration owner: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+schemaIdent+" CASCADE")
		_, _ = adminPool.Exec(cleanupCtx, "DROP OWNED BY "+roleIdent)
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+roleIdent)
	})

	pool := openTestPoolWithSearchPath(t, schema)
	versions := append([]string(nil), collectionMigrationVersions...)
	options := runOptions{
		Direction:             "up",
		Files:                 realMigrationFiles(t, versions, "up"),
		SchemaMigrationsTable: schema + ".schema_migrations",
		AdvisoryLockKey:       int64(rand.Uint64()&0x7fffffffffffffff) | 1,
		Hooks:                 hooksForDirection("up"),
	}
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatalf("apply collection migrations: %v", err)
	}

	workspaceID := "10000000-0000-4000-8000-000000000001"
	userID := "10000000-0000-4000-8000-000000000002"
	var collectionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO collection (
			workspace_id, name, created_by, create_request_id, create_fingerprint
		) VALUES ($1, 'Protected', $2, gen_random_uuid(), repeat('a', 64))
		RETURNING id
	`, workspaceID, userID).Scan(&collectionID); err != nil {
		t.Fatalf("seed protected collection: %v", err)
	}

	for _, table := range []string{"collection", "collection_field", "record", "schema_migrations"} {
		if _, err := adminPool.Exec(ctx, "ALTER TABLE "+schemaIdent+"."+pgx.Identifier{table}.Sanitize()+" OWNER TO "+roleIdent); err != nil {
			t.Fatalf("transfer %s ownership: %v", table, err)
		}
	}
	if _, err := adminPool.Exec(ctx, "ALTER SCHEMA "+schemaIdent+" OWNER TO "+roleIdent); err != nil {
		t.Fatalf("transfer schema ownership: %v", err)
	}

	ownerConfig, err := pgxpool.ParseConfig(testDatabaseURL())
	if err != nil {
		t.Fatal(err)
	}
	ownerConfig.ConnConfig.RuntimeParams["search_path"] = schema
	ownerConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+roleIdent)
		return err
	}
	ownerPool, err := pgxpool.NewWithConfig(ctx, ownerConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ownerPool.Close)
	if err := ownerPool.Ping(ctx); err != nil {
		t.Fatalf("ping migration-owner pool: %v", err)
	}

	var bypass bool
	if err := ownerPool.QueryRow(ctx, `SELECT rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass); err != nil {
		t.Fatal(err)
	}
	if bypass {
		t.Fatal("fixture migration owner unexpectedly bypasses RLS")
	}

	reversed := append([]string(nil), versions...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	options.Direction = "down"
	options.Files = realMigrationFiles(t, reversed, "down")
	options.Hooks = hooksForDirection("down")
	err = runMigrations(ctx, ownerPool, options)
	if err == nil || !strings.Contains(err.Error(), "inspect collection rollback ownership") {
		t.Fatalf("forced-RLS rollback error = %v, want fail-closed inspection error", err)
	}

	for _, table := range []string{"collection", "collection_field", "record"} {
		var rls, force bool
		if err := pool.QueryRow(ctx, `
			SELECT relrowsecurity, relforcerowsecurity
			FROM pg_class
			WHERE oid = $1::regclass
		`, schema+"."+table).Scan(&rls, &force); err != nil {
			t.Fatalf("inspect %s RLS: %v", table, err)
		}
		if !rls || !force {
			t.Fatalf("%s RLS changed after rejected rollback: enabled=%v force=%v", table, rls, force)
		}
	}
	assertSourceContextMigrationLedger(t, pool, schema, versions, true)
	for _, index := range []string{
		"collection_pkey", "collection_field_pkey", "record_pkey",
		"collection_create_uidx", "record_create_uidx", "collection_field_name_uidx",
		"collection_page_idx", "collection_field_page_idx", "record_page_idx",
	} {
		assertIndexExists(t, pool, schema, index, true)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM collection`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("protected data after rejected rollback: count=%d err=%v", count, err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM collection WHERE id = $1`, collectionID); err != nil {
		t.Fatalf("remove protected collection: %v", err)
	}
	// A globally-visible migration role can distinguish the now-empty tables
	// and complete the rollback. The non-bypass owner remains intentionally
	// fail-closed because PostgreSQL cannot prove that FORCE RLS hid no rows.
	if err := runMigrations(ctx, pool, options); err != nil {
		t.Fatalf("globally-visible empty collection rollback: %v", err)
	}
	assertSourceContextMigrationLedger(t, pool, schema, versions, false)
}
