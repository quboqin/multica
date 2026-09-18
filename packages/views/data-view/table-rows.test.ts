import { describe, expect, it, vi } from "vitest";
import { dataViewBranchKey, type DataViewBranch } from "./branch-model";
import { buildDataViewRows } from "./table-rows";

describe("buildDataViewRows", () => {
  it("flattens opaque groups, hierarchy branches and independent row pages", () => {
    const rootKey = dataViewBranchKey("opaque-group", null);
    const childKey = dataViewBranchKey("opaque-group", "parent");
    const branches = new Map<string, DataViewBranch>([
      [
        rootKey,
        {
          key: rootKey,
          groupKey: "opaque-group",
          parentRowId: null,
          ancestorRowIds: [],
          cursors: [null],
        },
      ],
      [
        childKey,
        {
          key: childKey,
          groupKey: "opaque-group",
          parentRowId: "parent",
          ancestorRowIds: ["parent"],
          cursors: [null],
        },
      ],
    ]);
    const load = vi.fn();
    const rows = buildDataViewRows({
      usesGrouping: true,
      groups: [{ key: "opaque-group", label: "Same label", count: 3 }],
      groupsPending: false,
      groupsError: false,
      hasNextGroupPage: true,
      fetchingNextGroupPage: false,
      refetchGroups: vi.fn(),
      fetchNextGroupPage: vi.fn(),
      collapsedGroupKeys: new Set(),
      collapsedRowIds: new Set(),
      hierarchy: true,
      branches,
      branchPages: {
        [rootKey]: {
          rows: [{ key: "parent", children: 1 }],
          total: 2,
          nextCursor: "root-tail",
          loading: false,
          error: false,
        },
        [childKey]: {
          rows: [{ key: "child", children: 0 }],
          total: 1,
          nextCursor: null,
          loading: false,
          error: false,
        },
      },
      rowId: (row) => row.key,
      directChildCount: (row) => row.children,
      projectRow: ({ row, depth }) => ({ key: row.key, depth }),
      activateBranch: vi.fn(),
      loadNextPage: load,
      retryBranch: vi.fn(),
    });

    expect(rows.map((row) => row.key)).toEqual([
      "opaque-group",
      "parent",
      "child",
      `end:${childKey}`,
      `more:${rootKey}:root-tail`,
      "more:groups",
    ]);
    const more = rows.find((row) => row.key.includes("root-tail"));
    if (more && "onLoad" in more) more.onLoad?.();
    expect(load).toHaveBeenCalledWith(rootKey, "root-tail");
  });

  it("deduplicates late rows and exposes branch-local retry", () => {
    const root = dataViewBranchKey(null, null);
    const retry = vi.fn();
    const rows = buildDataViewRows({
      usesGrouping: false,
      groups: [],
      groupsPending: false,
      groupsError: false,
      hasNextGroupPage: false,
      fetchingNextGroupPage: false,
      refetchGroups: vi.fn(),
      fetchNextGroupPage: vi.fn(),
      collapsedGroupKeys: new Set(),
      collapsedRowIds: new Set(),
      hierarchy: false,
      branches: new Map(),
      branchPages: {
        [root]: {
          rows: [
            { key: "same", children: 0 },
            { key: "same", children: 0 },
          ],
          total: 2,
          nextCursor: null,
          loading: false,
          error: true,
        },
      },
      rowId: (row) => row.key,
      directChildCount: (row) => row.children,
      projectRow: ({ row }) => ({ key: row.key }),
      activateBranch: vi.fn(),
      loadNextPage: vi.fn(),
      retryBranch: retry,
    });

    expect(rows.map((row) => row.key)).toEqual(["same", `retry:${root}`]);
    const retryRow = rows[1];
    if (retryRow && "onLoad" in retryRow) retryRow.onLoad?.();
    expect(retry).toHaveBeenCalledWith(root);
  });
});
