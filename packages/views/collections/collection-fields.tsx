"use client";

import {
  AtSign,
  Calendar,
  CheckSquare,
  CircleDot,
  Hash,
  Link2,
  ListChecks,
  Type,
  Users,
  type LucideIcon,
} from "lucide-react";
import type {
  CollectionField,
  CollectionRecord,
} from "@multica/core/collections";
import type { IssueProperty, IssuePropertyValue } from "@multica/core/types";
import { cn } from "@multica/ui/lib/utils";

/** Collections may hold at most this many active fields (server enforced). */
export const MAX_COLLECTION_FIELDS = 50;
/** The workspace issue-property quota a collection field does not consume. */
export const ISSUE_PROPERTY_QUOTA = 20;

const TYPE_ICONS: Record<string, LucideIcon> = {
  text: Type,
  number: Hash,
  select: CircleDot,
  multi_select: ListChecks,
  date: Calendar,
  checkbox: CheckSquare,
  url: Link2,
  actor: AtSign,
  multi_actor: Users,
};

export function FieldTypeIcon({
  type,
  className,
}: {
  type: string;
  className?: string;
}) {
  const Icon = TYPE_ICONS[type] ?? Type;
  return (
    <Icon
      aria-hidden
      className={cn("size-3.5 shrink-0 text-muted-foreground", className)}
    />
  );
}

export function fieldHasOptions(type: string): boolean {
  return type === "select" || type === "multi_select";
}

/**
 * Type changes the server accepts without losing values. Anything else needs
 * a new field and a migration.
 */
export function safeFieldConversions(type: string): string[] {
  switch (type) {
    case "select":
      return ["multi_select"];
    case "multi_select":
      return ["select"];
    case "actor":
      return ["multi_actor"];
    case "multi_actor":
      return ["actor"];
    case "number":
    case "date":
    case "url":
    case "checkbox":
      return ["text"];
    default:
      return [];
  }
}

/**
 * Collection fields share the issue-property type catalog, so the existing
 * property pickers edit them through this read-only projection.
 */
export function fieldAsProperty(field: CollectionField): IssueProperty {
  return {
    id: field.id,
    workspace_id: "",
    name: field.name,
    type: field.type,
    config: {
      options: field.config.options.map((option) => ({
        id: option.id,
        name: option.name,
        color: option.color ?? "#6b7280",
      })),
    },
    position: field.position,
    archived: false,
    created_at: "",
    updated_at: "",
  };
}

/** Narrows a stored JSON value to what the property pickers understand. */
export function recordValue(
  record: CollectionRecord,
  field: CollectionField,
): IssuePropertyValue | undefined {
  const value = record.fields[field.id];
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  )
    return value;
  if (Array.isArray(value))
    return value.filter((item): item is string => typeof item === "string");
  return undefined;
}

export function optionName(field: CollectionField, id: unknown): string {
  return field.config.options.find((option) => option.id === id)?.name ?? "";
}

/** Plain-text rendering used by gallery/board summaries and search labels. */
export function fieldText(field: CollectionField, value: unknown): string {
  if (value === undefined || value === null) return "";
  if (field.type === "select") return optionName(field, value);
  if (field.type === "multi_select" && Array.isArray(value))
    return value
      .map((id) => optionName(field, id))
      .filter(Boolean)
      .join(", ");
  if (Array.isArray(value)) return value.join(", ");
  return String(value);
}

/** Tinted pill for a select option — same color language as the prototype. */
export function OptionChip({
  name,
  color,
  className,
}: {
  name: string;
  color?: string;
  className?: string;
}) {
  const tint = color ?? "#6b7280";
  return (
    <span
      className={cn(
        "inline-flex max-w-40 items-center rounded-sm px-1.5 py-px text-caption font-medium leading-5",
        className,
      )}
      style={{ backgroundColor: `${tint}1f`, color: tint }}
    >
      <span className="truncate">{name}</span>
    </span>
  );
}
