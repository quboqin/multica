package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceCapabilitiesCreativeFactorySquad(t *testing.T) {
	ctx := t.Context()
	var workspaceID, squadID, agentID string
	if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug, issue_prefix) VALUES ('Factory binding test', 'factory-binding-test', 'FBT') RETURNING id::text`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID); err != nil {
			t.Errorf("clean up factory workspace: %v", err)
		}
	})
	if err := testPool.QueryRow(ctx, `INSERT INTO agent (workspace_id, name, owner_id, runtime_mode, runtime_config, runtime_id) VALUES ($1, 'Design leader', $2, 'local', '{}', $3) RETURNING id::text`, workspaceID, testUserID, testRuntimeID).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO squad (workspace_id, name, creator_id, leader_id) VALUES ($1, 'Renamed design team', $2, $3) RETURNING id::text`, workspaceID, testUserID, agentID).Scan(&squadID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO creative_factory_installation (workspace_id, squad_id) VALUES ($1, $2)`, workspaceID, squadID); err != nil {
		t.Fatal(err)
	}
	read := func(id string) workspaceCapabilitiesResponse {
		t.Helper()
		req := withURLParam(newRequest(http.MethodGet, "/api/workspaces/"+id+"/capabilities", nil), "id", id)
		w := httptest.NewRecorder()
		testHandler.ListWorkspaceCapabilities(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("capabilities: %d %s", w.Code, w.Body.String())
		}
		var result workspaceCapabilitiesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := read(workspaceID).CreativeFactorySquadID; got != squadID {
		t.Fatalf("binding = %q, want %q", got, squadID)
	}
	if got := read(testWorkspaceID).CreativeFactorySquadID; got == squadID {
		t.Fatal("factory binding leaked into another workspace")
	}
	if _, err := testPool.Exec(ctx, `DELETE FROM creative_factory_installation WHERE workspace_id = $1`, workspaceID); err != nil {
		t.Fatal(err)
	}
	if got := read(workspaceID).CreativeFactorySquadID; got != "" {
		t.Fatalf("uninstalled workspace binding = %q", got)
	}
}
