import type {
  DataSource,
  DataSourceCellCommand,
  DataSourceField,
} from "../data-source";
import type {
  CollectionDetail,
  CollectionRecord,
  CollectionRecordPage,
} from "../types";
import { collectionSourceIdentity, collectionTableQuery } from "./queries";

export type CollectionTableQuery = typeof collectionTableQuery;
export type CollectionTableCommand = DataSourceCellCommand<CollectionRecord>;

export type CollectionRecordExecutor = (input: {
  record: CollectionRecord;
  fieldId: string;
  change: CollectionTableCommand["change"];
}) => Promise<CollectionRecord>;

function fieldKind(type: string): DataSourceField<CollectionRecord>["kind"] {
  switch (type) {
    case "text":
    case "number":
    case "checkbox":
      return type;
    default:
      return "readonly";
  }
}

function isValidFieldChange(
  field: DataSourceField<CollectionRecord>,
  change: CollectionTableCommand["change"],
): boolean {
  if (change.op === "clear") {
    return (
      field.kind === "text" ||
      field.kind === "number" ||
      field.kind === "checkbox"
    );
  }
  switch (field.kind) {
    case "text":
      return typeof change.value === "string";
    case "number":
      return typeof change.value === "number" && Number.isFinite(change.value);
    case "checkbox":
      return typeof change.value === "boolean";
    default:
      return false;
  }
}

export function createCollectionRecordDataSource(options: {
  detail: CollectionDetail;
  read: (
    page: { limit?: number; cursor?: string | null },
    signal?: AbortSignal,
  ) => Promise<CollectionRecordPage>;
  execute?: CollectionRecordExecutor;
}): DataSource<
  CollectionRecord,
  CollectionTableQuery,
  CollectionTableCommand,
  DataSourceField<CollectionRecord>
> {
  const writable = options.detail.capabilities.writable && Boolean(options.execute);
  const titleField: DataSourceField<CollectionRecord, string> = {
    id: "title",
    label: "Title",
    kind: "text",
    value: (record) => record.title,
    sortable: false,
    groupable: false,
    canSet: () => writable,
    canClear: () => false,
  };
  const fields: DataSourceField<CollectionRecord>[] = [
    titleField,
    ...options.detail.fields.map(
      (field): DataSourceField<CollectionRecord> => ({
        id: `field:${field.id}`,
        label: field.name,
        kind: fieldKind(field.type),
        value: (record) => record.fields[field.id],
        sortable: false,
        groupable: false,
        canSet: () => writable && fieldKind(field.type) !== "readonly",
        canClear: () => writable && fieldKind(field.type) !== "readonly",
      }),
    ),
  ];
  const identity = collectionSourceIdentity(
    options.detail.collection.workspaceId,
    options.detail.collection.id,
  );
  return {
    identity,
    key: `collection:${options.detail.collection.id}`,
    fields,
    capabilities: {
      layouts: ["table"],
      grouping: false,
      hierarchy: false,
      writable,
      maxPageSize: Math.min(options.detail.capabilities.maxPageSize, 200),
    },
    rowId: (record) => record.id,
    read: (_query, page, signal) =>
      options.read(page, signal).then((result) => ({
        rows: result.records,
        total: result.total,
        nextCursor: result.nextCursor,
        metadata: {},
      })),
    execute: async ({ row, fieldId, change }) => {
      const field = fields.find((candidate) => candidate.id === fieldId);
      if (
        !writable ||
        !options.execute ||
        !field ||
        !isValidFieldChange(field, change) ||
        (fieldId === "title" && change.op !== "set")
      ) {
        return { status: "failed", error: new Error("This field is read-only") };
      }
      try {
        return {
          status: "accepted",
          row: await options.execute({ record: row, fieldId, change }),
        };
      } catch (error) {
        return {
          status: "failed",
          error: error instanceof Error ? error : new Error(String(error)),
        };
      }
    },
  };
}
