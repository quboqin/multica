"use client";

import { useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { api } from "@multica/core/api";
import { documentKeys, useDocumentPreferences } from "@multica/core/documents";
import {
  issueAttachmentsOptions,
  issueKeys,
} from "@multica/core/issues/queries";
import { useActorName } from "@multica/core/workspace/hooks";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Switch } from "@multica/ui/components/ui/switch";
import { cn } from "@multica/ui/lib/utils";
import { ReadonlyContent } from "../editor/readonly-content";
import { useT } from "../i18n";

// One controller connects the rail selection to the page preview. Server data
// stays in Query; browsing history never replaces the live editor's draft.
export function useDocumentHistory(issue: Issue | undefined, enabled: boolean) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<number | null>(null);
  const [compare, setCompare] = useState(false);
  const [restoreRevision, setRestoreRevision] = useState<number | null>(null);
  const wsId = issue?.workspace_id ?? "";
  const id = issue?.id ?? "";
  const draft = useDocumentPreferences(
    (s) => s.drafts[JSON.stringify([wsId, id])],
  );
  const versions = useInfiniteQuery({
    queryKey: ["documents", wsId, "versions", id],
    initialPageParam: undefined as number | undefined,
    queryFn: ({ pageParam }) => api.listDocumentVersions(id, wsId, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: enabled && !!id,
    retry: false,
  });
  const rows = versions.data?.pages.flatMap((page) => page.versions) ?? [];
  const current = selected ?? rows[0]?.version;
  const snapshot = useQuery({
    queryKey: ["documents", wsId, "version", id, current],
    queryFn: () => api.getDocumentVersion(id, current!, wsId),
    enabled: enabled && !!id && current !== undefined,
    retry: false,
  });
  const restore = useMutation({
    mutationFn: () =>
      api.restoreDocumentVersion(id, current!, restoreRevision!),
    onSuccess: async (updated) => {
      qc.setQueryData(issueKeys.detail(wsId, id), updated);
      await qc.invalidateQueries({ queryKey: documentKeys.all(wsId) });
      setRestoreRevision(null);
      setSelected(null);
    },
  });
  return {
    versions,
    rows,
    current,
    snapshot,
    restore,
    compare,
    setCompare,
    hasDraft: !!draft,
    latest: rows[0]?.version === current,
    confirming: restoreRevision !== null,
    beginRestore: () => setRestoreRevision(issue?.revision ?? 1),
    cancelRestore: () => setRestoreRevision(null),
    select: (version: number) => {
      setSelected(version);
      setRestoreRevision(null);
      restore.reset();
    },
  };
}
export type DocumentHistoryState = ReturnType<typeof useDocumentHistory>;

export function DocumentVersionList({
  history,
}: {
  history: DocumentHistoryState;
}) {
  const { t } = useT("issues");
  const { getActorName } = useActorName();
  const { versions, rows, current, compare, setCompare } = history;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <label className="flex shrink-0 cursor-pointer items-center justify-between gap-3 border-b px-3 py-3 text-caption">
        {t(($) => $.cortex_docs.compare_current)}
        <Switch
          size="sm"
          checked={compare}
          onCheckedChange={setCompare}
          disabled={!history.snapshot.data}
        />
      </label>
      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto p-3">
        {versions.isLoading && (
          <p role="status" className="text-caption">
            {t(($) => $.cortex_docs.loading)}
          </p>
        )}
        {versions.error && (
          <p role="alert" className="text-caption text-destructive">
            {versions.error.message}
          </p>
        )}
        {versions.isSuccess && rows.length === 0 && (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.cortex_docs.no_versions)}
          </p>
        )}
        {rows.map((row) => (
          <button
            key={row.version}
            type="button"
            aria-pressed={row.version === current}
            disabled={history.restore.isPending}
            onClick={() => history.select(row.version)}
            className={cn(
              "w-full space-y-1 rounded-md border px-3 py-3 text-left text-body",
              row.version === current
                ? "border-brand bg-brand/10 hover:bg-brand/15"
                : "border-transparent hover:bg-accent",
            )}
          >
            <span className="block font-medium">
              {`v${row.version}`} · {new Date(row.created_at).toLocaleString()}
            </span>
            <span className="block truncate text-caption text-muted-foreground">
              {row.actor_id
                ? getActorName(row.actor_type, row.actor_id)
                : t(($) => $.cortex_docs.history_baseline)}
            </span>
            <span className="block text-caption">
              {row.action === "restore"
                ? t(($) => $.cortex_docs.restored_from, {
                    version: row.restored_from,
                  })
                : row.action === "baseline"
                  ? t(($) => $.cortex_docs.history_baseline)
                  : row.action === "create"
                    ? t(($) => $.cortex_docs.version_created)
                    : t(($) => $.cortex_docs.version_edited)}
            </span>
          </button>
        ))}
        {versions.hasNextPage && (
          <Button
            variant="ghost"
            size="sm"
            disabled={versions.isFetchingNextPage}
            onClick={() => void versions.fetchNextPage()}
          >
            {t(($) => $.cortex_docs.more_versions)}
          </Button>
        )}
      </div>
    </div>
  );
}

