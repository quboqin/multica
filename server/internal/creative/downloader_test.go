package creative

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDownloaderAllowsExplicitTestHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-data"))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	downloader := NewDownloader(time.Second, 1024, []string{parsed.Hostname()})
	result, err := downloader.Fetch(context.Background(), server.URL+"/asset.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Data) != "png-data" || result.Extension != ".png" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDownloaderBlocksLoopbackByDefault(t *testing.T) {
	downloader := NewDownloader(time.Second, 1024, nil)
	_, err := downloader.Fetch(context.Background(), "http://127.0.0.1:9/private")
	if err == nil {
		t.Fatal("expected loopback URL to be blocked")
	}
}
