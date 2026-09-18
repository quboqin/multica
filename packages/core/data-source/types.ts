export type DataSourceLayout =
  | "table"
  | "board"
  | "gantt"
  | "swimlane"
  | "calendar"
  | "gallery";

export interface DataSourceCapabilities {
  layouts: readonly DataSourceLayout[];
  grouping: boolean;
  hierarchy: boolean;
  writable: boolean;
  maxPageSize: number;
}

/** Stable, structured identity for cache, draft, selection and view-state isolation. */
export interface DataSourceIdentity {
  workspaceId: string;
  namespace: string;
  sourceId: string;
}

export type DataSourceFieldKind =
  | "text"
  | "number"
  | "select"
  | "multi_select"
  | "checkbox"
  | "date"
  | "url"
  | "actor"
  | "multi_actor"
  | "readonly";

export interface DataSourceFieldOption<Value = unknown> {
  id: string;
  label: string;
  value: Value;
  color?: string;
}

/** A field projection is domain-free: adapters retain DTO and wire mappings. */
export interface DataSourceField<Row, Value = unknown> {
  id: string;
  label: string;
  kind: DataSourceFieldKind;
  value(row: Row): Value | undefined;
  sortable: boolean;
  /** Whether the adapter query supports filtering this field. */
  filterable: boolean;
  groupable: boolean;
  canSet(row: Row): boolean;
  canClear(row: Row): boolean;
  options?: readonly DataSourceFieldOption<Value>[];
}

export type DataSourceCellChange<Value = unknown> =
  | { op: "set"; value: Value }
  | { op: "clear" };

export interface DataSourceCellCommand<Row, Value = unknown> {
  row: Row;
  fieldId: string;
  change: DataSourceCellChange<Value>;
}

export interface DataSourceBranchRef {
  groupKey: string | null;
  parentRowId: string | null;
}

export type DataSourceGroupValueState = "value" | "unset" | "unavailable";

export interface DataSourceGroupDescriptor<Value = unknown> {
  key: string;
  label: string;
  count: number;
  valueState: DataSourceGroupValueState;
  value?: Value;
}

export interface DataSourceGroupPage<Value = unknown> {
  groups: DataSourceGroupDescriptor<Value>[];
  total: number;
  nextCursor: string | null;
}

export interface DataSourcePageRequest {
  limit?: number;
  cursor?: string | null;
}

export interface DataSourcePage<Row, Metadata = Record<string, never>> {
  rows: Row[];
  total: number;
  nextCursor: string | null;
  metadata: Metadata;
}

export type DataSourceActionResult<Row> =
  | { status: "accepted"; row?: Row }
  | { status: "cancelled" }
  | { status: "failed"; error: Error };

/**
 * Headless contract consumed by shared data views. Domain adapters own query
 * translation and side-effect policy; view code only sees capabilities, rows,
 * and commands.
 */
export interface DataSource<
  Row,
  Query,
  Command,
  Field = unknown,
  Metadata = Record<string, never>,
> {
  identity: DataSourceIdentity;
  /** @deprecated Prefer the structured identity. Kept for legacy diagnostics. */
  key: string;
  fields: readonly Field[];
  capabilities: DataSourceCapabilities;
  rowId(row: Row): string;
  read(
    query: Query,
    page: DataSourcePageRequest,
    signal?: AbortSignal,
  ): Promise<DataSourcePage<Row, Metadata>>;
  execute(command: Command): Promise<DataSourceActionResult<Row>>;
}
