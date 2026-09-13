/** @vitest-environment jsdom */

import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { setCurrentWorkspace } from "../platform";
import type { CollectionRecord } from "../types";
import { useUpdateCollectionRecord } from "./mutations";
import { collectionKeys } from "./queries";

function wrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const oldRecord: CollectionRecord = {
  id: "record-1",
  workspaceId: "ws-1",
  collectionId: "collection-1",
  title: "Original",
  fields: {},
  position: 0,
  revision: 1,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

describe("useUpdateCollectionRecord", () => {
  afterEach(() => {
    setCurrentWorkspace(null, null);
    vi.restoreAllMocks();
  });

  it("routes a late result only to the captured workspace cache", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const foreign = { ...oldRecord, workspaceId: "ws-2", title: "Foreign" };
    queryClient.setQueryData(
      collectionKeys.record("ws-1", "collection-1", "record-1"),
      oldRecord,
    );
    queryClient.setQueryData(
      collectionKeys.record("ws-2", "collection-1", "record-1"),
      foreign,
    );
    const response = deferred<CollectionRecord>();
    const updateCollectionRecord = vi.fn(() => response.promise);
    setApiInstance({ updateCollectionRecord } as unknown as ApiClient);
    const hook = renderHook(() => useUpdateCollectionRecord(), {
      wrapper: wrapper(queryClient),
    });
    const completion = hook.result.current.mutateAsync({
      collectionId: "collection-1",
      recordId: "record-1",
      input: {
        expectedRevision: 1,
        change: { fieldId: "title", op: "set", value: "Changed" },
      },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledOnce());

    setCurrentWorkspace("beta", "ws-2");
    await act(async () => {
      response.resolve({ ...oldRecord, title: "Changed", revision: 2 });
      await completion;
    });

    expect(updateCollectionRecord).toHaveBeenCalledWith(
      "collection-1",
      "record-1",
      {
        expectedRevision: 1,
        change: { fieldId: "title", op: "set", value: "Changed" },
      },
      "alpha",
    );
    expect(
      queryClient.getQueryData<CollectionRecord>(
        collectionKeys.record("ws-1", "collection-1", "record-1"),
      )?.title,
    ).toBe("Changed");
    expect(
      queryClient.getQueryData<CollectionRecord>(
        collectionKeys.record("ws-2", "collection-1", "record-1"),
      ),
    ).toEqual(foreign);
    hook.unmount();
    queryClient.clear();
  });
});
