import type { DataSourceField, DataSourceFieldKind } from "../data-source";
import { isKnownPropertyType, type Issue, type IssueProperty } from "../types";
import type { IssuePropertyValue } from "../types";
import {
  propertyIdFromViewKey,
  type SortField,
  type TableColumnKey,
  type TableSystemColumnKey,
} from "./stores/view-store";

const SYSTEM_SORT_KEYS: Partial<Record<TableSystemColumnKey, SortField>> = {
  title: "title",
  status: "status",
  priority: "priority",
  start_date: "start_date",
  due_date: "due_date",
  created_at: "created_at",
  updated_at: "updated_at",
};

const SYSTEM_FIELD_KINDS: Partial<
  Record<TableSystemColumnKey, DataSourceFieldKind>
> = {
  title: "text",
  status: "select",
  priority: "select",
  assignee: "actor",
  labels: "multi_select",
  project: "select",
  start_date: "date",
  due_date: "date",
};

/** Project the task catalog without giving the engine task DTOs or wire rules. */
export function createIssueTableFields(
  keys: readonly TableColumnKey[],
  properties: readonly IssueProperty[],
  label: (key: TableColumnKey) => string,
): DataSourceField<Issue, TableColumnKey>[] {
  const byId = new Map(properties.map((property) => [property.id, property]));
  return keys.map((id) => {
    const propertyId = propertyIdFromViewKey(id);
    const property = propertyId ? byId.get(propertyId) : undefined;
    const knownProperty = property && isKnownPropertyType(property.type);
    const sortKey = propertyId
      ? knownProperty &&
        !["multi_select", "checkbox", "actor", "multi_actor"].includes(
          property.type,
        )
        ? `property:${propertyId}`
        : undefined
      : SYSTEM_SORT_KEYS[id as TableSystemColumnKey];
    return {
      id,
      label: label(id),
      kind: propertyId
        ? knownProperty
          ? (property.type as DataSourceFieldKind)
          : "readonly"
        : (SYSTEM_FIELD_KINDS[id as TableSystemColumnKey] ?? "readonly"),
      value: (row) => issueTableFieldValue(row, id),
      sortKey,
      options: property?.config.options?.map((option) => ({
        id: option.id,
        label: option.name,
        color: option.color,
      })),
    };
  });
}

export function issueTableFieldValue(
  issue: Issue,
  columnKey: TableColumnKey,
): IssuePropertyValue | string | number | null | undefined {
  const propertyId = propertyIdFromViewKey(columnKey);
  if (propertyId) return issue.properties[propertyId];
  switch (columnKey) {
    case "identifier":
      return issue.identifier;
    case "title":
      return issue.title;
    case "status":
      return issue.status;
    case "priority":
      return issue.priority;
    case "assignee":
      return issue.assignee_id;
    case "labels":
      return issue.labels?.map((label) => label.name).join(", ");
    case "project":
      return issue.project_id;
    case "start_date":
      return issue.start_date;
    case "due_date":
      return issue.due_date;
    case "created_at":
      return issue.created_at;
    case "updated_at":
      return issue.updated_at;
    case "child_progress":
      return undefined;
    case "creator":
      return issue.creator_id;
  }
  return undefined;
}
