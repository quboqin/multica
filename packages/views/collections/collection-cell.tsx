"use client";

import { ExternalLink } from "lucide-react";
import type {
  CollectionField,
  CollectionRecord,
} from "@multica/core/collections";
import type { IssuePropertyValue } from "@multica/core/types";
import { formatDateOnly } from "@multica/core/issues/date";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { cn } from "@multica/ui/lib/utils";
import { useLocale } from "../i18n";
import { ActorPropertyDisplay } from "../issues/components/pickers/actor-property-picker";
import { CustomPropertyValueInput } from "../issues/components/pickers/custom-property-picker";
import { OptionChip, fieldAsProperty, recordValue } from "./collection-fields";

const empty = <span className="text-muted-foreground/60">—</span>;

/** Read-only rendering of one field value, shared by table, board and sheet. */
export function CollectionValue({
  field,
  value,
  compact = false,
}: {
  field: CollectionField;
  value: IssuePropertyValue | undefined;
  compact?: boolean;
}) {
  const locale = useLocale();
  if (value === undefined || value === "") return compact ? null : empty;
  switch (field.type) {
    case "select": {
      const option = field.config.options.find((item) => item.id === value);
      return option ? <OptionChip name={option.name} color={option.color} /> : empty;
    }
    case "multi_select": {
      const ids = Array.isArray(value) ? value : [];
      const options = field.config.options.filter((item) =>
        ids.includes(item.id),
      );
      if (!options.length) return compact ? null : empty;
      return (
        <span className="flex min-w-0 flex-wrap items-center gap-1">
          {options.map((option) => (
            <OptionChip key={option.id} name={option.name} color={option.color} />
          ))}
        </span>
      );
    }
    case "actor":
    case "multi_actor":
      return <ActorPropertyDisplay value={value} emptyLabel={empty} />;
    case "date":
      return (
        <span className="tabular-nums">
          {typeof value === "string"
            ? compact
              ? formatDateOnly(value, { month: "short", day: "numeric" }, locale)
              : value
            : String(value)}
        </span>
      );
    case "number":
      return <span className="tabular-nums">{String(value)}</span>;
    case "checkbox":
      return (
        <Checkbox checked={value === true} disabled aria-label={field.name} />
      );
    case "url":
      return (
        <span className="flex min-w-0 items-center gap-1 text-primary">
          <ExternalLink className="size-3 shrink-0" />
          <span className="truncate">{String(value)}</span>
        </span>
      );
    default:
      return <span className="truncate">{String(value)}</span>;
  }
}

/**
 * Editable value for one record field. Every type edits in place: pickers for
 * option and member fields (multi-select stays open while toggling), a
 * calendar for dates, an input popover for text-like values and a direct
 * toggle for checkboxes.
 */
export function CollectionFieldEditor({
  record,
  field,
  onChange,
  open,
  onOpenChange,
  readOnly = false,
  className,
}: {
  record: CollectionRecord;
  field: CollectionField;
  onChange: (value: unknown) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  readOnly?: boolean;
  className?: string;
}) {
  const value = recordValue(record, field);
  const label = `${field.name}: ${record.title}`;
  if (field.type === "checkbox") {
    return (
      <div className={cn("flex h-full items-center px-2", className)}>
        <Checkbox
          aria-label={label}
          checked={value === true}
          disabled={readOnly}
          onCheckedChange={(checked) => onChange(checked === true)}
        />
      </div>
    );
  }
  if (readOnly)
    return (
      <div className={cn("flex h-full min-w-0 items-center px-2", className)}>
        <CollectionValue field={field} value={value} />
      </div>
    );
  return (
    <CustomPropertyValueInput
      property={fieldAsProperty(field)}
      value={value}
      open={open}
      onOpenChange={onOpenChange}
      onChange={(next) => onChange(next ?? null)}
      trigger={<CollectionValue field={field} value={value} />}
      triggerRender={
        <button
          type="button"
          aria-label={label}
          className={cn(
            "flex h-full min-h-8 w-full min-w-0 items-center overflow-hidden px-2 text-left text-label outline-none hover:bg-accent/40 focus-visible:ring-2 focus-visible:ring-ring/50 data-[popup-open]:bg-accent/40",
            field.type === "number" && "justify-end",
            className,
          )}
        />
      }
    />
  );
}
