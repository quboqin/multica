"use client";
import type {
  DataSourceCapabilities,
  DataSourceField,
} from "@multica/core/data-source";

export function DataViewGallery<Row>({
  rows,
  rowId,
  title,
  fields,
  cover,
  onOpen,
  capabilities,
}: {
  rows: readonly Row[];
  rowId: (row: Row) => string;
  title: (row: Row) => string;
  fields: readonly DataSourceField<Row>[];
  cover?: (row: Row) => string | null;
  onOpen?: (row: Row) => void;
  capabilities: DataSourceCapabilities;
}) {
  if (!capabilities.layouts.includes("gallery")) return null;
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4 p-4">
      {rows.map((row) => {
        const url = cover?.(row);
        const safeCover = url && /^https?:\/\//i.test(url) ? url : null;
        return (
          <article
            key={rowId(row)}
            className="overflow-hidden rounded-lg border bg-card"
          >
            {safeCover && (
              <img
                alt=""
                src={safeCover}
                loading="lazy"
                referrerPolicy="no-referrer"
                className="aspect-video w-full object-cover"
              />
            )}
            <div className="space-y-2 p-3">
              <button
                className="break-words text-left text-body font-medium"
                onClick={() => onOpen?.(row)}
              >
                {title(row)}
              </button>
              <dl className="space-y-1 text-caption">
                {fields.map((field) => (
                  <div key={field.id} className="flex gap-2">
                    <dt className="text-muted-foreground">{field.label}</dt>
                    <dd className="min-w-0 break-words">
                      {String(field.value(row) ?? "—")}
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          </article>
        );
      })}
    </div>
  );
}
