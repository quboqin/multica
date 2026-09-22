import { z } from "zod";

export const DocumentRoleSchema = z.enum(["view", "edit"]);
export const DocumentAccessSchema = z.object({
  owner_id: z.string(),
  scope: z.enum(["private", "project", "workspace"]),
  scope_role: DocumentRoleSchema.catch("view").default("view"),
  project_id: z.string().nullable().default(null),
  revision: z.number(),
  can_edit: z.boolean().catch(false).default(false),
  can_manage: z.boolean().catch(false).default(false),
  collaborators: z
    .array(z.object({ user_id: z.string(), role: DocumentRoleSchema }))
    .default([]),
});
export type DocumentAccess = z.infer<typeof DocumentAccessSchema>;
export type DocumentSharingInput = Pick<
  DocumentAccess,
  "scope" | "scope_role" | "project_id" | "collaborators"
> & { expected_revision: number };
export const DocumentVersionSchema = z.object({
  version: z.number(),
  title: z.string(),
  actor_type: z.string(),
  actor_id: z.string().nullable(),
  action: z.string(),
  restored_from: z.number().nullable(),
  created_at: z.string(),
});
export const DocumentSnapshotSchema = DocumentVersionSchema.extend({
  body: z.string(),
});
export const DocumentVersionsSchema = z.object({
  versions: z.array(DocumentVersionSchema),
  next_cursor: z.number().nullable().default(null),
});
export type DocumentVersion = z.infer<typeof DocumentVersionSchema>;

export type DocumentSnapshot = z.infer<typeof DocumentSnapshotSchema>;
export type DocumentVersions = z.infer<typeof DocumentVersionsSchema>;
