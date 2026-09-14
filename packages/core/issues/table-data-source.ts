import { api } from "../api";
import type {
  DataSource,
  DataSourceActionResult,
  DataSourceCellCommand,
  DataSourceField,
  DataSourcePage,
  DataSourcePageRequest,
} from "../data-source";
import type {
  IssueProperty,
  IssueTableGroupDescriptor,
  IssueTableGroupsRequest,
  IssueTableQuerySpec,
  IssueTableRow,
  IssueTableRowsRequest,
  UpdateIssueRequest,
} from "../types";
import { hasUnknownActorRef } from "../types";
import {
  assertWorkspaceRequestContext,
  type WorkspaceRequestContext,
} from "../platform";

export type IssueTableBranchQuery = Omit<IssueTableRowsRequest, "page">;

export type IssueTableMutationCommand = {
  issue: IssueTableRow["issue"];
  updates: Partial<UpdateIssueRequest>;
};

export type IssueTableCommand = DataSourceCellCommand<IssueTableRow>;

export type IssueTableField = DataSourceField<IssueTableRow>;

export interface IssueTablePageMetadata {
  queryFingerprint: string;
  groupKey: string | null;
  parentId: string | null;
  branchTotal: number;
}

export interface IssueTableGroupPage {
  groups: IssueTableGroupDescriptor[];
  total: number;
  nextCursor: string | null;
  queryFingerprint: string;
}

export interface IssueTableDataSource
  extends DataSource<
    IssueTableRow,
    IssueTableBranchQuery,
    IssueTableCommand,
    IssueTableField,
    IssueTablePageMetadata
  > {
  readGroups(
    query: IssueTableQuerySpec,
    group: IssueTableGroupsRequest["group"],
    page: DataSourcePageRequest,
    signal?: AbortSignal,
  ): Promise<IssueTableGroupPage>;
}

type CreateIssueTableDataSourceOptions = {
  workspaceId?: string;
  workspaceSlug?: string;
  isContextActive?: () => boolean;
  fields?: readonly IssueProperty[];
  execute?: (
    command: IssueTableMutationCommand,
  ) =>
    | DataSourceActionResult<IssueTableRow>
    | Promise<DataSourceActionResult<IssueTableRow>>;
  setProperty?: (input: {
    issueId: string;
    propertyId: string;
    value: unknown;
  }) => Promise<void>;
  clearProperty?: (input: {
    issueId: string;
    propertyId: string;
  }) => Promise<void>;
  setLabels?: (input: {
    issue: IssueTableRow["issue"];
    labelIds: string[];
  }) => Promise<void>;
};

function executionError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}

const WRITABLE_SYSTEM_FIELDS = new Set([
  "title",
  "status",
  "priority",
  "assignee",
  "labels",
  "project",
  "start_date",
  "due_date",
]);
const CLEARABLE_SYSTEM_FIELDS = new Set([
  "assignee",
  "labels",
  "project",
  "start_date",
  "due_date",
]);
const SORTABLE_SYSTEM_FIELDS = new Set([
  "title",
  "status",
  "priority",
  "start_date",
  "due_date",
  "created_at",
  "updated_at",
]);
const GROUPABLE_SYSTEM_FIELDS = new Set([
  "status",
  "assignee",
  "project",
]);
const SYSTEM_FIELD_KINDS: Record<string, IssueTableField["kind"]> = {
  title: "text",
  identifier: "readonly",
  status: "select",
  priority: "select",
  assignee: "actor",
  labels: "multi_select",
  project: "select",
  start_date: "date",
  due_date: "date",
  created_at: "readonly",
  updated_at: "readonly",
  child_progress: "readonly",
  creator: "actor",
};

const SYSTEM_FIELD_VALUES = {
  title: (row: IssueTableRow) => row.issue.title,
  identifier: (row: IssueTableRow) => row.issue.identifier,
  status: (row: IssueTableRow) => row.issue.status,
  priority: (row: IssueTableRow) => row.issue.priority,
  assignee: (row: IssueTableRow) =>
    row.issue.assignee_type && row.issue.assignee_id
      ? { type: row.issue.assignee_type, id: row.issue.assignee_id }
      : undefined,
  labels: (row: IssueTableRow) => row.issue.labels ?? [],
  project: (row: IssueTableRow) => row.issue.project_id ?? undefined,
  start_date: (row: IssueTableRow) => row.issue.start_date ?? undefined,
  due_date: (row: IssueTableRow) => row.issue.due_date ?? undefined,
  created_at: (row: IssueTableRow) => row.issue.created_at,
  updated_at: (row: IssueTableRow) => row.issue.updated_at,
  child_progress: () => undefined,
  creator: (row: IssueTableRow) => ({
    type: row.issue.creator_type,
    id: row.issue.creator_id,
  }),
} satisfies Record<string, (row: IssueTableRow) => unknown>;

