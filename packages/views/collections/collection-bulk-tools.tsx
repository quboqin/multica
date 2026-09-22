"use client";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  collectionKeys,
  type CollectionField,
  type CollectionRecord,
  type CollectionQuery,
} from "@multica/core/collections";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { useT } from "../i18n";
import { CollectionFieldEditor } from "./collection-cell";

export function CollectionBulkTools({
  id,
  wsId,
  fields,
  query,
  selected,
  onSelection,
}: {
  id: string;
  wsId: string;
  fields: CollectionField[];
  query: CollectionQuery;
  selected: CollectionRecord[];
  onSelection: (rows: CollectionRecord[]) => void;
}) {
  const { t } = useT("issues");
  const cache = useQueryClient();
  const [csv, setCSV] = useState("");
  const [preview, setPreview] = useState<number | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [fieldId, setFieldId] = useState("");
  const [value, setValue] = useState<unknown>(null);
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const refresh = () =>
    cache.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
  const importer = useMutation({
    mutationFn: (dryRun: boolean) => api.importCollectionCSV(id, csv, dryRun),
    onSuccess: async (result) => {
      if (result.dry_run) setPreview(result.count);
      else {
        await refresh();
        setImportOpen(false);
        setCSV("");
        setPreview(null);
      }
    },
  });
  const selectionAbort = useRef<AbortController | null>(null);
  useEffect(() => () => selectionAbort.current?.abort(), []);
  const select = useMutation({
    mutationFn: async () => {
      selectionAbort.current?.abort();
      const controller = new AbortController();
      selectionAbort.current = controller;
      const rows: CollectionRecord[] = [];
      let cursor: string | null = null;
      do {
        const page = await api.listCollectionRecords(
          id,
          query,
          cursor,
          controller.signal,
          wsId,
        );
        rows.push(...page.records.slice(0, 500 - rows.length));
        cursor = page.next_cursor;
      } while (cursor && rows.length < 500);
      if (!controller.signal.aborted) onSelection(rows);
    },
  });
  const batch = useMutation({
    mutationFn: (action: "update" | "delete") =>
      api.batchCollectionRecords(id, {
        action,
        record_ids: selected.map((r) => r.id),
        expected_revisions: Object.fromEntries(
          selected.map((r) => [r.id, r.revision]),
        ),
        ...(action === "update"
          ? { fields: { [fieldId]: value } }
          : { confirmed: true }),
      }),
    onSuccess: async () => {
      onSelection([]);
      setDeleting(false);
      await refresh();
    },
  });
  const field = fields.find((f) => f.id === fieldId);
  const draft: CollectionRecord = {
    id: "batch-draft",
    workspace_id: wsId,
    collection_id: id,
    title: "",
    fields: { [fieldId]: value },
    links: {},
    revision: 1,
    created_at: "",
  };
  return (
    <div className="flex flex-wrap items-center gap-2 border-b px-4 py-2 text-label">
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogTrigger render={<Button size="sm" variant="outline" />}>
          {t(($) => $.cortex_bulk.import_csv)}
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(($) => $.cortex_bulk.import_csv)}</DialogTitle>
          </DialogHeader>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.cortex_bulk.csv_help)}
          </p>
          <label className="space-y-2">
            {t(($) => $.cortex_bulk.csv_file)}
            <input
              type="file"
              accept=".csv,text/csv"
              disabled={importer.isPending}
              onChange={async (e) => {
                const file = e.target.files?.[0];
                setPreview(null);
                importer.reset();
                if (file) setCSV(await file.text());
              }}
            />
          </label>
          <Button
            variant="outline"
            onClick={() => {
              const header = [
                "title",
                ...fields
                  .filter((f) => f.type !== "relation" && f.type !== "formula")
                  .map((f) => f.name),
              ]
                .map((s) => '"' + s.replaceAll('"', '""') + '"')
                .join(",");
              const url = URL.createObjectURL(
                new Blob(["\ufeff" + header + "\r\n"], {
                  type: "text/csv;charset=utf-8",
                }),
              );
              const a = document.createElement("a");
              a.href = url;
              a.download = "collection-template.csv";
              a.click();
              URL.revokeObjectURL(url);
            }}
          >
            {t(($) => $.cortex_bulk.template)}
          </Button>
          {preview !== null && (
            <p role="status">
              {t(($) => $.cortex_bulk.preview_count, { count: preview })}
            </p>
          )}
          {importer.error && (
            <p role="alert" className="text-destructive">
              {importer.error.message}
            </p>
          )}
          <Button
            disabled={!csv || importer.isPending}
            onClick={() => importer.mutate(preview === null)}
          >
            {preview === null
              ? t(($) => $.cortex_bulk.validate)
              : t(($) => $.cortex_bulk.import_csv)}
          </Button>
        </DialogContent>
      </Dialog>
      <Button
        size="sm"
        variant="outline"
        disabled={select.isPending}
        onClick={() => select.mutate()}
      >
        {t(($) => $.cortex_bulk.select_500)}
      </Button>
      {selected.length > 0 && (
        <>
          <span>
            {t(($) => $.cortex_bulk.selected, { count: selected.length })}
          </span>
          <Button size="sm" variant="ghost" onClick={() => onSelection([])}>
            {t(($) => $.cortex_bulk.clear)}
          </Button>
          <select
            aria-label={t(($) => $.cortex_bulk.field)}
            className="h-8 max-w-40 rounded border bg-background px-2"
            value={fieldId}
            onChange={(e) => {
              setFieldId(e.target.value);
              setValue(null);
            }}
          >
            <option value="">{t(($) => $.cortex_bulk.field)}</option>
            {fields
              .filter((f) => f.type !== "relation" && f.type !== "formula")
              .map((f) => (
                <option key={f.id} value={f.id}>
                  {f.name}
                </option>
              ))}
          </select>
          {field && (
            <div className="min-w-24 rounded border px-2">
              <CollectionFieldEditor
                record={draft}
                field={field}
                open={editing}
                onOpenChange={setEditing}
                onChange={setValue}
              />
            </div>
          )}
          {field && value === null && (
            <span className="text-caption text-muted-foreground">
              {t(($) => $.cortex_bulk.clear_values)}
            </span>
          )}
          <Button
            size="sm"
            disabled={!field || batch.isPending}
            onClick={() => batch.mutate("update")}
          >
            {t(($) => $.cortex_bulk.apply)}
          </Button>
          <Dialog open={deleting} onOpenChange={setDeleting}>
            <DialogTrigger render={<Button size="sm" variant="destructive" />}>
              {t(($) => $.cortex_bulk.delete)}
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>
                  {t(($) => $.cortex_bulk.delete_confirm, {
                    count: selected.length,
                  })}
                </DialogTitle>
              </DialogHeader>
              <p>{t(($) => $.cortex_bulk.retention)}</p>
              {batch.error && <p role="alert">{batch.error.message}</p>}
              <Button
                variant="destructive"
                disabled={batch.isPending}
                onClick={() => batch.mutate("delete")}
              >
                {t(($) => $.cortex_bulk.delete)}
              </Button>
            </DialogContent>
          </Dialog>
        </>
      )}
      {(batch.error || select.error) && (
        <p role="alert" className="w-full text-destructive">
          {(batch.error || select.error)?.message}
        </p>
      )}
    </div>
  );
}
