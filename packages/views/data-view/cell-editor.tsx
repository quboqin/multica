"use client";

import { useEffect, useRef, useState } from "react";
import type {
  DataSource,
  DataSourceCellChange,
  DataSourceCellCommand,
  DataSourceField,
} from "@multica/core/data-source";
import { dataSourceIdentityString } from "@multica/core/data-source";

type CellSource<Row> = Pick<
  DataSource<
    Row,
    unknown,
    DataSourceCellCommand<Row>,
    DataSourceField<Row>
  >,
  "capabilities" | "execute" | "identity" | "rowId"
>;

export type DataViewCellEditorState<Row> = {
  cellIdentity: string;
  draft: string;
  dirty: boolean;
  pending: boolean;
  error: string | null;
  baselineRow: Row;
  failedChange: DataSourceCellChange | null;
  operationId: number;
};

let nextOperationId = 0;

function inputValue(value: unknown) {
  if (value === undefined || value === null) return "";
  return String(value);
}

function editorValue<Row>(field: DataSourceField<Row>, row: Row) {
  const value = field.value(row);
  if (field.kind === "select") {
    return field.options?.find((option) => Object.is(option.value, value))?.id ?? "";
  }
  if (field.kind === "checkbox") return value === true ? "true" : "false";
  return inputValue(value);
}

export function dataViewCellEditorIdentity<Row>(
  source: CellSource<Row>,
  fieldId: string,
  row: Row,
) {
  return JSON.stringify([
    dataSourceIdentityString(source.identity),
    source.rowId(row),
    fieldId,
  ]);
}

function initialEditorState<Row>(
  cellIdentity: string,
  field: DataSourceField<Row>,
  row: Row,
): DataViewCellEditorState<Row> {
  return {
    cellIdentity,
    draft: editorValue(field, row),
    dirty: false,
    pending: false,
    error: null,
    baselineRow: row,
    failedChange: null,
    operationId: 0,
  };
}

function isActiveEditorState<Row>(state: DataViewCellEditorState<Row>) {
  return state.dirty || state.pending || state.error !== null;
}

/**
 * Default shared editor for scalar fields. Domain-specific editors may render
 * their own controls, but must keep the same command and final-result rules.
 */