function propertyKind(property: IssueProperty): IssueTableField["kind"] {
  return [
    "select",
    "multi_select",
    "date",
    "checkbox",
    "text",
    "number",
    "url",
    "actor",
    "multi_actor",
  ].includes(property.type)
    ? (property.type as IssueTableField["kind"])
    : "readonly";
}

export function createIssueTableFields(
  properties: readonly IssueProperty[],
  writable: boolean,
): IssueTableField[] {
  const system = Object.entries(SYSTEM_FIELD_VALUES).map(
    ([id, value]): IssueTableField => ({
      id,
      label: id,
      kind: SYSTEM_FIELD_KINDS[id] ?? "readonly",
      value,
      sortable: SORTABLE_SYSTEM_FIELDS.has(id),
      groupable: GROUPABLE_SYSTEM_FIELDS.has(id),
      canSet: () => writable && WRITABLE_SYSTEM_FIELDS.has(id),
      canClear: (row) => {
        if (!writable || !CLEARABLE_SYSTEM_FIELDS.has(id)) return false;
        const current = value(row);
        return Array.isArray(current)
          ? current.length > 0
          : current !== undefined;
      },
    }),
  );
  return [
    ...system,
    ...properties.map((property): IssueTableField => ({
      id: `property:${property.id}`,
      label: property.name,
      kind: propertyKind(property),
      value: (row) => row.issue.properties[property.id],
      sortable: ![
        "multi_select",
        "checkbox",
        "actor",
        "multi_actor",
      ].includes(property.type),
      groupable: ["select", "checkbox"].includes(property.type),
      canSet: (row) =>
        writable &&
        !property.archived &&
        propertyKind(property) !== "readonly" &&
        (property.type !== "actor" ||
          !hasUnknownActorRef(row.issue.properties[property.id])),
      canClear: (row) =>
        writable && row.issue.properties[property.id] !== undefined,
      options: property.config.options?.map((option) => ({
        id: option.id,
        label: option.name,
        value: option.id,
        color: option.color,
      })),
    })),
  ];
}

function failed(message: string): DataSourceActionResult<IssueTableRow> {
  return { status: "failed", error: new Error(message) };
}

function updateForCommand(
  command: IssueTableCommand,
): Partial<UpdateIssueRequest> | null {
  const value = command.change.op === "set" ? command.change.value : null;
  switch (command.fieldId) {
    case "title":
      return typeof value === "string" ? { title: value } : null;
    case "status":
      return typeof value === "string"
        ? { status: value as UpdateIssueRequest["status"] }
        : null;
    case "priority":
      return typeof value === "string"
        ? { priority: value as UpdateIssueRequest["priority"] }
        : null;
    case "assignee": {
      if (command.change.op === "clear") {
        return { assignee_type: null, assignee_id: null };
      }
      if (!value || typeof value !== "object") return null;
      const actor = value as { type?: unknown; id?: unknown };
      return typeof actor.type === "string" && typeof actor.id === "string"
        ? {
            assignee_type: actor.type as UpdateIssueRequest["assignee_type"],
            assignee_id: actor.id,
          }
        : null;
    }
    case "project":
      return command.change.op === "clear" || typeof value === "string"
        ? { project_id: value as string | null }
        : null;
    case "start_date":
      return command.change.op === "clear" || typeof value === "string"
        ? { start_date: value as string | null }
        : null;
    case "due_date":
      return command.change.op === "clear" || typeof value === "string"
        ? { due_date: value as string | null }
        : null;
    default:
      return null;
  }
}

