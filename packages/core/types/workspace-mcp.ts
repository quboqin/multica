export type WorkspaceMCPConnectionStatus = "active" | "disabled" | string;

export interface WorkspaceMCPConnection {
  id: string;
  workspace_id: string;
  name: string;
  capability: string;
  transport: string;
  server_url: string;
  tool_create: string;
  tool_get: string;
  status: WorkspaceMCPConnectionStatus;
  is_default: boolean;
  has_secret_headers: boolean;
  secret_header_names: string[];
  last_verified_at: string;
  last_error: string;
  created_at: string;
  updated_at: string;
}

export interface ListWorkspaceMCPConnectionsResponse {
  connections: WorkspaceMCPConnection[];
}

export interface SaveWorkspaceMCPConnectionRequest {
  name: string;
  server_url: string;
  transport?: "streamable_http";
  tool_create?: string;
  tool_get?: string;
  status?: "active" | "disabled";
  is_default?: boolean;
  secret_headers?: Record<string, string>;
  clear_secret_headers?: boolean;
}

export interface VerifyWorkspaceMCPConnectionResponse {
  ok: boolean;
  provider: string;
  tools: string[];
}