export function DocumentVersionPreview({
  issue,
  history,
  onClose,
}: {
  issue: Issue;
  history: DocumentHistoryState;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  const { data: attachments } = useQuery(issueAttachmentsOptions(issue.id));
  const { snapshot, restore, compare, latest, hasDraft, confirming } = history;
  return (
    <section
      aria-label={t(($) => $.cortex_docs.version_preview)}
      className="@container flex min-h-0 flex-1 flex-col"
    >
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
        <Button
          variant="ghost"
          size="sm"
          disabled={restore.isPending}
          onClick={onClose}
        >
          <ArrowLeft />
          {t(($) => $.cortex_docs.back_to_document)}
        </Button>
        <Button
          size="sm"
          disabled={!snapshot.data || latest || hasDraft || restore.isPending}
          onClick={history.beginRestore}
        >
          {t(($) => $.cortex_docs.restore_version)}
        </Button>
        {hasDraft && (
          <p className="w-full text-caption text-muted-foreground">
            {t(($) => $.cortex_docs.unsaved_blocks_transition)}
          </p>
        )}
      </div>
      {confirming && (
        <div className="space-y-2 border-b px-4 py-3">
          <p className="text-body">
            {t(($) => $.cortex_docs.restore_consequence)}
          </p>
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={restore.isPending || hasDraft}
              onClick={() => restore.mutate()}
            >
              {t(($) => $.cortex_docs.confirm_restore)}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={restore.isPending}
              onClick={history.cancelRestore}
            >
              {t(($) => $.cortex_docs.cancel_sharing)}
            </Button>
          </div>
        </div>
      )}
      {(snapshot.error || restore.error) && (
        <p role="alert" className="px-4 py-2 text-caption text-destructive">
          {(snapshot.error ?? restore.error)?.message}
        </p>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div
          className={cn(
            "mx-auto w-full space-y-6 px-4 py-8 md:px-8",
            compare
              ? "grid items-start gap-6 space-y-0 @2xl:grid-cols-2"
              : "max-w-[784px]",
          )}
        >
          {snapshot.data ? (
            <>
              <article className="min-w-0">
                <p className="mb-3 text-caption text-muted-foreground">
                  {`v${snapshot.data.version}`} ·{" "}
                  {new Date(snapshot.data.created_at).toLocaleString()}
                </p>
                <h1 className="mb-4 break-words text-title-lg font-semibold">
                  {snapshot.data.title}
                </h1>
                <ReadonlyContent
                  content={snapshot.data.body}
                  attachments={attachments}
                />
              </article>
              {compare && (
                <article className="min-w-0 border-t pt-6 @2xl:border-l @2xl:border-t-0 @2xl:pl-6 @2xl:pt-0">
                  <p className="mb-3 text-caption text-muted-foreground">
                    {t(($) => $.cortex_docs.current_saved)}
                  </p>
                  <h2 className="mb-4 break-words text-title-lg font-semibold">
                    {issue.title}
                  </h2>
                  <ReadonlyContent
                    content={issue.description ?? ""}
                    attachments={attachments}
                  />
                </article>
              )}
            </>
          ) : (
            !snapshot.error && (
              <p role="status">
                {t(($) =>
                  history.versions.isSuccess && history.rows.length === 0
                    ? $.cortex_docs.no_versions
                    : $.cortex_docs.loading,
                )}
              </p>
            )
          )}
        </div>
      </div>
    </section>
  );
}
