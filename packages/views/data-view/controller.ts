"use client";

import {
  useInfiniteQuery,
  useQueries,
  useQueryClient,
  type UseQueryResult,
} from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  dataViewBranchKey,
  rebaseDataViewBranchState,
  sameDataViewPath,
  type DataViewBranch,
  type DataViewBranchState,
} from "./branch-model";
import {
  buildDataViewRows,
  type DataViewBranchPage,
  type DataViewStructuralRow,
} from "./table-rows";
import type {
  DataViewGroupBy,
  DataViewQueryBinding,
} from "./query-binding";

type BranchTarget = {
  branch: DataViewBranch;
  cursor: string | null;
};

type BranchData<Row, RawRowPage> = DataViewBranchPage<Row> & {
  headUpdatedAt: number;
  headFetching: boolean;
  placeholder: boolean;
  headRaw?: RawRowPage;
};

export type DataViewControllerResult<Row, DisplayRow extends { key: string }> = {
  rows: Array<DisplayRow | DataViewStructuralRow>;
  /** Settled rows only; query-transition placeholder rows are excluded. */
  authoritativeRows: Row[];
  /** Authoritative total for the ungrouped root branch. */
  authoritativeTotal: number;
  groupError: unknown;
};

/**
 * Shared production row controller. It owns group-directory paging, the
 * hierarchy branch graph, independent row cursors, retry, previous-head
 * painting and stale-tail eviction. Domain assemblies only bind their cache
 * and project opaque rows into render rows.
 */
export function useDataViewController<
  Row,
  Query,
  RawRowPage,
  RawGroupPage,
  DisplayRow extends { key: string },
