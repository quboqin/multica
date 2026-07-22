package creative

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	maxMCPResponseBytes = 8 << 20
	mcpProtocolVersion  = "2025-03-26"
)

type MCPProviderConfig struct {
	ConnectionID string
	ServerURL    string
	Headers      map[string]string
	CreateTool   string
	GetTool      string
	Timeout      time.Duration
	AllowedHosts []string
}

type MCPProvider struct {
	connectionID string
	serverURL    string
	headers      map[string]string
	createTool   string
	getTool      string
	client       *http.Client
	nextID       atomic.Int64
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content           []mcpToolContent `json:"content"`
	StructuredContent json.RawMessage  `json:"structuredContent"`
	IsError           bool             `json:"isError"`
}

func NewMCPProvider(config MCPProviderConfig) (*MCPProvider, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.ServerURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, errors.New("creative MCP server_url must be an HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return nil, errors.New("creative MCP server_url must not contain user info")
	}
	if config.Timeout <= 0 {
		config.Timeout = 45 * time.Second
	}
	if strings.TrimSpace(config.CreateTool) == "" {
		config.CreateTool = CreateJobToolName
	}
	if strings.TrimSpace(config.GetTool) == "" {
		config.GetTool = GetJobToolName
	}
	allowedHosts := normalizeMCPAllowedHosts(config.AllowedHosts)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = mcpRestrictedDialContext(allowedHosts)
	provider := &MCPProvider{
		connectionID: strings.TrimSpace(config.ConnectionID),
		serverURL:    parsed.String(),
		headers:      copyMCPHeaders(config.Headers),
		createTool:   strings.TrimSpace(config.CreateTool),
		getTool:      strings.TrimSpace(config.GetTool),
		client: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("creative MCP redirect limit exceeded")
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return errors.New("creative MCP redirect must use HTTP or HTTPS")
				}
				if !strings.EqualFold(req.URL.Hostname(), parsed.Hostname()) {
					return errors.New("creative MCP redirect must stay on the configured host")
				}
				return nil
			},
		},
	}
	provider.nextID.Store(1)
	return provider, nil
}

func (p *MCPProvider) Name() string {
	if p.connectionID == "" {
		return "workspace_mcp"
	}
	return "workspace_mcp:" + p.connectionID
}

func (p *MCPProvider) CreateJob(ctx context.Context, request CreateJobRequest) (CreateJobResult, error) {
	candidates := make([]map[string]any, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		candidates = append(candidates, map[string]any{
			"id":           candidate.ID,
			"competitor":   candidate.Competitor,
			"title":        candidate.Title,
			"asset_type":   candidate.AssetType,
			"source_url":   AssetURL(candidate),
			"archived_url": candidate.ArchivedURL,
			"preview_url":  candidate.PreviewURL,
			"resource_url": candidate.ResourceURL,
			"poster_url":   candidate.PosterURL,
		})
	}
	payload, err := p.callTool(ctx, p.createTool, map[string]any{
		"job_id":          request.LocalJobID,
		"idempotency_key": request.LocalJobID,
		"prompt":          request.Prompt,
		"dynamic_rules":   request.Rules,
		"candidates":      candidates,
	})
	if err != nil {
		return CreateJobResult{}, err
	}
	data := unwrapMCPPayload(payload)
	externalJobID := firstMCPString(data, "job_id", "external_job_id", "id")
	if externalJobID == "" {
		return CreateJobResult{}, errors.New("creative MCP create result did not include job_id")
	}
	processData, err := parseMCPProcessData(data)
	if err != nil {
		return CreateJobResult{}, err
	}
	return CreateJobResult{
		ExternalJobID: externalJobID,
		Status:        firstNonEmpty(firstMCPString(data, "status"), "running"),
		Stage:         firstNonEmpty(firstMCPString(data, "stage"), p.createTool),
		Progress:      firstMCPInt(data, "progress"),
		PollAfter:     mcpPollAfter(data, 2*time.Second),
		ProcessData:   processData,
	}, nil
}

