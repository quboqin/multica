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
import { useCollectionGroupLabel, collectionGroups, collectionGroupValue } from "./collection-grouping";
import { groupValuesEqual } from "@multica/core/properties";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { RecordValue, hasRecordValue } from "./collection-cell";
import type { CollectionCommands } from "./use-collection-commands";

const NONE = "__none__";
const DRAG_TYPE = "application/x-collection-record";

/**
 * Server-derived columns cover the complete filtered result. Select options
 * also keep empty columns so a card always has somewhere to go.
 */
export function CollectionBoard({
  readOnly=false,
  collectionId,
  groupField,
  cardFields,
  query,
  commands,
  onOpenRecord,
}: {
  readOnly?: boolean;
  collectionId: string;
  groupField: CollectionField;
  cardFields: CollectionField[];
  query: CollectionQuery;
  commands: CollectionCommands;
  onOpenRecord: (recordId: string) => void;
}) {
  const [dragging, setDragging] = useState<CollectionRecord | null>(null);
  const wsId = useWorkspaceId();
  const { t } = useT("issues");
  const summary = useInfiniteQuery(collectionRecordsOptions(wsId, collectionId, { ...query, group_by: groupField.id }));
  const columns = collectionGroups(groupField, summary.data?.pages[0]?.groups ?? []);
  if (summary.isError) return <Button variant="outline" onClick={() => void summary.refetch()}>{t(($) => $.table.load_failed_retry)}</Button>;
  return (
    <div className="flex min-h-0 flex-1 gap-3 overflow-x-auto p-4">
      {columns.map((column) => (
        <BoardColumn
          readOnly={readOnly}
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
  readOnly=false,
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
  readOnly?: boolean;
  collectionId: string;
  column: { key: string; count: number; value?: unknown };
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
  const value = collectionGroupValue(groupField, column.key);
  const writable = !readOnly && !["formula", "relation"].includes(groupField.type);
  const empty = !pages.isLoading && count === 0;
  const label = useCollectionGroupLabel(groupField, column);
  const canDrop =
    !readOnly && writable && !!dragging && !groupValuesEqual(groupField.id === "title" ? dragging.title : dragging.fields[groupField.id], value);
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
          void (groupField.id === "title" ? commands.setTitle(dragging, String(value ?? "")) : commands.setField(dragging, groupField.id, value));
        onDragChange(null);
      }}
    >
      <header className="flex items-center gap-2 px-3 py-2">
        <span
          className={cn(
            "size-2.5 shrink-0 rounded-full",
            column.key === NONE && "border border-muted-foreground",
          )}
          style={{ backgroundColor: groupField.config.options.find((option) => option.id === column.key)?.color }}
        />
        <h3 className="min-w-0 truncate text-label font-medium">{label}</h3>
        <span className="text-caption text-muted-foreground tabular-nums">
          {count}
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="ml-auto"
          disabled={!writable}
          aria-label={t(($) => $.cortex_table.add_to_group, { name: label })}
          onClick={() => {
            if (groupField.id === "title") setTitle(String(value ?? ""));
            setAdding(true);
          }}
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
                  fields: groupField.id !== "title" && value !== null ? { [groupField.id]: value } : undefined,
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
            draggable={writable}
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
            {cardFields.some((field) => hasRecordValue(record, field)) && (
              <div className="flex flex-wrap items-center gap-1.5 text-caption">
                {cardFields.map((field) =>
                  hasRecordValue(record, field) ? (
                    <span key={field.id} className="inline-flex max-w-full items-center" title={field.name}>
                      <RecordValue record={record} field={field} compact />
                    </span>
                  ) : null,
                )}
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
