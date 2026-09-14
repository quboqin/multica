import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { dataSourceIdentityKey, type DataSourceIdentity } from "../data-source";
import { assertClientWorkspaceAccessAllowed } from "../platform";

export function collectionSourceIdentity(
  workspaceId: string,
  collectionId: string,
): DataSourceIdentity {
  return { workspaceId, namespace: "collection", sourceId: collectionId };
}

export const collectionKeys = {
  all: (workspaceId: string) => ["collections", workspaceId] as const,
  sources: (workspaceId: string) =>
    ["data-source", workspaceId, "collection"] as const,
  list: (
    workspaceId: string,
    page: { limit?: number; cursor?: string | null } = {},
  ) =>
    [
      ...collectionKeys.all(workspaceId),
      "list",
      page.limit ?? 50,
      page.cursor ?? null,
    ] as const,
  source: (workspaceId: string, collectionId: string) =>
    dataSourceIdentityKey(collectionSourceIdentity(workspaceId, collectionId)),
  detail: (workspaceId: string, collectionId: string) =>
    [...collectionKeys.source(workspaceId, collectionId), "collection"] as const,
  rows: (workspaceId: string, collectionId: string) =>
    [...collectionKeys.source(workspaceId, collectionId), "rows"] as const,
  record: (workspaceId: string, collectionId: string, recordId: string) =>
    [
      ...collectionKeys.source(workspaceId, collectionId),
      "record",
      recordId,
    ] as const,
};

export function collectionListOptions(
  workspaceId: string,
  workspaceSlug: string,
  page: { limit?: number; cursor?: string | null } = {},
) {
  return queryOptions({
    queryKey: collectionKeys.list(workspaceId, page),
    queryFn: ({ signal, client }) => {
      assertClientWorkspaceAccessAllowed(client, workspaceId);
      return api.listCollections(workspaceSlug, page, signal);
    },
    staleTime: 30_000,
  });
}

export function collectionDetailOptions(
  workspaceId: string,
  workspaceSlug: string,
  collectionId: string,
) {
  return queryOptions({
    queryKey: collectionKeys.detail(workspaceId, collectionId),
    queryFn: ({ signal, client }) => {
      assertClientWorkspaceAccessAllowed(client, workspaceId);
      return api.getCollection(collectionId, workspaceSlug, signal);
    },
    enabled: Boolean(workspaceId && workspaceSlug && collectionId),
    staleTime: 30_000,
  });
}

export function collectionRecordOptions(
  workspaceId: string,
  workspaceSlug: string,
  collectionId: string,
  recordId: string,
) {
  return queryOptions({
    queryKey: collectionKeys.record(workspaceId, collectionId, recordId),
    queryFn: ({ signal, client }) => {
      assertClientWorkspaceAccessAllowed(client, workspaceId);
      return api.getCollectionRecord(
        collectionId,
        recordId,
        workspaceSlug,
        signal,
      );
    },
    enabled: Boolean(workspaceId && workspaceSlug && collectionId && recordId),
    staleTime: 30_000,
  });
}

export const collectionTableQuery = {
  version: 1,
  sort: "created_at_asc",
} as const;
