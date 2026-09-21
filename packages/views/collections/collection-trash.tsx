"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Trash2 } from "lucide-react";
import {api} from "@multica/core/api";
import { collectionKeys, collectionTrashOptions } from "@multica/core/collections";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { useT, useTimeAgo } from "../i18n";
import type { CollectionCommands } from "./use-collection-commands";

/**
 * Deleted records wait here for the retention window. Records leave a table by
 * soft delete, unlike tasks, which stay in the system as cancelled.
 */
export function CollectionTrash({
  wsId,
  collectionId,
  commands,
}: {
  wsId: string;
  collectionId: string;
  commands: CollectionCommands;
}) {
  const { t } = useT("issues");
  const timeAgo = useTimeAgo();
  const { data } = useQuery(collectionTrashOptions(wsId, collectionId));
  const client = useQueryClient();
  const restoreBatch = useMutation({mutationFn: ()=>api.batchCollectionRecords(collectionId,{action:"restore",record_ids:(data?.records ?? []).map(r=>r.id)}),onSuccess:()=>client.invalidateQueries({queryKey:collectionKeys.all(wsId)})});
  const total = data?.total ?? 0;
  const days = data?.retention_days ?? 30;
  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type="button"
            className="inline-flex items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-caption text-muted-foreground hover:bg-accent hover:text-foreground"
          />
        }
      >
        <Trash2 className="size-3.5" />
        {t(($) => $.cortex_table.trash, { count: total, days })}
      </PopoverTrigger>
      <PopoverContent align="end" side="top" className="w-80 p-1">
        {!!data?.records.length && <Button size="sm" variant="outline" disabled={restoreBatch.isPending} onClick={()=>restoreBatch.mutate()}>{t($=>$.cortex_bulk.restore_loaded,{count:data.records.length})}</Button>}
        {restoreBatch.error && <p role="alert">{restoreBatch.error.message}</p>}
        {total === 0 ? (
          <p className="px-2 py-3 text-center text-caption text-muted-foreground">
            {t(($) => $.cortex_table.trash_empty)}
          </p>
        ) : (
          <ul className="max-h-72 overflow-y-auto">
            {data?.records.map((record) => (
              <li
                key={record.id}
                className="flex items-center gap-2 rounded-sm px-2 py-1.5 hover:bg-accent/50"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-label">
                    {record.title || t(($) => $.cortex_table.untitled)}
                  </p>
                  {record.deleted_at && (
                    <p className="text-caption text-muted-foreground">
                      {t(($) => $.cortex_table.deleted_ago, {
                        time: timeAgo(record.deleted_at),
                      })}
                    </p>
                  )}
                </div>
                <Button
                  variant="ghost"
                  size="xs"
                  disabled={commands.restoreRecord.isPending}
                  onClick={() => commands.restoreRecord.mutate(record.id)}
                >
                  <RotateCcw />
                  {t(($) => $.cortex_table.restore)}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
