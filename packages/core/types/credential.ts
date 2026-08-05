export type CredentialProfileStatus =
  | "pending"
  | "active"
  | "need_reauth"
  | "revoked"
  | string;

export interface CredentialConnector {
  id: string;
  display_name: string;
  login_url: string;
  capabilities: string[];
  scope: "workspace" | "deployment" | string;
}

export interface CredentialProfileManager {
  user_id: string;
  name: string;
  email: string;
  created_at: string;
}

export interface CredentialProfile {
  id: string;
  connector_id: string;
  label: string;
  status: CredentialProfileStatus;
  scope: "workspace" | "deployment" | string;
  can_manage: boolean;
  managers: CredentialProfileManager[];
  last_used_at?: string | null;
  expires_hint?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CredentialLoginSession {
  id: string;
  profile_id: string;
  connector_id: string;
  browser_url: string;
  status: string;
  expires_at: string;
  created_at: string;
}

export interface ListCredentialConnectorsResponse {
  connectors: CredentialConnector[];
}

export interface ListCredentialProfilesResponse {
  profiles: CredentialProfile[];
}

export interface StartCredentialLoginSessionRequest {
  connector_id: string;
  profile_id?: string;
  label?: string;
}

export interface StartCredentialLoginSessionResponse {
  profile: CredentialProfile;
  session: CredentialLoginSession;
}

export interface AddCredentialProfileManagerRequest {
  user_id: string;
}

export interface RunCredentialCrawlRequest {
  profile_id: string;
  issue_id?: string;
  capability: string;
  params?: Record<string, unknown>;
}

export interface CredentialCrawlResult {
  status: string;
  downloaded: number;
  output_prefix: string;
  message: string;
  raw?: unknown;
}
