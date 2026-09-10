export interface WorkspaceCapability {
  key: string;
  enabled: boolean;
  updated_at?: string;
}

export interface WorkspaceCapabilitiesResponse {
  items: WorkspaceCapability[];
  can_manage: boolean;
  creative_factory_squad_id?: string;
}

export interface CreativeFactoryInitializationRequest {
  brand?: string;
  market?: string;
  locale?: string;
  currency?: string;
  competitors?: string[];
  priority_competitors?: string[];
}
