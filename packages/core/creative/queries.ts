import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { CreativeMaterialLibraryQuery, CreativeResourceKind } from "../types";

export const creativeKeys = {
  retrySettings: (wsId: string) => [...creativeKeys.all(wsId), "retry-settings"] as const,
  all: (wsId: string) => ["creative", wsId] as const,
  resources: (wsId: string, kind?: CreativeResourceKind) =>
    [...creativeKeys.all(wsId), "resources", kind ?? "all"] as const,
  resourceFiles: (wsId: string, resourceId: string) =>
    [...creativeKeys.all(wsId), "resource-files", resourceId] as const,
  materials: (wsId: string) => [...creativeKeys.all(wsId), "materials"] as const,
  issue: (wsId: string, issueId: string) =>
    [...creativeKeys.all(wsId), "issue", issueId] as const,
  orders: (wsId: string) => [...creativeKeys.all(wsId), "orders"] as const,
  order: (wsId: string, orderId: string) => [...creativeKeys.orders(wsId), orderId] as const,
  analyses: (wsId: string) => [...creativeKeys.all(wsId), "source-analyses"] as const,
  feedback: (wsId: string, subjectType = "", subjectId = "") =>
    [...creativeKeys.all(wsId), "feedback", subjectType, subjectId] as const,
  feedbackMetrics: (wsId: string) => [...creativeKeys.all(wsId), "feedback-metrics"] as const,
  feedbackDashboard: (wsId: string) => [...creativeKeys.all(wsId), "feedback-dashboard"] as const,
};

export const creativeRetrySettingsOptions = (wsId: string) => queryOptions({
  queryKey: creativeKeys.retrySettings(wsId),
  queryFn: ({ signal }) => api.getCreativeRetrySettings(signal),
  enabled: !!wsId,
});

export const creativeMaterialLibraryOptions = (wsId: string, params?: CreativeMaterialLibraryQuery) =>
  queryOptions({
    queryKey: params ? [...creativeKeys.materials(wsId), params] : creativeKeys.materials(wsId),
    queryFn: ({ signal }) => api.listCreativeMaterialLibrary(params, signal),
    enabled: !!wsId,
  });

export const creativeResourcesOptions = (wsId: string, kind?: CreativeResourceKind) =>
  queryOptions({
    queryKey: creativeKeys.resources(wsId, kind),
    queryFn: ({ signal }) => api.listCreativeResources(kind, signal),
    enabled: !!wsId,
  });

export const creativeResourceFilesOptions = (wsId: string, resourceId: string) =>
  queryOptions({
    queryKey: creativeKeys.resourceFiles(wsId, resourceId),
    queryFn: ({ signal }) => api.listCreativeResourceFiles(resourceId, signal),
    enabled: !!wsId && !!resourceId,
  });

export const creativeMaterialsOptions = (wsId: string, issueId: string) =>
  queryOptions({
    queryKey: creativeKeys.issue(wsId, issueId),
    queryFn: ({ signal }) => api.getCreativeMaterials(issueId, signal),
    enabled: !!wsId && !!issueId,
  });

export const creativeOrdersOptions = (wsId: string) => queryOptions({ queryKey: creativeKeys.orders(wsId), queryFn: ({ signal }) => api.listCreativeOrders(signal), enabled: !!wsId });
export const creativeOrderOptions = (wsId: string, orderId: string) => queryOptions({ queryKey: creativeKeys.order(wsId, orderId), queryFn: ({ signal }) => api.getCreativeOrder(orderId, signal), enabled: !!wsId && !!orderId });
export const creativeSourceAnalysesOptions = (wsId: string) => queryOptions({ queryKey: creativeKeys.analyses(wsId), queryFn: ({ signal }) => api.listCreativeSourceAnalyses(undefined, signal), enabled: !!wsId });
export const creativeFeedbackOptions = (wsId: string, subjectType = "", subjectId = "") => queryOptions({
  queryKey: creativeKeys.feedback(wsId, subjectType, subjectId),
  queryFn: ({ signal }) => api.listCreativeFeedback(subjectType, subjectId, signal),
  enabled: !!wsId,
});
export const creativeFeedbackMetricsOptions = (wsId: string) => queryOptions({
  queryKey: creativeKeys.feedbackMetrics(wsId),
  queryFn: ({ signal }) => api.getCreativeFeedbackMetrics(signal),
  enabled: !!wsId,
});
export const creativeFeedbackDashboardOptions = (wsId: string) => queryOptions({
  queryKey: creativeKeys.feedbackDashboard(wsId),
  queryFn: ({ signal }) => api.getCreativeFeedbackDashboard(signal),
  enabled: !!wsId,
});
