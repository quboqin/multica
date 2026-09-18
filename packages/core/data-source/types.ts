/** Presentation identity; server queries remain owned by each source adapter. */
export interface DataSourceIdentity {
  workspaceId: string;
  namespace: string;
  sourceId: string;
}

export const FIELD_TYPES = [
  "text",
  "number",
  "select",
  "multi_select",
  "date",
  "checkbox",
  "url",
  "actor",
  "multi_actor",
] as const;
export type FieldType = (typeof FIELD_TYPES)[number];
export type DataSourceFieldKind = FieldType | "readonly";

/** Wire names, permission checks and mutations belong to the adapter. */
export interface DataSourceField<Row, Key extends string = string> {
  id: Key;
  label: string;
  kind: DataSourceFieldKind;
  value: (row: Row) => unknown;
  sortKey?: string;
  options?: ReadonlyArray<{ id: string; label: string; color?: string }>;
}

/** A renderer never infers business actions from a field name or row shape. */
export interface DataSourceCapabilities {
  layouts: readonly string[];
  editing: "adapter" | "none";
  sideEffects: "adapter" | "none";
  sorting: "adapter" | "none";
}

export function dataSourceIdentityKey(identity: DataSourceIdentity): string {
  return JSON.stringify([
    identity.workspaceId,
    identity.namespace,
    identity.sourceId,
  ]);
}

/** Source adapters own Query reads and writes; renderers receive only presentation data. */
export interface DataSource<Row> {
  identity: DataSourceIdentity;
  capabilities: DataSourceCapabilities;
  fields: readonly DataSourceField<Row>[];
  rowId: (row: Row) => string;
}
