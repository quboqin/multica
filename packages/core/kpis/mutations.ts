import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import type { CreateKpiMetricRequest, KpiMetric, ListKpiMetricsResponse, UpdateKpiMetricRequest } from "../types";
import { kpiKeys } from "./queries";

export function useCreateKpiMetric() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (data: CreateKpiMetricRequest) => api.createKpiMetric(data, wsId),
    onSuccess: (metric) => {
      qc.setQueryData<ListKpiMetricsResponse>(kpiKeys.list(wsId), (old) =>
        old && !old.metrics.some((item) => item.id === metric.id)
          ? { ...old, metrics: [...old.metrics, metric], total: old.total + 1 }
          : old,
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: kpiKeys.list(wsId) });
    },
  });
}

export function useUpdateKpiMetric() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & UpdateKpiMetricRequest) =>
      api.updateKpiMetric(id, data, wsId),
    onMutate: async ({ id, ...data }) => {
      await qc.cancelQueries({ queryKey: kpiKeys.list(wsId) });
      const prevList = qc.getQueryData<ListKpiMetricsResponse>(kpiKeys.list(wsId));
      const prevDetail = qc.getQueryData<KpiMetric>(kpiKeys.detail(wsId, id));
      qc.setQueryData<ListKpiMetricsResponse>(kpiKeys.list(wsId), (old) =>
        old ? { ...old, metrics: old.metrics.map((metric) => (metric.id === id ? { ...metric, ...data } : metric)) } : old,
      );
      qc.setQueryData<KpiMetric>(kpiKeys.detail(wsId, id), (old) =>
        old ? { ...old, ...data } : old,
      );
      return { prevList, prevDetail, id };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prevList) qc.setQueryData(kpiKeys.list(wsId), ctx.prevList);
      if (ctx?.prevDetail) qc.setQueryData(kpiKeys.detail(wsId, ctx.id), ctx.prevDetail);
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: kpiKeys.detail(wsId, vars.id) });
      qc.invalidateQueries({ queryKey: kpiKeys.list(wsId) });
    },
  });
}

export function useDeleteKpiMetric() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (id: string) => api.deleteKpiMetric(id, wsId),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: kpiKeys.list(wsId) });
      const prevList = qc.getQueryData<ListKpiMetricsResponse>(kpiKeys.list(wsId));
      qc.setQueryData<ListKpiMetricsResponse>(kpiKeys.list(wsId), (old) =>
        old ? { ...old, metrics: old.metrics.filter((metric) => metric.id !== id), total: old.total - 1 } : old,
      );
      qc.removeQueries({ queryKey: kpiKeys.detail(wsId, id) });
      return { prevList };
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prevList) qc.setQueryData(kpiKeys.list(wsId), ctx.prevList);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: kpiKeys.list(wsId) });
    },
  });
}
