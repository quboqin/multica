import { dataViewBranchKey, type DataViewBranch } from "./branch-model";

export type DataViewGroupRow = {
  kind: "group";
  key: string;
  label: string;
  count: number;
  collapsed: boolean;
};

export type DataViewLoadingRow = {
  kind: "load_more";
  key: string;
  state: "loading" | "has_more" | "error" | "end";
  total: number;
  onLoad?: () => void;
};

export type DataViewSkeletonRow = { kind: "skeleton"; key: string };
export type DataViewStructuralRow =
  | DataViewGroupRow
  | DataViewLoadingRow
  | DataViewSkeletonRow;

export type DataViewBranchPage<Row> = {
  rows: Row[];
  total: number;
  nextCursor: string | null;
  loading: boolean;
  error: boolean;
};

type Group = { key: string; label: string; count: number };

/** Shared group/hierarchy/branch-page flattener used by production TableView. */
export function buildDataViewRows<Row, DomainRow extends { key: string }>(options: {
  usesGrouping: boolean;
  groups: readonly Group[];
  groupsPending: boolean;
  groupsError: boolean;
  hasNextGroupPage: boolean;
  fetchingNextGroupPage: boolean;
  refetchGroups: () => void;
  fetchNextGroupPage: () => void;
  collapsedGroupKeys: ReadonlySet<string>;
  collapsedRowIds: ReadonlySet<string>;
  hierarchy: boolean;
  branches: ReadonlyMap<string, DataViewBranch>;
  branchPages: Readonly<Record<string, DataViewBranchPage<Row>>>;
  rowId: (row: Row) => string;
  directChildCount: (row: Row) => number;
  projectRow: (input: {
    row: Row;
    depth: number;
    hasChildren: boolean;
    collapsed: boolean;
  }) => DomainRow;
  activateBranch: (
    groupKey: string | null,
    parentRowId: string | null,
    ancestorRowIds: string[],
  ) => void;
  loadNextPage: (branchKey: string, cursor: string) => void;
  retryBranch: (branchKey: string) => void;
  skeletonCount?: number;
}): Array<DomainRow | DataViewStructuralRow> {
  const result: Array<DomainRow | DataViewStructuralRow> = [];
  const seenRowIds = new Set<string>();
  let domainRowCount = 0;
  const appendBranch = (
    groupKey: string | null,
    parentRowId: string | null,
    depth: number,
    ancestorRowIds: string[],
  ) => {
    const key = dataViewBranchKey(groupKey, parentRowId);
    const data = options.branchPages[key];
    if (!data) {
      const registered = options.branches.has(key);
      result.push({
        kind: "load_more",
        key: `${registered ? "loading" : "activate"}:${key}`,
        state: registered ? "loading" : "has_more",
        total: 0,
        ...(!registered
          ? {
              onLoad: () =>
                options.activateBranch(groupKey, parentRowId, ancestorRowIds),
            }
          : {}),
      });
      return;
    }
    if (data.rows.length === 0 && data.loading) {
      result.push({
        kind: "load_more",
        key: `loading:${key}`,
        state: "loading",
        total: 0,
      });
    }
    for (const row of data.rows) {
      const id = options.rowId(row);
      if (seenRowIds.has(id)) continue;
      seenRowIds.add(id);
      const collapsed = options.collapsedRowIds.has(id);
      const hasChildren =
        options.hierarchy && options.directChildCount(row) > 0;
      result.push(options.projectRow({ row, depth, hasChildren, collapsed }));
      domainRowCount += 1;
      if (hasChildren && !collapsed) {
        appendBranch(groupKey, id, depth + 1, [...ancestorRowIds, id]);
      }
    }
    if (data.error) {
      result.push({
        kind: "load_more",
        key: `retry:${key}`,
        state: "error",
        total: data.total,
        onLoad: () => options.retryBranch(key),
      });
    } else if (data.nextCursor) {
      const nextCursor = data.nextCursor;
      result.push({
        kind: "load_more",
        key: `more:${key}:${nextCursor}`,
        state: data.loading ? "loading" : "has_more",
        total: data.total,
        onLoad: () => options.loadNextPage(key, nextCursor),
      });
    } else if (data.rows.length > 0) {
      result.push({
        kind: "load_more",
        key: `end:${key}`,
        state: "end",
        total: data.total,
      });
    }
  };

  if (options.usesGrouping) {
    for (const group of options.groups) {
      const collapsed = options.collapsedGroupKeys.has(group.key);
      result.push({ ...group, kind: "group", collapsed });
      if (!collapsed) appendBranch(group.key, null, 0, []);
    }
  } else {
    appendBranch(null, null, 0, []);
  }

  if (options.usesGrouping && options.groups.length === 0 && options.groupsPending) {
    result.push({
      kind: "load_more",
      key: "loading:groups",
      state: "loading",
      total: 0,
    });
  } else if (options.usesGrouping && options.groupsError) {
    result.push({
      kind: "load_more",
      key: "retry:groups",
      state: "error",
      total: 0,
      onLoad: options.refetchGroups,
    });
  } else if (options.usesGrouping && options.hasNextGroupPage) {
    result.push({
      kind: "load_more",
      key: "more:groups",
      state: options.fetchingNextGroupPage ? "loading" : "has_more",
      total: 0,
      onLoad: options.fetchNextGroupPage,
    });
  }

  const cold =
    domainRowCount === 0 &&
    result.some(
      (row) => "kind" in row && row.kind === "load_more" && row.state === "loading",
    );
  return cold
    ? Array.from({ length: options.skeletonCount ?? 12 }, (_, index) => ({
        kind: "skeleton" as const,
        key: `skeleton:${index}`,
      }))
    : result;
}
