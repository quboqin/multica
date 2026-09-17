import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createIssueTableDataSource } from "@multica/core/issues/table-data-source";
import { createDocumentDataSource } from "@multica/core/documents";
import { createCollectionRecordDataSource } from "@multica/core/collections/data-source";
import type { Issue, CollectionRecord } from "@multica/core/types";
import type { DataSource, DataSourceCellCommand, DataSourceField } from "@multica/core/data-source";
import { createDataViewFieldColumn } from "./field-column";
import { DataViewCellEditor } from "./cell-editor";

const issue: Issue = {
  id: "i", workspace_id: "ws", number: 1, identifier: "T-1", title: "Original",
  description: null, status: "todo", priority: "none", assignee_type: null,
  assignee_id: null, creator_type: "member", creator_id: "u", parent_issue_id: null,
  project_id: null, position: 1, stage: null, start_date: null, due_date: null,
  labels: [], metadata: {}, properties: {}, created_at: "2026-01-01", updated_at: "2026-01-01",
};
const record: CollectionRecord = {
  id: "r", workspaceId: "ws", collectionId: "c", title: "Original", fields: {},
  revision: 1, position: 0, createdAt: "2026-01-01", updatedAt: "2026-01-01",
};

function setup() {
  const taskWrite = vi.fn(async () => ({ status: "accepted" as const }));
  const recordWrite = vi.fn(async () => ({ ...record, title: "Changed", revision: 2 }));
  return {
    taskWrite, recordWrite,
    task: createIssueTableDataSource({ execute: taskWrite }),
    document: createDocumentDataSource("ws", "workspace"),
    record: createCollectionRecordDataSource({
      detail: {
        collection: { id: "c", workspaceId: "ws", name: "Data", revision: 1,
          archivedAt: null, createdAt: "2026-01-01", updatedAt: "2026-01-01" },
        fields: [], capabilities: { layouts: ["table"], grouping: false,
          hierarchy: false, writable: true, maxPageSize: 200 },
      },
      read: async () => ({ records: [record], total: 1, nextCursor: null }),
      execute: recordWrite,
    }),
  };
}

afterEach(cleanup);

function checkColumns<Row>(fields: readonly DataSourceField<Row>[], row: Row) {
  for (const field of fields) {
    const cell = vi.fn(() => null);
    const column = createDataViewFieldColumn<{ value: Row } | null, Row>({
      field, sourceRow: (display) => display?.value ?? null,
      presentation: { cell },
    });
    expect(column.id).toBe(field.id);
    expect(column.enableSorting).toBe(field.sortable);
    expect(column.enableColumnFilter).toBe(field.filterable);
    expect(column.enableGrouping).toBe(field.groupable);
    expect(column.cell).toBe(cell);
    if (!("accessorFn" in column) || !column.accessorFn) throw new Error("Missing accessor");
    expect(column.accessorFn({ value: row }, 0)).toEqual(field.value(row));
    expect(column.accessorFn(null, 0)).toBeUndefined();
  }
}

function Editor<Row, Query>(props: {
  source: DataSource<Row, Query, DataSourceCellCommand<Row>, DataSourceField<Row>, unknown>;
  row: Row;
}) {
  const field = props.source.fields.find((candidate) => candidate.id === "title")!;
  return <DataViewCellEditor {...props} field={field} saveLabel="Save" clearLabel="Clear" />;
}

describe("three adapters share the field contract", () => {
  it("projects values and capabilities through the same column builder", () => {
    const sources = setup();
    checkColumns(sources.task.fields, { issue, direct_child_count: 0 });
    checkColumns(sources.document.fields, { ...issue, kind: "doc" });
    checkColumns(sources.record.fields, record);
    expect(sources.task.fields.find((field) => field.id === "status")?.filterable).toBe(true);
    expect(sources.document.fields.every((field) => !field.filterable)).toBe(true);
    expect(sources.record.fields.every((field) => !field.sortable && !field.filterable)).toBe(true);
  });

  it("uses the same text editor and command for task and record titles", async () => {
    const sources = setup();
    const mounted = render(<Editor source={sources.task} row={{ issue, direct_child_count: 0 }} />);
    fireEvent.change(screen.getByDisplayValue("Original"), { target: { value: "Changed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sources.taskWrite).toHaveBeenCalledWith({ issue, updates: { title: "Changed" } }));
    mounted.unmount();
    render(<Editor source={sources.record} row={record} />);
    fireEvent.change(screen.getByDisplayValue("Original"), { target: { value: "Changed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sources.recordWrite).toHaveBeenCalledWith({ record, fieldId: "title", change: { op: "set", value: "Changed" } }));
  });

  it("keeps document table fields read-only without an object-kind condition", async () => {
    const { document } = setup();
    render(<Editor source={document} row={{ ...issue, kind: "doc" }} />);
    expect(screen.getByText("Original")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).toBeNull();
    const result = await document.execute({ row: issue, fieldId: "title", change: { op: "set", value: "Changed" } });
    expect(result.status).toBe("failed");
  });
});
