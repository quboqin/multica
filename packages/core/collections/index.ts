import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { CollectionQuery } from "./schema";
export * from "./schema";
export * from "./import";
export { collectionHasHierarchy, flattenCollectionTree } from "./hierarchy";

export const collectionKeys = {
  all: (wsId: string) => ["collections", wsId] as const,
  detail: (wsId: string, id: string) =>
    ["collections", wsId, id, "detail"] as const,
  records: (wsId: string, id: string, query: CollectionQuery) =>
    ["collections", wsId, id, "records", query] as const,
  /** Reverse lookups. "links" sits where a table id would, and is never one. */
  backlinks: (wsId: string) => ["collections", wsId, "links"] as const,
};
export function collectionListOptions(wsId: string, archived = false) {
  return queryOptions({
    queryKey: [
      ...collectionKeys.all(wsId),
      "list",
      ...(archived ? ["archived"] : []),
    ],
    queryFn: ({ signal }) =>
      api.listCollections({ workspaceId: wsId, signal, archived }),
    enabled: !!wsId,
  });
}
export function collectionTrashOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: [...collectionKeys.all(wsId), id, "trash"],
    queryFn: ({ signal }) =>
      api.listCollectionTrash(id, { workspaceId: wsId, signal }),
    enabled: !!wsId && !!id,
  });
}
export function collectionDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: collectionKeys.detail(wsId, id),
    queryFn: ({ signal }) =>
      api.getCollection(id, { workspaceId: wsId, signal }),
    enabled: !!wsId && !!id,
  });
}
export function collectionRecordsOptions(
  wsId: string,
  id: string,
  query: CollectionQuery,
) {
  return infiniteQueryOptions({
    queryKey: collectionKeys.records(wsId, id, query),
    queryFn: ({ pageParam, signal }) =>
      api.listCollectionRecords(id, query, pageParam, signal, wsId),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: !!wsId && !!id,
  });
}

/** A page of direct children, owned by Query just like the root list. */
export function collectionRecordPageOptions(
  wsId: string,
  id: string,
  query: CollectionQuery,
  cursor: string | null,
) {
  return queryOptions({
    queryKey: [...collectionKeys.records(wsId, id, query), "page", cursor],
    queryFn: ({ signal }) => api.listCollectionRecords(id, query, cursor, signal, wsId),
    enabled: !!wsId && !!id,
  });
}

/** The edited row must remain readable after filters or pagination move it out. */
export function collectionRecordOptions(
  wsId: string,
  id: string,
  recordId: string | null,
) {
  return queryOptions({
    queryKey: [...collectionKeys.all(wsId), id, "focused", recordId],
    queryFn: async ({ signal }) => {
      const page = await api.listCollectionRecords(
        id,
        { record_id: recordId! },
        null,
        signal,
        wsId,
      );
      return page.records[0] ?? null;
    },
    enabled: !!wsId && !!id && !!recordId,
  });
}

/** Records whose relation fields point at one task. */
export function issueRecordLinksOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: [...collectionKeys.backlinks(wsId), "issue", issueId],
    queryFn: ({ signal }) =>
      api.listIssueRecordLinks(issueId, { workspaceId: wsId, signal }),
    enabled: !!wsId && !!issueId,
  });
}

/** Records whose relation fields point at one record. */
export function recordBacklinksOptions(
  wsId: string,
  id: string,
  recordId: string,
) {
  return queryOptions({
    queryKey: [...collectionKeys.backlinks(wsId), "record", recordId],
    queryFn: ({ signal }) =>
      api.listCollectionRecordBacklinks(id, recordId, {
        workspaceId: wsId,
        signal,
      }),
    enabled: !!wsId && !!id && !!recordId,
  });
}

export function archivedCollectionFieldsOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: [...collectionKeys.all(wsId), id, "archived-fields"],
    queryFn: ({ signal }) => api.listArchivedCollectionFields(id, wsId, signal),
    enabled: !!wsId && !!id,
  });
}
