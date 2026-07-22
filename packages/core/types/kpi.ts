export type KpiMetricStatus = "on_track" | "at_risk" | "missed" | "pending";
export type KpiMetricLinkType = "none" | "milestone" | "project" | "issue";

export interface KpiMetric {
  id: string;
  workspace_id: string;
  name: string;
  owner: string;
  target: string;
  current: string;
  status: KpiMetricStatus;
  note: string;
  link_type: KpiMetricLinkType;
  link_id: string;
  completion_rate: number;
  position: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateKpiMetricRequest {
  name: string;
  owner?: string;
  target?: string;
  current?: string;
  status?: KpiMetricStatus;
  note?: string;
  link_type?: KpiMetricLinkType;
  link_id?: string;
  completion_rate?: number;
  position?: number;
}

export interface UpdateKpiMetricRequest {
  name?: string;
  owner?: string;
  target?: string;
  current?: string;
  status?: KpiMetricStatus;
  note?: string;
  link_type?: KpiMetricLinkType;
  link_id?: string;
  completion_rate?: number;
  position?: number;
}

export interface ListKpiMetricsResponse {
  metrics: KpiMetric[];
  total: number;
}
