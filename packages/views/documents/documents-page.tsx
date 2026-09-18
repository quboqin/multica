"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, ColumnSizingState } from "@tanstack/react-table";
import { useFeatureEnabled } from "@multica/core/config";
import { CORTEX_DOCS_FLAG } from "@multica/core/feature-flags";
import { useWorkspaceId } from "@multica/core";
import { issueKeys } from "@multica/core/issues";
import { api, ApiError } from "@multica/core/api";
import { useRequiredWorkspaceSlug } from "@multica/core/paths";
import {
  createDocumentDataSource,
  documentKeys,
  documentQuery,
  useSaveDocument,
} from "@multica/core/documents";
import type {
  DataSourceGroupPage,
  DataSourcePage,
} from "@multica/core/data-source";
import type { Issue, IssueTableQuerySpec } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { ContentEditor, type ContentEditorRef } from "../editor";
import {
  createDataViewFieldColumn,
  TableView,
  useDataViewController,
  type DataViewQueryBinding,
  type DataViewStructuralRow,
} from "../data-view";
import { useT } from "../i18n";

type DisplayRow = { kind: "document"; key: string; issue: Issue };
type TableRow = DisplayRow | DataViewStructuralRow;
const emptySet = new Set<string>();

export function DocumentsPage() {
  const enabled = useFeatureEnabled(CORTEX_DOCS_FLAG, false);
  const { t } = useT("issues");
  const workspaceId = useWorkspaceId();
  const workspaceSlug = useRequiredWorkspaceSlug();
  if (!enabled)
    return <p className="p-6">{t(($) => $.documents.unavailable)}</p>;
  return (
    <DocumentSource
      key={workspaceId}
      workspaceId={workspaceId}
      workspaceSlug={workspaceSlug}
    />
  );
}

function DocumentSource({
  workspaceId,
  workspaceSlug,
}: {
  workspaceId: string;
  workspaceSlug: string;
}) {
  const { t } = useT("issues");
  const client = useQueryClient();
  const [editing, setEditing] = useState<string | null>(null);
  const [sizing, setSizing] = useState<ColumnSizingState>({});
  const source = useMemo(
    () => createDocumentDataSource(workspaceId, workspaceSlug),
    [workspaceId, workspaceSlug],
  );
  const binding = useMemo<
    DataViewQueryBinding<
      Issue,
      IssueTableQuerySpec,
      DataSourcePage<Issue>,
      DataSourceGroupPage
    >
  >(
    () => ({
      identity: source.identity,
      rowPageKey: (request) => [...documentKeys.rows(workspaceId), request],
      rowBranchKey: (request) => [...documentKeys.rows(workspaceId), request],
      readRowPage: (request, signal) =>
        source.read(request.query, request.page, signal),
      mapRowPage: (page) => ({ ...page, branchTotal: page.total }),
      groupPagesKey: () => [...documentKeys.rows(workspaceId), "groups"],
      readGroupPage: async () => ({ groups: [], total: 0, nextCursor: null }),
      mapGroupPage: (page) => page,
    }),
    [source, workspaceId],
  );
  const data = useDataViewController({
    binding,
    query: documentQuery,
    groupBy: null,
    hierarchy: false,
    collapsedGroupKeys: emptySet,
    collapsedRowIds: emptySet,
    rowId: source.rowId,
    directChildCount: () => 0,
    projectRow: ({ row }): DisplayRow => ({
      kind: "document",
      key: row.id,
      issue: row,
    }),
  });
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible")
        void client.invalidateQueries({
          queryKey: documentKeys.rows(workspaceId),
        });
    };
    const interval = setInterval(refresh, 30000);
    window.addEventListener("focus", refresh);
    window.addEventListener("online", refresh);
    return () => {
      clearInterval(interval);
      window.removeEventListener("focus", refresh);
      window.removeEventListener("online", refresh);
    };
  }, [client, workspaceId]);
  const columns = useMemo<ColumnDef<TableRow>[]>(
    () => source.fields.filter((field) => field.id === "title").map((field) =>
      createDataViewFieldColumn<TableRow, Issue>({
        field,
        sourceRow: (row) => row.kind === "document" ? row.issue : null,
        presentation: {
          header: t(($) => $.documents.name),
          cell: ({ row, getValue }) => {
            const value = row.original;
            return value.kind === "document" ? (
              <Button variant="ghost" onClick={() => setEditing(value.issue.id)}>
                {String(getValue() ?? "")}
              </Button>
            ) : null;
          },
        },
      }),
    ),
    [source.fields, t],
  );
  return (
    <main className="flex min-h-0 flex-1 flex-col gap-4 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-title">{t(($) => $.documents.title)}</h1>
        <Button onClick={() => setEditing("new")} disabled={editing !== null}>
          {t(($) => $.documents.new)}
        </Button>
      </div>
      {editing !== null ? (
        <DocumentLoader
          key={editing}
          id={editing}
          workspaceId={workspaceId}
          workspaceSlug={workspaceSlug}
          close={() => setEditing(null)}
        />
      ) : (
        <TableView
          sourceIdentity={source.identity}
          writable={false}
          rows={data.rows}
          columns={columns}
          rowId={(row) => row.key}
          visibleColumnIds={["title"]}
          columnSizing={sizing}
          onColumnSizingChange={setSizing}
          onReorderColumn={() => {}}
          emptyMessage={t(($) => $.documents.empty)}
          renderStructuralRow={(row) =>
            row.original.kind === "document"
              ? null
              : {
                  content:
                    row.original.kind === "load_more" &&
                    row.original.state !== "end" ? (
                      <Button
                        onClick={row.original.onLoad}
                        disabled={row.original.state === "loading"}
                      >
                        {row.original.state === "loading"
                          ? t(($) => $.documents.loading)
                          : row.original.state === "error"
                            ? t(($) => $.documents.retry)
                            : t(($) => $.documents.more)}
                      </Button>
                    ) : null,
                }
          }
        />
      )}
    </main>
  );
}