func (p *MCPProvider) GetJob(ctx context.Context, request GetJobRequest) (GetJobResult, error) {
	payload, err := p.callTool(ctx, p.getTool, map[string]any{"job_id": request.ExternalJobID})
	if err != nil {
		return GetJobResult{}, err
	}
	data := unwrapMCPPayload(payload)
	status := firstNonEmpty(firstMCPString(data, "status"), "running")
	processData, err := parseMCPProcessData(data)
	if err != nil {
		return GetJobResult{}, err
	}
	result := GetJobResult{
		Status:       status,
		Stage:        firstNonEmpty(firstMCPString(data, "stage"), p.getTool),
		Progress:     firstMCPInt(data, "progress"),
		PollAfter:    mcpPollAfter(data, 2*time.Second),
		ErrorMessage: firstMCPString(data, "error_message", "error"),
		ProcessData:  processData,
	}
	if status == "completed" {
		result.Variants, err = parseMCPVariants(data["variants"])
		if err != nil {
			return GetJobResult{}, err
		}
		if len(result.Variants) == 0 {
			return GetJobResult{}, errors.New("creative MCP completed result did not include variants")
		}
	}
	return result, nil
}

func (p *MCPProvider) Verify(ctx context.Context) ([]string, error) {
	result, err := p.callRPC(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var response struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("decode creative MCP tools/list: %w", err)
	}
	names := make([]string, 0, len(response.Tools))
	foundCreate := false
	foundGet := false
	for _, tool := range response.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
		foundCreate = foundCreate || name == p.createTool
		foundGet = foundGet || name == p.getTool
	}
	if !foundCreate || !foundGet {
		return names, fmt.Errorf("creative MCP must expose %q and %q", p.createTool, p.getTool)
	}
	return names, nil
}

func (p *MCPProvider) callTool(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	result, err := p.callRPC(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	var toolResult mcpToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("decode creative MCP tool result: %w", err)
	}
	if toolResult.IsError {
		return nil, fmt.Errorf("creative MCP tool %s failed: %s", name, mcpToolText(toolResult.Content))
	}
	var payload map[string]any
	if len(toolResult.StructuredContent) > 0 && string(toolResult.StructuredContent) != "null" {
		if err := json.Unmarshal(toolResult.StructuredContent, &payload); err != nil {
			return nil, fmt.Errorf("decode creative MCP structured content: %w", err)
		}
	} else {
		text := mcpToolText(toolResult.Content)
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("creative MCP tool %s returned no JSON content", name)
		}
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			return nil, fmt.Errorf("decode creative MCP tool %s text content: %w", name, err)
		}
	}
	return payload, nil
}

func (p *MCPProvider) callRPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	sessionID, err := p.initialize(ctx)
	if err != nil {
		return nil, err
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      p.nextID.Add(1),
		"method":  method,
		"params":  params,
	}
	result, _, err := p.sendRPC(ctx, sessionID, request)
	return result, err
}

func (p *MCPProvider) initialize(ctx context.Context) (string, error) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      p.nextID.Add(1),
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "multica-creative-worker",
				"version": "0.1.0",
			},
		},
	}
	_, sessionID, err := p.sendRPC(ctx, "", request)
	if err != nil {
		return "", fmt.Errorf("initialize creative MCP: %w", err)
	}
	_, _, err = p.sendRPC(ctx, sessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	if err != nil {
		return "", fmt.Errorf("notify creative MCP initialized: %w", err)
	}
	return sessionID, nil
}

