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
