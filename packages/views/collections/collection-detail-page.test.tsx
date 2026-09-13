/** @vitest-environment jsdom */

import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { collectionKeys } from "@multica/core/collections";
import { setCurrentWorkspace } from "@multica/core/platform";
import type { CollectionDetail, CollectionRecord } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { CollectionDetailPage } from "./collection-detail-page";

vi.mock("@multica/core", async () => ({
  ...(await vi.importActual<typeof import("@multica/core")>("@multica/core")),
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/paths", async () => ({
  ...(await vi.importActual<typeof import("@multica/core/paths")>(
    "@multica/core/paths",
  )),
  useRequiredWorkspaceSlug: () => "alpha",
}));

vi.mock("@multica/core/config", async () => ({
  ...(await vi.importActual<typeof import("@multica/core/config")>(
    "@multica/core/config",
  )),
  useFeatureEnabled: () => true,
}));

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 41,
        end: (index + 1) * 41,
        size: 41,
      })),
    getTotalSize: () => count * 41,
    measureElement: () => {},
  }),
}));

const detail: CollectionDetail = {
  collection: {
    id: "collection-1",
    workspaceId: "ws-1",
    name: "Orders",
    revision: 1,
    archivedAt: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
  fields: [
    {
      id: "field-note",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      name: "Note",
      type: "text",
      position: 0,
      revision: 1,
    },
  ],
  capabilities: {
    layouts: ["table"],
    grouping: false,
    hierarchy: false,
    writable: true,
    maxPageSize: 200,
  },
};

function authorizeWorkspace(queryClient: QueryClient) {
  queryClient.setQueryData(["workspaces", "list"], [
    { id: "ws-1", slug: "alpha" },
  ]);
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

describe("CollectionDetailPage", () => {
  beforeEach(() => setCurrentWorkspace("alpha", "ws-1"));
  afterEach(() => {
    cleanup();
    setCurrentWorkspace(null, null);
    vi.restoreAllMocks();
  });

  it("mounts the production shared table and persists a title edit", async () => {
    let serverRecord: CollectionRecord = {
      id: "record-1",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      title: "Original",
      fields: { "field-note": "Read only note" },
      position: 0,
      revision: 1,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    const getCollection = vi.fn(async () => detail);
    const queryCollectionRecords = vi.fn(async () => ({
      records: [serverRecord],
      total: 1,
      nextCursor: null,
    }));
    const updateCollectionRecord = vi.fn(async () => {
      serverRecord = { ...serverRecord, title: "Changed", revision: 2 };
      return serverRecord;
    });
    setApiInstance({
      getCollection,
      queryCollectionRecords,
      updateCollectionRecord,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(queryClient);

    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-1" />
      </QueryClientProvider>,
    );

    const title = await screen.findByRole("textbox", { name: "Title" });
    expect(title).toHaveValue("Original");
    expect(screen.getByText("Read only note")).toBeInTheDocument();
    expect(document.querySelector("[data-source-identity]")).not.toBeNull();

    fireEvent.change(title, { target: { value: "Changed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledOnce());
    expect(updateCollectionRecord).toHaveBeenCalledWith(
      "collection-1",
      "record-1",
      {
        expectedRevision: 1,
        change: { fieldId: "title", op: "set", value: "Changed" },
      },
      "alpha",
    );
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
        "Changed",
      ),
    );
    expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(getCollection).toHaveBeenCalledWith(
      "collection-1",
      "alpha",
      expect.any(AbortSignal),
    );
  });

  it("keeps the title draft when the final CAS result fails", async () => {
    const serverRecord: CollectionRecord = {
      id: "record-1",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      title: "Original",
      fields: { "field-note": "Read only note" },
      position: 0,
      revision: 2,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    const queryCollectionRecords = vi.fn(async () => ({
      records: [serverRecord],
      total: 1,
      nextCursor: null,
    }));
    const response = deferred<CollectionRecord>();
    const updateCollectionRecord = vi.fn(() => response.promise);
    setApiInstance({
      getCollection: vi.fn(async () => detail),
      queryCollectionRecords,
      updateCollectionRecord,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(queryClient);
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-1" />
      </QueryClientProvider>,
    );

    const title = await screen.findByRole("textbox", { name: "Title" });
    fireEvent.change(title, { target: { value: "My draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledOnce());
    fireEvent.change(screen.getByRole("textbox", { name: "Record title" }), {
      target: { value: "Parent redraw during conflict" },
    });
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
      "My draft",
    );
    await act(async () => {
      response.reject(new Error("revision conflict"));
      await expect(response.promise).rejects.toThrow("revision conflict");
    });

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "revision conflict",
    );
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
      "My draft",
    );
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(2),
    );
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
      "My draft",
    );
  });

  it("preserves the active draft and pending state across parent redraw and refetch", async () => {
    let serverRecords: CollectionRecord[] = [
      {
        id: "record-1",
        workspaceId: "ws-1",
        collectionId: "collection-1",
        title: "First",
        fields: { "field-note": "First note" },
        position: 0,
        revision: 1,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      },
      {
        id: "record-2",
        workspaceId: "ws-1",
        collectionId: "collection-1",
        title: "Second",
        fields: { "field-note": "Second note" },
        position: 1,
        revision: 1,
        createdAt: "2026-01-01T00:00:01Z",
        updatedAt: "2026-01-01T00:00:01Z",
      },
    ];
    const response = deferred<CollectionRecord>();
    const queryCollectionRecords = vi.fn(async () => ({
      records: serverRecords,
      total: 240,
      nextCursor: null,
    }));
    const updateCollectionRecord = vi.fn(() => response.promise);
    setApiInstance({
      getCollection: vi.fn(async () => detail),
      queryCollectionRecords,
      updateCollectionRecord,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(queryClient);
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-1" />
      </QueryClientProvider>,
    );

    const titles = await screen.findAllByRole("textbox", { name: "Title" });
    fireEvent.change(titles[1]!, {
      target: { value: "Unsaved draft" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Record title" }), {
      target: { value: "Parent redraw" },
    });
    expect(screen.getAllByRole("textbox", { name: "Title" })[1]).toHaveValue(
      "Unsaved draft",
    );
    expect(screen.getByText("240")).toBeInTheDocument();

    serverRecords = serverRecords.map((record) =>
      record.id === "record-2"
        ? { ...record, title: "Refetched", revision: 2 }
        : record,
    );
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: collectionKeys.rows("ws-1", "collection-1"),
      });
    });
    expect(screen.getAllByRole("textbox", { name: "Title" })[1]).toHaveValue(
      "Unsaved draft",
    );

    fireEvent.change(screen.getAllByRole("textbox", { name: "Title" })[0]!, {
      target: { value: "Saved first" },
    });
    fireEvent.click(screen.getAllByRole("button", { name: "Save" })[0]!);
    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledOnce());
    expect(screen.getAllByRole("textbox", { name: "Title" })[1]).toHaveValue(
      "Unsaved draft",
    );

    await act(async () => {
      const saved = { ...serverRecords[0]!, title: "Saved first", revision: 2 };
      serverRecords = [saved, serverRecords[1]!];
      response.resolve(saved);
      await response.promise;
    });
    await waitFor(() =>
      expect(screen.getAllByRole("textbox", { name: "Title" })[0]).toHaveValue(
        "Saved first",
      ),
    );
    await waitFor(() =>
      expect(screen.getAllByRole("textbox", { name: "Title" })[1]).toHaveValue(
        "Unsaved draft",
      ),
    );
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(3),
    );
    expect(screen.getAllByRole("textbox", { name: "Title" })[1]).toHaveValue(
      "Unsaved draft",
    );
  });
});