func (p *MCPProvider) sendRPC(ctx context.Context, sessionID string, payload any) (json.RawMessage, string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, sessionID, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.serverURL, bytes.NewReader(body))
	if err != nil {
		return nil, sessionID, err
	}
	for key, value := range p.headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocolVersion)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, sessionID, fmt.Errorf("call creative MCP: %w", err)
	}
	defer resp.Body.Close()
	if nextSessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); nextSessionID != "" {
		sessionID = nextSessionID
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMCPResponseBytes+1))
	if err != nil {
		return nil, sessionID, err
	}
	if len(data) > maxMCPResponseBytes {
		return nil, sessionID, errors.New("creative MCP response exceeded 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, sessionID, fmt.Errorf("creative MCP returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, sessionID, nil
	}
	data, err = mcpJSONFromHTTPBody(data, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, sessionID, err
	}
	var rpcResponse mcpRPCResponse
	if err := json.Unmarshal(data, &rpcResponse); err != nil {
		return nil, sessionID, fmt.Errorf("decode creative MCP JSON-RPC response: %w", err)
	}
	if rpcResponse.Error != nil {
		return nil, sessionID, fmt.Errorf("creative MCP RPC error %d: %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}
	return rpcResponse.Result, sessionID, nil
}

func mcpJSONFromHTTPBody(data []byte, contentType string) ([]byte, error) {
	if !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return data, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), maxMCPResponseBytes)
	var latest []byte
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			candidate := bytes.TrimSpace([]byte(strings.TrimPrefix(line, "data:")))
			if len(candidate) > 0 {
				latest = append(latest[:0], candidate...)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(latest) == 0 {
		return nil, errors.New("creative MCP SSE response did not include data")
	}
	return latest, nil
}

func mcpToolText(content []mcpToolContent) string {
	parts := make([]string, 0, len(content))
	for _, item := range content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func unwrapMCPPayload(payload map[string]any) map[string]any {
	for _, key := range []string{"data", "job", "result"} {
		if nested, ok := payload[key].(map[string]any); ok {
			return nested
		}
	}
	return payload
}

func parseMCPVariants(raw any) ([]Variant, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("creative MCP variants must be an array")
	}
	variants := make([]Variant, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("creative MCP variant must be an object")
		}
		variant := Variant{
			CandidateID: firstMCPString(row, "candidate_id"),
			Index:       firstMCPInt(row, "variant_index", "index"),
			Title:       firstMCPString(row, "title"),
			Description: firstMCPString(row, "description"),
			QCStatus:    firstNonEmpty(firstMCPString(row, "qc_status"), "pending"),
		}
		assetItems, _ := row["assets"].([]any)
		for _, assetItem := range assetItems {
			assetRow, ok := assetItem.(map[string]any)
			if !ok {
				return nil, errors.New("creative MCP asset must be an object")
			}
			variant.Assets = append(variant.Assets, Asset{
				Width:       firstMCPInt(assetRow, "width"),
				Height:      firstMCPInt(assetRow, "height"),
				Label:       firstMCPString(assetRow, "label"),
				URL:         firstMCPString(assetRow, "asset_url", "url", "image_url"),
				ContentType: firstNonEmpty(firstMCPString(assetRow, "content_type"), "image/png"),
				StorageKey:  firstMCPString(assetRow, "storage_key"),
			})
		}
		variants = append(variants, variant)
	}
	return variants, nil
}

func parseMCPProcessData(data map[string]any) (json.RawMessage, error) {
	raw, exists := data["process_data"]
	if !exists || raw == nil {
		return nil, nil
	}
	if _, ok := raw.(map[string]any); !ok {
		return nil, errors.New("creative MCP process_data must be an object")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode creative MCP process_data: %w", err)
	}
	if len(encoded) > 512*1024 {
		return nil, errors.New("creative MCP process_data exceeds 512 KiB")
	}
	return json.RawMessage(encoded), nil
}

func firstMCPString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstMCPInt(data map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return int(value)
		case int:
			return value
		case json.Number:
			parsed, _ := strconv.Atoi(value.String())
			return parsed
		}
	}
	return 0
}

func mcpPollAfter(data map[string]any, fallback time.Duration) time.Duration {
	if milliseconds := firstMCPInt(data, "poll_after_ms"); milliseconds > 0 {
		return time.Duration(milliseconds) * time.Millisecond
	}
	if seconds := firstMCPInt(data, "poll_after_seconds"); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func copyMCPHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = http.CanonicalHeaderKey(strings.TrimSpace(key))
		if key != "" && strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func normalizeMCPAllowedHosts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func mcpRestrictedDialContext(allowedHosts []string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if mcpHostExplicitlyAllowed(host, allowedHosts) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, resolved := range addresses {
			if publicCreativeAddress(resolved) {
				return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
			}
		}
		return nil, fmt.Errorf("creative MCP host %q did not resolve to a public address", host)
	}
}

func mcpHostExplicitlyAllowed(host string, allowedHosts []string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, allowed := range allowedHosts {
		if strings.HasPrefix(allowed, "*.") {
			if strings.HasSuffix(host, strings.TrimPrefix(allowed, "*")) {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}
