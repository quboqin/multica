package creative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMCPProviderCreateGetAndVerify(t *testing.T) {
	var mu sync.Mutex
	toolCalls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer workspace-secret" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("MCP-Protocol-Version") != mcpProtocolVersion {
			t.Errorf("MCP-Protocol-Version = %q", r.Header.Get("MCP-Protocol-Version"))
		}
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			writeMCPTestResult(t, w, request.ID, map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "creative-test", "version": "1"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeMCPTestResult(t, w, request.ID, map[string]any{"tools": []map[string]any{
				{"name": CreateJobToolName},
				{"name": GetJobToolName},
			}})
		case "tools/call":
			name, _ := request.Params["name"].(string)
			mu.Lock()
			toolCalls = append(toolCalls, name)
			mu.Unlock()
			var payload map[string]any
			switch name {
			case CreateJobToolName:
				arguments := request.Params["arguments"].(map[string]any)
				if arguments["idempotency_key"] != "local-job-1" {
					t.Errorf("idempotency_key = %v", arguments["idempotency_key"])
				}
				payload = map[string]any{
					"job_id":        "remote-job-1",
					"status":        "running",
					"progress":      10,
					"poll_after_ms": 250,
					"process_data": map[string]any{
						"schema_version": "1",
						"summary":        map[string]any{"submission_count": 1},
					},
				}
			case GetJobToolName:
				payload = map[string]any{
					"status":       "completed",
					"progress":     100,
					"process_data": map[string]any{"quality_summary": map[string]any{"pass_rate": 1.0}},
					"variants": []map[string]any{{
						"candidate_id":  "candidate-1",
						"variant_index": 1,
						"title":         "Variant 1",
						"qc_status":     "passed",
						"assets": []map[string]any{{
							"width":        1080,
							"height":       1080,
							"label":        "1080x1080",
							"asset_url":    "https://cdn.example.com/creative.png",
							"content_type": "image/png",
						}},
					}},
				}
			default:
				http.Error(w, "unknown tool", http.StatusBadRequest)
				return
			}
			structured, _ := json.Marshal(payload)
			writeMCPTestResult(t, w, request.ID, map[string]any{
				"content":           []map[string]any{{"type": "text", "text": string(structured)}},
				"structuredContent": payload,
			})
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	provider, err := NewMCPProvider(MCPProviderConfig{
		ConnectionID: "connection-1",
		ServerURL:    server.URL,
		Headers:      map[string]string{"Authorization": "Bearer workspace-secret"},
		AllowedHosts: []string{"127.0.0.1"},
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewMCPProvider: %v", err)
	}
	tools, err := provider.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %v", tools)
	}
	created, err := provider.CreateJob(context.Background(), CreateJobRequest{
		LocalJobID: "local-job-1",
		Prompt:     "keep layout",
		Rules:      map[string]any{"variant_count": 1},
		Candidates: []Candidate{{ID: "candidate-1", ArchivedURL: "https://cdn.example.com/source.png"}},
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if created.ExternalJobID != "remote-job-1" || created.PollAfter != 250*time.Millisecond {
		t.Fatalf("created = %+v", created)
	}
	var createdProcess map[string]any
	if err := json.Unmarshal(created.ProcessData, &createdProcess); err != nil || createdProcess["schema_version"] != "1" {
		t.Fatalf("created process_data = %s, err = %v", created.ProcessData, err)
	}
	completed, err := provider.GetJob(context.Background(), GetJobRequest{ExternalJobID: created.ExternalJobID})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if completed.Status != "completed" || len(completed.Variants) != 1 || len(completed.Variants[0].Assets) != 1 {
		t.Fatalf("completed = %+v", completed)
	}
	var completedProcess map[string]any
	if err := json.Unmarshal(completed.ProcessData, &completedProcess); err != nil {
		t.Fatalf("decode completed process_data: %v", err)
	}
	quality, _ := completedProcess["quality_summary"].(map[string]any)
	if quality["pass_rate"] != float64(1) {
		t.Fatalf("completed process_data = %v", completedProcess)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(toolCalls) != 2 || toolCalls[0] != CreateJobToolName || toolCalls[1] != GetJobToolName {
		t.Fatalf("tool calls = %v", toolCalls)
	}
}

func TestParseMCPProcessDataRejectsInvalidAndOversizedPayloads(t *testing.T) {
	if _, err := parseMCPProcessData(map[string]any{"process_data": "not-an-object"}); err == nil {
		t.Fatal("expected non-object process_data to fail")
	}
	if _, err := parseMCPProcessData(map[string]any{
		"process_data": map[string]any{"value": strings.Repeat("x", 512*1024)},
	}); err == nil {
		t.Fatal("expected oversized process_data to fail")
	}
}

func writeMCPTestResult(t *testing.T, w http.ResponseWriter, id any, result any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}); err != nil {
		t.Errorf("encode MCP response: %v", err)
	}
}

func TestMCPProviderBlocksPrivateHostsWithoutAllowlist(t *testing.T) {
	provider, err := NewMCPProvider(MCPProviderConfig{
		ServerURL: "http://127.0.0.1:9/mcp",
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("NewMCPProvider: %v", err)
	}
	_, err = provider.Verify(context.Background())
	if err == nil {
		t.Fatal("expected private host to be blocked")
	}
}
