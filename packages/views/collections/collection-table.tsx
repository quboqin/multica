"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import type { ColumnDef, ColumnSizingState } from "@tanstack/react-table";
import { Maximize2, Plus, Trash2 } from "lucide-react";
import {
  collectionRecordOptions,
  collectionRecordsOptions,
  type CollectionField,
  type CollectionQuery,
  type CollectionRecord,
} from "@multica/core/collections";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@multica/ui/components/ui/context-menu";
import { cn } from "@multica/ui/lib/utils";
import { DataViewColumnHeader, DataViewTable, useFrozenRows } from "../data-view";
import { useT } from "../i18n";
import { CollectionFieldEditor } from "./collection-cell";
import { CollectionFieldMenu, CollectionTitleMenu } from "./collection-field-menu";
import { FieldTypeIcon, fieldIsQueryable, isRelation } from "./collection-fields";
import type { CollectionCommands } from "./use-collection-commands";

export const tableCapabilities = {
  layouts: ["table"],
  editing: "adapter",
  sideEffects: "none",
  sorting: "adapter",
} as const;

export interface CollectionTableActions {
  readOnly?: boolean;
  selection?: { rows: CollectionRecord[]; toggle: (row: CollectionRecord, checked: boolean) => void };
  canManage: boolean;
  fieldCount: number;
  sortBy: string;
  sortDir: "asc" | "desc";
  onSort: (fieldId: string, direction: "asc" | "desc") => void;
  onGroup: (fieldId: string) => void;
  onFilter: (fieldId: string) => void;
  onHide: (fieldId: string) => void;
  /** `anchor` is the header cell the field panel hangs under. */
  onEditField: (field: CollectionField, anchor: Element | null) => void;
  /** Renames the title column, the only thing about it that can change. */
  onEditTitle: (anchor: Element | null) => void;
  onArchiveField: (field: CollectionField) => void;
  onAddField: (anchor: Element | null) => void;
  onReorderField: (activeId: string, overId: string) => void;
  onOpenRecord: (recordId: string) => void;
}

/**
 * One page-able table of records. A grouped table renders one of these per
 * group so each group keeps its own server cursor.
 */