>(options: {
  binding: DataViewQueryBinding<Row, Query, RawRowPage, RawGroupPage>;
  query: Query;
  groupBy: DataViewGroupBy;
  hierarchy: boolean;
  collapsedGroupKeys: ReadonlySet<string>;
  collapsedRowIds: ReadonlySet<string>;
  rowId: (row: Row) => string;
  directChildCount: (row: Row) => number;
  projectRow(input: {
    row: Row;
    depth: number;
    hasChildren: boolean;
    collapsed: boolean;
  }): DisplayRow;
  rowPageSize?: number;
  groupPageSize?: number;
  skeletonCount?: number;
}): DataViewControllerResult<Row, DisplayRow> {
  const {
    binding,
    query,
    groupBy,
    hierarchy,
    collapsedGroupKeys,
    collapsedRowIds,
    rowId,
    directChildCount,
    projectRow,
    rowPageSize = 50,
    groupPageSize = 100,
    skeletonCount,
  } = options;
  const queryClient = useQueryClient();
  const usesGrouping = groupBy !== null;
  const fallbackGroupBy = groupBy ?? { fieldId: "__ungrouped__" };
  const groupQuery = useInfiniteQuery({
    queryKey: binding.groupPagesKey({ query, groupBy: fallbackGroupBy }),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      binding.readGroupPage(
        {
          query,
          groupBy: fallbackGroupBy,
          page: { limit: groupPageSize, cursor: pageParam },
        },
        signal,
      ),
    getNextPageParam: (lastPage) =>
      binding.mapGroupPage(lastPage).nextCursor ?? undefined,
    retry: false,
    enabled: usesGrouping,
  });
  const groups = useMemo(
    () =>
      groupQuery.data?.pages.flatMap(
        (page) => binding.mapGroupPage(page).groups,
      ) ?? [],
    [binding, groupQuery.data?.pages],
  );
  const identity = useMemo(
    () => JSON.stringify([binding.identity, query, groupBy, hierarchy]),
    [binding.identity, groupBy, hierarchy, query],
  );
  const structureIdentity = useMemo(
    () => JSON.stringify([binding.identity, groupBy, hierarchy]),
    [binding.identity, groupBy, hierarchy],
  );
  const [branchState, setBranchState] = useState<DataViewBranchState>({
    identity: "",
    structureIdentity: "",
    branches: new Map(),
  });
  const rebasedBranchState = useMemo(
    () =>
      rebaseDataViewBranchState(
        branchState,
        identity,
        structureIdentity,
        usesGrouping,
      ),
    [branchState, identity, structureIdentity, usesGrouping],
  );
  useEffect(() => {
    if (rebasedBranchState !== branchState) setBranchState(rebasedBranchState);
  }, [branchState, rebasedBranchState]);

  const activeBranches = rebasedBranchState.branches;
  const placeholderRef = useRef(new Map<string, RawRowPage>());
  const targets = useMemo<BranchTarget[]>(
    () =>
      [...activeBranches.values()].flatMap((branch) =>
        branch.cursors.map((cursor) => ({ branch, cursor })),
      ),
    [activeBranches],
  );
  const queries = useMemo(
    () =>
      targets.map(({ branch, cursor }) => {
        const request = {
          query,
          groupBy,
          branch: {
            groupKey: branch.groupKey,
            parentRowId: branch.parentRowId,
          },
          hierarchy,
          page: { limit: rowPageSize, cursor },
        };
        const placeholder =
          cursor === null
            ? placeholderRef.current.get(`${structureIdentity}:${branch.key}`)
            : undefined;
        return {
          queryKey: binding.rowPageKey(request),
          queryFn: ({ signal }: { signal: AbortSignal }) =>
            binding.readRowPage(request, signal),
          ...(placeholder ? { placeholderData: placeholder } : {}),
          enabled:
            (branch.groupKey === null ||
              !collapsedGroupKeys.has(branch.groupKey)) &&
            !branch.ancestorRowIds.some((id) => collapsedRowIds.has(id)),
          retry: false,
          retryOnMount: false,
          refetchOnMount: (state: { state: { status: string } }) =>
            state.state.status !== "error",
        };
      }),
    [
      binding,
      collapsedGroupKeys,
      collapsedRowIds,
      groupBy,
      hierarchy,
      query,
      rowPageSize,
      structureIdentity,
      targets,
    ],
  );
  const combine = useCallback(
    (results: Array<UseQueryResult<RawRowPage, Error>>) => {
      const byBranch: Record<string, BranchData<Row, RawRowPage>> = {};
      for (let index = 0; index < targets.length; index += 1) {
        const target = targets[index];
        const result = results[index];
        if (!target || !result) continue;
        const current = byBranch[target.branch.key] ?? {
          rows: [],
          total: 0,
          nextCursor: null,
          headUpdatedAt: 0,
          headFetching: false,
          loading: false,
          error: false,
          placeholder: false,
        };
        if (result.data) {
          const page = binding.mapRowPage(result.data);
          current.rows.push(...page.rows);
          if (target.cursor === null) {
            current.total = page.branchTotal;
            current.headRaw = result.data;
          }
          current.nextCursor = page.nextCursor;
        }
        if (target.cursor === null) {
          current.headUpdatedAt = result.dataUpdatedAt;
          current.headFetching = result.isFetching;
        }
        current.loading ||= result.isPending || result.isFetching;
        current.error ||= result.isError;
        current.placeholder ||= result.isPlaceholderData;
        byBranch[target.branch.key] = current;
      }
      return byBranch;
    },
    [binding, targets],
  );
  const branchData = useQueries({ queries, combine });

  useEffect(() => {
    const next = new Map<string, RawRowPage>();
    for (const branch of activeBranches.values()) {
      const key = `${structureIdentity}:${branch.key}`;
      const data = branchData[branch.key];
      if (!data || data.placeholder || data.error || !data.headRaw) {
        const previous = placeholderRef.current.get(key);
        if (previous) next.set(key, previous);
      } else {
        next.set(key, data.headRaw);
      }
    }
    placeholderRef.current = next;
  }, [activeBranches, branchData, structureIdentity]);

  useEffect(() => {
    const desired = new Map<string, string[]>();
    const visited = new Set<string>();
    const visit = (
      groupKey: string | null,
      parentRowId: string | null,
      ancestors: string[],
    ) => {
      const key = dataViewBranchKey(groupKey, parentRowId);
      if (visited.has(key)) return;
      visited.add(key);
      desired.set(key, ancestors);
      for (const row of branchData[key]?.rows ?? []) {
        if (directChildCount(row) > 0) {
          const id = rowId(row);
          visit(groupKey, id, [...ancestors, id]);
        }
      }
    };
    if (usesGrouping) {
      for (const group of groups) visit(group.key, null, []);
    } else {
      visit(null, null, []);
    }
    setBranchState((previous) => {
      if (previous.identity !== identity) return previous;
      let branches: Map<string, DataViewBranch> | null = null;
      for (const [key, ancestors] of desired) {
        const branch = previous.branches.get(key);
        if (!branch || sameDataViewPath(branch.ancestorRowIds, ancestors)) continue;
        branches ??= new Map(previous.branches);
        branches.set(key, { ...branch, ancestorRowIds: ancestors });
      }
      return branches ? { ...previous, branches } : previous;
    });
  }, [branchData, directChildCount, groups, identity, rowId, usesGrouping]);

  const activateBranch = useCallback(
    (
      groupKey: string | null,
      parentRowId: string | null,
      ancestorRowIds: string[],
    ) => {
      setBranchState((previous) => {
        if (previous.identity !== identity) return previous;
        const key = dataViewBranchKey(groupKey, parentRowId);
        const existing = previous.branches.get(key);
        if (
          existing &&
          sameDataViewPath(existing.ancestorRowIds, ancestorRowIds)
        ) {
          return previous;
        }
        const branches = new Map(previous.branches);
        branches.set(
          key,
          existing
            ? { ...existing, ancestorRowIds }
            : {
                key,
                groupKey,
                parentRowId,
                ancestorRowIds,
                cursors: [null],
              },
        );
        return { ...previous, branches };
      });
    },
    [identity],
  );
  const headRevisionRef = useRef<Record<string, number>>({});
  useEffect(() => {
    const previous = headRevisionRef.current;
    const next: Record<string, number> = {};
    const trim = new Set<string>();
    for (const [key, branch] of activeBranches) {
      const revision = branchData[key]?.headUpdatedAt ?? 0;
      if (revision === 0) continue;
      next[key] = revision;
      if (
        branch.cursors.length > 1 &&
        (branchData[key]?.headFetching ||
          (previous[key] !== undefined && previous[key] !== revision))
      ) {
        trim.add(key);
      }
    }
    headRevisionRef.current = next;
    if (trim.size === 0) return;
    setBranchState((state) => {
      if (state.identity !== identity) return state;
      let branches: Map<string, DataViewBranch> | null = null;
      for (const [key, branch] of state.branches) {
        if (!trim.has(key)) continue;
        branches ??= new Map(state.branches);
        branches.set(key, { ...branch, cursors: [null] });
      }
      return branches ? { ...state, branches } : state;
    });
  }, [activeBranches, branchData, identity]);
  const loadNextPage = useCallback(
    (branchKey: string, cursor: string) => {
      setBranchState((previous) => {
        if (previous.identity !== identity) return previous;
        const branch = previous.branches.get(branchKey);
        if (!branch || branch.cursors.includes(cursor)) return previous;
        const branches = new Map(previous.branches);
        branches.set(branchKey, {
          ...branch,
          cursors: [...branch.cursors, cursor],
        });
        return { ...previous, branches };
      });
    },
    [identity],
  );
  const retryBranch = useCallback(
    (branchKey: string) => {
      const branch = activeBranches.get(branchKey);
      if (!branch) return;
      void queryClient.refetchQueries({
        queryKey: binding.rowBranchKey({
          query,
          groupBy,
          branch: {
            groupKey: branch.groupKey,
            parentRowId: branch.parentRowId,
          },
          hierarchy,
        }),
        exact: false,
        type: "active",
      });
    },
    [activeBranches, binding, groupBy, hierarchy, query, queryClient],
  );
  const displayRows = useMemo(
    () =>
      buildDataViewRows({
        usesGrouping,
        groups,
        groupsPending: groupQuery.isPending,
        groupsError: groupQuery.isError,
        hasNextGroupPage: groupQuery.hasNextPage,
        fetchingNextGroupPage: groupQuery.isFetchingNextPage,
        refetchGroups: () => void groupQuery.refetch(),
        fetchNextGroupPage: () => void groupQuery.fetchNextPage(),
        collapsedGroupKeys,
        collapsedRowIds,
        hierarchy,
        branches: activeBranches,
        branchPages: branchData,
        rowId,
        directChildCount,
        projectRow,
        activateBranch,
        loadNextPage,
        retryBranch,
        skeletonCount,
      }),
    [
      activateBranch,
      activeBranches,
      branchData,
      collapsedGroupKeys,
      collapsedRowIds,
      directChildCount,
      groupQuery,
      groups,
      hierarchy,
      loadNextPage,
      projectRow,
      retryBranch,
      rowId,
      skeletonCount,
      usesGrouping,
    ],
  );
  const authoritativeRows = useMemo(() => {
    const byId = new Map<string, Row>();
    for (const branch of Object.values(branchData)) {
      if (branch.placeholder) continue;
      for (const row of branch.rows) byId.set(rowId(row), row);
    }
    return [...byId.values()];
  }, [branchData, rowId]);
  const authoritativeTotal =
    branchData[dataViewBranchKey(null, null)]?.total ?? authoritativeRows.length;

  return {
    rows: displayRows,
    authoritativeRows,
    authoritativeTotal,
    groupError: groupQuery.error,
  };
}
