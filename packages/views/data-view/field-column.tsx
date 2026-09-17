import type { ColumnDef } from "@tanstack/react-table";
import type { DataSourceField } from "@multica/core/data-source";

/** Adapters may supply rich controls without duplicating field capabilities. */
export function createDataViewFieldColumn<DisplayRow, SourceRow>({
  field,
  sourceRow,
  presentation = {},
}: {
  field: DataSourceField<SourceRow>;
  sourceRow: (row: DisplayRow) => SourceRow | null;
  presentation?: Pick<
    ColumnDef<DisplayRow>,
    "header" | "cell" | "size" | "minSize" | "maxSize" | "enableResizing"
  >;
}): ColumnDef<DisplayRow> {
  return {
    header: field.label,
    cell: ({ getValue }) => {
      const value = getValue();
      return value == null ? "" : String(value);
    },
    ...presentation,
    id: field.id,
    accessorFn: (row) => {
      const value = sourceRow(row);
      return value === null ? undefined : field.value(value);
    },
    enableSorting: field.sortable,
    enableColumnFilter: field.filterable,
    enableGrouping: field.groupable,
  };
}
