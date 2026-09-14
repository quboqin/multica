"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type FormEvent,
  type SetStateAction,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  CellContext,
  ColumnDef,
  ColumnSizingState,
} from "@tanstack/react-table";
import { AlertCircle, Database, TableProperties } from "lucide-react";
import { useWorkspaceId } from "@multica/core";
import { api, ApiError } from "@multica/core/api";
import {
  advanceClientCollectionSourceGeneration,
  captureClientCollectionSourceGeneration,
  collectionDetailOptions,
  collectionKeys,
  collectionTableQuery,
  createCollectionRecordDataSource,
  isClientCollectionSourceGenerationCurrent,
  useCreateCollectionRecord,
  useUpdateCollectionRecord,
} from "@multica/core/collections";
import type {
  DataSourceGroupPage,
  DataSourcePage,
} from "@multica/core/data-source";
import type { CollectionRecord } from "@multica/core/types";
import { useFeatureEnabled } from "@multica/core/config";
import { CORTEX_COLLECTIONS_FLAG } from "@multica/core/feature-flags";
import {
  assertClientWorkspaceAccessAllowed,
  captureClientSessionGeneration,
  captureClientWorkspaceAccessGeneration,
  getCurrentSlug,
  getCurrentWsId,
  isClientSessionGenerationCurrent,
  isClientWorkspaceAccessGenerationCurrent,
} from "@multica/core/platform";
import { useRequiredWorkspaceSlug } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  DataViewCellEditor,
  TableView,
  dataViewCellEditorIdentity,
  useDataViewController,
  type DataViewCellEditorState,
  type DataViewQueryBinding,
  type DataViewStructuralRow,
} from "../data-view";
import { useT } from "../i18n";
import {
  CollectionPageHeader,
  CollectionPageState,
} from "../layout/collection-page";
import { useVisibleCollectionRefresh } from "./use-visible-collection-refresh";

type DisplayRecord = {
  kind: "record";
  key: string;
  sourceRow: CollectionRecord;
};
type CollectionTableRow = DisplayRecord | DataViewStructuralRow;
type RawPage = DataSourcePage<CollectionRecord>;
type CollectionSource = ReturnType<typeof createCollectionRecordDataSource>;

function collectionSourceAccessError(reason: unknown): ApiError | null {
  return reason instanceof ApiError &&
    (reason.status === 403 || reason.status === 404)
    ? reason
    : null;
}

type CollectionCellContextValue = {
  source: CollectionSource;
  saveLabel: string;
  clearLabel: string;
  retryLabel: string;
  editorStates: ReadonlyMap<string, DataViewCellEditorState<CollectionRecord>>;
  onEditorStateChange: (
    key: string,
    state: DataViewCellEditorState<CollectionRecord> | null,
  ) => void;
};

const CollectionCellContext = createContext<CollectionCellContextValue | null>(
  null,
);

function CollectionRecordCell({
  row,
  column,
}: CellContext<CollectionTableRow, unknown>) {
  const context = useContext(CollectionCellContext);
  if (!context || row.original.kind !== "record") return null;
  const field = context.source.fields.find((candidate) => candidate.id === column.id);
  if (!field) return null;
  const sourceRow = row.original.sourceRow;
  const editorKey = dataViewCellEditorIdentity(
    context.source,
    field.id,
    sourceRow,
  );
  return (
    <DataViewCellEditor
      source={context.source}
      field={field}
      row={sourceRow}
      persistedState={context.editorStates.get(editorKey)}
      onStateChange={(state) => context.onEditorStateChange(editorKey, state)}
      saveLabel={context.saveLabel}
      clearLabel={context.clearLabel}
      retryLabel={context.retryLabel}
      hideClearWhenUnavailable
    />
  );
}

function requestId() {
  return globalThis.crypto.randomUUID();
}

type PendingRecordCreate = {
  intent: string;
  id: string;
  sessionGeneration: number;
  workspaceAccessGeneration: number;
};

export function CollectionDetailPage({ collectionId }: { collectionId: string }) {
  const workspaceId = useWorkspaceId();
  return (
    <CollectionDetailPageSource
      key={JSON.stringify([workspaceId, collectionId])}
      workspaceId={workspaceId}
      collectionId={collectionId}
    />
  );
}

