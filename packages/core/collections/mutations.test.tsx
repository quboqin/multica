/** @vitest-environment jsdom */

import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import {
  clearClientSessionData,
  revokeClientWorkspaceAccess,
  setCurrentWorkspace,
} from "../platform";
import type { CollectionRecord, Workspace } from "../types";
import { workspaceKeys } from "../workspace/queries";
import {
  useCreateCollectionRecord,
  useUpdateCollectionRecord,
} from "./mutations";
import {
  advanceClientCollectionSourceGeneration,
  collectionKeys,
} from "./queries";

function wrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function workspace(id: string, slug: string): Workspace {
  return { id, slug } as Workspace;
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
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
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [
      workspace("ws-1", "alpha"),
      workspace("ws-2", "beta"),
    ]);
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

  it("does not coordinate a late result after its collection source is fenced", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [workspace("ws-1", "alpha")]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
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
        change: { fieldId: "title", op: "set", value: "Late" },
      },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledOnce());

    advanceClientCollectionSourceGeneration(
      queryClient,
      "ws-1",
      "collection-1",
    );
    queryClient.removeQueries({
      queryKey: collectionKeys.source("ws-1", "collection-1"),
    });
    invalidate.mockClear();

    await act(async () => {
      response.resolve({ ...oldRecord, title: "Late", revision: 2 });
      await completion;
    });

    expect(
      queryClient.getQueryData(
        collectionKeys.record("ws-1", "collection-1", "record-1"),
      ),
    ).toBeUndefined();
    expect(invalidate).not.toHaveBeenCalled();
    hook.unmount();
    queryClient.clear();
  });

  it("does not repopulate or invalidate after logout clears the session", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [workspace("ws-1", "alpha")]);
    queryClient.setQueryData(
      collectionKeys.record("ws-1", "collection-1", "record-1"),
      oldRecord,
    );
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const response = deferred<CollectionRecord>();
    setApiInstance({
      updateCollectionRecord: vi.fn(() => response.promise),
    } as unknown as ApiClient);
    const hook = renderHook(() => useUpdateCollectionRecord(), {
      wrapper: wrapper(queryClient),
    });

    const completion = hook.result.current.mutateAsync({
      collectionId: "collection-1",
      recordId: "record-1",
      input: {
        expectedRevision: 1,
        change: { fieldId: "title", op: "set", value: "Late" },
      },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(hook.result.current.isPending).toBe(true));
    clearClientSessionData(queryClient, {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
      keys: () => [],
    });
    invalidate.mockClear();

    await act(async () => {
      response.resolve({ ...oldRecord, title: "Late", revision: 2 });
      await completion;
    });

    expect(
      queryClient.getQueryData(
        collectionKeys.record("ws-1", "collection-1", "record-1"),
      ),
    ).toBeUndefined();
    expect(invalidate).not.toHaveBeenCalled();
    hook.unmount();
  });

  it("does not recreate a record cache when create completes after account cleanup", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [workspace("ws-1", "alpha")]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const response = deferred<{ record: CollectionRecord; replayed: boolean }>();
    setApiInstance({
      createCollectionRecord: vi.fn(() => response.promise),
    } as unknown as ApiClient);
    const hook = renderHook(() => useCreateCollectionRecord(), {
      wrapper: wrapper(queryClient),
    });

    const completion = hook.result.current.mutateAsync({
      collectionId: "collection-1",
      input: { clientRequestId: "request-1", title: "Late", fields: {} },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(hook.result.current.isPending).toBe(true));
    clearClientSessionData(queryClient, {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
      keys: () => [],
    });
    invalidate.mockClear();
    await act(async () => {
      response.resolve({
        record: { ...oldRecord, title: "Late", revision: 1 },
        replayed: false,
      });
      await completion;
    });

    expect(
      queryClient.getQueryData(
        collectionKeys.record("ws-1", "collection-1", "record-1"),
      ),
    ).toBeUndefined();
    expect(invalidate).not.toHaveBeenCalled();
    hook.unmount();
  });

  it("does not invalidate a revoked workspace when a late request fails", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [workspace("ws-1", "alpha")]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const response = deferred<CollectionRecord>();
    setApiInstance({
      updateCollectionRecord: vi.fn(() => response.promise),
    } as unknown as ApiClient);
    const hook = renderHook(() => useUpdateCollectionRecord(), {
      wrapper: wrapper(queryClient),
    });
    const completion = hook.result.current.mutateAsync({
      collectionId: "collection-1",
      recordId: "record-1",
      input: {
        expectedRevision: 1,
        change: { fieldId: "title", op: "set", value: "Late" },
      },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(hook.result.current.isPending).toBe(true));
    // The production realtime responder advances this generation before its
    // asynchronous workspace-list refresh settles. Keep the stale membership
    // row here to cover that exact window.
    revokeClientWorkspaceAccess(queryClient, "ws-1");
    invalidate.mockClear();

    await act(async () => {
      response.reject(new Error("late failure"));
      await expect(completion).rejects.toThrow("late failure");
    });
    expect(invalidate).not.toHaveBeenCalled();
    hook.unmount();
    queryClient.clear();
  });

  it("does not repopulate or issue new writes after realtime revocation", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [workspace("ws-1", "alpha")]);
    queryClient.setQueryData(
      collectionKeys.record("ws-1", "collection-1", "record-1"),
      oldRecord,
    );
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
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
        change: { fieldId: "title", op: "set", value: "Late" },
      },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(hook.result.current.isPending).toBe(true));
    revokeClientWorkspaceAccess(queryClient, "ws-1");
    queryClient.removeQueries({
      queryKey: collectionKeys.record(
        "ws-1",
        "collection-1",
        "record-1",
      ),
    });
    invalidate.mockClear();

    await act(async () => {
      response.resolve({ ...oldRecord, title: "Late", revision: 2 });
      await completion;
    });

    expect(
      queryClient.getQueryData(
        collectionKeys.record("ws-1", "collection-1", "record-1"),
      ),
    ).toBeUndefined();
    expect(invalidate).not.toHaveBeenCalled();

    await act(async () => {
      await expect(
        hook.result.current.mutateAsync({
          collectionId: "collection-1",
          recordId: "record-1",
          input: {
            expectedRevision: 2,
            change: { fieldId: "title", op: "set", value: "Rejected" },
          },
          workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
        }),
      ).rejects.toThrow("Workspace access was revoked");
    });
    expect(updateCollectionRecord).toHaveBeenCalledOnce();
    expect(invalidate).not.toHaveBeenCalled();
    hook.unmount();
    queryClient.clear();
  });

  it("keeps the newest record when an idempotent create replay arrives late", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const queryClient = makeQueryClient();
    queryClient.setQueryData(workspaceKeys.list(), [workspace("ws-1", "alpha")]);
    const newest = { ...oldRecord, title: "Newest", revision: 4 };
    queryClient.setQueryData(
      collectionKeys.record("ws-1", "collection-1", "record-1"),
      newest,
    );
    const response = deferred<{ record: CollectionRecord; replayed: boolean }>();
    setApiInstance({
      createCollectionRecord: vi.fn(() => response.promise),
    } as unknown as ApiClient);
    const hook = renderHook(() => useCreateCollectionRecord(), {
      wrapper: wrapper(queryClient),
    });

    const completion = hook.result.current.mutateAsync({
      collectionId: "collection-1",
      input: {
        clientRequestId: "request-1",
        title: "Older replay",
        fields: {},
      },
      workspaceContext: { workspaceId: "ws-1", workspaceSlug: "alpha" },
    });
    await waitFor(() => expect(hook.result.current.isPending).toBe(true));
    await act(async () => {
      response.resolve({
        record: { ...oldRecord, title: "Older replay", revision: 2 },
        replayed: true,
      });
      await completion;
    });

    expect(
      queryClient.getQueryData<CollectionRecord>(
        collectionKeys.record("ws-1", "collection-1", "record-1"),
      ),
    ).toEqual(newest);
    hook.unmount();
    queryClient.clear();
  });
});
