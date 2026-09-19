"use client";

import { useLayoutEffect, useRef, type ReactNode } from "react";
import { useDndContext } from "@dnd-kit/core";
import { useSortable } from "@dnd-kit/sortable";
import { ArrowDown, ArrowUp, EyeOff, GripVertical } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";

export function DataViewColumnHeader<SortKey extends string>({
  columnKey,
  reorderable,
  label,
  sortField,
  sortBy,
  sortDirection,
  onSort,
  onHide,
  ascendingLabel,
  descendingLabel,
  hideLabel,
  reorderLabel,
  icon,
  menuContent,
  menuClassName,
}: {
  columnKey: string;
  reorderable: boolean;
  label: string;
  sortField?: SortKey;
  sortBy: SortKey;
  sortDirection: "asc" | "desc";
  onSort: (field: SortKey, direction: "asc" | "desc") => void;
  onHide?: () => void;
  ascendingLabel: string;
  descendingLabel: string;
  hideLabel: string;
  reorderLabel: string;
  /** Leading glyph, e.g. the field type. */
  icon?: ReactNode;
  /** Replaces the default sort / hide items with a source-specific menu. */
  menuContent?: ReactNode;
  menuClassName?: string;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: columnKey, disabled: !reorderable });
  const active = sortField === sortBy;
  // Any column in flight, not only this one: the neighbours shift to open a
  // gap, and each is clipped by its own cell just the same.
  const isReordering = useDndContext().active != null;
  const nodeRef = useRef<HTMLDivElement | null>(null);

  // The cell clips its own content, which is what made a dragged column look
  // like it vanished rather than travelled. The clip only earns its keep at
  // rest, capping a label wider than its column, so it is lifted for the length
  // of a reorder and the column in hand is raised over its neighbours.
  //
  // Only overflow and stacking are touched. Transforming the <th> itself would
  // carry the header's full height along, but `transform` on a table cell is a
  // corner of the spec browsers take liberties with — Chromium lifts the cell
  // out of the table's box model and its geometry stops matching the row. The
  // wrapper below is padded out to the cell's size instead.
  useLayoutEffect(() => {
    const cell = nodeRef.current?.closest("th");
    if (!cell || !isReordering) return;
    cell.style.overflow = "visible";
    if (isDragging) cell.style.zIndex = "20";
    return () => {
      cell.style.removeProperty("overflow");
      cell.style.removeProperty("z-index");
    };
  }, [isDragging, isReordering]);

  return (
    <div
      ref={(node) => {
        nodeRef.current = node;
        setNodeRef(node);
      }}
      // Horizontal travel only. dnd-kit's layout animation also hands back
      // scaleX/scaleY — old rect over new rect — to tween an item into the
      // shape of the slot it landed in. Between two tabs of equal width that
      // ratio is 1 and never shows; between two columns it is not, so a 174px
      // column swapping with a 96px one gets stretched to 1.8x on the way.
      // Reordering columns changes no column's width, so there is nothing for
      // a shape tween to say here. The move and the settle stay animated
      // through `transition`.
      style={{
        transform: transform
          ? `translate3d(${transform.x}px, 0, 0)`
          : undefined,
        transition,
      }}
      // The wrapper spans the cell's own box — the negative margins undo the
      // <th>'s padding and put it back inside — so it renders exactly as at
      // rest while being what travels: a header-sized block rather than the
      // line of text in it. Height is derived rather than fixed at h-8: the
      // strip is taller than the cell's nominal height once row borders are in.
      className={cn(
        "group/header -mx-4 -my-2 flex h-[calc(100%+1rem)] min-w-0 items-center px-4",
        isDragging && "opacity-60",
      )}
    >
      {reorderable && (
        <button
          type="button"
          aria-label={reorderLabel}
          className={cn(
            "-ml-2 mr-0.5 rounded-xs p-0.5 text-muted-foreground opacity-0 hover:bg-accent hover:text-muted-foreground group-hover/header:opacity-100 focus-visible:opacity-100",
            isDragging ? "cursor-grabbing opacity-100" : "cursor-grab",
          )}
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-3" />
        </button>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger className="flex min-w-0 items-center gap-1 rounded-xs px-1.5 py-1 hover:bg-accent">
          {icon}
          <span className="truncate">{label}</span>
          {active &&
            (sortDirection === "asc" ? (
              <ArrowUp className="size-3 shrink-0" />
            ) : (
              <ArrowDown className="size-3 shrink-0" />
            ))}
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className={cn(menuContent ? "w-60" : "w-40", menuClassName)}
        >
          {menuContent}
          {!menuContent && sortField && (
            <>
              <DropdownMenuItem onClick={() => onSort(sortField, "asc")}>
                <ArrowUp />
                {ascendingLabel}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => onSort(sortField, "desc")}>
                <ArrowDown />
                {descendingLabel}
              </DropdownMenuItem>
            </>
          )}
          {!menuContent && sortField && onHide && <DropdownMenuSeparator />}
          {!menuContent && onHide && (
            <DropdownMenuItem onClick={onHide}>
              <EyeOff />
              {hideLabel}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
