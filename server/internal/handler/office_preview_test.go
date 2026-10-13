package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestOfficePreviewConversion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"pdf", 200, "%PDF-1.7\npreview", false},
		{"invalid body", 200, "<html>failure</html>", true},
		{"converter error", 503, "busy", true},
		{"redirect", 302, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			converter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/forms/libreoffice/convert" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("credentials forwarded")
				}
				file, header, err := r.FormFile("files")
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer file.Close()
				body, _ := io.ReadAll(file)
				if header.Filename != "document.pptx" || string(body) != "slides" {
					t.Errorf("unexpected upload %s %q", header.Filename, body)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer converter.Close()
			result, err := convertOfficePreview(context.Background(), converter.URL, ".pptx", []byte("slides"))
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected conversion error: %v", err)
			}
			if !tc.wantError && string(result) != tc.body {
				t.Fatalf("unexpected PDF %q", result)
			}
		})
	}
}

func TestOfficePreviewEndpoint(t *testing.T) {
	store := &mockStorage{}
	originalStorage, originalURL := testHandler.Storage, testHandler.cfg.OfficePreviewURL
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage; testHandler.cfg.OfficePreviewURL = originalURL })
	converter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.7\npreview")
	}))
	defer converter.Close()
	testHandler.cfg.OfficePreviewURL = converter.URL
	for _, ext := range []string{"doc", "docx", "ppt", "pptx"} {
		t.Run(ext, func(t *testing.T) {
			key := "workspaces/" + testWorkspaceID + "/preview." + ext
			url, err := store.Upload(context.Background(), key, []byte("office"), "application/octet-stream", "preview."+ext)
			if err != nil {
				t.Fatal(err)
			}
			id := dbfx.Insert(t, "attachment", testutil.Cols{"workspace_id": testWorkspaceID, "uploader_type": "member", "uploader_id": testUserID, "filename": "preview." + ext, "url": url, "content_type": "application/octet-stream", "size_bytes": 6})
			req, _ := newPreviewRequest(t, id, testWorkspaceID)
			w := testutil.Call(t, testHandler.GetAttachmentOfficePreview, req).Want(http.StatusOK).ResponseRecorder
			if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("invalid preview %s", w.Body.String())
			}
			req, w = newPreviewRequest(t, id, "00000000-0000-0000-0000-000000000001")
			testHandler.GetAttachmentOfficePreview(w, req)
			if w.Code == http.StatusOK {
				t.Fatal("cross-workspace preview allowed")
			}
			testHandler.cfg.OfficePreviewURL = ""
			req, _ = newPreviewRequest(t, id, testWorkspaceID)
			testutil.Call(t, testHandler.GetAttachmentOfficePreview, req).Want(http.StatusServiceUnavailable)
			testHandler.cfg.OfficePreviewURL = converter.URL
		})
	}
}

func TestOfficePreviewRejectsPrivateDocumentAndOversizedFiles(t *testing.T) {
	store := &mockStorage{}
	originalStorage, originalURL := testHandler.Storage, testHandler.cfg.OfficePreviewURL
	testHandler.Storage, testHandler.cfg.OfficePreviewURL = store, "http://127.0.0.1:1"
	t.Cleanup(func() { testHandler.Storage, testHandler.cfg.OfficePreviewURL = originalStorage, originalURL })
	doc := createTestDocument(t, "")
	viewer := dbfx.User(t, "Preview reader", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, viewer, "member")
	id := dbfx.Insert(t, "attachment", testutil.Cols{"workspace_id": testWorkspaceID, "issue_id": doc.ID, "uploader_type": "member", "uploader_id": testUserID, "filename": "secret.docx", "url": "/uploads/secret.docx", "content_type": "application/octet-stream", "size_bytes": maxOfficePreviewSize + 1})
	req, _ := newPreviewRequest(t, id, testWorkspaceID)
	req.Header.Set("X-User-ID", viewer)
	testutil.Call(t, testHandler.GetAttachmentOfficePreview, req).Want(http.StatusNotFound)
	req, _ = newPreviewRequest(t, id, testWorkspaceID)
	testutil.Call(t, testHandler.GetAttachmentOfficePreview, req).Want(http.StatusRequestEntityTooLarge)
	req, _ = newPreviewRequest(t, "not-a-uuid", testWorkspaceID)
	testutil.Call(t, testHandler.GetAttachmentOfficePreview, req).Want(http.StatusBadRequest)
}
