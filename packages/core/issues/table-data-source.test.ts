// @vitest-environment node

import { afterEach, describe, expect, it, vi } from "vitest";

import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import type {
  Issue,
  IssueTableGroupsRequest,
  IssueTableRowsRequest,
} from "../types";
import { createIssueTableDataSource } from "./table-data-source";

const query = {
  scope: { kind: "workspace" as const },
  filters: {},
  sort: { field: "position" as const, direction: "asc" as const },
};

function makeIssue(): Issue {
  return {
    id: "issue-1",
    workspace_id: "ws-1",
    number: 1,
    identifier: "MUL-1",
    title: "First issue",
    description: null,
    status: "todo",
    priority: "none",
    assignee_type: null,
    assignee_id: null,
    creator_type: "member",
    creator_id: "member-1",
    parent_issue_id: null,
    project_id: null,
    position: 1,
    stage: null,
    start_date: null,
    due_date: null,
    labels: [],
    metadata: {},
    properties: {},
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("createIssueTableDataSource", () => {
  it("translates the issue row API into the shared page contract", async () => {
    const issue = makeIssue();
    const listIssueTableRows = vi.fn(
      async (_request: IssueTableRowsRequest) => ({
        query_fingerprint: "rows-fingerprint",
        group_key: null,
        parent_id: null,
        total: 1,
        rows: [{ issue, direct_child_count: 2 }],
        branch_total: 1,
        next_cursor: "next-page",
      }),
    );
    setApiInstance({ listIssueTableRows } as unknown as ApiClient);
    const source = createIssueTableDataSource();

    const result = await source.read(
      {
        query,
        group: { kind: "none" },
        group_key: null,
        hierarchy: { enabled: true },
        parent_id: null,
      },
      { limit: 50, cursor: null },
    );

    expect(source.rowId(result.rows[0]!)).toBe(issue.id);
    expect(result).toEqual({
      rows: [{ issue, direct_child_count: 2 }],
      total: 1,
      nextCursor: "next-page",
      metadata: {
        queryFingerprint: "rows-fingerprint",
        groupKey: null,
        parentId: null,
        branchTotal: 1,
      },
    });
    expect(listIssueTableRows).toHaveBeenCalledWith({
      query,
      group: { kind: "none" },
      group_key: null,
      hierarchy: { enabled: true },
      parent_id: null,
      page: { limit: 50, cursor: null },
    });
  });

  it("translates groups and delegates writes to the issue adapter", async () => {
    const issue = makeIssue();
    const listIssueTableGroups = vi.fn(
      async (_request: IssueTableGroupsRequest) => ({
        query_fingerprint: "groups-fingerprint",
        total: 1,
        groups: [
          {
            key: "todo",
            value: { kind: "status" as const, status: "todo" },
            count: 1,
          },
        ],
        next_cursor: null,
      }),
    );
    const execute = vi.fn();
    setApiInstance({ listIssueTableGroups } as unknown as ApiClient);
    const source = createIssueTableDataSource({ execute });

    await expect(
      source.readGroups(query, { kind: "status" }, { limit: 100, cursor: null }),
    ).resolves.toEqual({
      groups: [
        {
          key: "todo",
          value: { kind: "status", status: "todo" },
          count: 1,
        },
      ],
      total: 1,
      nextCursor: null,
      queryFingerprint: "groups-fingerprint",
    });
    await expect(
      source.execute({ issue, updates: { title: "Renamed" } }),
    ).resolves.toEqual({ status: "accepted" });
    expect(execute).toHaveBeenCalledWith({
      issue,
      updates: { title: "Renamed" },
    });
  });
});
