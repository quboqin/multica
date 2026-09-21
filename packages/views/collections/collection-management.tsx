"use client";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  archivedCollectionFieldsOptions,
  collectionKeys,
  type Collection,
} from "@multica/core/collections";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { useT } from "../i18n";

export function CollectionManagement({
  collection,
  wsId,
}: {
  collection: Collection;
  wsId: string;
}) {
  const { t } = useT("issues");
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [icon, setIcon] = useState(collection.icon ?? "");
  const [description, setDescription] = useState(collection.description ?? "");
  const archived = useQuery({
    ...archivedCollectionFieldsOptions(wsId, collection.id),
    enabled: open,
  });
  const refresh = () =>
    client.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
  const save = useMutation({
    mutationFn: () =>
      api.updateCollection(collection.id, { icon, description }),
    onSuccess: async () => {
      await refresh();
      setOpen(false);
    },
  });
  const restore = useMutation({
    mutationFn: (id: string) => api.restoreCollectionField(collection.id, id),
    onSuccess: refresh,
  });
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setIcon(collection.icon ?? "");
          setDescription(collection.description ?? "");
        }
      }}
    >
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        {t(($) => $.cortex_bulk.settings)}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t(($) => $.cortex_bulk.settings)}</DialogTitle>
        </DialogHeader>
        <label>
          {t(($) => $.cortex_bulk.icon)}
          <Input
            maxLength={32}
            value={icon}
            onChange={(e) => setIcon(e.target.value)}
          />
        </label>
        <label>
          {t(($) => $.cortex_bulk.description)}
          <textarea
            className="w-full rounded-md border bg-background p-2 text-body"
            maxLength={4000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <Button disabled={save.isPending} onClick={() => save.mutate()}>
          {t(($) => $.cortex_bulk.save)}
        </Button>
        <h3 className="text-label font-medium">
          {t(($) => $.cortex_bulk.archived_fields)}
        </h3>
        <div className="max-h-48 overflow-auto">
          {archived.data?.map((f) => (
            <div
              key={f.id}
              className="flex items-center justify-between gap-2 py-1"
            >
              <span className="truncate">{f.name}</span>
              <Button
                size="sm"
                variant="outline"
                disabled={restore.isPending}
                onClick={() => restore.mutate(f.id)}
              >
                {t(($) => $.cortex_bulk.restore)}
              </Button>
            </div>
          ))}
        </div>
        {(save.error || restore.error || archived.error) && (
          <p role="alert">
            {(save.error || restore.error || archived.error)?.message}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
