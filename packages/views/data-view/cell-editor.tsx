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

/**
 * Default shared editor for scalar fields. Domain-specific editors may render
 * their own controls, but must keep the same command and final-result rules.
 */
export function DataViewCellEditor<Row>({
  source,
  field,
  row,
  onAccepted,
  saveLabel,
  clearLabel,
  hideClearWhenUnavailable = false,
}: {
  source: CellSource<Row>;
  field: DataSourceField<Row>;
  row: Row;
  onAccepted?: () => void | Promise<void>;
  saveLabel: string;
  clearLabel: string;
  hideClearWhenUnavailable?: boolean;
}) {
  const authoritative = field.value(row);
  const latestRowRef = useRef(row);
  latestRowRef.current = row;
  const editBaselineRowRef = useRef(row);
  const authoritativeEditorValueRef = useRef(editorValue(field, row));
  authoritativeEditorValueRef.current = editorValue(field, row);
  const cellIdentity = JSON.stringify([
    dataSourceIdentityString(source.identity),
    source.rowId(row),
    field.id,
  ]);
  const [draft, setDraft] = useState(() => editorValue(field, row));
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const canSet = source.capabilities.writable && field.canSet(row);
  const canClear = source.capabilities.writable && field.canClear(row);

  useEffect(() => {
    if (!dirty && !pending && error === null) {
      editBaselineRowRef.current = row;
      setDraft(editorValue(field, row));
    }
  }, [authoritative, dirty, error, field, pending, row]);

  useEffect(() => {
    setDraft(editorValue(field, row));
    setDirty(false);
    setError(null);
    setPending(false);
    latestRowRef.current = row;
    editBaselineRowRef.current = row;
    // DTO and field objects may be recreated by an unrelated cache refresh.
    // Only a stable source/row/field transition starts a new edit session.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cellIdentity]);

  const commit = async (change: DataSourceCellChange) => {
    // Preserve the row/revision the user actually edited. After a failed
    // attempt, a second explicit Save is the acknowledgement boundary that
    // may adopt the latest refetched row and try again.
    const commandRow =
      error === null ? editBaselineRowRef.current : latestRowRef.current;
    const currentlyAllowed =
      source.capabilities.writable &&
      (change.op === "clear"
        ? field.canClear(latestRowRef.current)
        : field.canSet(latestRowRef.current));
    if (!currentlyAllowed) {
      setError("This field is read-only");
      return;
    }
    setPending(true);
    setDirty(true);
    setError(null);
    try {
      const result = await source.execute({
        row: commandRow,
        fieldId: field.id,
        change,
      });
      if (result.status === "accepted") {
        await onAccepted?.();
        editBaselineRowRef.current = result.row ?? latestRowRef.current;
        setDraft(
          result.row === undefined
            ? authoritativeEditorValueRef.current
            : editorValue(field, result.row),
        );
        setDirty(false);
      } else if (result.status === "cancelled") {
        editBaselineRowRef.current = latestRowRef.current;
        setDraft(editorValue(field, latestRowRef.current));
        setDirty(false);
      } else {
        setError(result.error.message);
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setPending(false);
    }
  };

  if (!canSet && !canClear) {
    return <span>{inputValue(authoritative)}</span>;
  }

  if (field.kind === "checkbox") {
    return (
      <div>
        <label>
          <input
            aria-label={field.label}
            type="checkbox"
            checked={draft === "true"}
            disabled={!canSet || pending}
            onChange={(event) => {
              setDraft(event.currentTarget.checked ? "true" : "false");
              void commit({ op: "set", value: event.currentTarget.checked });
            }}
          />
        </label>
        {!hideClearWhenUnavailable || canClear ? (
          <button
            type="button"
            disabled={!canClear || pending}
            onClick={() => {
              setDraft("false");
              void commit({ op: "clear" });
            }}
          >
            {clearLabel}
          </button>
        ) : null}
        {error && <span role="alert">{error}</span>}
      </div>
    );
  }

  if (field.kind === "select") {
    return (
      <label>
        <span className="sr-only">{field.label}</span>
        <select
          aria-label={field.label}
          value={draft}
          disabled={pending || (!canSet && !canClear)}
          onChange={(event) => {
            setDraft(event.currentTarget.value);
            const option = field.options?.find(
              (candidate) => candidate.id === event.currentTarget.value,
            );
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
        {error && <span role="alert">{error}</span>}
      </label>
    );
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        const value = field.kind === "number" ? Number(draft) : draft;
        void commit({ op: "set", value });
      }}
    >
      <input
        aria-label={field.label}
        type={field.kind === "number" ? "number" : "text"}
        value={draft}
        disabled={!canSet || pending}
        onChange={(event) => {
          if (!dirty) editBaselineRowRef.current = row;
          setDraft(event.currentTarget.value);
          setDirty(true);
        }}
      />
      <button type="submit" disabled={!canSet || pending}>
        {saveLabel}
      </button>
      {!hideClearWhenUnavailable || canClear ? (
        <button
          type="button"
          disabled={!canClear || pending}
          onClick={() => {
            setDraft("");
            void commit({ op: "clear" });
          }}
        >
          {clearLabel}
        </button>
      ) : null}
      {error && <span role="alert">{error}</span>}
    </form>
  );
}
