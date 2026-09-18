import { z } from "zod";
import type {
  Collection,
  CollectionDetail,
  CollectionField,
  CollectionPage,
  CollectionRecord,
  CollectionRecordPage,
  CreateCollectionRecordResult,
  CreateCollectionResult,
} from "../types";

export const CollectionSchema = z
  .object({
    id: z.string().min(1),
    workspace_id: z.string().min(1),
    name: z.string(),
    revision: z.number().int().positive(),
    archived_at: z.string().nullable().optional().default(null),
    created_at: z.string(),
    updated_at: z.string(),
  })
  .loose()
  .transform(
    (value): Collection => ({
      id: value.id,
      workspaceId: value.workspace_id,
      name: value.name,
      revision: value.revision,
      archivedAt: value.archived_at,
      createdAt: value.created_at,
      updatedAt: value.updated_at,
    }),
  );

export const CollectionFieldSchema = z
  .object({
    id: z.string().min(1),
    workspace_id: z.string().min(1),
    collection_id: z.string().min(1),
    name: z.string(),
    type: z.string(),
    position: z.number().int().nonnegative(),
    revision: z.number().int().positive(),
  })
  .loose()
  .transform(
    (value): CollectionField => ({
      id: value.id,
      workspaceId: value.workspace_id,
      collectionId: value.collection_id,
      name: value.name,
      type: value.type,
      position: value.position,
      revision: value.revision,
    }),
  );

export const CollectionRecordSchema = z
  .object({
    id: z.string().min(1),
    workspace_id: z.string().min(1),
    collection_id: z.string().min(1),
    title: z.string(),
    fields: z.record(z.string(), z.unknown()),
    position: z.number().finite(),
    revision: z.number().int().positive(),
    created_at: z.string(),
    updated_at: z.string(),
  })
  .loose()
  .transform(
    (value): CollectionRecord => ({
      id: value.id,
      workspaceId: value.workspace_id,
      collectionId: value.collection_id,
      title: value.title,
      fields: value.fields,
      position: value.position,
      revision: value.revision,
      createdAt: value.created_at,
      updatedAt: value.updated_at,
    }),
  );

export const CollectionCapabilitiesSchema = z
  .object({
    layouts: z.array(z.string()),
    grouping: z.boolean(),
    hierarchy: z.boolean(),
    writable: z.boolean(),
    max_page_size: z.number().int().positive().max(200),
  })
  .loose()
  .transform((value) => ({
    layouts: value.layouts,
    grouping: value.grouping,
    hierarchy: value.hierarchy,
    writable: value.writable,
    maxPageSize: value.max_page_size,
  }));

export const CollectionPageSchema = z
  .object({
    collections: z.array(CollectionSchema),
    total: z.number().int().nonnegative(),
    next_cursor: z.string().nullable(),
  })
  .loose()
  .transform(
    (value): CollectionPage => ({
      collections: value.collections,
      total: value.total,
      nextCursor: value.next_cursor,
    }),
  );

export const CollectionDetailSchema = z
  .object({
    collection: CollectionSchema,
    fields: z.array(CollectionFieldSchema),
    capabilities: CollectionCapabilitiesSchema,
  })
  .loose()
  .transform((value): CollectionDetail => value);

export const CollectionRecordPageSchema = z
  .object({
    records: z.array(CollectionRecordSchema),
    total: z.number().int().nonnegative(),
    next_cursor: z.string().nullable(),
  })
  .loose()
  .transform(
    (value): CollectionRecordPage => ({
      records: value.records,
      total: value.total,
      nextCursor: value.next_cursor,
    }),
  );

export const CreateCollectionResultSchema = z
  .object({
    collection: CollectionSchema,
    fields: z.array(CollectionFieldSchema),
    replayed: z.boolean(),
  })
  .loose()
  .transform((value): CreateCollectionResult => value);

export const CreateCollectionRecordResultSchema = z
  .object({ record: CollectionRecordSchema, replayed: z.boolean() })
  .loose()
  .transform((value): CreateCollectionRecordResult => value);

export const CollectionRecordResultSchema = z
  .object({ record: CollectionRecordSchema })
  .loose()
  .transform((value) => value.record);

export const EMPTY_COLLECTION_PAGE: CollectionPage = {
  collections: [],
  total: 0,
  nextCursor: null,
};

export const EMPTY_COLLECTION_RECORD_PAGE: CollectionRecordPage = {
  records: [],
  total: 0,
  nextCursor: null,
};
