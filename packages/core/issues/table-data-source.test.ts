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
    const source = createIssueTableDataSource({ workspaceId: "ws-1" });

    expect(source.identity).toEqual({
      workspaceId: "ws-1",
      namespace: "issues",
      sourceId: "tasks",
    });

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

  it("translates groups and returns the issue executor's final result", async () => {
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
    const execute = vi.fn(async () => ({ status: "accepted" as const }));
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
      source.execute({
        row: { issue, direct_child_count: 0 },
        fieldId: "title",
        change: { op: "set", value: "Renamed" },
      }),
    ).resolves.toEqual({ status: "accepted" });
    expect(execute).toHaveBeenCalledWith({
      issue,
      updates: { title: "Renamed" },
    });
  });

  it("is read-only without an executor and rejects writes explicitly", async () => {
    const source = createIssueTableDataSource();

    expect(source.capabilities.writable).toBe(false);
    const result = await source.execute({
      row: { issue: makeIssue(), direct_child_count: 0 },
      fieldId: "title",
      change: { op: "set", value: "Renamed" },
    });

    expect(result.status).toBe("failed");
    if (result.status === "failed") {
      expect(result.error.message).toBe("This issue data source is read-only");
    }
  });

  it("projects stable system and custom fields with separate set/clear gates", () => {
    const issue = makeIssue();
    issue.properties = { archived: "old", active: false };
    const source = createIssueTableDataSource({
      workspaceId: "ws-1",
      fields: [
        {
          id: "archived",
          workspace_id: "ws-1",
          name: "Archived",
          type: "text",
          config: {},
          archived: true,
          position: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
        {
          id: "active",
          workspace_id: "ws-1",
          name: "Active flag",
          type: "checkbox",
          config: {},
          archived: false,
          position: 2,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      execute: async () => ({ status: "accepted" }),
      setProperty: async () => {},
      clearProperty: async () => {},
    });
    const archived = source.fields.find((field) => field.id === "property:archived")!;
    const active = source.fields.find((field) => field.id === "property:active")!;
    const identifier = source.fields.find((field) => field.id === "identifier")!;

    const row = { issue, direct_child_count: 0 };
    expect(archived.canSet(row)).toBe(false);
    expect(archived.canClear(row)).toBe(true);
    expect(active.value(row)).toBe(false);
    expect(active.groupable).toBe(true);
    expect(identifier.canSet(row)).toBe(false);
    expect(identifier.canClear(row)).toBe(false);
  });

  it("routes property set and clear through per-request completion channels", async () => {
    const issue = makeIssue();
    issue.properties = { estimate: 3 };
    const setProperty = vi.fn(async () => {});
    const clearProperty = vi.fn(async () => {});
    const source = createIssueTableDataSource({
      workspaceId: "ws-1",
      fields: [
        {
          id: "estimate",
          workspace_id: "ws-1",
          name: "Estimate",
          type: "number",
          config: {},
          archived: false,
          position: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      execute: vi.fn(async () => ({ status: "accepted" as const })),
      setProperty,
      clearProperty,
    });

    await expect(
      source.execute({
        row: { issue, direct_child_count: 0 },
        fieldId: "property:estimate",
        change: { op: "set", value: 0 },
      }),
    ).resolves.toEqual({ status: "accepted" });
    await expect(
      source.execute({
        row: { issue, direct_child_count: 0 },
        fieldId: "property:estimate",
        change: { op: "clear" },
      }),
    ).resolves.toEqual({ status: "accepted" });
    expect(setProperty).toHaveBeenCalledWith({
      issueId: issue.id,
      propertyId: "estimate",
      value: 0,
    });
    expect(clearProperty).toHaveBeenCalledWith({
      issueId: issue.id,
      propertyId: "estimate",
    });
  });

  it("rechecks read-only and clear-only field capabilities at execute time", async () => {
    const issue = makeIssue();
    issue.properties = { archived: "legacy" };
    const clearProperty = vi.fn(async () => {});
    const source = createIssueTableDataSource({
      fields: [
        {
          id: "archived",
          workspace_id: "ws-1",
          name: "Archived",
          type: "text",
          config: {},
          archived: true,
          position: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      execute: vi.fn(async () => ({ status: "accepted" as const })),
      clearProperty,
    });

    const denied = await source.execute({
      row: { issue, direct_child_count: 0 },
      fieldId: "property:archived",
      change: { op: "set", value: "overwrite" },
    });
    expect(denied.status).toBe("failed");
    await expect(
      source.execute({
        row: { issue, direct_child_count: 0 },
        fieldId: "property:archived",
        change: { op: "clear" },
      }),
    ).resolves.toEqual({ status: "accepted" });
    expect(clearProperty).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["synchronous throw", () => {
      throw new Error("sync boom");
    }, "sync boom"],
    ["asynchronous rejection", async () => {
      throw new Error("async boom");
    }, "async boom"],
  ])("converts an executor %s into a failed result", async (_label, execute, message) => {
    const source = createIssueTableDataSource({ execute });

    const result = await source.execute({
      row: { issue: makeIssue(), direct_child_count: 0 },
      fieldId: "title",
      change: { op: "set", value: "Renamed" },
    });

    expect(result.status).toBe("failed");
    if (result.status === "failed") expect(result.error.message).toBe(message);
  });
});
