"use client";

import { useCallback, useRef } from "react";
import {
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { useT } from "../i18n";
import {
  collectionKeys,
  type CollectionFieldPatch,
  type CollectionRecord,
} from "@multica/core/collections";

type RecordPatch = (record: CollectionRecord) => CollectionRecord;

/**
 * Applies a patch to every cached copy of one record: paged lists (any
 * filter/group/sort), the focused single-record read and the record sheet.
 */
function patchCachedRecord(
  qc: QueryClient,
  wsId: string,
  collectionId: string,
  recordId: string,
  patch: RecordPatch,
) {
  qc.setQueriesData(
    { queryKey: [...collectionKeys.all(wsId), collectionId] },
    (data: unknown) => {
      if (!data || typeof data !== "object") return data;
      if ("pages" in data && Array.isArray(data.pages)) {
        return {
          ...data,
          pages: data.pages.map((page: { records?: CollectionRecord[] }) =>
            Array.isArray(page?.records)
              ? {
                  ...page,
                  records: page.records.map((record) =>
                    record.id === recordId ? patch(record) : record,
                  ),
                }
              : page,
          ),
        };
      }
      if ("id" in data && data.id === recordId && "fields" in data)
        return patch(data as CollectionRecord);
      return data;
    },
  );
}

const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : String(error);

export function useCollectionCommands(
  wsId: string,
  collectionId: string,
  describeValue: (fieldId: string, value: unknown) => string = (_, value) =>
    value == null ? "—" : String(value),
) {
  const qc = useQueryClient();
  const { t } = useT("issues");
  // Writes to one cell run in order: each carries the value the previous one
  // produced as its expected value, so fast multi-select toggles never race
  // each other into a conflict.
  const chains = useRef(new Map<string, Promise<unknown>>());
  // The value the last queued write for a cell will leave on the server.
  const queued = useRef(new Map<string, unknown>());
  const refresh = useCallback(
    () =>
      qc.invalidateQueries({
        queryKey: [...collectionKeys.all(wsId), collectionId],
      }),
    [qc, wsId, collectionId],
  );
  const refreshAll = useCallback(
    () => qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) }),
    [qc, wsId],
  );

  const enqueue = useCallback(
    (key: string, run: () => Promise<unknown>) => {
      const previous = chains.current.get(key) ?? Promise.resolve();
      const next = previous.catch(() => undefined).then(run);
      chains.current.set(key, next);
      void next
        .finally(() => {
          if (chains.current.get(key) === next) {
            chains.current.delete(key);
            queued.current.delete(key);
            void refresh();
          }
        })
        .catch(() => undefined);
      return next;
    },
    [refresh],
  );

  const setField = useCallback(
    (record: CollectionRecord, fieldId: string, value: unknown) => {
      const key = `${record.id}:${fieldId}`;
      const expected = queued.current.has(key)
        ? queued.current.get(key)
        : (record.fields[fieldId] ?? null);
      queued.current.set(key, value ?? null);
      patchCachedRecord(qc, wsId, collectionId, record.id, (current) => {
        const fields = { ...current.fields };
        if (value === undefined || value === null) delete fields[fieldId];
        else fields[fieldId] = value;
        return { ...current, fields };
      });
      const write = (): Promise<unknown> =>
        enqueue(key, () =>
          api.setCollectionRecordField(
            collectionId,
            record.id,
            fieldId,
            value ?? null,
            expected,
          ),
        ).catch(async (error: unknown) => {
          void refresh();
          if (!(error instanceof ApiError) || error.status !== 409) {
            toast.error(errorMessage(error));
            return;
          }
          // Someone else changed this cell first. Keep the user's value in
          // the toast so it can be written over the current one on purpose.
          const latest = await api
            .listCollectionRecords(collectionId, { record_id: record.id }, null, undefined, wsId)
            .then((page) => page.records[0])
            .catch(() => undefined);
          toast.error(t(($) => $.cortex_table.conflict_title), {
            description: t(($) => $.cortex_table.conflict_current, {
              value: describeValue(fieldId, latest?.fields[fieldId]),
            }),
            action: latest
              ? {
                  label: t(($) => $.cortex_table.conflict_overwrite, {
                    value: describeValue(fieldId, value),
                  }),
                  onClick: () => void setFieldRef.current(latest, fieldId, value),
                }
              : undefined,
          });
        });
      return write();
    },
    [qc, wsId, collectionId, enqueue, refresh, describeValue, t],
  );
  const setFieldRef = useRef(setField);
  setFieldRef.current = setField;

  const setTitle = useCallback(
    (record: CollectionRecord, title: string) => {
      const base = record.title;
      if (title === base) return Promise.resolve();
      patchCachedRecord(qc, wsId, collectionId, record.id, (current) => ({
        ...current,
        title,
      }));
      return enqueue(`${record.id}:title`, () =>
        api.updateCollectionRecord(collectionId, record.id, title, base),
      ).catch((error: unknown) => {
        toast.error(errorMessage(error));
        void refresh();
      });
    },
    [qc, wsId, collectionId, enqueue, refresh],
  );

  const createRecord = useMutation({
    mutationFn: ({
      title,
      fields,
    }: {
      title: string;
      fields?: Record<string, unknown>;
    }) => api.createCollectionRecord(collectionId, title, fields),
    onSuccess: () => refreshAll(),
    onError: (error) => toast.error(errorMessage(error)),
  });

  const deleteRecord = useMutation({
    mutationFn: (recordId: string) =>
      api.deleteCollectionRecord(collectionId, recordId),
    onSuccess: () => refreshAll(),
    onError: (error) => toast.error(errorMessage(error)),
  });

  const restoreRecord = useMutation({
    mutationFn: (recordId: string) =>
      api.restoreCollectionRecord(collectionId, recordId),
    onSuccess: () => refreshAll(),
    onError: (error) => toast.error(errorMessage(error)),
  });

  const createField = useMutation({
    mutationFn: (field: Parameters<typeof api.createCollectionField>[1]) =>
      api.createCollectionField(collectionId, field),
    onSuccess: () => refresh(),
  });

  const updateField = useMutation({
    mutationFn: ({
      fieldId,
      patch,
    }: {
      fieldId: string;
      patch: CollectionFieldPatch;
    }) => api.updateCollectionField(collectionId, fieldId, patch),
    onSuccess: () => refresh(),
  });

  const renameCollection = useMutation({
    mutationFn: (name: string) => api.updateCollection(collectionId, { name }),
    onSuccess: () => refreshAll(),
    onError: (error) => toast.error(errorMessage(error)),
  });

  return {
    setField,
    setTitle,
    createRecord,
    deleteRecord,
    restoreRecord,
    createField,
    updateField,
    renameCollection,
    refresh,
  };
}

export type CollectionCommands = ReturnType<typeof useCollectionCommands>;
