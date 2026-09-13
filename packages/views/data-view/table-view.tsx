"use client";

import { useCallback, useRef } from "react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { restrictToHorizontalAxis } from "@dnd-kit/modifiers";
import {
  SortableContext,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
} from "@dnd-kit/sortable";
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type ColumnPinningState,
  type ColumnSizingState,
  type OnChangeFn,
  type Row,
  type TableMeta,
} from "@tanstack/react-table";
import { DataTable } from "@multica/ui/components/ui/data-table";
import {
  TableCell,
  TableRow,
} from "@multica/ui/components/ui/table";
import {
  dataSourceIdentityString,
  type DataSourceIdentity,
} from "@multica/core/data-source";

const EMPTY_COLUMN_PINNING: ColumnPinningState = { left: [], right: [] };

export type DataViewTableProps<DataRow> = {
  sourceIdentity: DataSourceIdentity;
  writable: boolean;
  rows: DataRow[];
  columns: ColumnDef<DataRow>[];
  rowId: (row: DataRow) => string;
  visibleColumnIds: string[];
  columnSizing: ColumnSizingState;
  onColumnSizingChange: OnChangeFn<ColumnSizingState>;
  onReorderColumn: (activeId: string, overId: string) => void;
  meta?: TableMeta<DataRow>;
  columnPinning?: ColumnPinningState;
  emptyMessage?: string;
  onRowClick?: (row: Row<DataRow>, event: React.MouseEvent) => void;
  renderStructuralRow?: (
    row: Row<DataRow>,
  ) =>
    | {
        content: React.ReactNode;
        rowClassName?: string;
        cellClassName?: string;
        onClick?: () => void;
      }
    | null;
  editingKey?: string | null;
  refreshFrozenRows?: (
    snapshot: DataRow[],
    liveRows: DataRow[],
  ) => DataRow[];
  className?: string;
};

/**
 * Shared production table shell. It owns the TanStack row loop, virtualization,
 * column sizing and reorder lifecycle; domain assemblies provide fields,
 * display rows and business-only renderers through typed inputs.
 */
export function TableView<DataRow>({
  sourceIdentity,
  writable,
  ...props
}: DataViewTableProps<DataRow>) {
  const instanceKey = dataSourceIdentityString(sourceIdentity);
  return (
    <TableViewInstance
      key={instanceKey}
      sourceIdentity={sourceIdentity}
      writable={writable}
      {...props}
    />
  );
}

function TableViewInstance<DataRow>({
  sourceIdentity,
  writable,
  rows,
  columns,
  rowId,
  visibleColumnIds,
  columnSizing,
  onColumnSizingChange,
  onReorderColumn,
  meta,
  columnPinning,
  emptyMessage,
  onRowClick,
  renderStructuralRow,
  editingKey = null,
  refreshFrozenRows,
  className,
}: DataViewTableProps<DataRow>) {
  const frozenRowsRef = useRef<DataRow[] | null>(null);
  if (editingKey === null) frozenRowsRef.current = null;
  else if (frozenRowsRef.current === null) frozenRowsRef.current = rows;
  const frozenRows = frozenRowsRef.current;
  const displayRows =
    frozenRows && frozenRows !== rows
      ? (refreshFrozenRows?.(frozenRows, rows) ?? frozenRows)
      : rows;
  const table = useReactTable({
    data: displayRows,
    columns,
    getRowId: rowId,
    getCoreRowModel: getCoreRowModel(),
    state: {
      columnSizing,
      columnPinning: columnPinning ?? EMPTY_COLUMN_PINNING,
    },
    meta,
    onColumnSizingChange,
    columnResizeMode: "onChange",
  });
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );
  const handleDragEnd = useCallback(
    ({ active, over }: DragEndEvent) => {
      if (!over || active.id === over.id) return;
      onReorderColumn(String(active.id), String(over.id));
    },
    [onReorderColumn],
  );

  return (
    <div
      className="contents"
      data-source-identity={dataSourceIdentityString(sourceIdentity)}
      data-source-writable={writable ? "true" : "false"}
    >
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        modifiers={[restrictToHorizontalAxis]}
        autoScroll={{ threshold: { x: 0.05, y: 0 } }}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={visibleColumnIds}
          strategy={horizontalListSortingStrategy}
        >
          <DataTable
            table={table}
            virtualizeRows
            emptyMessage={emptyMessage}
            onRowClick={onRowClick}
            renderRow={
              renderStructuralRow
                ? (row) => {
                    const structural = renderStructuralRow(row);
                    return structural ? (
                      <TableRow
                        className={structural.rowClassName}
                        onClick={structural.onClick}
                      >
                        <TableCell
                          colSpan={table.getVisibleLeafColumns().length}
                          className={structural.cellClassName}
                        >
                          {structural.content}
                        </TableCell>
                      </TableRow>
                    ) : null;
                  }
                : undefined
            }
            className={className}
          />
        </SortableContext>
      </DndContext>
    </div>
  );
}
