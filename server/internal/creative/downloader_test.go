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

func TestDownloaderAllowsLoopbackWithoutHostConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-data"))
	}))
	defer server.Close()
	downloader := NewDownloader(time.Second, 1024)
	result, err := downloader.Fetch(context.Background(), server.URL+"/asset.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Data) != "png-data" || result.Extension != ".png" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDownloaderUsesProxyForInternalHostWithoutAllowlist(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "10.114.29.62" {
			t.Fatalf("proxy target host = %q", r.URL.Host)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("proxied-image"))
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)

	downloader := NewDownloader(time.Second, 1024)
	downloader.client.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
	result, err := downloader.Fetch(context.Background(), "http://10.114.29.62/image.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Data) != "proxied-image" {
		t.Fatalf("data = %q", result.Data)
	}
}

func TestDownloaderFollowsRedirectToDifferentLocalHost(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("redirected-image")) }))
	defer target.Close()
	location := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, location+"/private.png", http.StatusFound)
	}))
	defer server.Close()
	downloader := NewDownloader(time.Second, 1024)
	result, err := downloader.Fetch(context.Background(), server.URL+"/asset.png")
	if err != nil || string(result.Data) != "redirected-image" {
		t.Fatalf("redirect download=%+v %v", result, err)
	}
}

func TestDownloaderRetainsDownloadSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("too-large")) }))
	defer server.Close()
	_, err := NewDownloader(time.Second, 3).Fetch(t.Context(), server.URL+"/image.png")
	if err == nil || !strings.Contains(err.Error(), "exceeds 3 bytes") {
		t.Fatalf("size limit=%v", err)
	}
}
