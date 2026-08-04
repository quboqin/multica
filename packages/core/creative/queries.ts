import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { CreativeResourceKind } from "../types";

export const creativeKeys = {
  all: (wsId: string) => ["creative", wsId] as const,
  resources: (wsId: string, kind?: CreativeResourceKind) =>
    [...creativeKeys.all(wsId), "resources", kind ?? "all"] as const,
  resourceFiles: (wsId: string, resourceId: string) =>
    [...creativeKeys.all(wsId), "resource-files", resourceId] as const,
  materials: (wsId: string) => [...creativeKeys.all(wsId), "materials"] as const,
  copyEntries: (wsId: string, libraryId: string) =>
    [...creativeKeys.all(wsId), "copy-entries", libraryId] as const,
  issue: (wsId: string, issueId: string) =>
    [...creativeKeys.all(wsId), "issue", issueId] as const,
  orders: (wsId: string) => [...creativeKeys.all(wsId), "orders"] as const,
  order: (wsId: string, orderId: string) => [...creativeKeys.orders(wsId), orderId] as const,
  analyses: (wsId: string) => [...creativeKeys.all(wsId), "source-analyses"] as const,
  feedback: (wsId: string, subjectType = "", subjectId = "") =>
    [...creativeKeys.all(wsId), "feedback", subjectType, subjectId] as const,
};

export const creativeMaterialLibraryOptions = (wsId: string) =>
  queryOptions({
    queryKey: creativeKeys.materials(wsId),
    queryFn: () => api.listCreativeMaterialLibrary(),
    enabled: !!wsId,
  });

export const creativeResourcesOptions = (wsId: string, kind?: CreativeResourceKind) =>
  queryOptions({
    queryKey: creativeKeys.resources(wsId, kind),
    queryFn: () => api.listCreativeResources(kind),
    enabled: !!wsId,
  });

export const creativeResourceFilesOptions = (wsId: string, resourceId: string) =>
  queryOptions({
    queryKey: creativeKeys.resourceFiles(wsId, resourceId),
    queryFn: () => api.listCreativeResourceFiles(resourceId),
    enabled: !!wsId && !!resourceId,
  });

export const creativeCopyEntriesOptions = (wsId: string, libraryId: string) =>
  queryOptions({
    queryKey: creativeKeys.copyEntries(wsId, libraryId),
    queryFn: () => api.listCreativeCopyEntries(libraryId),
    enabled: !!wsId && !!libraryId,
  });

export const creativeMaterialsOptions = (wsId: string, issueId: string) =>
  queryOptions({
    queryKey: creativeKeys.issue(wsId, issueId),
    queryFn: () => api.getCreativeMaterials(issueId),
    enabled: !!wsId && !!issueId,
  });

export const creativeOrdersOptions = (wsId: string) => queryOptions({ queryKey: creativeKeys.orders(wsId), queryFn: () => api.listCreativeOrders(), enabled: !!wsId });
export const creativeOrderOptions = (wsId: string, orderId: string) => queryOptions({ queryKey: creativeKeys.order(wsId, orderId), queryFn: () => api.getCreativeOrder(orderId), enabled: !!wsId && !!orderId });
export const creativeSourceAnalysesOptions = (wsId: string) => queryOptions({ queryKey: creativeKeys.analyses(wsId), queryFn: () => api.listCreativeSourceAnalyses(), enabled: !!wsId });
export const creativeFeedbackOptions = (wsId: string, subjectType = "", subjectId = "") => queryOptions({
  queryKey: creativeKeys.feedback(wsId, subjectType, subjectId),
  queryFn: () => api.listCreativeFeedback(subjectType, subjectId),
  enabled: !!wsId,
});
