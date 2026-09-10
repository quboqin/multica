package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestDeploymentCredentialMigrationChoosesNonOrphanCanonical(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "credential_migration_" + uuid.NewString()[:8]
	if _, err := tx.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE "user" (id UUID PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL);
		CREATE TABLE workspace (id UUID PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE member (
			workspace_id UUID NOT NULL,
			user_id UUID NOT NULL,
			role TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE credential_profile (
			id UUID PRIMARY KEY,
			workspace_id UUID NOT NULL,
			authorized_by_id UUID,
			connector_id TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			last_used_at TIMESTAMPTZ,
			expires_hint TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE UNIQUE INDEX credential_profile_workspace_connector_idx
			ON credential_profile(workspace_id, connector_id);
	`); err != nil {
		t.Fatal(err)
	}

	workspaceID := uuid.New()
	validProfileID := uuid.New()
	orphanProfileA := uuid.New()
	orphanProfileB := uuid.New()
	validUserID := uuid.New()
	orphanUserA := uuid.New()
	orphanUserB := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workspace (id, name) VALUES ($1, 'Live workspace')`, workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO "user" (id, name, email) VALUES
			($1, 'Live owner', 'live@example.test'),
			($2, 'Legacy A', 'legacy-a@example.test'),
			($3, 'Legacy B', 'legacy-b@example.test')
	`, validUserID, orphanUserA, orphanUserB); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, workspaceID, validUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO credential_profile (
			id, workspace_id, authorized_by_id, connector_id, label, status, last_used_at, updated_at
		) VALUES
			($1, $2, $3, 'appgrowing', 'live', 'active', now() - interval '1 day', now() - interval '1 day'),
			($4, $5, $6, 'appgrowing', 'orphan-newest', 'active', now(), now()),
			($7, $8, $9, 'appgrowing', 'orphan-second', 'active', now() - interval '1 hour', now() - interval '1 hour')
	`, validProfileID, workspaceID, validUserID, orphanProfileA, uuid.New(), orphanUserA, orphanProfileB, uuid.New(), orphanUserB); err != nil {
		t.Fatal(err)
	}

	migrationPath := filepath.Join("..", "..", "migrations", "263_deployment_shared_credentials.up.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("migration failed against duplicate and orphan profiles: %v", err)
	}

	var canonicalID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM credential_profile
		WHERE connector_id = 'appgrowing' AND scope = 'deployment' AND status <> 'revoked'
		LIMIT 1
	`).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	var deploymentCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM credential_profile
		WHERE connector_id = 'appgrowing' AND scope = 'deployment' AND status <> 'revoked'
	`).Scan(&deploymentCount); err != nil {
		t.Fatal(err)
	}
	if canonicalID != validProfileID || deploymentCount != 1 {
		t.Fatalf("canonical = %s count=%d, want live profile %s", canonicalID, deploymentCount, validProfileID)
	}

	var revokedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM credential_profile
		WHERE id IN ($1, $2) AND status = 'revoked'
	`, orphanProfileA, orphanProfileB).Scan(&revokedCount); err != nil {
		t.Fatal(err)
	}
	if revokedCount != 2 {
		t.Fatalf("revoked orphan count = %d, want 2", revokedCount)
	}

	var managerCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM credential_profile_manager WHERE profile_id = $1
	`, validProfileID).Scan(&managerCount); err != nil {
		t.Fatal(err)
	}
	if managerCount != 3 {
		t.Fatalf("canonical manager count = %d, want 3 historical authorizers", managerCount)
	}
}
