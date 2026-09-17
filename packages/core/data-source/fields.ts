import type { DataSourceCellChange, DataSourceField } from "./types";

/** One permission gate for default editors and adapter command execution. */
export function canChangeDataSourceField<Row>(
  writable: boolean,
  field: DataSourceField<Row>,
  row: Row,
  change: DataSourceCellChange,
): boolean {
  return writable && (change.op === "clear" ? field.canClear(row) : field.canSet(row));
}
