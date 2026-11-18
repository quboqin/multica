import { z } from "zod";
import { DocumentAccessSchema } from "../documents/schema";

export const CollectionSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  name: z.string(),
  icon: z.string().catch(""),
  description: z.string().catch(""),
  archived_at: z.string().nullable().catch(null),
  created_by: z.string().catch(""),
  project_id: z.string().nullable().catch(null),
  revision: z.number().default(1),
  record_count: z.number().nonnegative().optional().catch(undefined),
  /** Label of the title column; empty means the client's localized default. */
  title_name: z.string().catch(""),
});
/**
 * What a relation field points at: workspace tasks (`to_type: "issue"`) or the
 * records of one table (`to_type: "record"` plus its `collection_id`). Kept as
 * a plain string so a target a newer server adds still parses.
 */
export const RelationTargetSchema = z.object({
  to_type: z.string(),
  collection_id: z.string().catch(""),
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
      formula: z
        .object({
          expression: z.string(),
          bindings: z
            .record(z.string(), z.string())
            .optional()
            .catch(undefined),
        })
        .optional()
        .catch(undefined),
      /** Set on relation fields only. */
      relation: RelationTargetSchema.optional().catch(undefined),
    })
    .default({ options: [] }),
  position: z.number().default(0),
});
/**
 * One edge of a relation cell with what its target looks like now. `missing`
 * means the task or record was deleted: the edge stays so the cell can say so.
 */
export const RecordLinkSchema = z.object({
  id: z.string(),
  to_type: z.string(),
  to_id: z.string(),
  title: z.string().catch(""),
  /** Task targets only, e.g. "MUL-12". */
  identifier: z.string().catch(""),
  /** Task targets only: the status key. */
  status: z.string().catch(""),
  /** Record targets only: the table the record lives in. */
  collection_id: z.string().catch(""),
  missing: z.boolean().catch(false),
});
export const CollectionRecordSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  collection_id: z.string(),
  title: z.string(),
  parent_id: z.string().uuid().nullable().optional().catch(undefined),
  child_count: z.number().int().nonnegative().optional().catch(undefined),
  fields: z.record(z.string(), z.unknown()).catch({}),
  formula_errors: z.record(z.string(), z.string()).optional().catch(undefined),
  /** Relation cells by field id. Empty on servers that predate relations. */
  links: z.record(z.string(), z.array(RecordLinkSchema).catch([])).catch({}),
  revision: z.number().int().positive(),
  created_at: z.string(),
});
/** A record that points at a task or at another record. */
export const RecordBacklinkSchema = z.object({
  id: z.string(),
  collection_id: z.string(),
  collection_name: z.string().catch(""),
  record_id: z.string(),
  record_title: z.string().catch(""),
  field_id: z.string().catch(""),
  field_name: z.string().catch(""),
});
export const RecordBacklinksSchema = z.object({
  links: z.array(RecordBacklinkSchema).catch([]),
});
export const CollectionTrashSchema = z.object({
  records: z.array(
    CollectionRecordSchema.extend({ deleted_at: z.string().catch("") }),
  ),
  total: z.number().nonnegative().catch(0),
  retention_days: z.number().catch(30),
});
export const CollectionAccessSchema = DocumentAccessSchema;
export type CollectionAccess = z.infer<typeof CollectionAccessSchema>;

export const CollectionDetailSchema = z.object({
  collection: CollectionSchema,
  access: CollectionAccessSchema.nullable().default(null),
  fields: z.array(CollectionFieldSchema),
});
export const CollectionPageSchema = z.object({
  records: z.array(CollectionRecordSchema),
  total: z.number().nonnegative(),
  groups: z.array(z.object({ key: z.string(), count: z.number(), value: z.unknown().optional() })),
  next_cursor: z.string().nullable(),
});
export type Collection = z.infer<typeof CollectionSchema>;
export type CollectionField = z.infer<typeof CollectionFieldSchema>;
export type CollectionRecord = z.infer<typeof CollectionRecordSchema>;
export type RelationTarget = z.infer<typeof RelationTargetSchema>;
export type RecordLink = z.infer<typeof RecordLinkSchema>;
export type RecordBacklink = z.infer<typeof RecordBacklinkSchema>;
export interface CollectionQuery {
  /** Omit for a flat list, "root" for roots, or a UUID for direct children. */
  parent_id?: string;
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
  icon?: string;
  description?: string;
  /** Empty returns the title column to its localized default label. */
  title_name?: string;
  /** Archiving removes the table from the workspace; restoration uses the explicit restore endpoint. */
  archived?: true;
}
export interface CollectionFieldInput {
  name: string;
  type: string;
  config?:
    | { formula: { expression: string; bindings?: Record<string, string> } }
    | { options: { name: string; color: string }[] }
    /** Relation fields name their target once; it cannot change later. */
    | {
        relation:
          | { to_type: "issue" }
          | { to_type: "record"; collection_id: string };
      };
}
export interface CollectionFieldPatch {
  name?: string;
  type?: string;
  config?:
    | { options: { id?: string; name: string; color: string }[] }
    | { formula: { expression: string; bindings?: Record<string, string> } };
  position?: number;
  archived?: boolean;
}
export type CollectionTrash = z.infer<typeof CollectionTrashSchema>;

export const CollectionBatchResultSchema = z.object({
  count: z.number().int().nonnegative(),
  dry_run: z.boolean().optional(),
});
export interface CollectionBatchInput {
  action: "update" | "delete" | "restore";
  record_ids: string[];
  fields?: Record<string, unknown>;
  expected_revisions?: Record<string, number>;
  confirmed?: boolean;
}
