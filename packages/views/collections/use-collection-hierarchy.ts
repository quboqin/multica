"use client";

import { useState } from "react";
import { useQueries } from "@tanstack/react-query";
import {
  collectionRecordPageOptions,
  flattenCollectionTree,
  type CollectionQuery,
  type CollectionRecord,
} from "@multica/core/collections";

interface Expansion {
  scope: string;
  expanded: Set<string>;
  cursors: Record<string, (string | null)[]>;
}

/** Each expanded branch has independent pages; collapsing never loses edits. */
export function useCollectionHierarchy(
  wsId: string,
  collectionId: string,
  query: CollectionQuery,
  roots: CollectionRecord[],
  enabled: boolean,
) {
  const scope = JSON.stringify([wsId, collectionId, query]);
  const [state, setState] = useState<Expansion>({
    scope, expanded: new Set(), cursors: {},
  });
  const empty: Expansion = { scope, expanded: new Set(), cursors: {} };
  const current = state.scope === scope ? state : empty;
  const requests = enabled
    ? [...current.expanded].flatMap((parentId) =>
        (current.cursors[parentId] ?? [null]).map((cursor) => ({ parentId, cursor })),
      )
    : [];
  const pages = useQueries({
    queries: requests.map(({ parentId, cursor }) =>
      collectionRecordPageOptions(wsId, collectionId, { ...query, parent_id: parentId }, cursor),
    ),
  });
  const children = new Map<string, CollectionRecord[]>();
  requests.forEach(({ parentId }, index) => {
    const records = children.get(parentId) ?? [];
    records.push(...(pages[index]?.data?.records ?? []));
    children.set(parentId, records);
  });
  const toggle = (parentId: string, open?: boolean) =>
    setState((previous) => {
      const base = previous.scope === scope ? previous : empty;
      const expanded = new Set(base.expanded);
      if (open ?? !expanded.has(parentId)) expanded.add(parentId);
      else expanded.delete(parentId);
      return { ...base, expanded };
    });
  const branch = (parentId: string) => {
    const indices = requests.flatMap((request, index) => request.parentId === parentId ? [index] : []);
    const last = pages[indices.at(-1) ?? -1];
    const error = indices.map((index) => pages[index]?.error).find(Boolean);
    return {
      expanded: enabled && current.expanded.has(parentId),
      loading: indices.some((index) => pages[index]?.isFetching),
      error,
      hasMore: !!last?.data?.next_cursor,
      retry: () => indices.forEach((index) => {
        if (pages[index]?.error) void pages[index]!.refetch();
      }),
      loadMore: () => {
        const cursor = last?.data?.next_cursor;
        if (cursor) setState((previous) => ({
          ...previous,
          cursors: {
            ...previous.cursors,
            [parentId]: [...new Set([...(previous.cursors[parentId] ?? [null]), cursor])],
          },
        }));
      },
    };
  };
  return {
    ...(enabled ? flattenCollectionTree(roots, current.expanded, children) : { records: roots, depths: new Map<string, number>() }),
    branch, toggle,
  };
}
