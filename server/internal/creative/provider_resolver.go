package creative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

type connectionQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ResolvedProvider struct {
	Provider     Provider
	ConnectionID string
}

type ProviderResolver interface {
	ResolveDefault(context.Context, string) (ResolvedProvider, error)
	Resolve(context.Context, string, string) (ResolvedProvider, error)
}

type DatabaseProviderResolver struct {
	db           connectionQueryer
	box          *secretbox.Box
	mock         Provider
	timeout      time.Duration
	allowedHosts []string
}

func NewDatabaseProviderResolver(
	db connectionQueryer,
	box *secretbox.Box,
	mock Provider,
	timeout time.Duration,
	allowedHosts []string,
) *DatabaseProviderResolver {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &DatabaseProviderResolver{
		db:           db,
		box:          box,
		mock:         mock,
		timeout:      timeout,
		allowedHosts: append([]string(nil), allowedHosts...),
	}
}

func (r *DatabaseProviderResolver) ResolveDefault(ctx context.Context, workspaceID string) (ResolvedProvider, error) {
	if r == nil || r.db == nil {
		return r.mockResult()
	}
	connection, err := r.load(ctx, workspaceID, "", true)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.mockResult()
	}
	if err != nil {
		return ResolvedProvider{}, err
	}
	return r.provider(connection)
}

func (r *DatabaseProviderResolver) Resolve(ctx context.Context, workspaceID, connectionID string) (ResolvedProvider, error) {
	if strings.TrimSpace(connectionID) == "" {
		return r.mockResult()
	}
	if r == nil || r.db == nil {
		return ResolvedProvider{}, errors.New("creative provider resolver is not configured")
	}
	connection, err := r.load(ctx, workspaceID, connectionID, false)
	if err != nil {
		return ResolvedProvider{}, err
	}
	return r.provider(connection)
}

type workspaceMCPConnection struct {
	ID               string
	ServerURL        string
	Transport        string
	CreateTool       string
	GetTool          string
	EncryptedHeaders []byte
}

func (r *DatabaseProviderResolver) load(ctx context.Context, workspaceID, connectionID string, defaultOnly bool) (workspaceMCPConnection, error) {
	query := `
SELECT id::text, server_url, transport, tool_create, tool_get,
       COALESCE(secret_headers_encrypted, ''::bytea)
FROM workspace_mcp_connection
WHERE workspace_id = $1::uuid AND capability = 'creative_edit'`
	args := []any{workspaceID}
	if defaultOnly {
		query += " AND status = 'active' AND is_default = true ORDER BY created_at LIMIT 1"
	} else {
		query += " AND id = $2::uuid LIMIT 1"
		args = append(args, connectionID)
	}
	var connection workspaceMCPConnection
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&connection.ID,
		&connection.ServerURL,
		&connection.Transport,
		&connection.CreateTool,
		&connection.GetTool,
		&connection.EncryptedHeaders,
	)
	return connection, err
}

func (r *DatabaseProviderResolver) provider(connection workspaceMCPConnection) (ResolvedProvider, error) {
	if connection.Transport != "streamable_http" {
		return ResolvedProvider{}, fmt.Errorf("unsupported creative MCP transport %q", connection.Transport)
	}
	headers := map[string]string{}
	if len(connection.EncryptedHeaders) > 0 {
		if r.box == nil {
			return ResolvedProvider{}, errors.New("workspace MCP secret key is not configured")
		}
		plaintext, err := r.box.Open(connection.EncryptedHeaders)
		if err != nil {
			return ResolvedProvider{}, fmt.Errorf("decrypt workspace MCP headers: %w", err)
		}
		if err := json.Unmarshal(plaintext, &headers); err != nil {
			return ResolvedProvider{}, fmt.Errorf("decode workspace MCP headers: %w", err)
		}
	}
	provider, err := NewMCPProvider(MCPProviderConfig{
		ConnectionID: connection.ID,
		ServerURL:    connection.ServerURL,
		Headers:      headers,
		CreateTool:   connection.CreateTool,
		GetTool:      connection.GetTool,
		Timeout:      r.timeout,
		AllowedHosts: r.allowedHosts,
	})
	if err != nil {
		return ResolvedProvider{}, err
	}
	return ResolvedProvider{Provider: provider, ConnectionID: connection.ID}, nil
}

func (r *DatabaseProviderResolver) mockResult() (ResolvedProvider, error) {
	if r == nil || r.mock == nil {
		return ResolvedProvider{}, errors.New("mock creative provider is not configured")
	}
	return ResolvedProvider{Provider: r.mock}, nil
}