function DocumentLoader({
  id,
  workspaceId,
  workspaceSlug,
  close,
}: {
  id: string;
  workspaceId: string;
  workspaceSlug: string;
  close: () => void;
}) {
  const { t } = useT("issues");
  const detail = useQuery({
    queryKey: issueKeys.detail(workspaceId, id),
    enabled: id !== "new",
    queryFn: ({ signal }) => api.getIssue(id, { signal, workspaceSlug }),
    retry: false,
  });
  if (
    id !== "new" &&
    (!detail.data ||
      (detail.error instanceof ApiError &&
        [401, 403, 404].includes(detail.error.status)))
  )
    return (
      <div>
        {detail.error ? (
          <p role="alert">{detail.error.message}</p>
        ) : (
          t(($) => $.documents.loading)
        )}
        <Button onClick={close}>{t(($) => $.documents.cancel)}</Button>
      </div>
    );
  if (
    detail.data &&
    (detail.data.kind !== "doc" || detail.data.workspace_id !== workspaceId)
  )
    return <p role="alert">{t(($) => $.documents.invalid)}</p>;
  return (
    <DocumentForm
      initial={detail.data}
      workspaceId={workspaceId}
      workspaceSlug={workspaceSlug}
      close={close}
    />
  );
}

function DocumentForm({
  initial,
  workspaceId,
  workspaceSlug,
  close,
}: {
  initial?: Issue;
  workspaceId: string;
  workspaceSlug: string;
  close: () => void;
}) {
  const { t } = useT("issues");
  const [baseline, setBaseline] = useState(initial);
  const [title, setTitle] = useState(initial?.title ?? "");
  const [body, setBody] = useState(initial?.description ?? "");
  const [version, setVersion] = useState(0);
  const editor = useRef<ContentEditorRef>(null);
  const active = useRef(true);
  const save = useSaveDocument(workspaceId, workspaceSlug);
  const dirty =
    title !== (baseline?.title ?? "") || body !== (baseline?.description ?? "");
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  useEffect(() => {
    const prevent = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", prevent);
    return () => window.removeEventListener("beforeunload", prevent);
  }, [dirty]);
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={async (event) => {
        event.preventDefault();
        if (save.isPending) return;
        const sentBody = editor.current?.getMarkdown() ?? body;
        try {
          const accepted = await save.mutateAsync({
            id: baseline?.id,
            revision: baseline?.revision,
            title,
            description: sentBody,
          });
          if (active.current) {
            setBaseline(accepted);
            setBody(sentBody);
          }
        } catch {
          /* The mutation owns the error and the draft is retained. */
        }
      }}
    >
      <Input
        aria-label={t(($) => $.documents.name)}
        value={title}
        onChange={(event) => setTitle(event.target.value)}
        disabled={save.isPending}
        required
      />
      <div
        className={save.isPending ? "pointer-events-none opacity-60" : ""}
        inert={save.isPending}
      >
        <ContentEditor
          key={version}
          ref={editor}
          defaultValue={body}
          onUpdate={(markdown) => setBody(markdown)}
          debounceMs={0}
          placeholder={t(($) => $.documents.body)}
        />
      </div>
      {save.error ? (
        <p role="alert">{save.error.message}</p>
      ) : save.isSuccess ? (
        <p role="status">{t(($) => $.documents.saved)}</p>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" disabled={save.isPending || !title.trim()}>
          {t(($) => $.documents.save)}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={save.isPending}
          onClick={close}
        >
          {t(($) => $.documents.cancel)}
        </Button>
        {save.error && baseline ? (
          <Button
            type="button"
            variant="outline"
            onClick={async () => {
              const latest = await api.getIssue(baseline.id, { workspaceSlug });
              if (active.current) {
                setBaseline(latest);
                setTitle(latest.title);
                setBody(latest.description ?? "");
                setVersion((v) => v + 1);
                save.reset();
              }
            }}
          >
            {t(($) => $.documents.reload)}
          </Button>
        ) : null}
      </div>
    </form>
  );
}
