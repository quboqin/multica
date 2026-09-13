/** @vitest-environment jsdom */

import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import {
  dataSourceGroupQueryKey,
  dataSourceIdentityKey,
  dataSourceRowQueryKey,
  type DataSourceGroupPage,
  type DataSourceIdentity,
} from "@multica/core/data-source";
import { useDataViewController } from "./controller";
import type { DataViewQueryBinding, DataViewRowPage } from "./query-binding";

type Row = { id: string; children: number };
type Query = { search: string };
type RawPage = DataViewRowPage<Row>;

function binding(
  identity: DataSourceIdentity,
  options: { failChildOnce?: boolean } = {},
): DataViewQueryBinding<Row, Query, RawPage, DataSourceGroupPage> {
  let childAttempts = 0;
  return {
    identity,
    rowPageKey: ({ query, groupBy, branch, hierarchy, page }) =>
      dataSourceRowQueryKey({
        identity,
        query,
        groupBy,
        branch,
        hierarchy,
        page,
      }),
    rowBranchKey: ({ query, groupBy, branch, hierarchy }) => [
      ...dataSourceIdentityKey(identity),
      "rows",
      query,
      groupBy,
      branch,
      hierarchy,
    ],
    readRowPage: async ({ branch }) => {
      if (branch.parentRowId === "root") {
        childAttempts += 1;
        if (options.failChildOnce && childAttempts === 1) {
          throw new Error("child failed");
        }
        return {
          rows: [{ id: `${identity.sourceId}-child`, children: 0 }],
          total: 1,
          branchTotal: 1,
          nextCursor: null,
        };
      }
      return {
        rows: [{ id: "root", children: 1 }],
        total: 2,
        branchTotal: 1,
        nextCursor: null,
      };
    },
    mapRowPage: (page) => page,
    groupPagesKey: ({ query, groupBy }) =>
      dataSourceGroupQueryKey({
        identity,
        query,
        groupBy,
        page: { cursor: null },
      }),
    readGroupPage: async () => ({ groups: [], total: 0, nextCursor: null }),
    mapGroupPage: (page) => page,
  };
}

const EMPTY = new Set<string>();
const QUERY = { search: "" };

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useDataViewController branch identity", () => {
  it("keeps the literal root child branch independent, retries it, and clears it on source switch", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const sourceA = binding(
      { workspaceId: "ws", namespace: "sample", sourceId: "a::b" },
      { failChildOnce: true },
    );
    const sourceB = binding({
      workspaceId: "ws",
      namespace: "sample",
      sourceId: "a",
    });
    const { result, rerender } = renderHook(
      ({ currentBinding }) =>
        useDataViewController({
          binding: currentBinding,
          query: QUERY,
          groupBy: null,
          hierarchy: true,
          collapsedGroupKeys: EMPTY,
          collapsedRowIds: EMPTY,
          rowId: (row) => row.id,
          directChildCount: (row) => row.children,
          projectRow: ({ row }) => ({ kind: "row" as const, key: row.id }),
          skeletonCount: 1,
        }),
      {
        initialProps: { currentBinding: sourceA },
        wrapper: wrapper(client),
      },
    );

    await waitFor(() =>
      expect(result.current.rows.some((row) => row.key === "root")).toBe(true),
    );
    const activateChild = result.current.rows.find(
      (row) =>
        row.kind === "load_more" &&
        row.state === "has_more" &&
        row.key.startsWith("activate:"),
    );
    act(() => {
      if (activateChild?.kind === "load_more") activateChild.onLoad?.();
    });
    await waitFor(() =>
      expect(
        result.current.rows.some(
          (row) => row.kind === "load_more" && row.state === "error",
        ),
      ).toBe(true),
    );
    const retry = result.current.rows.find(
      (row) => row.kind === "load_more" && row.state === "error",
    );
    act(() => {
      if (retry?.kind === "load_more") retry.onLoad?.();
    });
    await waitFor(() =>
      expect(result.current.rows.some((row) => row.key === "a::b-child")).toBe(
        true,
      ),
    );

    rerender({ currentBinding: sourceB });
    await waitFor(() =>
      expect(
        result.current.rows.some((row) => row.key === "a::b-child") === false &&
          result.current.rows.some((row) => row.key === "root"),
      ).toBe(true),
    );
    client.clear();
  });
});
