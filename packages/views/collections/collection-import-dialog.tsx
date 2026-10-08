"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Upload } from "lucide-react";
import { api } from "@multica/core/api";
import {
  collectionKeys,
  type CollectionImportPreview,
  type ImportColumn,
} from "@multica/core/collections";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import { useT } from "../i18n";
import { useNavigation } from "../navigation";
import { FieldTypeLabel } from "./collection-fields";

const types = ["text", "number", "checkbox", "date", "url", "select"] as const;
const selectClass =
  "h-8 min-w-0 rounded-md border bg-background px-2 text-label";

export function CollectionImportDialog({
  trigger,
}: {
  trigger?: ReactElement;
}) {
  const { t } = useT("issues");
  const wsId = useCurrentWorkspace()?.id ?? "";
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const cache = useQueryClient();
  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<CollectionImportPreview | null>(null);
  const [columns, setColumns] = useState<ImportColumn[]>([]);
  const [name, setName] = useState("");
  const [project, setProject] = useState("");
  const [titleColumn, setTitleColumn] = useState(0);
  const [headerRow, setHeaderRow] = useState("1");
  const [reading, setReading] = useState(false);
  const [error, setError] = useState("");
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const { data: projects = [] } = useQuery({
    ...projectListOptions(wsId),
    enabled: open && !!wsId,
  });
  const create = useMutation({
    mutationFn: () => {
      if (!file || !preview) throw new Error("Choose a file first");
      return api.createCollectionFromFile(wsId, file, {
        name,
        project_id: project || undefined,
        sheet: preview.sheet,
        header_row: preview.header_row,
        title_column: titleColumn,
        columns,
      });
    },
    onSuccess: async (result) => {
      await cache.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
      setOpen(false);
      reset();
      navigation.push(paths.collectionDetail(result.collection.id));
    },
  });
  function reset() {
    controller.current?.abort();
    setFile(null);
    setPreview(null);
    setColumns([]);
    setName("");
    setProject("");
    setHeaderRow("1");
    setError("");
    setReading(false);
    create.reset();
  }
  async function inspect(nextFile: File, sheet = "", row = 1, newFile = false) {
    controller.current?.abort();
    const current = new AbortController();
    controller.current = current;
    setFile(nextFile);
    setPreview(null);
    setError("");
    create.reset();
    if (
      nextFile.size > 24 * 1024 * 1024 ||
      !/\.(xlsx|csv)$/i.test(nextFile.name)
    ) {
      setError(t(($) => $.cortex_import.file_limit));
      setReading(false);
      return;
    }
    setReading(true);
    try {
      const result = await api.previewCollectionImport(
        wsId,
        nextFile,
        { sheet, header_row: row },
        current.signal,
      );
      if (current.signal.aborted) return;
      setPreview(result);
      setColumns(result.columns);
      setTitleColumn(result.title_column);
      setHeaderRow(String(result.header_row));
      if (newFile) setName(result.name);
    } catch (cause) {
      if (!current.signal.aborted)
        setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (!current.signal.aborted) setReading(false);
    }
  }
  function changeColumn(index: number, patch: Partial<ImportColumn>) {
    setColumns((current) =>
      current.map((column) =>
        column.index === index ? { ...column, ...patch } : column,
      ),
    );
  }
  const fieldCount = columns.filter(
    (column) => !column.skip && column.index !== titleColumn,
  ).length;
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (create.isPending) return;
        setOpen(next);
        if (!next) reset();
      }}
    >
      <DialogTrigger
        render={
          trigger ?? (
            <Button size="sm" variant="outline" disabled={!wsId}>
              <Upload />
              {t(($) => $.cortex_import.open)}
            </Button>
          )
        }
      />
      <DialogContent className="flex max-h-[90dvh] flex-col sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t(($) => $.cortex_import.title)}</DialogTitle>
          <DialogDescription>
            {t(($) => $.cortex_import.description)}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 space-y-4 overflow-y-auto px-1">
          <label className="flex flex-col gap-2 rounded-lg border border-dashed p-4 text-label">
            {t(($) => $.cortex_import.file)}
            <input
              type="file"
              accept=".xlsx,.csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
              disabled={reading || create.isPending}
              onChange={(event) => {
                const next = event.target.files?.[0];
                if (next) void inspect(next, "", 1, true);
              }}
            />
            <span className="text-caption text-muted-foreground">
              {t(($) => $.cortex_import.file_limit)}
            </span>
          </label>
          {reading && <p role="status">{t(($) => $.cortex_import.reading)}</p>}
          {preview && (
            <>
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="flex flex-col gap-1 text-label">
                  {t(($) => $.cortex_docs.table_name)}
                  <Input
                    maxLength={80}
                    value={name}
                    disabled={create.isPending}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <label className="flex flex-col gap-1 text-label">
                  {t(($) => $.cortex_bulk.project)}
                  <select
                    className={selectClass}
                    disabled={create.isPending}
                    value={project}
                    onChange={(e) => setProject(e.target.value)}
                  >
                    <option value="">
                      {t(($) => $.cortex_bulk.workspace)}
                    </option>
                    {projects.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.title}
                      </option>
                    ))}
                  </select>
                </label>
                {preview.sheets.length > 0 && (
                  <label className="flex flex-col gap-1 text-label">
                    {t(($) => $.cortex_import.sheet)}
                    <select
                      className={selectClass}
                      value={preview.sheet}
                      disabled={create.isPending}
                      onChange={(e) => {
                        if (file)
                          void inspect(file, e.target.value, Number(headerRow));
                      }}
                    >
                      {preview.sheets.map((sheet) => (
                        <option key={sheet}>{sheet}</option>
                      ))}
                    </select>
                  </label>
                )}
                <label className="flex flex-col gap-1 text-label">
                  {t(($) => $.cortex_import.header_row)}
                  <Input
                    type="number"
                    min={1}
                    max={100}
                    value={headerRow}
                    disabled={create.isPending}
                    onChange={(e) => setHeaderRow(e.target.value)}
                    onBlur={() => {
                      if (file && Number(headerRow) !== preview.header_row)
                        void inspect(file, preview.sheet, Number(headerRow));
                    }}
                  />
                </label>
                <label className="flex flex-col gap-1 text-label">
                  {t(($) => $.cortex_import.title_column)}
                  <select
                    className={selectClass}
                    value={titleColumn}
                    disabled={create.isPending}
                    onChange={(e) => {
                      const index = Number(e.target.value);
                      setTitleColumn(index);
                      changeColumn(index, { skip: false });
                    }}
                  >
                    {columns.map((c) => (
                      <option key={c.index} value={c.index}>
                        {c.name}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <p role="status" className="text-label">
                {t(($) => $.cortex_import.summary, {
                  rows: preview.row_count,
                  fields: fieldCount,
                })}
              </p>
              {preview.formula_count > 0 && (
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.cortex_import.formulas)}
                </p>
              )}
              <div className="overflow-x-auto rounded-md border">
                <table className="w-full text-left text-label">
                  <thead className="bg-muted">
                    <tr>
                      <th className="p-2">
                        {t(($) => $.cortex_import.include)}
                      </th>
                      <th className="p-2">
                        {t(($) => $.cortex_import.field_name)}
                      </th>
                      <th className="p-2">
                        {t(($) => $.cortex_import.field_type)}
                      </th>
                      <th className="p-2">
                        {t(($) => $.cortex_import.samples)}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {columns.map((column) => (
                      <tr key={column.index} className="border-t">
                        <td className="p-2">
                          <input
                            type="checkbox"
                            aria-label={t(
                              ($) => $.cortex_import.include_column,
                              { name: column.name },
                            )}
                            checked={!column.skip}
                            disabled={
                              column.index === titleColumn || create.isPending
                            }
                            onChange={(e) =>
                              changeColumn(column.index, {
                                skip: !e.target.checked,
                              })
                            }
                          />
                        </td>
                        <td className="p-2">
                          <Input
                            className="min-w-32"
                            aria-label={t(($) => $.cortex_import.column_name, {
                              index: column.index + 1,
                            })}
                            value={column.name}
                            maxLength={32}
                            disabled={column.skip || create.isPending}
                            onChange={(e) =>
                              changeColumn(column.index, {
                                name: e.target.value,
                              })
                            }
                          />
                        </td>
                        <td className="p-2">
                          {column.index === titleColumn ? (
                            <span>
                              {t(($) => $.cortex_import.title_column)}
                            </span>
                          ) : (
                            <select
                              className={selectClass}
                              aria-label={t(
                                ($) => $.cortex_import.column_type,
                                { name: column.name },
                              )}
                              value={column.type}
                              disabled={column.skip || create.isPending}
                              onChange={(e) => {
                                const type = types.find(
                                  (value) => value === e.target.value,
                                );
                                if (type) changeColumn(column.index, { type });
                              }}
                            >
                              {types.map((type) => (
                                <option key={type} value={type}>
                                  <FieldTypeLabel type={type} />
                                </option>
                              ))}
                            </select>
                          )}
                        </td>
                        <td className="max-w-64 p-2 text-caption text-muted-foreground">
                          <div className="line-clamp-3 break-words">
                            {column.samples.slice(0, 3).join(" · ")}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {fieldCount > 50 && (
                <p role="alert" className="text-destructive">
                  {t(($) => $.cortex_import.field_limit)}
                </p>
              )}
            </>
          )}
          {(error || create.error) && (
            <p role="alert" className="text-label text-destructive">
              {error || create.error?.message}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            disabled={create.isPending}
            onClick={() => {
              setOpen(false);
              reset();
            }}
          >
            {t(($) => $.cortex_docs.cancel)}
          </Button>
          <Button
            disabled={
              !preview ||
              preview.row_count === 0 ||
              !name.trim() ||
              reading ||
              create.isPending ||
              fieldCount > 50 ||
              Number(headerRow) !== preview.header_row
            }
            onClick={() => create.mutate()}
            aria-busy={create.isPending}
          >
            {create.isPending
              ? t(($) => $.cortex_import.creating)
              : t(($) => $.cortex_import.create)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