export function DataViewCellEditor<Row>({
  source,
  field,
  row,
  onAccepted,
  persistedState,
  onStateChange,
  saveLabel,
  clearLabel,
  retryLabel = saveLabel,
  hideClearWhenUnavailable = false,
}: {
  source: CellSource<Row>;
  field: DataSourceField<Row>;
  row: Row;
  onAccepted?: () => void | Promise<void>;
  persistedState?: DataViewCellEditorState<Row>;
  onStateChange?: (state: DataViewCellEditorState<Row> | null) => void;
  saveLabel: string;
  clearLabel: string;
  retryLabel?: string;
  hideClearWhenUnavailable?: boolean;
}) {
  const authoritative = field.value(row);
  const latestRowRef = useRef(row);
  latestRowRef.current = row;
  const authoritativeEditorValueRef = useRef(editorValue(field, row));
  authoritativeEditorValueRef.current = editorValue(field, row);
  const cellIdentity = dataViewCellEditorIdentity(source, field.id, row);
  const [editorState, setEditorState] = useState<DataViewCellEditorState<Row>>(
    () =>
      persistedState?.cellIdentity === cellIdentity
        ? persistedState
        : initialEditorState(cellIdentity, field, row),
  );
  const stateRef = useRef(editorState);
  stateRef.current = editorState;
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;
  const previousPersistedStateRef = useRef(persistedState);
  const canSet = source.capabilities.writable && field.canSet(row);
  const canClear = source.capabilities.writable && field.canClear(row);

  const replaceEditorState = (
    next: DataViewCellEditorState<Row>,
    publish = true,
  ) => {
    stateRef.current = next;
    setEditorState(next);
    if (publish) {
      onStateChangeRef.current?.(isActiveEditorState(next) ? next : null);
    }
  };
  const updateEditorState = (
    update: (
      current: DataViewCellEditorState<Row>,
    ) => DataViewCellEditorState<Row>,
  ) => replaceEditorState(update(stateRef.current));

  useEffect(() => {
    const previous = previousPersistedStateRef.current;
    previousPersistedStateRef.current = persistedState;
    if (
      persistedState?.cellIdentity === cellIdentity &&
      persistedState !== stateRef.current
    ) {
      replaceEditorState(persistedState, false);
      return;
    }
    // A pending editor can be virtualized out and mounted again while its old
    // instance owns the request. The old completion removes the persisted
    // active state; reset the replacement instance to the now-authoritative
    // row instead of leaving it permanently pending.
    if (
      previous?.cellIdentity === cellIdentity &&
      persistedState === undefined &&
      isActiveEditorState(stateRef.current)
    ) {
      replaceEditorState(initialEditorState(cellIdentity, field, row), false);
    }
    // State replacement is deliberately driven only by the persisted object
    // identity. Equivalent DTO/field objects must not clear an edit session.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cellIdentity, persistedState]);

  useEffect(() => {
    if (stateRef.current.cellIdentity !== cellIdentity) {
      replaceEditorState(
        persistedState?.cellIdentity === cellIdentity
          ? persistedState
          : initialEditorState(cellIdentity, field, row),
        false,
      );
      return;
    }
    if (!isActiveEditorState(stateRef.current)) {
      replaceEditorState(initialEditorState(cellIdentity, field, row), false);
    }
    // DTO and field objects may be recreated by an unrelated cache refresh.
    // Only the stable source/row/field identity and authoritative value matter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authoritative, cellIdentity]);

  const commit = async (change: DataSourceCellChange) => {
    const previous = stateRef.current;
    if (previous.pending) return;
    // Preserve the row/revision the user actually edited. After a failed
    // attempt, a second explicit Save/Retry is the acknowledgement boundary
    // that may adopt the latest refetched row and try again.
    const commandRow =
      previous.error === null ? previous.baselineRow : latestRowRef.current;
    const currentlyAllowed =
      source.capabilities.writable &&
      (change.op === "clear"
        ? field.canClear(latestRowRef.current)
        : field.canSet(latestRowRef.current));
    if (!currentlyAllowed) {
      updateEditorState((current) => ({
        ...current,
        dirty: true,
        error: "This field is read-only",
        failedChange: change,
      }));
      return;
    }
    const operationId = ++nextOperationId;
    updateEditorState((current) => ({
      ...current,
      pending: true,
      dirty: true,
      error: null,
      failedChange: null,
      operationId,
    }));
    try {
      const result = await source.execute({
        row: commandRow,
        fieldId: field.id,
        change,
      });
      if (stateRef.current.operationId !== operationId) return;
      if (result.status === "accepted") {
        await onAccepted?.();
        if (stateRef.current.operationId !== operationId) return;
        const acceptedRow = result.row ?? latestRowRef.current;
        replaceEditorState({
          cellIdentity,
          draft:
            result.row === undefined
              ? authoritativeEditorValueRef.current
              : editorValue(field, result.row),
          dirty: false,
          pending: false,
          error: null,
          baselineRow: acceptedRow,
          failedChange: null,
          operationId,
        });
      } else if (result.status === "cancelled") {
        replaceEditorState({
          ...initialEditorState(cellIdentity, field, latestRowRef.current),
          operationId,
        });
      } else {
        updateEditorState((current) => ({
          ...current,
          pending: false,
          error: result.error.message,
          failedChange: change,
        }));
      }
    } catch (reason) {
      if (stateRef.current.operationId !== operationId) return;
      updateEditorState((current) => ({
        ...current,
        pending: false,
        error: reason instanceof Error ? reason.message : String(reason),
        failedChange: change,
      }));
    }
  };

  if (!canSet && !canClear) {
    return <span>{inputValue(authoritative)}</span>;
  }

  const retryButton = editorState.failedChange ? (
    <button
      type="button"
      disabled={editorState.pending}
      onClick={() => void commit(editorState.failedChange!)}
    >
      {retryLabel}
    </button>
  ) : null;

  if (field.kind === "checkbox") {
    return (
      <div>
        <label>
          <input
            aria-label={field.label}
            type="checkbox"
            checked={editorState.draft === "true"}
            disabled={!canSet || editorState.pending}
            onChange={(event) => {
              const checked = event.currentTarget.checked;
              updateEditorState((current) => ({
                ...current,
                baselineRow: current.dirty ? current.baselineRow : row,
                draft: checked ? "true" : "false",
                dirty: true,
                error: null,
                failedChange: null,
              }));
              void commit({ op: "set", value: checked });
            }}
          />
        </label>
        {!hideClearWhenUnavailable || canClear ? (
          <button
            type="button"
            disabled={!canClear || editorState.pending}
            onClick={() => {
              updateEditorState((current) => ({
                ...current,
                baselineRow: current.dirty ? current.baselineRow : row,
                draft: "false",
                dirty: true,
                error: null,
                failedChange: null,
              }));
              void commit({ op: "clear" });
            }}
          >
            {clearLabel}
          </button>
        ) : null}
        {retryButton}
        {editorState.error && <span role="alert">{editorState.error}</span>}
      </div>
    );
  }

  if (field.kind === "select") {
    return (
      <label>
        <span className="sr-only">{field.label}</span>
        <select
          aria-label={field.label}
          value={editorState.draft}
          disabled={editorState.pending || (!canSet && !canClear)}
          onChange={(event) => {
            const nextDraft = event.currentTarget.value;
            const option = field.options?.find(
              (candidate) => candidate.id === nextDraft,
            );
            updateEditorState((current) => ({
              ...current,
              baselineRow: current.dirty ? current.baselineRow : row,
              draft: nextDraft,
              dirty: true,
              error: null,
              failedChange: null,
            }));
            void commit(
              option
                ? { op: "set", value: option.value }
                : { op: "clear" },
            );
          }}
        >
          <option value="" disabled={!canClear}>
            —
          </option>
          {field.options?.map((option) => (
            <option key={option.id} value={option.id} disabled={!canSet}>
              {option.label}
            </option>
          ))}
        </select>
        {retryButton}
        {editorState.error && <span role="alert">{editorState.error}</span>}
      </label>
    );
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (field.kind === "number") {
          const trimmed = editorState.draft.trim();
          if (trimmed === "") {
            updateEditorState((current) => ({
              ...current,
              dirty: true,
              error: "Enter a number or use Clear",
              failedChange: null,
            }));
            return;
          }
          const value = Number(trimmed);
          if (!Number.isFinite(value)) {
            updateEditorState((current) => ({
              ...current,
              dirty: true,
              error: "Enter a valid number",
              failedChange: null,
            }));
            return;
          }
          void commit({ op: "set", value });
          return;
        }
        void commit({ op: "set", value: editorState.draft });
      }}
    >
      <input
        aria-label={field.label}
        type={field.kind === "number" ? "number" : "text"}
        step={field.kind === "number" ? "any" : undefined}
        value={editorState.draft}
        disabled={!canSet || editorState.pending}
        onChange={(event) => {
          const draft = event.currentTarget.value;
          updateEditorState((current) => ({
            ...current,
            baselineRow: current.dirty ? current.baselineRow : row,
            draft,
            dirty: true,
            error: null,
            failedChange: null,
          }));
        }}
      />
      <button type="submit" disabled={!canSet || editorState.pending}>
        {saveLabel}
      </button>
      {!hideClearWhenUnavailable || canClear ? (
        <button
          type="button"
          disabled={!canClear || editorState.pending}
          onClick={() => {
            updateEditorState((current) => ({
              ...current,
              baselineRow: current.dirty ? current.baselineRow : row,
              draft: "",
              dirty: true,
              error: null,
              failedChange: null,
            }));
            void commit({ op: "clear" });
          }}
        >
          {clearLabel}
        </button>
      ) : null}
      {editorState.error && <span role="alert">{editorState.error}</span>}
    </form>
  );
}
