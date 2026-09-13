import { api } from "../api";
import type {
  DataSource,
  DataSourceActionResult,
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

export type IssueTableBranchQuery = Omit<IssueTableRowsRequest, "page">;

export type IssueTableCommand = {
  issue: IssueTableRow["issue"];
  updates: Partial<UpdateIssueRequest>;
};

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
    IssueProperty,
    IssueTablePageMetadata
  > {
  readGroups(
    query: IssueTableQuerySpec,
    group: IssueTableGroupsRequest["group"],
    page: DataSourcePageRequest,
  ): Promise<IssueTableGroupPage>;
}

type CreateIssueTableDataSourceOptions = {
  fields?: readonly IssueProperty[];
  execute?: (
    command: IssueTableCommand,
  ) =>
    | void
    | DataSourceActionResult<IssueTableRow>
    | Promise<void | DataSourceActionResult<IssueTableRow>>;
};

export function createIssueTableDataSource(
  options: CreateIssueTableDataSourceOptions = {},
): IssueTableDataSource {
  return {
    key: "issues",
    fields: options.fields ?? [],
    capabilities: {
      layouts: ["table", "board", "gantt", "swimlane"],
      grouping: true,
      hierarchy: true,
      writable: options.execute !== undefined,
      maxPageSize: 100,
    },
    rowId: (row) => row.issue.id,
    read: async (
      query,
      page,
    ): Promise<DataSourcePage<IssueTableRow, IssueTablePageMetadata>> => {
      const response = await api.listIssueTableRows({
        ...query,
        page,
      });
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
    readGroups: async (query, group, page) => {
      const response = await api.listIssueTableGroups({ query, group, page });
      return {
        groups: response.groups,
        total: response.total,
        nextCursor: response.next_cursor,
        queryFingerprint: response.query_fingerprint,
      };
    },
    execute: async (command) => {
      if (!options.execute) {
        return {
          status: "failed",
          error: new Error("This issue data source is read-only"),
        };
      }
      const result = await options.execute(command);
      return result ?? { status: "accepted" };
    },
  };
}

export const issueTableDataSource = createIssueTableDataSource();