export function CollectionTable({
  collectionId,
  titleName,
  fields,
  query,
  commands,
  actions,
  selectedRecordId,
  newRecordFields,
  showHeader = true,
}: {
  collectionId: string;
  /** Label of the title column. */
  titleName: string;
  fields: CollectionField[];
  query: CollectionQuery;
  commands: CollectionCommands;
  actions: CollectionTableActions;
  selectedRecordId: string | null;
  /** Values a row created in this table starts with, e.g. its group. */
  newRecordFields?: Record<string, unknown>;
  showHeader?: boolean;
}) {
  const wsId = useWorkspaceId();
  const { t } = useT("issues");
  const pages = useInfiniteQuery(
    collectionRecordsOptions(wsId, collectionId, query),
  );
  const [editing, setEditing] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [sizing, setSizing] = useState<ColumnSizingState>({ title: 240 });
  const headers = useRef(new Map<string, HTMLElement>());
  // The row being edited is read on its own, so it stays editable even after
  // a filter, group or page change would move it out of this table.
  const focused = useQuery(
    collectionRecordOptions(wsId, collectionId, editing?.split(":")[0] ?? null),
  );
  const loaded = useMemo(
    () => pages.data?.pages.flatMap((page) => page.records) ?? [],
    [pages.data],
  );
  const identity = useMemo(
    () => ({
      workspaceId: wsId,
      namespace: "collection",
      sourceId: `${collectionId}:${query.group_key ?? ""}`,
    }),
    [wsId, collectionId, query.group_key],
  );
  const reconcile = useCallback(
    (snapshot: CollectionRecord[]) => {
      const byId = new Map(loaded.map((row) => [row.id, row]));
      if (focused.data) {
        const visible = byId.get(focused.data.id);
        if (!visible || visible.revision <= focused.data.revision)
          byId.set(focused.data.id, focused.data);
      }
      return snapshot.map((row) => byId.get(row.id) ?? row);
    },
    [loaded, focused.data],
  );
  const rows = useFrozenRows(identity, loaded, editing, reconcile);

  const selectionRows = useRef(rows);
  selectionRows.current = rows;
  const columns = useMemo<ColumnDef<CollectionRecord>[]>(() => {
    const titleColumn: ColumnDef<CollectionRecord> = {
      id: "title",
      header: () => (
        <div
          ref={(node) => {
            if (node) headers.current.set("title", node);
            else headers.current.delete("title");
          }}
          className="normal-case tracking-normal"
        >
          <DataViewColumnHeader
            columnKey="title"
            reorderable={false}
            fillCell
            label={titleName}
            icon={<FieldTypeIcon type="text" />}
            sortField="title"
            sortBy={actions.sortBy}
            sortDirection={actions.sortDir}
            onSort={(id, direction) => actions.onSort(id, direction)}
            ascendingLabel={t(($) => $.cortex_table.sort_ascending)}
            descendingLabel={t(($) => $.cortex_table.sort_descending)}
            hideLabel={t(($) => $.cortex_table.hide_in_view)}
            reorderLabel={t(($) => $.cortex_table.reorder_field, { name: titleName })}
            menuContent={
              <CollectionTitleMenu
                name={titleName}
                canManage={actions.canManage}
                onEdit={() =>
                  actions.onEditTitle(
                    headers.current.get("title")?.closest("th") ?? null,
                  )
                }
                onSort={(direction) => actions.onSort("title", direction)}
              />
            }
          />
        </div>
      ),
      cell: ({ row }) => (
        <TitleCell
          readOnly={actions.readOnly}
          record={row.original}
          renaming={renaming === row.original.id}
          onRenamingChange={(active) => {
            setRenaming(active ? row.original.id : null);
            setEditing(active ? `${row.original.id}:title` : null);
          }}
          onRename={(title) => void commands.setTitle(row.original, title)}
          onOpen={() => actions.onOpenRecord(row.original.id)}
          onDelete={() => commands.deleteRecord.mutate(row.original.id)}
        />
      ),
    };
    const fieldColumns = fields.map<ColumnDef<CollectionRecord>>((field) => ({
      id: field.id,
      size:
        field.type === "checkbox"
          ? 100
          : field.type === "multi_select" || field.type === "multi_actor" || isRelation(field)
            ? 200
            : 150,
      header: () => (
        <div
          ref={(node) => {
            if (node) headers.current.set(field.id, node);
            else headers.current.delete(field.id);
          }}
          className="normal-case tracking-normal"
        >
          <DataViewColumnHeader
            columnKey={field.id}
            reorderable={actions.canManage}
            fillCell
            label={field.name}
            icon={<FieldTypeIcon type={field.type} />}
            sortField={fieldIsQueryable(field) ? field.id : undefined}
            sortBy={actions.sortBy}
            sortDirection={actions.sortDir}
            onSort={(id, direction) => actions.onSort(id, direction)}
            ascendingLabel={t(($) => $.cortex_table.sort_ascending)}
            descendingLabel={t(($) => $.cortex_table.sort_descending)}
            hideLabel={t(($) => $.cortex_table.hide_in_view)}
            reorderLabel={t(($) => $.cortex_table.reorder_field, { name: field.name })}
            menuContent={
              <CollectionFieldMenu
                field={field}
                fieldCount={actions.fieldCount}
                canManage={actions.canManage}
                onEdit={() =>
                  actions.onEditField(
                    field,
                    headers.current.get(field.id)?.closest("th") ?? null,
                  )
                }
                onSort={
                  fieldIsQueryable(field)
                    ? (direction) => actions.onSort(field.id, direction)
                    : undefined
                }
                onGroup={
                  field.type === "select"
                    ? () => actions.onGroup(field.id)
                    : undefined
                }
                onFilter={
                  fieldIsQueryable(field)
                    ? () => actions.onFilter(field.id)
                    : undefined
                }
                onHide={() => actions.onHide(field.id)}
                onArchive={() => actions.onArchiveField(field)}
              />
            }
          />
        </div>
      ),
      cell: ({ row }) => {
        const key = `${row.original.id}:${field.id}`;
        return (
          <div className="-mx-4 -my-2 h-[calc(100%+1rem)]">
            <CollectionFieldEditor
              readOnly={actions.readOnly}
              record={row.original}
              field={field}
              open={editing === key}
              onOpenChange={(open) => setEditing(open ? key : null)}
              onChange={(value) =>
                void commands.setField(row.original, field.id, value)
              }
              relation={{ link: commands.linkRecord, unlink: commands.unlinkRecord }}
              className={cn(field.type !== "checkbox" && "px-4")}
            />
          </div>
        );
      },
    }));
    // Takes the width the fields leave over, so "+" always sits right after
    // the last field instead of drifting to the far edge of a stretched table.
    const addColumn: ColumnDef<CollectionRecord> = {
      id: "__add",
      size: 48,
      enableResizing: false,
      meta: { grow: true },
      header: () =>
        actions.canManage ? (
          <Button
            variant="ghost"
            size={fields.length ? "icon-xs" : "xs"}
            className="-mx-2 text-muted-foreground normal-case tracking-normal"
            aria-label={t(($) => $.cortex_table.new_field)}
            onClick={(event) =>
              actions.onAddField(event.currentTarget.closest("th"))
            }
          >
            <Plus />
            {/* A table with no fields yet spells the action out. */}
            {fields.length === 0 && t(($) => $.cortex_table.new_field)}
          </Button>
        ) : null,
      cell: () => null,
    };
    const selectionColumns: ColumnDef<CollectionRecord>[] = actions.selection ? [{id: "__select", size: 44, enableResizing: false,
      header: () => <input type="checkbox" aria-label={t($=>$.cortex_bulk.select_page)} checked={selectionRows.current.length>0 && selectionRows.current.every(r=>actions.selection!.rows.some(s=>s.id===r.id))} onChange={e=>selectionRows.current.forEach(r=>actions.selection!.toggle(r,e.target.checked))}/>,
      cell: ({row}) => <input type="checkbox" aria-label={t($=>$.cortex_bulk.select_row,{title:row.original.title})} checked={actions.selection!.rows.some(r=>r.id===row.original.id)} onChange={e=>actions.selection!.toggle(row.original,e.target.checked)}/>
    }] : [];
    return [...selectionColumns, titleColumn, ...fieldColumns, addColumn];
  }, [titleName, fields, actions, commands, editing, renaming, t]);

  const total = pages.data?.pages[0]?.total ?? 0;
  return (
    <div
      className="flex min-h-0 flex-col"
      data-selected-record={selectedRecordId ?? undefined}
    >
      <DataViewTable
        sourceIdentity={identity}
        rows={rows}
        rowId={(row) => row.id}
        columns={columns}
        visibleColumnIds={fields.map((field) => field.id)}
        columnSizing={sizing}
        onColumnSizingChange={setSizing}
        onReorderColumn={actions.onReorderField}
        columnPinning={{ left: ["title"], right: [] }}
        capabilities={tableCapabilities}
        emptyMessage={
          pages.isLoading
            ? t(($) => $.cortex.loading)
            : t(($) => $.cortex_table.no_records)
        }
        className={cn(
          !showHeader && "[&_thead]:hidden",
          "[&_tbody_tr]:h-9",
        )}
      />
      {pages.error && (
        <p role="alert" className="px-4 py-2 text-caption text-destructive">
          {pages.error.message}
        </p>
      )}
      {pages.hasNextPage && (
        <button
          type="button"
          className="border-b px-4 py-2 text-left text-caption text-muted-foreground hover:bg-accent/40"
          disabled={pages.isFetching}
          onClick={() => void pages.fetchNextPage()}
        >
          {t(($) => $.cortex_table.load_more, {
            loaded: loaded.length,
            total,
          })}
        </button>
      )}
      {!actions.readOnly && <NewRecordRow
        pending={commands.createRecord.isPending}
        onCreate={(title) =>
          commands.createRecord.mutateAsync({ title, fields: newRecordFields })
        }
      />}
    </div>
  );
}

