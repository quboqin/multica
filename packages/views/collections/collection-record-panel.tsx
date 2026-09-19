"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { MessageSquareOff, MoreHorizontal, Trash2, X } from "lucide-react";
import {
  collectionRecordOptions,
  type CollectionField,
} from "@multica/core/collections";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { useT, useTimeAgo } from "../i18n";
import { CollectionFieldEditor } from "./collection-cell";
import { FieldTypeIcon } from "./collection-fields";
import type { CollectionCommands } from "./use-collection-commands";

/**
 * Side panel for one record (S6). A record is plain data: no status, no
 * comments and no agent assignment, so the panel explains where discussion
 * belongs instead of leaving the space empty.
 */
export function CollectionRecordPanel({
  wsId,
  collectionId,
  recordId,
  fields,
  commands,
  onClose,
}: {
  wsId: string;
  collectionId: string;
  recordId: string;
  fields: CollectionField[];
  commands: CollectionCommands;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  const timeAgo = useTimeAgo();
  const { data: record, isLoading } = useQuery(
    collectionRecordOptions(wsId, collectionId, recordId),
  );
  const [title, setTitle] = useState(record?.title ?? "");
  const [editing, setEditing] = useState<string | null>(null);
  useEffect(() => {
    setTitle(record?.title ?? "");
  }, [record?.title]);
  const commitTitle = () => {
    if (record && title.trim() !== record.title)
      void commands.setTitle(record, title.trim());
  };

  return (
    <aside
      aria-label={t(($) => $.cortex_table.record_details)}
      className="flex w-[420px] min-w-0 shrink-0 flex-col border-l bg-background"
    >
      <header className="flex items-center gap-2 border-b px-4 py-2.5">
        <span className="rounded-sm bg-muted px-1.5 py-px font-mono text-caption text-muted-foreground">
          {t(($) => $.cortex_table.record_badge)}
        </span>
        <input
          aria-label={t(($) => $.cortex_table.record_title)}
          className="min-w-0 flex-1 bg-transparent text-body font-semibold outline-none"
          value={title}
          disabled={!record}
          onChange={(event) => setTitle(event.target.value)}
          onBlur={commitTitle}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.currentTarget.blur();
            if (event.key === "Escape") {
              setTitle(record?.title ?? "");
              event.currentTarget.blur();
            }
          }}
        />
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t(($) => $.cortex_table.record_actions)}
              />
            }
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-44">
            <DropdownMenuItem
              variant="destructive"
              disabled={!record || commands.deleteRecord.isPending}
              onClick={() =>
                record &&
                commands.deleteRecord.mutate(record.id, { onSuccess: onClose })
              }
            >
              <Trash2 />
              {t(($) => $.cortex_table.move_to_trash)}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t(($) => $.cortex_table.close)}
          onClick={onClose}
        >
          <X />
        </Button>
      </header>
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-4">
        {isLoading && (
          <p role="status" className="text-caption text-muted-foreground">
            {t(($) => $.cortex.loading)}
          </p>
        )}
        {!isLoading && !record && (
          <p role="alert" className="text-caption text-muted-foreground">
            {t(($) => $.cortex_table.record_missing)}
          </p>
        )}
        {record && (
          <>
            <dl className="grid grid-cols-[8rem_minmax(0,1fr)] items-center gap-x-3 gap-y-1">
              {fields.map((field) => (
                <div key={field.id} className="contents">
                  <dt className="flex min-w-0 items-center gap-2 py-1.5 text-label text-muted-foreground">
                    <FieldTypeIcon type={field.type} />
                    <span className="truncate">{field.name}</span>
                  </dt>
                  <dd className="min-w-0 rounded-sm">
                    <CollectionFieldEditor
                      record={record}
                      field={field}
                      open={editing === field.id}
                      onOpenChange={(open) => setEditing(open ? field.id : null)}
                      onChange={(value) => void commands.setField(record, field.id, value)}
                      className="rounded-sm"
                    />
                  </dd>
                </div>
              ))}
            </dl>
            <p className="text-caption text-muted-foreground">
              {t(($) => $.cortex_table.created_ago, {
                time: timeAgo(record.created_at),
              })}
            </p>
            <section className="rounded-md border border-l-4 border-l-destructive/70 bg-muted/30 p-3">
              <h3 className="flex items-center gap-2 text-label font-medium">
                <MessageSquareOff className="size-4 text-muted-foreground" />
                {t(($) => $.cortex_table.no_comments_title)}
              </h3>
              <p className="mt-1.5 text-caption leading-5 text-muted-foreground">
                {t(($) => $.cortex_table.no_comments_body)}
              </p>
            </section>
          </>
        )}
      </div>
    </aside>
  );
}