export function createIssueTableDataSource(
  options: CreateIssueTableDataSourceOptions = {},
): IssueTableDataSource {
  const writable = options.execute !== undefined;
  const workspaceContext: WorkspaceRequestContext | undefined =
    options.workspaceId && options.workspaceSlug
      ? {
          workspaceId: options.workspaceId,
          workspaceSlug: options.workspaceSlug,
          isActive: options.isContextActive,
        }
      : undefined;
  const assertActiveContext = () => {
    if (workspaceContext) assertWorkspaceRequestContext(workspaceContext);
  };
  const fields = createIssueTableFields(options.fields ?? [], writable).map(
    (field): IssueTableField => {
      if (field.id.startsWith("property:")) {
        return {
          ...field,
          canSet: (row) =>
            options.setProperty !== undefined && field.canSet(row),
          canClear: (row) =>
            options.clearProperty !== undefined && field.canClear(row),
        };
      }
      if (field.id === "labels") {
        return {
          ...field,
          canSet: (row) => options.setLabels !== undefined && field.canSet(row),
          canClear: (row) =>
            options.setLabels !== undefined && field.canClear(row),
        };
      }
      return field;
    },
  );
  return {
    identity: {
      workspaceId: options.workspaceId ?? "__legacy__",
      namespace: "issues",
      sourceId: "tasks",
    },
    key: "issues",
    fields,
    capabilities: {
      layouts: ["table", "board", "gantt", "swimlane"],
      grouping: true,
      hierarchy: true,
      writable,
      maxPageSize: 100,
    },
    rowId: (row) => row.issue.id,
    read: async (
      query,
      page,
      signal,
    ): Promise<DataSourcePage<IssueTableRow, IssueTablePageMetadata>> => {
      assertActiveContext();
      const request = { ...query, page };
      const response =
        options.workspaceSlug || signal
          ? await api.listIssueTableRows(request, {
              workspaceSlug: options.workspaceSlug,
              signal,
            })
          : await api.listIssueTableRows(request);
      return {
        rows: response.rows,
        total: response.total,
        nextCursor: response.next_cursor,
        metadata: {
          queryFingerprint: response.query_fingerprint,
          groupKey: response.group_key,
          parentId: response.parent_id,
          branchTotal: response.branch_total,
        },
      };
    },
    readGroups: async (query, group, page, signal) => {
      assertActiveContext();
      const request = { query, group, page };
      const response =
        options.workspaceSlug || signal
          ? await api.listIssueTableGroups(request, {
              workspaceSlug: options.workspaceSlug,
              signal,
            })
          : await api.listIssueTableGroups(request);
      return {
        groups: response.groups,
        total: response.total,
        nextCursor: response.next_cursor,
        queryFingerprint: response.query_fingerprint,
      };
    },
    execute: async (command) => {
      try {
        assertActiveContext();
        if (!options.execute) return failed("This issue data source is read-only");
        const field = fields.find((candidate) => candidate.id === command.fieldId);
        if (!field) return failed(`Unknown issue table field: ${command.fieldId}`);
        const allowed =
          command.change.op === "clear"
            ? field.canClear(command.row)
            : field.canSet(command.row);
        if (!allowed) return failed("This issue table field is read-only");
        if (command.fieldId.startsWith("property:")) {
          const propertyId = command.fieldId.slice("property:".length);
          if (command.change.op === "clear") {
            if (!options.clearProperty) {
              return failed("This issue property data source is read-only");
            }
            await options.clearProperty({
              issueId: command.row.issue.id,
              propertyId,
            });
          } else {
            if (!options.setProperty) {
              return failed("This issue property data source is read-only");
            }
            await options.setProperty({
              issueId: command.row.issue.id,
              propertyId,
              value: command.change.value,
            });
          }
          return { status: "accepted" };
        }
        if (command.fieldId === "labels") {
          if (!options.setLabels) {
            return failed("This issue label data source is read-only");
          }
          const labelIds =
            command.change.op === "clear"
              ? []
              : Array.isArray(command.change.value) &&
                  command.change.value.every((value) => typeof value === "string")
                ? command.change.value
                : null;
          if (!labelIds) return failed("Invalid issue labels value");
          await options.setLabels({ issue: command.row.issue, labelIds });
          return { status: "accepted" };
        }
        const updates = updateForCommand(command);
        if (!updates) return failed(`Unsupported issue table field: ${command.fieldId}`);
        return await options.execute({ issue: command.row.issue, updates });
      } catch (error) {
        return { status: "failed", error: executionError(error) };
      }
    },
  };
}

export const issueTableDataSource = createIssueTableDataSource();
