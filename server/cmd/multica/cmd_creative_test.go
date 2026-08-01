package main

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstCreativeCandidateSourcePrefersArchivedAsset(t *testing.T) {
	candidate := creativeMaterialCandidateCLI{
		PreviewURL:  "https://cdn.example.test/preview.png",
		ResourceURL: "https://cdn.example.test/original.png",
		ArchivedURL: "https://multica.example.test/uploads/source.png",
	}
	if got := firstCreativeCandidateSource(candidate); got != candidate.ArchivedURL {
		t.Fatalf("firstCreativeCandidateSource() = %q, want %q", got, candidate.ArchivedURL)
	}
}

func TestValidateGPTImageSize(t *testing.T) {
	for _, size := range []string{"auto", "1024x1536", "1088x1360"} {
		if err := validateGPTImageSize(size); err != nil {
			t.Fatalf("validateGPTImageSize(%q): %v", size, err)
		}
	}
	for _, size := range []string{"1080x1350", "1025x1536", "4000x1024"} {
		if err := validateGPTImageSize(size); err == nil {
			t.Fatalf("validateGPTImageSize(%q) unexpectedly passed", size)
		}
	}
}

func TestNormalizedOpenAIImageBaseURL(t *testing.T) {
	got, err := normalizedOpenAIImageBaseURL("https://example.test/v1/")
	if err != nil || got != "https://example.test/v1/images/edits" {
		t.Fatalf("normalizedOpenAIImageBaseURL() = %q, %v", got, err)
	}
	if got, err := openAIImageEditEndpoint("http://one-ai.adakamicorp.id/", "/images/edits"); err != nil || got != "http://one-ai.adakamicorp.id/images/edits" {
		t.Fatalf("internal one-ai endpoint = %q, %v", got, err)
	}
	if got, err := openAIImageEditEndpoint("http://one-ai.adakamicorp.id", "custom/edits"); err != nil || got != "http://one-ai.adakamicorp.id/custom/edits" {
		t.Fatalf("custom edit path = %q, %v", got, err)
	}
	if _, err := normalizedOpenAIImageBaseURL("one-ai"); err == nil {
		t.Fatal("expected a bare host name to be rejected")
	}
}

func TestRequestGPTImageEdit(t *testing.T) {
	input := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(input, []byte("source-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "gpt-image-2" {
			t.Fatalf("model = %q", got)
		}
		if got := r.FormValue("size"); got != "1088x1360" {
			t.Fatalf("size = %q", got)
		}
		files := r.MultipartForm.File["image"]
		if len(files) != 1 || files[0].Filename != "reference.png" {
			t.Fatalf("image files = %#v", files)
		}
		if got := files[0].Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("image content type = %q", got)
		}
		opened, err := files[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		body, err := io.ReadAll(opened)
		if err != nil || string(body) != "source-bytes" {
			t.Fatalf("input body = %q, %v", body, err)
		}
		w.Header().Set("x-request-id", "req_image_test")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png-bytes")) + `"}]}`))
	}))
	defer server.Close()
	image, requestID, err := requestGPTImageEdit(context.Background(), server.Client(), server.URL+"/v1/images/edits", "test-key", "gpt-image-2", "image", []string{input}, "", "change the layout", "1088x1360", "medium")
	if err != nil || string(image) != "png-bytes" || requestID != "req_image_test" {
		t.Fatalf("requestGPTImageEdit() = %q, %q, %v", image, requestID, err)
	}
}

func TestImageEditFileContentType(t *testing.T) {
	cases := map[string]string{
		"reference.png":  "image/png",
		"reference.JPEG": "image/jpeg",
		"reference.webp": "image/webp",
		"reference.bin":  "application/octet-stream",
	}
	for path, want := range cases {
		if got := imageEditFileContentType(path); got != want {
			t.Errorf("imageEditFileContentType(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestTruncateCLIError(t *testing.T) {
	if got := truncateCLIError([]byte("  hello  "), 20); got != "hello" {
		t.Fatalf("truncateCLIError = %q", got)
	}
	if got := truncateCLIError([]byte(strings.Repeat("x", 10)), 4); got != "xxxx..." {
		t.Fatalf("truncateCLIError = %q", got)
	}
}
