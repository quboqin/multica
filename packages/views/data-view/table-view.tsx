"use client";

import { useCallback } from "react";
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
  dataSourceIdentityKey,
  type DataSourceIdentity,
  type DataSource,
} from "@multica/core/data-source";

export interface DataViewTableProps<DataRow>
  extends Pick<DataSource<DataRow>, "capabilities" | "rowId"> {
  sourceIdentity: DataSourceIdentity;
  rows: DataRow[];
  columns: ColumnDef<DataRow>[];
  visibleColumnIds: string[];
  columnSizing: ColumnSizingState;
  onColumnSizingChange: OnChangeFn<ColumnSizingState>;
  onReorderColumn: (activeId: string, overId: string) => void;
  columnPinning?: ColumnPinningState;
  meta?: TableMeta<DataRow>;
  emptyMessage?: string;
  onRowClick?: (row: Row<DataRow>, event: React.MouseEvent) => void;
  renderRow?: (row: Row<DataRow>, columnCount: number) => React.ReactNode;
  className?: string;
}

/** Shared rendering only. Adapters own Query data, permissions and all writes. */
export function DataViewTable<DataRow>(props: DataViewTableProps<DataRow>) {
  if (!props.capabilities.layouts.includes("table")) return null;
  return (
    <TableInstance
      key={dataSourceIdentityKey(props.sourceIdentity)}
      {...props}
    />
  );
}

const EMPTY_PINNING: ColumnPinningState = { left: [], right: [] };

function TableInstance<DataRow>({
  sourceIdentity,
  rows,
  rowId,
  columns,
  visibleColumnIds,
  columnSizing,
  onColumnSizingChange,
  onReorderColumn,
  columnPinning = EMPTY_PINNING,
  meta,
  emptyMessage,
  onRowClick,
  renderRow,
  className,
}: DataViewTableProps<DataRow>) {
  const table = useReactTable({
    data: rows,
    columns,
    getRowId: rowId,
    getCoreRowModel: getCoreRowModel(),
    state: { columnSizing, columnPinning },
    meta,
    onColumnSizingChange,
    columnResizeMode: "onChange",
    // Filtering, sorting and pagination are already applied by the source.
    manualFiltering: true,
    manualSorting: true,
    manualPagination: true,
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
      const activeId = String(active.id);
      const overId = String(over.id);
      if (
        visibleColumnIds.includes(activeId) &&
        visibleColumnIds.includes(overId)
      ) {
        onReorderColumn(activeId, overId);
      }
    },
    [onReorderColumn, visibleColumnIds],
  );

  return (
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
          data-source-identity={dataSourceIdentityKey(sourceIdentity)}
          table={table}
          virtualizeRows
          emptyMessage={emptyMessage}
          onRowClick={onRowClick}
          renderRow={
            renderRow
              ? (row) => renderRow(row, table.getVisibleLeafColumns().length)
              : undefined
          }
          className={className}
        />
      </SortableContext>
    </DndContext>
  );
}
