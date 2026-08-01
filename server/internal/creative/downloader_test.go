package creative

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestDownloaderUsesProxyOnlyForAllowedHost(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "assets.example.com" {
			t.Fatalf("proxy target host = %q", r.URL.Host)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("proxied-image"))
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)

	downloader := NewDownloader(time.Second, 1024, []string{"assets.example.com"})
	downloader.allowedHostClient.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
	result, err := downloader.Fetch(context.Background(), "http://assets.example.com/image.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Data) != "proxied-image" {
		t.Fatalf("data = %q", result.Data)
	}
}

func TestDownloaderBlocksRedirectOutsideAllowedHosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://localhost:9/private", http.StatusFound)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)

	downloader := NewDownloader(time.Second, 1024, []string{parsed.Hostname()})
	_, err := downloader.Fetch(context.Background(), server.URL+"/asset.png")
	if err == nil || !strings.Contains(err.Error(), "redirect host") {
		t.Fatalf("Fetch error = %v", err)
	}
}
