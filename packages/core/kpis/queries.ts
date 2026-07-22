import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const kpiKeys = {
  all: (wsId: string) => ["kpis", wsId] as const,
  list: (wsId: string) => [...kpiKeys.all(wsId), "list"] as const,
  detail: (wsId: string, id: string) => [...kpiKeys.all(wsId), "detail", id] as const,
};

export function kpiMetricListOptions(wsId: string) {
  return queryOptions({
    queryKey: kpiKeys.list(wsId),
    queryFn: () => api.listKpiMetrics(wsId),
    select: (data) => data.metrics,
  });
}

export function kpiMetricDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: kpiKeys.detail(wsId, id),
    queryFn: () => api.getKpiMetric(id, wsId),
    enabled: Boolean(id),
  });
}
