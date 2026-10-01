import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { api } from "@/data/api";

export const documentKeys = {
  all: (wsId: string | null) => ["documents", wsId] as const,
  list: (wsId: string | null) => [...documentKeys.all(wsId), "list"] as const,
  access: (wsId: string | null, id: string) => [...documentKeys.all(wsId), "access", id] as const,
};

export const documentListOptions = (wsId: string | null) => queryOptions({
  queryKey: documentKeys.list(wsId),
  queryFn: ({ signal }) => api.listDocuments({ signal }),
  enabled: !!wsId,
});

export const documentAccessOptions = (wsId: string | null, id: string) => queryOptions({
  queryKey: documentKeys.access(wsId, id),
  queryFn: ({ signal }) => api.getDocumentAccess(id, { signal }),
  enabled: !!wsId && !!id,
});

export const collectionKeys = {
  all: (wsId: string | null) => ["collections", wsId] as const,
  list: (wsId: string | null) => [...collectionKeys.all(wsId), "list"] as const,
  detail: (wsId: string | null, id: string) => [...collectionKeys.all(wsId), "detail", id] as const,
  records: (wsId: string | null, id: string) => [...collectionKeys.all(wsId), "records", id] as const,
};

export const collectionListOptions = (wsId: string | null) => queryOptions({
  queryKey: collectionKeys.list(wsId),
  queryFn: ({ signal }) => api.listCollections({ signal }),
  enabled: !!wsId,
});

export const collectionDetailOptions = (wsId: string | null, id: string) => queryOptions({
  queryKey: collectionKeys.detail(wsId, id),
  queryFn: ({ signal }) => api.getCollection(id, { signal }),
  enabled: !!wsId && !!id,
});

export const collectionRecordsOptions = (wsId: string | null, id: string) => infiniteQueryOptions({
  queryKey: collectionKeys.records(wsId, id),
  queryFn: ({ pageParam, signal }) => api.listCollectionRecords(id, pageParam, { signal }),
  initialPageParam: null as string | null,
  getNextPageParam: (page) => page.next_cursor ?? undefined,
  enabled: !!wsId && !!id,
});
