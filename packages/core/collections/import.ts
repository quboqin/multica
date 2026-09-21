import { z } from "zod";

export const ImportFieldTypeSchema = z.enum([
  "text",
  "number",
  "checkbox",
  "date",
  "url",
  "select",
]);
export const ImportColumnSchema = z.object({
  index: z.number().int().nonnegative(),
  name: z.string(),
  type: ImportFieldTypeSchema,
  skip: z.boolean().default(false),
  samples: z.array(z.string()).default([]),
});
export const CollectionImportPreviewSchema = z.object({
  name: z.string(),
  sheets: z.array(z.string()),
  sheet: z.string(),
  header_row: z.number().int().positive(),
  title_column: z.number().int().nonnegative(),
  columns: z.array(ImportColumnSchema).max(256),
  row_count: z.number().int().nonnegative().max(10000),
  formula_count: z.number().int().nonnegative().default(0),
});
export type ImportColumn = z.infer<typeof ImportColumnSchema>;
export type CollectionImportPreview = z.infer<
  typeof CollectionImportPreviewSchema
>;
export interface CollectionImportOptions {
  sheet?: string;
  header_row?: number;
  name?: string;
  project_id?: string;
  title_column?: number;
  columns?: ImportColumn[];
}
