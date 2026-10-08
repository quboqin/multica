import { z } from "zod";
import { IssueSchema } from "@multica/core/api/schemas";

export const DocumentListSchema = z.array(IssueSchema);

export const ResourceAccessSchema = z.object({
  owner_id: z.string(),
  scope: z.enum(["private", "project", "workspace"]),
  scope_role: z.enum(["view", "edit"]).catch("view"),
  project_id: z.string().nullable().catch(null),
  revision: z.number(),
  can_edit: z.boolean().catch(false),
  can_manage: z.boolean().catch(false),
});

export const CollectionSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  name: z.string(),
  icon: z.string().catch(""),
  description: z.string().catch(""),
  archived_at: z.string().nullable().catch(null),
  project_id: z.string().nullable().catch(null),
  record_count: z.number().nonnegative().optional().catch(undefined),
  title_name: z.string().catch(""),
});

export const CollectionFieldSchema = z.object({
  id: z.string(),
  name: z.string(),
  type: z.string(),
  position: z.number().catch(0),
});

export const CollectionRecordSchema = z.object({
  id: z.string(),
  collection_id: z.string(),
  title: z.string(),
  fields: z.record(z.string(), z.unknown()).catch({}),
  revision: z.number().int().positive(),
});

export const CollectionListSchema = z.array(CollectionSchema);
export const CollectionDetailSchema = z.object({
  collection: CollectionSchema,
  access: ResourceAccessSchema.nullable().catch(null),
  fields: z.array(CollectionFieldSchema),
});
export const CollectionPageSchema = z.object({
  records: z.array(CollectionRecordSchema),
  total: z.number().nonnegative(),
  next_cursor: z.string().nullable().catch(null),
});

export type Collection = z.infer<typeof CollectionSchema>;
export type CollectionDetail = z.infer<typeof CollectionDetailSchema>;
export type CollectionRecord = z.infer<typeof CollectionRecordSchema>;
export type CollectionPage = z.infer<typeof CollectionPageSchema>;
