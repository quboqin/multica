"use client";

import { useEffect, useState } from "react";
import type {
  DataSource,
  DataSourceCellChange,
  DataSourceCellCommand,
  DataSourceField,
} from "@multica/core/data-source";

type CellSource<Row> = Pick<
  DataSource<
    Row,
    unknown,
    DataSourceCellCommand<Row>,
    DataSourceField<Row>
  >,
  "capabilities" | "execute" | "identity"
>;

function inputValue(value: unknown) {
  if (value === undefined || value === null) return "";
  return String(value);
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
}: {
  source: CellSource<Row>;
  field: DataSourceField<Row>;
  row: Row;
  onAccepted?: () => void | Promise<void>;
  saveLabel: string;
  clearLabel: string;
}) {
  const authoritative = field.value(row);
  const [draft, setDraft] = useState(() => inputValue(authoritative));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const canSet = source.capabilities.writable && field.canSet(row);
  const canClear = source.capabilities.writable && field.canClear(row);

  useEffect(() => {
    if (!pending && error === null) setDraft(inputValue(authoritative));
  }, [authoritative, error, pending]);

  useEffect(() => {
    setDraft(inputValue(field.value(row)));
    setError(null);
    setPending(false);
  }, [field, row, source.identity]);

  const commit = async (change: DataSourceCellChange) => {
    const currentlyAllowed =
      source.capabilities.writable &&
      (change.op === "clear" ? field.canClear(row) : field.canSet(row));
    if (!currentlyAllowed) {
      setError("This field is read-only");
      return;
    }
    setPending(true);
    setError(null);
    try {
      const result = await source.execute({ row, fieldId: field.id, change });
      if (result.status === "accepted") {
        await onAccepted?.();
      } else if (result.status === "cancelled") {
        setDraft(inputValue(field.value(row)));
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
      <label>
        <input
          aria-label={field.label}
          type="checkbox"
          checked={authoritative === true}
          disabled={!canSet || pending}
          onChange={(event) => {
            void commit({ op: "set", value: event.currentTarget.checked });
          }}
        />
        {error && <span role="alert">{error}</span>}
      </label>
    );
  }

  if (field.kind === "select") {
    return (
      <label>
        <span className="sr-only">{field.label}</span>
        <select
          aria-label={field.label}
          value={inputValue(authoritative)}
          disabled={pending || (!canSet && !canClear)}
          onChange={(event) => {
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
        void commit(draft === "" ? { op: "clear" } : { op: "set", value });
      }}
    >
      <input
        aria-label={field.label}
        type={field.kind === "number" ? "number" : "text"}
        value={draft}
        disabled={!canSet || pending}
        onChange={(event) => setDraft(event.currentTarget.value)}
      />
      <button type="submit" disabled={!canSet || pending}>
        {saveLabel}
      </button>
      <button
        type="button"
        disabled={!canClear || pending}
        onClick={() => void commit({ op: "clear" })}
      >
        {clearLabel}
      </button>
      {error && <span role="alert">{error}</span>}
    </form>
  );
}
