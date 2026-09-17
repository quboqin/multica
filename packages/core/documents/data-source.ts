import { api } from "../api";
import type {
  DataSource,
  DataSourceCellCommand,
  DataSourceField,
} from "../data-source";
import type { Issue, IssueTableQuerySpec } from "../types";

export const documentQuery: IssueTableQuerySpec = {
  kind: "doc",
  scope: { kind: "workspace" },
  filters: {},
  sort: { field: "created_at", direction: "asc" },
};
export const documentKeys = {
  rows: (workspaceId: string) => ["documents", workspaceId, "rows"] as const,
};
export function createDocumentDataSource(
  workspaceId: string,
  workspaceSlug: string,
): DataSource<
  Issue,
  IssueTableQuerySpec,
  DataSourceCellCommand<Issue>,
  DataSourceField<Issue>
> {
  return {
    identity: { workspaceId, namespace: "issues", sourceId: "docs" },
    key: "documents",
    capabilities: {
      layouts: ["table"],
      grouping: false,
      hierarchy: false,
      writable: false,
      maxPageSize: 100,
    },
    fields: [
      {
        id: "title",
        label: "Title",
        kind: "text",
        value: (row) => row.title,
        sortable: false,
        groupable: false,
        canSet: () => false,
        canClear: () => false,
      },
      {
        id: "updated_at",
        label: "Updated",
        kind: "readonly",
        value: (row) => row.updated_at,
        sortable: false,
        groupable: false,
        canSet: () => false,
        canClear: () => false,
      },
    ],
    rowId: (row) => row.id,
    read: async (_query, page, signal) => {
      const response = await api.listIssueTableRows(
        {
          query: documentQuery,
          group: { kind: "none" },
          group_key: null,
          hierarchy: { enabled: false },
          parent_id: null,
          page,
        },
        { workspaceSlug, signal },
      );
      if (
        response.rows.some(
          (row) =>
            row.issue.kind !== "doc" || row.issue.workspace_id !== workspaceId,
        )
      )
        throw new Error("Invalid document source response");
      return {
        rows: response.rows.map((row) => row.issue),
        total: response.total,
        nextCursor: response.next_cursor,
        metadata: {},
      };
    },
    execute: async () => ({
      status: "failed",
      error: new Error("Open the document to edit"),
    }),
  };
}
