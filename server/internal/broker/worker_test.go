package broker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPWorkerClientRunCrawlClassifiesWorkerBusy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl" {
			t.Fatalf("path = %s, want /crawl", r.URL.Path)
		}
		writeWorkerTestJSON(w, http.StatusTooManyRequests, `{"error":"crawler worker is busy; wait for the active browser task to finish"}`)
	}))
	defer server.Close()

	client := NewHTTPWorkerClientWithTimeouts(server.URL, time.Second, time.Second)
	_, err := client.RunCrawl(context.Background(), WorkerCrawlRequest{ProfileID: "profile", ConnectorID: "appgrowing"})
	if !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("RunCrawl error = %v, want ErrWorkerBusy", err)
	}
}

func TestHTTPWorkerClientRunCrawlClassifiesWorkerUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeWorkerTestJSON(w, http.StatusInternalServerError, `{"error":"browser launch failed"}`)
	}))
	defer server.Close()

	client := NewHTTPWorkerClientWithTimeouts(server.URL, time.Second, time.Second)
	_, err := client.RunCrawl(context.Background(), WorkerCrawlRequest{ProfileID: "profile", ConnectorID: "appgrowing"})
	if !errors.Is(err, ErrWorkerUnavailable) {
		t.Fatalf("RunCrawl error = %v, want ErrWorkerUnavailable", err)
	}
}

func TestHTTPWorkerClientRunCrawlClassifiesClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		writeWorkerTestJSON(w, http.StatusOK, `{}`)
	}))
	defer server.Close()

	client := NewHTTPWorkerClientWithTimeouts(server.URL, time.Second, 10*time.Millisecond)
	_, err := client.RunCrawl(context.Background(), WorkerCrawlRequest{ProfileID: "profile", ConnectorID: "appgrowing"})
	if !errors.Is(err, ErrWorkerTimeout) {
		t.Fatalf("RunCrawl error = %v, want ErrWorkerTimeout", err)
	}
}

func writeWorkerTestJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
