"use client";
import { useRef, useState } from "react";
import type { DataSourceFieldKind } from "@multica/core/data-source";

export function DataFieldEditor({
  label,
  kind,
  value,
  options = [],
  save,
  onEditingChange,
  labels,
}: {
  label: string;
  kind: DataSourceFieldKind;
  value: unknown;
  options?: readonly { id: string; label: string }[];
  save: (value: unknown, base: unknown) => Promise<unknown>;
  onEditingChange?: (editing: boolean) => void;
  labels?: { current: string; retry: string; discard: string };
}) {
  const [draft, setDraft] = useState<{ value: unknown } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const base = useRef(value);
  const current = draft ? draft.value : value;
  const discard = () => {
    setDraft(null);
    setError(null);
    onEditingChange?.(false);
  };
  const commit = async (next: unknown, expected = base.current) => {
    setDraft({ value: next });
    setPending(true);
    onEditingChange?.(true);
    try {
      await save(next, expected);
      discard();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setPending(false);
    }
  };
  const begin = () => {
    onEditingChange?.(true);
    if (draft === null) base.current = value;
  };
  const multiple = kind === "multi_select" || kind === "multi_actor";
  const display = (input: unknown): string =>
    Array.isArray(input)
      ? input.map(display).join(", ")
      : (options.find((option) => option.id === input)?.label ??
        String(input ?? "—"));
  let control;
  if (kind === "readonly") control = <span>{display(value)}</span>;
  else if (kind === "checkbox")
    control = (
      <input
        type="checkbox"
        aria-label={label}
        checked={current === true}
        disabled={pending}
        onChange={(event) => {
          if (draft === null) base.current = value;
          void commit(event.target.checked);
        }}
      />
    );
  else if (kind === "select" || kind === "actor" || multiple)
    control = (
      <select
        aria-label={label}
        multiple={multiple}
        disabled={pending}
        className="w-full rounded bg-transparent p-1"
        value={
          multiple
            ? Array.isArray(current)
              ? current
              : []
            : String(current ?? "")
        }
        onFocus={begin}
        onChange={(event) => {
          const next = multiple
            ? Array.from(event.target.selectedOptions, (option) => option.value)
            : event.target.value;
          void commit(
            Array.isArray(next) ? (next.length ? next : null) : next || null,
          );
        }}
      >
        {!multiple && <option value="">—</option>}
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.label}
          </option>
        ))}
      </select>
    );
  else
    control = (
      <input
        aria-label={label}
        disabled={pending}
        className="w-full min-w-20 rounded bg-transparent p-1"
        type={
          kind === "date"
            ? "date"
            : kind === "number"
              ? "number"
              : kind === "url"
                ? "url"
                : "text"
        }
        value={current == null ? "" : String(current)}
        onFocus={begin}
        onChange={(event) =>
          setDraft({
            value:
              event.target.value === ""
                ? null
                : kind === "number"
                  ? Number(event.target.value)
                  : event.target.value,
          })
        }
        onBlur={() => {
          if (draft !== null && !error) void commit(draft.value);
          else if (draft === null) onEditingChange?.(false);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") discard();
        }}
      />
    );
  return (
    <div className="min-w-0">
      {control}
      {error && (
        <div className="text-caption text-destructive">
          <span role="alert">{error}</span>
          {labels && (
            <>
              <p>
                {labels.current}: {display(value)}
              </p>
              <button
                type="button"
                disabled={pending || !draft}
                className="mr-2 underline"
                onClick={() => draft && void commit(draft.value, value)}
              >
                {labels.retry}
              </button>
              <button
                type="button"
                className="underline"
                disabled={pending}
                onClick={discard}
              >
                {labels.discard}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
