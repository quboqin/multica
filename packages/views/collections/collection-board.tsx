"use client";

import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import {
  collectionRecordsOptions,
  type CollectionField,
  type CollectionQuery,
  type CollectionRecord,
} from "@multica/core/collections";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { CollectionValue } from "./collection-cell";
import { recordValue } from "./collection-fields";
import type { CollectionCommands } from "./use-collection-commands";

const NONE = "__none__";
const DRAG_TYPE = "application/x-collection-record";

/**
 * Board layout for a select field. Every option keeps a column, even when the
 * current filter leaves it empty, so a card always has somewhere to go.
 */
export function CollectionBoard({
  collectionId,
  groupField,
  cardFields,
  query,
  commands,
  onOpenRecord,
}: {
  collectionId: string;
  groupField: CollectionField;
  cardFields: CollectionField[];
  query: CollectionQuery;
  commands: CollectionCommands;
  onOpenRecord: (recordId: string) => void;
}) {
  const [dragging, setDragging] = useState<CollectionRecord | null>(null);
  const columns = [
    ...groupField.config.options.map((option) => ({
      key: option.id,
      name: option.name,
      color: option.color,
    })),
    { key: NONE, name: "", color: undefined },
  ];
  return (
    <div className="flex min-h-0 flex-1 gap-3 overflow-x-auto p-4">
      {columns.map((column) => (
        <BoardColumn
          key={column.key}
          collectionId={collectionId}
          column={column}
          groupField={groupField}
          cardFields={cardFields}
          query={{ ...query, group_by: groupField.id, group_key: column.key }}
          commands={commands}
          dragging={dragging}
          onDragChange={setDragging}
          onOpenRecord={onOpenRecord}
        />
      ))}
    </div>
  );
}

function BoardColumn({
  collectionId,
  column,
  groupField,
  cardFields,
  query,
  commands,
  dragging,
  onDragChange,
  onOpenRecord,
}: {
  collectionId: string;
  column: { key: string; name: string; color?: string };
  groupField: CollectionField;
  cardFields: CollectionField[];
  query: CollectionQuery;
  commands: CollectionCommands;
  dragging: CollectionRecord | null;
  onDragChange: (record: CollectionRecord | null) => void;
  onOpenRecord: (recordId: string) => void;
}) {
  const wsId = useWorkspaceId();
  const { t } = useT("issues");
  const pages = useInfiniteQuery(
    collectionRecordsOptions(wsId, collectionId, query),
  );
  const [over, setOver] = useState(false);
  const [adding, setAdding] = useState(false);
  const [title, setTitle] = useState("");
  const records = pages.data?.pages.flatMap((page) => page.records) ?? [];
  const count =
    pages.data?.pages[0]?.groups.find((group) => group.key === column.key)
      ?.count ?? 0;
  const value = column.key === NONE ? null : column.key;
  const empty = !pages.isLoading && count === 0;
  const label = column.key === NONE ? t(($) => $.cortex_table.no_value) : column.name;
  const canDrop =
    !!dragging && (dragging.fields[groupField.id] ?? null) !== value;
  return (
    <section
      aria-label={label}
      className={cn(
        "flex max-h-full w-72 shrink-0 flex-col rounded-lg bg-muted/40",
        empty && "border border-dashed bg-transparent opacity-70",
        over && canDrop && "ring-2 ring-primary/40",
      )}
      onDragOver={(event) => {
        if (!canDrop) return;
        event.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(event) => {
        event.preventDefault();
        setOver(false);
        if (dragging && canDrop)
          void commands.setField(dragging, groupField.id, value);
        onDragChange(null);
      }}
    >
      <header className="flex items-center gap-2 px-3 py-2">
        <span
          className={cn(
            "size-2.5 shrink-0 rounded-full",
            column.key === NONE && "border border-muted-foreground",
          )}
          style={column.color ? { backgroundColor: column.color } : undefined}
        />
        <h3 className="min-w-0 truncate text-label font-medium">{label}</h3>
        <span className="text-caption text-muted-foreground tabular-nums">
          {count}
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="ml-auto"
          aria-label={t(($) => $.cortex_table.add_to_group, { name: label })}
          onClick={() => setAdding(true)}
        >
          <Plus />
        </Button>
      </header>
      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto px-2 pb-2">
        {adding && (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!title.trim()) return;
              void commands.createRecord
                .mutateAsync({
                  title: title.trim(),
                  fields: value ? { [groupField.id]: value } : undefined,
                })
                .then(() => setTitle(""));
            }}
          >
            <input
              autoFocus
              aria-label={t(($) => $.cortex_table.new_row_title)}
              placeholder={t(($) => $.cortex_table.new_row_placeholder)}
              className="w-full rounded-md border bg-background px-2.5 py-2 text-label outline-none focus:ring-2 focus:ring-ring/40"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              onBlur={() => {
                if (!title.trim()) setAdding(false);
              }}
              onKeyDown={(event) => {
                if (event.key === "Escape") setAdding(false);
              }}
            />
          </form>
        )}
        {records.map((record) => (
          <article
            key={record.id}
            draggable
            onDragStart={(event) => {
              event.dataTransfer.setData(DRAG_TYPE, record.id);
              event.dataTransfer.effectAllowed = "move";
              onDragChange(record);
            }}
            onDragEnd={() => onDragChange(null)}
            className={cn(
              "cursor-grab space-y-2 rounded-md border bg-background p-2.5 shadow-xs active:cursor-grabbing",
              dragging?.id === record.id && "opacity-50",
            )}
          >
            <button
              type="button"
              className="block w-full text-left text-label hover:underline"
              onClick={() => onOpenRecord(record.id)}
            >
              {record.title || (
                <span className="text-muted-foreground">
                  {t(($) => $.cortex_table.untitled)}
                </span>
              )}
            </button>
            {cardFields.some((field) => recordValue(record, field) !== undefined) && (
              <div className="flex flex-wrap items-center gap-1.5 text-caption">
                {cardFields.map((field) => {
                  const fieldValue = recordValue(record, field);
                  if (fieldValue === undefined) return null;
                  return (
                    <span key={field.id} className="inline-flex max-w-full items-center" title={field.name}>
                      <CollectionValue field={field} value={fieldValue} compact />
                    </span>
                  );
                })}
              </div>
            )}
          </article>
        ))}
        {pages.hasNextPage && (
          <Button
            variant="ghost"
            size="sm"
            className="w-full"
            disabled={pages.isFetching}
            onClick={() => void pages.fetchNextPage()}
          >
            {t(($) => $.cortex.load_more)}
          </Button>
        )}
      </div>
    </section>
  );
}
