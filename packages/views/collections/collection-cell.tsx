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
import { useLocale, useT } from "../i18n";
import { ActorPropertyDisplay } from "../issues/components/pickers/actor-property-picker";
import { CustomPropertyValueInput } from "../issues/components/pickers/custom-property-picker";
import {
  OptionChip,
  fieldAsProperty,
  isRelation,
  recordLinks,
  recordValue,
} from "./collection-fields";
import { RelationChips, RelationEditor } from "./collection-relation";

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
      return option ? (
        <OptionChip name={option.name} color={option.color} />
      ) : (
        empty
      );
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
            <OptionChip
              key={option.id}
              name={option.name}
              color={option.color}
            />
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
              ? formatDateOnly(
                  value,
                  { month: "short", day: "numeric" },
                  locale,
                )
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
 * Read-only rendering of one cell of a record. A relation's content is its
 * links rather than an entry in the value bag, so cards and cells that only
 * display go through here instead of reading `record.fields` themselves.
 */
export function RecordValue({
  record,
  field,
  compact = false,
}: {
  record: CollectionRecord;
  field: CollectionField;
  compact?: boolean;
}) {
  if (field.type === "formula")
    return <FormulaValue record={record} field={field} compact={compact} />;
  if (isRelation(field))
    return (
      <RelationChips links={recordLinks(record, field)} compact={compact} />
    );
  return (
    <CollectionValue
      field={field}
      value={recordValue(record, field)}
      compact={compact}
    />
  );
}

/** Whether a record has anything to show for a field. */
export function hasRecordValue(
  record: CollectionRecord,
  field: CollectionField,
): boolean {
  if (record.formula_errors?.[field.id]) return true;
  return isRelation(field)
    ? recordLinks(record, field).length > 0
    : recordValue(record, field) !== undefined;
}

/** How a relation cell writes: one edge at a time, never as a value. */
export interface RelationActions {
  link: (recordId: string, fieldId: string, toId: string) => Promise<unknown>;
  unlink: (
    recordId: string,
    fieldId: string,
    linkId: string,
  ) => Promise<unknown>;
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
  relation,
  open,
  onOpenChange,
  readOnly = false,
  className,
}: {
  record: CollectionRecord;
  field: CollectionField;
  onChange: (value: unknown) => void;
  /** Required for relation fields to be editable; without it they only display. */
  relation?: RelationActions;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  readOnly?: boolean;
  className?: string;
}) {
  const value = recordValue(record, field);
  const label = `${field.name}: ${record.title}`;
  if (field.type === "formula")
    return (
      <div
        aria-label={label}
        className={cn(
          "flex h-full min-h-8 min-w-0 items-center px-2 text-label",
          className,
        )}
      >
        <FormulaValue record={record} field={field} />
      </div>
    );
  if (isRelation(field)) {
    const chips = <RelationChips links={recordLinks(record, field)} />;
    if (readOnly || !relation)
      return (
        <div className={cn("flex h-full min-w-0 items-center px-2", className)}>
          {chips}
        </div>
      );
    return (
      <RelationEditor
        record={record}
        field={field}
        open={open}
        onOpenChange={onOpenChange}
        onLink={(toId) => relation.link(record.id, field.id, toId)}
        onUnlink={(linkId) => relation.unlink(record.id, field.id, linkId)}
        trigger={chips}
        triggerRender={
          <button
            type="button"
            aria-label={label}
            className={cn(
              "flex h-full min-h-8 w-full min-w-0 items-center overflow-hidden px-2 text-left text-label outline-none hover:bg-accent/40 focus-visible:ring-2 focus-visible:ring-ring/50 data-[popup-open]:bg-accent/40",
              className,
            )}
          />
        }
      />
    );
  }
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

/** Formula errors remain visible in every layout and the record panel. */
function FormulaValue({
  record,
  field,
  compact = false,
}: {
  record: CollectionRecord;
  field: CollectionField;
  compact?: boolean;
}) {
  const { t } = useT("issues");
  const error = record.formula_errors?.[field.id];
  if (error) {
    const message =
      error === "#REF!"
        ? t(($) => $.cortex_formula.error_ref)
        : error === "#DIV/0!"
          ? t(($) => $.cortex_formula.error_div)
          : error === "#NUM!"
            ? t(($) => $.cortex_formula.error_num)
            : t(($) => $.cortex_formula.error_value);
    return (
      <span
        className="text-destructive"
        title={message}
        aria-label={`${error}: ${message}`}
      >
        {error}
      </span>
    );
  }
  return (
    <CollectionValue
      field={field}
      value={recordValue(record, field)}
      compact={compact}
    />
  );
}
