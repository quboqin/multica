"use client";
import type { ReactNode } from "react";
import type {
  DataSourceCapabilities,
  DataSourceField,
} from "@multica/core/data-source";

const text = (value: unknown): string => {
  if (value === null || value === undefined || value === "") return "";
  if (Array.isArray(value)) return value.map(text).filter(Boolean).join(", ");
  return String(value);
};

/**
 * Card grid. The cover is optional: without a cover field, or with an empty
 * or non-http value, the card degrades to title and details only.
 */
export function DataViewGallery<Row>({
  rows,
  rowId,
  title,
  fields,
  cover,
  onOpen,
  capabilities,
  toolbar,
  renderField,
}: {
  rows: readonly Row[];
  rowId: (row: Row) => string;
  title: (row: Row) => string;
  fields: readonly DataSourceField<Row>[];
  cover?: (row: Row) => string | null;
  onOpen?: (row: Row) => void;
  capabilities: DataSourceCapabilities;
  /** Source controls shown above the grid, e.g. cover and shown fields. */
  toolbar?: ReactNode;
  /** Rich rendering for a field value; plain text is used otherwise. */
  renderField?: (field: DataSourceField<Row>, row: Row) => ReactNode;
}) {
  if (!capabilities.layouts.includes("gallery")) return null;
  return (
    <div className="space-y-3 p-4">
      {toolbar && <div className="flex flex-wrap items-center gap-2">{toolbar}</div>}
      <div className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-3">
        {rows.map((row) => {
          const url = cover?.(row);
          const safeCover = url && /^https?:\/\//i.test(url) ? url : null;
          const details = fields
            .map((field) => ({ field, value: text(field.value(row)) }))
            .filter((item) => item.value);
          return (
            <article
              key={rowId(row)}
              className="group overflow-hidden rounded-lg border bg-card transition-shadow hover:shadow-sm"
            >
              {safeCover && (
                <img
                  alt=""
                  src={safeCover}
                  loading="lazy"
                  referrerPolicy="no-referrer"
                  className="aspect-[16/7] w-full border-b object-cover"
                />
              )}
              <div className="space-y-1 p-3">
                <button
                  type="button"
                  className="block w-full break-words text-left text-label font-medium group-hover:underline"
                  onClick={() => onOpen?.(row)}
                >
                  {title(row) || "—"}
                </button>
                {details.length > 0 && (
                  <dl className="flex flex-wrap items-center gap-x-2 gap-y-1 pt-0.5 text-caption text-muted-foreground">
                    {details.map(({ field, value }) => (
                      <div key={field.id} className="flex min-w-0 items-center">
                        <dt className="sr-only">{field.label}</dt>
                        <dd className="min-w-0 break-words">
                          {renderField ? renderField(field, row) : value}
                        </dd>
                      </div>
                    ))}
                  </dl>
                )}
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}
