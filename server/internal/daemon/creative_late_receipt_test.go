package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreativeImageTaskExpectedReceiptCountUsesProviderScope(t *testing.T) {
	tests := []struct {
		name    string
		context string
		want    int
	}{
		{
			name:    "production missing sizes override delivery sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_production","missing_sizes":["1200x628"],"expected_sizes":["1080x1080","1200x628","800x1000"]}`,
			want:    1,
		},
		{
			name:    "direct edit sizes override delivery sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_direct_edit","edit_sizes":["800x1000"],"expected_sizes":["1080x1080","1200x628","800x1000"]}`,
			want:    1,
		},
		{
			name:    "explicit empty provider scope invokes nothing",
			context: `{"type":"creative_domain_task","workflow":"creative_production","missing_sizes":[],"expected_sizes":["1080x1080","1200x628","800x1000"]}`,
			want:    0,
		},
		{
			name:    "legacy context falls back to expected sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_production","expected_sizes":["1080x1080","1200x628","800x1000"]}`,
			want:    3,
		},
		{
			name:    "late receipt continuation never invokes provider",
			context: `{"type":"creative_domain_task","workflow":"creative_production","missing_sizes":["1200x628"],"late_receipt_recovery":{"operation_id":"op"}}`,
			want:    0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := creativeImageTaskExpectedReceiptCount(json.RawMessage(test.context)); got != test.want {
				t.Fatalf("receipt count = %d, want %d", got, test.want)
			}
		})
	}
}

func TestReportAtomicCreativeImageReceiptUsesDaemonCredentialAndMarksReceipt(t *testing.T) {
	workDir := t.TempDir()
	imagePath := filepath.Join(workDir, "late.png")
	image := []byte("provider image bytes")
	if err := os.WriteFile(imagePath, image, 0o600); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(workDir, "image-edit-result-1080x1080.json")
	receipt := `{"operation_id":"operation-1","operation_attempt":2,"task_id":"task-1","output_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_asset":{"completed":true,"path":` + mustJSONQuote(t, imagePath) + `}}`
	if err := os.WriteFile(receiptPath, []byte(receipt), 0o600); err != nil {
		t.Fatal(err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		wantPath := "/api/daemon/runtimes/runtime-1/tasks/task-1/creative-image-operations/operation-1/attempts/2/late-success"
		if r.Method != http.MethodPost || r.URL.Path != wantPath {
			t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mdt_test" {
			t.Errorf("authorization = %q", got)
		}
		if err := r.ParseMultipartForm(maxUploadSizeForDaemonReceipt + maxCreativeLateReceiptJSON); err != nil {
			t.Errorf("parse multipart: %v", err)
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if got := r.FormValue("receipt"); got != receipt {
			t.Errorf("receipt changed: %s", got)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("image file: %v", err)
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		gotImage, _ := io.ReadAll(file)
		if string(gotImage) != string(image) {
			t.Errorf("image changed: %q", gotImage)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.SetToken("mdt_test")
	daemon := &Daemon{client: client}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	settled := daemon.reportAtomicCreativeImageReceipts(context.Background(), Task{ID: "task-1", RuntimeID: "runtime-1"}, workDir, logger)
	if settled != 1 || requests != 1 {
		t.Fatalf("settled/requests = %d/%d, want 1/1", settled, requests)
	}
	if _, err := os.Stat(receiptPath); !os.IsNotExist(err) {
		t.Fatalf("unreported receipt still exists: %v", err)
	}
	if _, err := os.Stat(receiptPath + ".reported"); err != nil {
		t.Fatalf("reported receipt marker: %v", err)
	}

	settled = daemon.reportAtomicCreativeImageReceipts(context.Background(), Task{ID: "task-1", RuntimeID: "runtime-1"}, workDir, logger)
	if settled != 1 || requests != 1 {
		t.Fatalf("re-scan settled/requests = %d/%d, want 1/1", settled, requests)
	}
}

func TestPathWithinCreativeReceiptRootRejectsSymlinkEscape(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.png")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workDir, "late.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if resolved, ok := pathWithinCreativeReceiptRoot(workDir, link); ok || strings.TrimSpace(resolved) != "" {
		t.Fatalf("symlink escape accepted as %q", resolved)
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