function CollectionDetailPageSource({
  workspaceId,
  collectionId,
}: {
  workspaceId: string;
  collectionId: string;
}) {
  const { t } = useT("collections");
  const enabled = useFeatureEnabled(CORTEX_COLLECTIONS_FLAG, false);
  const workspaceSlug = useRequiredWorkspaceSlug();
  const queryClient = useQueryClient();
  const [sourceAccessError, setSourceAccessError] = useState<ApiError | null>(
    null,
  );
  const detailQuery = useQuery({
    ...collectionDetailOptions(workspaceId, workspaceSlug, collectionId),
    enabled: enabled && sourceAccessError === null,
  });
  const detectedSourceAccessError = collectionSourceAccessError(
    detailQuery.error,
  );
  const effectiveSourceAccessError =
    sourceAccessError ?? detectedSourceAccessError;
  const usableDetail = effectiveSourceAccessError ? undefined : detailQuery.data;
  const createRecord = useCreateCollectionRecord();
  const updateRecord = useUpdateCollectionRecord();
  const updateRecordAsync = updateRecord.mutateAsync;
  const [newTitle, setNewTitle] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);
  const [columnSizing, setColumnSizing] = useState<ColumnSizingState>({});
  const [editorStates, setEditorStates] = useState(
    () => new Map<string, DataViewCellEditorState<CollectionRecord>>(),
  );
  const [authoritativeRecords, setAuthoritativeRecords] = useState(
    () => new Map<string, CollectionRecord>(),
  );
  const onEditorStateChange = useCallback(
    (
      key: string,
      state: DataViewCellEditorState<CollectionRecord> | null,
    ) => {
      setEditorStates((current) => {
        const existing = current.get(key);
        if (state === null && existing === undefined) return current;
        if (state === existing) return current;
        const next = new Map(current);
        if (state === null) next.delete(key);
        else next.set(key, state);
        return next;
      });
    },
    [],
  );
  const pendingCreateRequest = useRef<PendingRecordCreate | null>(null);
  const recoveryRequests = useRef(new Set<AbortController>());
  const pageActive = useRef(false);
  useEffect(() => {
    const requests = recoveryRequests.current;
    pageActive.current = true;
    return () => {
      pageActive.current = false;
      for (const controller of requests) controller.abort();
      requests.clear();
    };
  }, []);

  useEffect(() => {
    // A background directory failure retains detailQuery.data and therefore
    // preserves edits. Explicit feature/access loss removes that protected
    // data, so no draft survives the authorization boundary.
    if (!enabled || !usableDetail) {
      setEditorStates((current) =>
        current.size === 0 ? current : new Map(),
      );
      setAuthoritativeRecords((current) =>
        current.size === 0 ? current : new Map(),
      );
    }
  }, [enabled, usableDetail]);

  useEffect(() => {
    if (!detectedSourceAccessError) return;
    setSourceAccessError((current) => current ?? detectedSourceAccessError);
    pendingCreateRequest.current = null;
    setNewTitle("");
    setCreateError(null);
    for (const controller of recoveryRequests.current) controller.abort();
    recoveryRequests.current.clear();
    advanceClientCollectionSourceGeneration(
      queryClient,
      workspaceId,
      collectionId,
    );
    const queryKey = collectionKeys.source(workspaceId, collectionId);
    void queryClient.cancelQueries({ queryKey });
    queryClient.removeQueries({ queryKey });
  }, [collectionId, detectedSourceAccessError, queryClient, workspaceId]);

  const source = useMemo(() => {
    if (!usableDetail) return null;
    const collectionSourceGeneration =
      captureClientCollectionSourceGeneration(
        queryClient,
        workspaceId,
        collectionId,
      );
    return createCollectionRecordDataSource({
      detail: usableDetail,
      read: async (page, signal) => {
        assertClientWorkspaceAccessAllowed(queryClient, workspaceId);
        if (
          !isClientCollectionSourceGenerationCurrent(
            queryClient,
            workspaceId,
            collectionId,
            collectionSourceGeneration,
          )
        ) {
          throw new Error("Collection access expired");
        }
        const result = await api.queryCollectionRecords(
          collectionId,
          page,
          workspaceSlug,
          signal,
        );
        if (
          !isClientCollectionSourceGenerationCurrent(
            queryClient,
            workspaceId,
            collectionId,
            collectionSourceGeneration,
          )
        ) {
          throw new Error("Collection access expired");
        }
        return result;
      },
      execute: usableDetail.capabilities.writable
        ? async ({ record, fieldId, change }) => {
            const sessionGeneration = captureClientSessionGeneration(queryClient);
            const workspaceAccessGeneration =
              captureClientWorkspaceAccessGeneration(queryClient, workspaceId);
            const collectionSourceGeneration =
              captureClientCollectionSourceGeneration(
                queryClient,
                workspaceId,
                collectionId,
              );
            const wireFieldId = fieldId.startsWith("field:")
              ? fieldId.slice("field:".length)
              : fieldId;
            if (wireFieldId === "title" && change.op !== "set") {
              throw new Error("This field is read-only");
            }
            try {
              return await updateRecordAsync({
                collectionId,
                recordId: record.id,
                input: {
                  expectedRevision: record.revision,
                  change:
                    change.op === "clear"
                      ? { fieldId: wireFieldId, op: "clear" }
                      : { fieldId: wireFieldId, op: "set", value: change.value },
                },
                workspaceContext: { workspaceId, workspaceSlug },
              });
            } catch (reason) {
              const ownsRecovery = () =>
                pageActive.current &&
                getCurrentWsId() === workspaceId &&
                getCurrentSlug() === workspaceSlug &&
                isClientSessionGenerationCurrent(
                  queryClient,
                  sessionGeneration,
                ) &&
                isClientWorkspaceAccessGenerationCurrent(
                  queryClient,
                  workspaceId,
                  workspaceAccessGeneration,
                ) &&
                isClientCollectionSourceGenerationCurrent(
                  queryClient,
                  workspaceId,
                  collectionId,
                  collectionSourceGeneration,
                );
              if (!ownsRecovery()) throw reason;
              const recovery = new AbortController();
              recoveryRequests.current.add(recovery);
              try {
                const authoritative = await api.getCollectionRecord(
                  collectionId,
                  record.id,
                  workspaceSlug,
                  recovery.signal,
                );
                if (!recovery.signal.aborted && ownsRecovery()) {
                  queryClient.setQueryData<CollectionRecord>(
                    collectionKeys.record(
                      workspaceId,
                      collectionId,
                      authoritative.id,
                    ),
                    (current) =>
                      !current || authoritative.revision >= current.revision
                        ? authoritative
                        : current,
                  );
                  setAuthoritativeRecords((current) => {
                    const existing = current.get(authoritative.id);
                    if (existing && existing.revision >= authoritative.revision) {
                      return current;
                    }
                    const next = new Map(current);
                    next.set(authoritative.id, authoritative);
                    return next;
                  });
                }
              } catch {
                // Preserve the original write failure. A failed authoritative
                // read cannot safely replace the frozen editing baseline.
              } finally {
                recoveryRequests.current.delete(recovery);
              }
              throw reason;
            }
          }
        : undefined,
    });
  }, [
    collectionId,
    queryClient,
    usableDetail,
    updateRecordAsync,
    workspaceId,
    workspaceSlug,
  ]);

  const binding = useMemo<
    DataViewQueryBinding<
      CollectionRecord,
      typeof collectionTableQuery,
      RawPage,
      DataSourceGroupPage
    > | null
  >(() => {
    if (!source) return null;
    return {
      identity: source.identity,
      rowPageKey: ({ query, groupBy, branch, hierarchy, page }) => [
        ...collectionKeys.rows(workspaceId, collectionId),
        query,
        groupBy,
        branch,
        hierarchy,
        page,
      ],
      rowBranchKey: ({ query, groupBy, branch, hierarchy }) => [
        ...collectionKeys.rows(workspaceId, collectionId),
        query,
        groupBy,
        branch,
        hierarchy,
      ],
      readRowPage: ({ query, page }, signal) => source.read(query, page, signal),
      mapRowPage: (page) => ({
        rows: page.rows,
        total: page.total,
        branchTotal: page.total,
        nextCursor: page.nextCursor,
      }),
      groupPagesKey: ({ query, groupBy }) => [
        ...collectionKeys.rows(workspaceId, collectionId),
        "groups",
        query,
        groupBy,
      ],
      readGroupPage: async () => {
        throw new Error("Collection grouping is not available in T2a");
      },
      mapGroupPage: (page) => page,
    };
  }, [collectionId, source, workspaceId]);

  useVisibleCollectionRefresh({
    enabled: enabled && source !== null,
    workspaceId,
    workspaceSlug,
    queryKey: collectionKeys.source(workspaceId, collectionId),
  });

  const submitRecord = async (event: FormEvent) => {
    event.preventDefault();
    const title = newTitle.trim();
    if (!title || createRecord.isPending) return;
    setCreateError(null);
    const operation: PendingRecordCreate = {
      intent: title,
      id:
        pendingCreateRequest.current?.intent === title
          ? pendingCreateRequest.current.id
          : requestId(),
      sessionGeneration: captureClientSessionGeneration(queryClient),
      workspaceAccessGeneration: captureClientWorkspaceAccessGeneration(
        queryClient,
        workspaceId,
      ),
    };
    pendingCreateRequest.current = operation;
    const ownsCompletion = () =>
      pendingCreateRequest.current === operation &&
      pageActive.current &&
      getCurrentWsId() === workspaceId &&
      getCurrentSlug() === workspaceSlug &&
      isClientSessionGenerationCurrent(
        queryClient,
        operation.sessionGeneration,
      ) &&
      isClientWorkspaceAccessGenerationCurrent(
        queryClient,
        workspaceId,
        operation.workspaceAccessGeneration,
      );
    try {
      await createRecord.mutateAsync({
        collectionId,
        input: {
          clientRequestId: operation.id,
          title,
          fields: {},
        },
        workspaceContext: { workspaceId, workspaceSlug },
      });
      if (!ownsCompletion()) return;
      pendingCreateRequest.current = null;
      setNewTitle("");
    } catch (reason) {
      if (!ownsCompletion()) return;
      setCreateError(reason instanceof Error ? reason.message : String(reason));
      await queryClient.invalidateQueries({
        queryKey: collectionKeys.rows(workspaceId, collectionId),
      });
    }
  };
  if (!enabled) {
    return (
      <CollectionPageState
        icon={Database}
        title={t(($) => $.not_available)}
        description={t(($) => $.not_available_description)}
      />
    );
  }
  if (effectiveSourceAccessError) {
    return (
      <CollectionPageState
        icon={AlertCircle}
        title={effectiveSourceAccessError.message}
        tone="destructive"
        role="alert"
        actions={
          <Button type="button" onClick={() => setSourceAccessError(null)}>
            {t(($) => $.retry)}
          </Button>
        }
      />
    );
  }
  if (detailQuery.isError && !detailQuery.data) {
    return (
      <CollectionPageState
        icon={AlertCircle}
        title={detailQuery.error.message}
        tone="destructive"
        role="alert"
        actions={
          <Button type="button" onClick={() => void detailQuery.refetch()}>
            {t(($) => $.retry)}
          </Button>
        }
      />
    );
  }
  if (detailQuery.isPending || !source || !binding) {
    return <CollectionPageState icon={Database} title={t(($) => $.loading)} />;
  }

  return (
    <CollectionTableContent
      source={source}
      binding={binding}
      collectionName={detailQuery.data.collection.name}
      newTitle={newTitle}
      setNewTitle={setNewTitle}
      createError={createError}
      creating={createRecord.isPending}
      submitRecord={submitRecord}
      columnSizing={columnSizing}
      setColumnSizing={setColumnSizing}
      editorStates={editorStates}
      onEditorStateChange={onEditorStateChange}
      authoritativeRecords={authoritativeRecords}
      detailError={detailQuery.isError ? detailQuery.error : null}
      retryDetail={() => void detailQuery.refetch()}
    />
  );
}