function TitleCell({
  readOnly=false,
  record,
  renaming,
  onRenamingChange,
  onRename,
  onOpen,
  onDelete,
}: {
  record: CollectionRecord;
  readOnly?: boolean;
  renaming: boolean;
  onRenamingChange: (renaming: boolean) => void;
  onRename: (title: string) => void;
  onOpen: () => void;
  onDelete: () => void;
}) {
  const { t } = useT("issues");
  const [draft, setDraft] = useState(record.title);
  if (renaming && !readOnly)
    return (
      <input
        autoFocus
        aria-label={t(($) => $.cortex_table.record_title_of, { title: record.title })}
        className="-mx-1 w-full rounded-xs bg-background px-1 text-label outline-none ring-2 ring-ring/40"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={() => {
          if (draft.trim() !== record.title) onRename(draft.trim());
          onRenamingChange(false);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") {
            setDraft(record.title);
            onRenamingChange(false);
          }
        }}
      />
    );
  return (
    <ContextMenu>
      <ContextMenuTrigger className="flex min-w-0 items-center gap-1">
        <button
          type="button"
          className="min-w-0 flex-1 truncate text-left text-body"
          aria-label={t(($) => readOnly ? $.cortex_table.open_record : $.cortex_table.rename_record, { title: record.title })}
          onClick={() => {
            if(readOnly) {onOpen();return;}
            setDraft(record.title);
            onRenamingChange(true);
          }}
        >
          {record.title || (
            <span className="font-normal text-muted-foreground">
              {t(($) => $.cortex_table.untitled)}
            </span>
          )}
        </button>
        <Button
          variant="outline"
          size="xs"
          className="h-6 shrink-0 gap-1 px-1.5 text-caption opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
          aria-label={t(($) => $.cortex_table.open_record, { title: record.title })}
          onClick={onOpen}
        >
          <Maximize2 className="size-3" />
          {t(($) => $.cortex_table.open)}
        </Button>
      </ContextMenuTrigger>
      <ContextMenuContent className="w-44">
        <ContextMenuItem onClick={onOpen}>
          <Maximize2 />
          {t(($) => $.cortex_table.open)}
        </ContextMenuItem>
        <ContextMenuItem variant="destructive" disabled={readOnly} onClick={onDelete}>
          <Trash2 />
          {t(($) => $.cortex_table.move_to_trash)}
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

/** Inline "+ New row" line under a table (Enter keeps it open for the next). */
function NewRecordRow({
  pending,
  onCreate,
}: {
  pending: boolean;
  onCreate: (title: string) => Promise<unknown>;
}) {
  const { t } = useT("issues");
  const [active, setActive] = useState(false);
  const [title, setTitle] = useState("");
  if (!active)
    return (
      <button
        type="button"
        className="flex items-center gap-1.5 border-b px-4 py-2 text-left text-label text-muted-foreground hover:bg-accent/40"
        onClick={() => setActive(true)}
      >
        <Plus className="size-3.5" />
        {t(($) => $.cortex_table.new_row)}
      </button>
    );
  return (
    <form
      className="flex items-center gap-1.5 border-b px-4 py-1.5"
      onSubmit={(event) => {
        event.preventDefault();
        if (!title.trim() || pending) return;
        void onCreate(title.trim()).then(() => setTitle(""));
      }}
    >
      <Plus className="size-3.5 text-muted-foreground" />
      <input
        autoFocus
        aria-label={t(($) => $.cortex_table.new_row_title)}
        placeholder={t(($) => $.cortex_table.new_row_placeholder)}
        className="min-w-0 flex-1 bg-transparent text-label outline-none"
        value={title}
        disabled={pending}
        onChange={(event) => setTitle(event.target.value)}
        onBlur={() => {
          if (!title.trim()) setActive(false);
        }}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            setTitle("");
            setActive(false);
          }
        }}
      />
    </form>
  );
}
