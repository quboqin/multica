export interface WorkspaceCapability {
  key: string;
  enabled: boolean;
  updated_at?: string;
}

export interface WorkspaceCapabilitiesResponse {
  items: WorkspaceCapability[];
  can_manage: boolean;
}
