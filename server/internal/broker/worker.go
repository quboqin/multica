package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type WorkerClient interface {
	Configured() bool
	StartLoginSession(ctx context.Context, req WorkerLoginSessionRequest) (WorkerLoginSessionResponse, error)
	RunCrawl(ctx context.Context, req WorkerCrawlRequest) (WorkerCrawlResponse, error)
}

type WorkerLoginSessionRequest struct {
	ProfileID    string `json:"profile_id"`
	ConnectorID  string `json:"connector_id"`
	SessionToken string `json:"session_token"`
	LoginURL     string `json:"login_url"`
}

type WorkerLoginSessionResponse struct {
	BrowserURL       string `json:"browser_url"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type WorkerCrawlRequest struct {
	ProfileID   string          `json:"profile_id"`
	ConnectorID string          `json:"connector_id"`
	Capability  string          `json:"capability"`
	Params      json.RawMessage `json:"params"`
}

type WorkerCrawlResponse struct {
	Status       string          `json:"status"`
	Downloaded   int             `json:"downloaded"`
	OutputPrefix string          `json:"output_prefix"`
	Message      string          `json:"message"`
	Raw          json.RawMessage `json:"raw,omitempty"`
}

type disabledWorkerClient struct{}

func NewDisabledWorkerClient() WorkerClient {
	return disabledWorkerClient{}
}

func (disabledWorkerClient) Configured() bool { return false }

func (disabledWorkerClient) StartLoginSession(context.Context, WorkerLoginSessionRequest) (WorkerLoginSessionResponse, error) {
	return WorkerLoginSessionResponse{}, ErrWorkerNotConfigured
}

func (disabledWorkerClient) RunCrawl(context.Context, WorkerCrawlRequest) (WorkerCrawlResponse, error) {
	return WorkerCrawlResponse{}, ErrWorkerNotConfigured
}

type HTTPWorkerClient struct {
	BaseURL     string
	Client      *http.Client
	CrawlClient *http.Client
}

func NewHTTPWorkerClient(baseURL string, timeout time.Duration) WorkerClient {
	return NewHTTPWorkerClientWithTimeouts(baseURL, timeout, timeout)
}

func NewHTTPWorkerClientWithTimeouts(baseURL string, timeout time.Duration, crawlTimeout time.Duration) WorkerClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return NewDisabledWorkerClient()
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if crawlTimeout <= 0 {
		crawlTimeout = timeout
	}
	return &HTTPWorkerClient{
		BaseURL:     baseURL,
		Client:      &http.Client{Timeout: timeout},
		CrawlClient: &http.Client{Timeout: crawlTimeout},
	}
}

func (c *HTTPWorkerClient) Configured() bool {
	return c != nil && c.BaseURL != ""
}

func (c *HTTPWorkerClient) StartLoginSession(ctx context.Context, req WorkerLoginSessionRequest) (WorkerLoginSessionResponse, error) {
	var out WorkerLoginSessionResponse
	if err := c.post(ctx, c.Client, "/login-sessions", req, &out); err != nil {
		return WorkerLoginSessionResponse{}, err
	}
	return out, nil
}

func (c *HTTPWorkerClient) RunCrawl(ctx context.Context, req WorkerCrawlRequest) (WorkerCrawlResponse, error) {
	var out WorkerCrawlResponse
	if err := c.post(ctx, c.CrawlClient, "/crawl", req, &out); err != nil {
		return WorkerCrawlResponse{}, err
	}
	return out, nil
}

func (c *HTTPWorkerClient) post(ctx context.Context, client *http.Client, path string, in any, out any) error {
	if !c.Configured() {
		return ErrWorkerNotConfigured
	}
	if client == nil {
		client = c.Client
	}
	if client == nil {
		client = http.DefaultClient
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Error == "" {
			body.Error = resp.Status
		}
		return fmt.Errorf("credential worker: %s", body.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
