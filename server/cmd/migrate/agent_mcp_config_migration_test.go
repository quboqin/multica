package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRetireWorkspaceMCPMigrationPreservesAgentMCPConfig(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	schema := fmt.Sprintf("mcp_retire_test_%d_%d", time.Now().UnixNano(), rand.Uint32())

	if _, err := pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", pgx.Identifier{schema}.Sanitize())); err != nil {
			t.Logf("drop schema %s: %v", schema, err)
		}
	})

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := conn.Exec(ctx, `
CREATE TABLE agent (
    id INTEGER PRIMARY KEY,
    mcp_config JSONB,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE workspace_mcp_connection (id INTEGER PRIMARY KEY);
INSERT INTO agent (id, mcp_config)
VALUES (1, '{"mcpServers":{"existing":{"command":"npx"}}}'::jsonb);
`); err != nil {
		t.Fatalf("seed migration schema: %v", err)
	}

	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "248_retire_workspace_and_agent_mcp.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := conn.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	var mcpConfig string
	if err := conn.QueryRow(ctx, "SELECT mcp_config::text FROM agent WHERE id = 1").Scan(&mcpConfig); err != nil {
		t.Fatalf("read preserved agent MCP config: %v", err)
	}
	if mcpConfig != `{"mcpServers": {"existing": {"command": "npx"}}}` {
		t.Fatalf("agent mcp_config = %s, want preserved JSON", mcpConfig)
	}

	var workspaceTableExists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('workspace_mcp_connection') IS NOT NULL").Scan(&workspaceTableExists); err != nil {
		t.Fatalf("check workspace MCP table: %v", err)
	}
	if workspaceTableExists {
		t.Fatal("workspace MCP table still exists after retirement migration")
	}
}

func TestRemoveWorkspaceMCPRefsMigrationPreservesAgentServers(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	schema := fmt.Sprintf("mcp_refs_test_%d_%d", time.Now().UnixNano(), rand.Uint32())

	if _, err := pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", pgx.Identifier{schema}.Sanitize())); err != nil {
			t.Logf("drop schema %s: %v", schema, err)
		}
	})

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := conn.Exec(ctx, `
CREATE TABLE agent (
    id INTEGER PRIMARY KEY,
    mcp_config JSONB,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO agent (id, mcp_config)
VALUES
    (1, '{"mcpServers":{"github":{"command":"npx"}},"workspaceMcpRefs":[{"connectionId":"camel"}],"workspace_mcp_refs":[{"connection_id":"snake"}],"runtimeOption":true}'::jsonb),
    (2, '{"workspaceMcpRefs":[{"connectionId":"only-workspace"}]}'::jsonb);
`); err != nil {
		t.Fatalf("seed migration schema: %v", err)
	}

	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "272_remove_workspace_mcp_refs.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := conn.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	var raw []byte
	if err := conn.QueryRow(ctx, "SELECT mcp_config FROM agent WHERE id = 1").Scan(&raw); err != nil {
		t.Fatalf("read cleaned agent MCP config: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode cleaned agent MCP config: %v", err)
	}
	want := map[string]any{
		"mcpServers": map[string]any{
			"github": map[string]any{"command": "npx"},
		},
		"runtimeOption": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleaned agent mcp_config = %#v, want %#v", got, want)
	}

	var configCleared bool
	if err := conn.QueryRow(ctx, "SELECT mcp_config IS NULL FROM agent WHERE id = 2").Scan(&configCleared); err != nil {
		t.Fatalf("read workspace-only agent MCP config: %v", err)
	}
	if !configCleared {
		t.Fatal("workspace-only mcp_config was not cleared")
	}
}
