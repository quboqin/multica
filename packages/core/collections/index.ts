import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { z } from "zod";
import { api } from "../api";

export const CollectionSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  name: z.string(),
  created_by: z.string().catch(""),
  project_id: z.string().nullable().catch(null),
  revision: z.number().default(1),
  record_count: z.number().nonnegative().optional().catch(undefined),
  /** Label of the title column; empty means the client's localized default. */
  title_name: z.string().catch(""),
});
export const CollectionFieldSchema = z.object({
  id: z.string(),
  name: z.string(),
  type: z.string(),
  config: z
    .object({
      options: z
        .array(
          z.object({
            id: z.string(),
            name: z.string(),
            color: z.string().optional(),
          }),
        )
        .default([]),
    })
    .default({ options: [] }),
  position: z.number().default(0),
});
export const CollectionRecordSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  collection_id: z.string(),
  title: z.string(),
  fields: z.record(z.string(), z.unknown()).catch({}),
  revision: z.number().int().positive(),
  created_at: z.string(),
});
export const CollectionTrashSchema = z.object({
  records: z.array(
    CollectionRecordSchema.extend({ deleted_at: z.string().catch("") }),
  ),
  total: z.number().nonnegative().catch(0),
  retention_days: z.number().catch(30),
});
export const CollectionDetailSchema = z.object({
  collection: CollectionSchema,
  fields: z.array(CollectionFieldSchema),
});
export const CollectionPageSchema = z.object({
  records: z.array(CollectionRecordSchema),
  total: z.number().nonnegative(),
  groups: z.array(z.object({ key: z.string(), count: z.number() })),
  next_cursor: z.string().nullable(),
});
export type Collection = z.infer<typeof CollectionSchema>;
export type CollectionField = z.infer<typeof CollectionFieldSchema>;
export type CollectionRecord = z.infer<typeof CollectionRecordSchema>;
export interface CollectionQuery {
  record_id?: string;
  date_field?: string;
  date_start?: string;
  date_end?: string;
  search?: string;
  properties?: Record<string, unknown[]>;
  group_by?: string;
  group_key?: string;
  /** A field id, "title" or "created_at". */
  sort_by?: string;
  sort_dir?: "asc" | "desc";
}
export interface CollectionPatch {
  name?: string;
  /** Empty returns the title column to its localized default label. */
  title_name?: string;
  /** Archiving removes the table from the workspace; there is no unarchive yet. */
  archived?: true;
}
export interface CollectionFieldPatch {
  name?: string;
  type?: string;
  config?: { options: { id?: string; name: string; color: string }[] };
  position?: number;
  archived?: boolean;
}
export type CollectionTrash = z.infer<typeof CollectionTrashSchema>;
export const collectionKeys = {
  all: (wsId: string) => ["collections", wsId] as const,
  detail: (wsId: string, id: string) =>
    ["collections", wsId, id, "detail"] as const,
  records: (wsId: string, id: string, query: CollectionQuery) =>
    ["collections", wsId, id, "records", query] as const,
};
export function collectionListOptions(wsId: string) {
  return queryOptions({
    queryKey: [...collectionKeys.all(wsId), "list"],
    queryFn: ({ signal }) => api.listCollections({ workspaceId: wsId, signal }),
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