function CollectionTableContent({
  source,
  binding,
  collectionName,
  newTitle,
  setNewTitle,
  createError,
  creating,
  submitRecord,
  columnSizing,
  setColumnSizing,
  editorStates,
  onEditorStateChange,
  authoritativeRecords,
  detailError,
  retryDetail,
}: {
  source: CollectionSource;
  binding: DataViewQueryBinding<
    CollectionRecord,
    typeof collectionTableQuery,
    RawPage,
    DataSourceGroupPage
  >;
  collectionName: string;
  newTitle: string;
  setNewTitle: (value: string) => void;
  createError: string | null;
  creating: boolean;
  submitRecord: (event: FormEvent) => void;
  columnSizing: ColumnSizingState;
  setColumnSizing: Dispatch<SetStateAction<ColumnSizingState>>;
  editorStates: ReadonlyMap<
    string,
    DataViewCellEditorState<CollectionRecord>
  >;
  onEditorStateChange: (
    key: string,
    state: DataViewCellEditorState<CollectionRecord> | null,
  ) => void;
  authoritativeRecords: ReadonlyMap<string, CollectionRecord>;
  detailError: Error | null;
  retryDetail: () => void;
}) {
  const { t } = useT("collections");
  const dataView = useDataViewController({
    binding,
    query: collectionTableQuery,
    groupBy: null,
    hierarchy: false,
    collapsedGroupKeys: new Set<string>(),
    collapsedRowIds: new Set<string>(),
    rowId: source.rowId,
    directChildCount: () => 0,
    projectRow: ({ row }) => ({
      kind: "record" as const,
      key: row.id,
      sourceRow: row,
    }),
    rowPageSize: source.capabilities.maxPageSize,
  });
  const fields = source.fields;
  const columns = useMemo<ColumnDef<CollectionTableRow>[]>(
    () =>
      fields.map((field) => ({
        id: field.id,
        header: field.label,
        cell: CollectionRecordCell,
      })),
    [fields],
  );
  const cellContext = useMemo<CollectionCellContextValue>(
    () => ({
      source,
      saveLabel: t(($) => $.save),
      clearLabel: t(($) => $.clear),
      retryLabel: t(($) => $.retry),
      editorStates,
      onEditorStateChange,
    }),
    [editorStates, onEditorStateChange, source, t],
  );
  const editingKey = useMemo(
    () =>
      editorStates.size === 0
        ? null
        : JSON.stringify([...editorStates.keys()].sort()),
    [editorStates],
  );
  const refreshFrozenRows = useCallback(
    (snapshot: CollectionTableRow[], liveRows: CollectionTableRow[]) => {
      const liveByKey = new Map(liveRows.map((row) => [row.key, row]));
      return snapshot.map((row) => {
        if (row.kind !== "record") return row;
        const live = liveByKey.get(row.key);
        const authoritative = authoritativeRecords.get(row.key);
        const liveRecord = live?.kind === "record" ? live.sourceRow : null;
        const newest = [row.sourceRow, liveRecord, authoritative].reduce<
          CollectionRecord
        >(
          (current, candidate) =>
            candidate && candidate.revision > current.revision
              ? candidate
              : current,
          row.sourceRow,
        );
        return newest === row.sourceRow
          ? row
          : { kind: "record" as const, key: row.key, sourceRow: newest };
      });
    },
    [authoritativeRecords],
  );
  const structuralRow = useCallback(
    (row: { original: CollectionTableRow }) => {
      if (row.original.kind === "record") return null;
      const value = row.original;
      return {
        content:
          value.kind === "load_more" ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={!value.onLoad || value.state === "loading"}
              onClick={value.onLoad}
            >
              {value.state === "loading"
                ? t(($) => $.loading)
                : value.state === "error"
                  ? t(($) => $.retry)
                  : value.state === "has_more"
                    ? t(($) => $.load_more)
                    : null}
            </Button>
          ) : null,
      };
    },
    [t],
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <CollectionPageHeader
        icon={TableProperties}
        title={collectionName}
        count={dataView.authoritativeTotal}
        description={t(($) => $.records)}
        actions={
          source.capabilities.writable ? (
            <form className="flex items-center gap-2" onSubmit={submitRecord}>
              <Input
                aria-label={t(($) => $.record_title)}
                placeholder={t(($) => $.new_record)}
                value={newTitle}
                maxLength={500}
                disabled={creating}
                onChange={(event) => setNewTitle(event.currentTarget.value)}
              />
              <Button type="submit" size="sm" disabled={!newTitle.trim() || creating}>
                {creating ? t(($) => $.adding) : t(($) => $.add_record)}
              </Button>
            </form>
          ) : null
        }
      />
      {createError ? (
        <p role="alert" className="px-6 py-2 text-sm text-destructive">
          {createError}
        </p>
      ) : null}
      {detailError ? (
        <div
          role="alert"
          className="flex items-center justify-between gap-3 px-6 py-2 text-sm text-destructive"
        >
          <span>{detailError.message}</span>
          <Button type="button" variant="outline" size="sm" onClick={retryDetail}>
            {t(($) => $.retry)}
          </Button>
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-auto p-6">
        <CollectionCellContext.Provider value={cellContext}>
          <TableView
            sourceIdentity={source.identity}
            writable={source.capabilities.writable}
            rows={dataView.rows}
            columns={columns}
            rowId={(row) => row.key}
            visibleColumnIds={fields.map((field) => field.id)}
            columnSizing={columnSizing}
            onColumnSizingChange={setColumnSizing}
            onReorderColumn={() => {}}
            editingKey={editingKey}
            refreshFrozenRows={refreshFrozenRows}
            emptyMessage={t(($) => $.empty)}
            renderStructuralRow={structuralRow}
          />
        </CollectionCellContext.Provider>
      </div>
    </div>
  );
}
