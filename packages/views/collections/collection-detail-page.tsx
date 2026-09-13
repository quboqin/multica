"use client";

import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type FormEvent,
  type SetStateAction,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, ColumnSizingState } from "@tanstack/react-table";
import { AlertCircle, Database, TableProperties } from "lucide-react";
import { useWorkspaceId } from "@multica/core";
import { api } from "@multica/core/api";
import {
  collectionDetailOptions,
  collectionKeys,
  collectionTableQuery,
  createCollectionRecordDataSource,
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
import { useRequiredWorkspaceSlug } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  DataViewCellEditor,
  TableView,
  useDataViewController,
  type DataViewQueryBinding,
  type DataViewStructuralRow,
} from "../data-view";
import { useT } from "../i18n";
import {
  CollectionPageHeader,
  CollectionPageState,
} from "../layout/collection-page";

type DisplayRecord = {
  kind: "record";
  key: string;
  sourceRow: CollectionRecord;
};
type CollectionTableRow = DisplayRecord | DataViewStructuralRow;
type RawPage = DataSourcePage<CollectionRecord>;

function requestId() {
  return globalThis.crypto.randomUUID();
}

export function CollectionDetailPage({ collectionId }: { collectionId: string }) {
  const { t } = useT("collections");
  const enabled = useFeatureEnabled(CORTEX_COLLECTIONS_FLAG, false);
  const workspaceId = useWorkspaceId();
  const workspaceSlug = useRequiredWorkspaceSlug();
  const queryClient = useQueryClient();
  const detailQuery = useQuery({
    ...collectionDetailOptions(workspaceId, workspaceSlug, collectionId),
    enabled,
  });
  const createRecord = useCreateCollectionRecord();
  const updateRecord = useUpdateCollectionRecord();
  const [newTitle, setNewTitle] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);
  const [columnSizing, setColumnSizing] = useState<ColumnSizingState>({});
  const pendingCreateRequest = useRef<{ intent: string; id: string } | null>(
    null,
  );

  const source = useMemo(() => {
    if (!detailQuery.data) return null;
    return createCollectionRecordDataSource({
      detail: detailQuery.data,
      read: (page, signal) =>
        api.queryCollectionRecords(collectionId, page, workspaceSlug, signal),
      execute: detailQuery.data.capabilities.writable
        ? async ({ record, fieldId, change }) => {
            if (fieldId !== "title" || change.op !== "set") {
              throw new Error("This field is read-only");
            }
            return updateRecord.mutateAsync({
              collectionId,
              recordId: record.id,
              input: {
                expectedRevision: record.revision,
                change: { fieldId: "title", op: "set", value: String(change.value) },
              },
              workspaceContext: { workspaceId, workspaceSlug },
            });
          }
        : undefined,
    });
  }, [
    collectionId,
    detailQuery.data,
    updateRecord,
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

  const submitRecord = async (event: FormEvent) => {
    event.preventDefault();
    const title = newTitle.trim();
    if (!title || createRecord.isPending) return;
    setCreateError(null);
    if (pendingCreateRequest.current?.intent !== title) {
      pendingCreateRequest.current = { intent: title, id: requestId() };
    }
    try {
      await createRecord.mutateAsync({
        collectionId,
        input: {
          clientRequestId: pendingCreateRequest.current.id,
          title,
          fields: {},
        },
        workspaceContext: { workspaceId, workspaceSlug },
      });
      pendingCreateRequest.current = null;
      setNewTitle("");
    } catch (reason) {
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
  if (detailQuery.isError) {
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
      onAccepted={async () => {
        await queryClient.invalidateQueries({
          queryKey: collectionKeys.rows(workspaceId, collectionId),
        });
      }}
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
  onAccepted,
}: {
  source: ReturnType<typeof createCollectionRecordDataSource>;
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
  onAccepted: () => Promise<void>;
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
        cell: ({ row }) =>
          row.original.kind === "record" ? (
            <DataViewCellEditor
              source={source}
              field={field}
              row={row.original.sourceRow}
              saveLabel={t(($) => $.save)}
              clearLabel={t(($) => $.clear)}
              hideClearWhenUnavailable
              onAccepted={onAccepted}
            />
          ) : null,
      })),
    [fields, onAccepted, source, t],
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
        count={dataView.authoritativeRows.length}
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
      <div className="min-h-0 flex-1 overflow-auto p-6">
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
          emptyMessage={t(($) => $.empty)}
          renderStructuralRow={structuralRow}
        />
      </div>
    </div>
  );
}
