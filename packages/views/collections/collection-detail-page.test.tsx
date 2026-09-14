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
import {
  isClientWorkspaceAccessAllowed,
  revokeClientWorkspaceAccess,
  setCurrentWorkspace,
} from "@multica/core/platform";
import type { CollectionDetail, CollectionRecord } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { CollectionDetailPage } from "./collection-detail-page";
import {
  CollectionRealtimeHarness,
  createCollectionTestWs,
} from "./realtime-test-harness";

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
    {
      id: "field-amount",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      name: "Amount",
      type: "number",
      position: 1,
      revision: 1,
    },
    {
      id: "field-done",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      name: "Done",
      type: "checkbox",
      position: 2,
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

function detailFor(collectionId: string, name: string): CollectionDetail {
  return {
    ...detail,
    collection: {
      ...detail.collection,
      id: collectionId,
      name,
    },
    fields: detail.fields.map((field) => ({
      ...field,
      collectionId,
    })),
  };
}

function createdRecord(collectionId: string): CollectionRecord {
  return {
    id: `record-${collectionId}`,
    workspaceId: "ws-1",
    collectionId,
    title: "Created",
    fields: {},
    position: 0,
    revision: 1,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
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

describe("CollectionDetailPage", () => {
  beforeEach(() => setCurrentWorkspace("alpha", "ws-1"));
  afterEach(() => {
    cleanup();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
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
    expect(screen.getByRole("textbox", { name: "Note" })).toHaveValue(
      "Read only note",
    );
    expect(document.querySelector("[data-source-identity]")).not.toBeNull();

    fireEvent.change(title, { target: { value: "Changed" } });
    fireEvent.click(title.closest("form")!.querySelector('button[type="submit"]')!);

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

  it("persists and reloads text, number, checkbox, and clear changes", async () => {
    let serverRecord: CollectionRecord = {
      id: "record-1",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      title: "Original",
      fields: {
        "field-note": "note",
        "field-amount": 7,
        "field-done": true,
      },
      position: 0,
      revision: 1,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    const queryCollectionRecords = vi.fn(async () => ({
      records: [serverRecord],
      total: 1,
      nextCursor: null,
    }));
    const updateCollectionRecord = vi.fn(
      async (
        _collectionId: string,
        _recordId: string,
        input: {
          expectedRevision: number;
          change:
            | { fieldId: string; op: "set"; value: unknown }
            | { fieldId: string; op: "clear" };
        },
      ) => {
        expect(input.expectedRevision).toBe(serverRecord.revision);
        const fields = { ...serverRecord.fields };
        if (input.change.op === "clear") delete fields[input.change.fieldId];
        else fields[input.change.fieldId] = input.change.value;
        serverRecord = {
          ...serverRecord,
          fields,
          revision: serverRecord.revision + 1,
        };
        return serverRecord;
      },
    );
    setApiInstance({
      getCollection: vi.fn(async () => detail),
      queryCollectionRecords,
      updateCollectionRecord,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(queryClient);
    const view = renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-1" />
      </QueryClientProvider>,
    );

    const submitField = async (name: string, value: string, revision: number) => {
      const input = await screen.findByRole("textbox", { name });
      fireEvent.change(input, { target: { value } });
      fireEvent.click(input.closest("form")!.querySelector('button[type="submit"]')!);
      await waitFor(() => expect(serverRecord.revision).toBe(revision));
      await waitFor(() => expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(revision));
    };

    await submitField("Note", "", 2);
    const amount = screen.getByRole("spinbutton", { name: "Amount" });
    fireEvent.change(amount, { target: { value: "0" } });
    fireEvent.click(amount.closest("form")!.querySelector('button[type="submit"]')!);
    await waitFor(() => expect(serverRecord.revision).toBe(3));
    await waitFor(() => expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(3));
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "Done" })).toBeEnabled());
    fireEvent.click(screen.getByRole("checkbox", { name: "Done" }));
    await waitFor(() => expect(serverRecord.revision).toBe(4));
    await waitFor(() => expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(4));

    const note = screen.getByRole("textbox", { name: "Note" });
    fireEvent.click(note.closest("form")!.querySelector('button[type="button"]')!);
    await waitFor(() => expect(serverRecord.revision).toBe(5));
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(5),
    );
    const currentAmount = screen.getByRole("spinbutton", { name: "Amount" });
    fireEvent.click(
      currentAmount.closest("form")!.querySelector('button[type="button"]')!,
    );
    await waitFor(() => expect(serverRecord.revision).toBe(6));
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(6),
    );
    const done = screen.getByRole("checkbox", { name: "Done" });
    fireEvent.click(done.closest("div")!.querySelector("button")!);
    await waitFor(() => expect(serverRecord.revision).toBe(7));
    expect(serverRecord.fields).toEqual({});
    expect(updateCollectionRecord.mock.calls.map((call) => call[2])).toEqual([
      { expectedRevision: 1, change: { fieldId: "field-note", op: "set", value: "" } },
      { expectedRevision: 2, change: { fieldId: "field-amount", op: "set", value: 0 } },
      { expectedRevision: 3, change: { fieldId: "field-done", op: "set", value: false } },
      { expectedRevision: 4, change: { fieldId: "field-note", op: "clear" } },
      { expectedRevision: 5, change: { fieldId: "field-amount", op: "clear" } },
      { expectedRevision: 6, change: { fieldId: "field-done", op: "clear" } },
    ]);

    view.unmount();
    const remountClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(remountClient);
    renderWithI18n(
      <QueryClientProvider client={remountClient}>
        <CollectionDetailPage collectionId="collection-1" />
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("spinbutton", { name: "Amount" })).toHaveValue(null);
    expect(screen.getByRole("checkbox", { name: "Done" })).not.toBeChecked();
    expect(screen.getByRole("textbox", { name: "Note" })).toHaveValue("");
  });

  it("polls every 30 seconds only while visible and refreshes on visibility recovery", async () => {
    const serverRecord = createdRecord("collection-1");
    const queryCollectionRecords = vi.fn(async () => ({
      records: [serverRecord],
      total: 1,
      nextCursor: null,
    }));
    setApiInstance({
      getCollection: vi.fn(async () => detail),
      queryCollectionRecords,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(queryClient);
    const intervalSpy = vi.spyOn(window, "setInterval");
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-1" />
      </QueryClientProvider>,
    );
    await screen.findByRole("textbox", { name: "Title" });
    expect(intervalSpy).toHaveBeenCalledWith(expect.any(Function), 30_000);
    const poll = intervalSpy.mock.calls.find((call) => call[1] === 30_000)?.[0];
    expect(typeof poll).toBe("function");
    const readsBeforePoll = queryCollectionRecords.mock.calls.length;
    await act(async () => {
      (poll as () => void)();
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThan(
        readsBeforePoll,
      ),
    );

    invalidateSpy.mockClear();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    window.dispatchEvent(new Event("focus"));
    expect(invalidateSpy).not.toHaveBeenCalledWith({
      queryKey: collectionKeys.source("ws-1", "collection-1"),
    });

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: collectionKeys.source("ws-1", "collection-1"),
    });

    invalidateSpy.mockClear();
    revokeClientWorkspaceAccess(queryClient, "ws-1");
    window.dispatchEvent(new Event("focus"));
    expect(invalidateSpy).not.toHaveBeenCalledWith({
      queryKey: collectionKeys.source("ws-1", "collection-1"),
    });
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
    fireEvent.click(title.closest("form")!.querySelector('button[type="submit"]')!);
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

  it("freezes a dirty record from the tail page across source refresh and failure", async () => {
    const pagedDetail: CollectionDetail = {
      ...detail,
      capabilities: { ...detail.capabilities, maxPageSize: 1 },
    };
    const first = createdRecord("collection-1");
    const second: CollectionRecord = {
      ...createdRecord("collection-1"),
      id: "record-201",
      title: "Tail record",
      fields: { "field-note": "original second" },
      position: 200,
    };
    const queryCollectionRecords = vi.fn(
      async (
        _collectionId: string,
        page: { cursor?: string | null },
      ) => ({
        records: page.cursor === "tail" ? [second] : [first],
        total: 201,
        nextCursor: page.cursor === "tail" ? null : "tail",
      }),
    );
    const updateCollectionRecord = vi.fn(
      async (_collectionId: string, _recordId: string, _input: unknown) => {
        throw new Error("revision conflict");
      },
    );
    setApiInstance({
      getCollection: vi.fn(async () => pagedDetail),
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

    fireEvent.click(await screen.findByRole("button", { name: "Load more" }));
    await waitFor(() =>
      expect(screen.getAllByRole("textbox", { name: "Note" })).toHaveLength(2),
    );
    const notes = screen.getAllByRole("textbox", { name: "Note" });
    fireEvent.change(notes[1]!, {
      target: { value: "unsaved second page" },
    });

    const readsBeforeRefresh = queryCollectionRecords.mock.calls.length;
    window.dispatchEvent(new Event("focus"));
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThan(
        readsBeforeRefresh,
      ),
    );
    expect(screen.getAllByRole("textbox", { name: "Note" })[1]).toHaveValue(
      "unsaved second page",
    );

    const tailEditor = screen.getAllByRole("textbox", { name: "Note" })[1]!;
    fireEvent.submit(tailEditor.closest("form")!);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "revision conflict",
    );
    expect(screen.getAllByRole("textbox", { name: "Note" })[1]).toHaveValue(
      "unsaved second page",
    );
    expect(updateCollectionRecord).toHaveBeenCalledOnce();
    expect(updateCollectionRecord.mock.calls[0]?.[2]).toMatchObject({
      expectedRevision: second.revision,
    });
  });

  it("submits the edited revision before rebasing an explicit conflict retry", async () => {
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
    const queryCollectionRecords = vi.fn(async () => ({
      records: [serverRecord],
      total: 1,
      nextCursor: null,
    }));
    const updateCollectionRecord = vi.fn(
      async (
        _collectionId: string,
        _recordId: string,
        input: { expectedRevision: number; change: { value: string } },
      ) => {
        if (input.expectedRevision === 1) {
          throw new Error("revision conflict");
        }
        serverRecord = {
          ...serverRecord,
          title: input.change.value,
          revision: 3,
        };
        return serverRecord;
      },
    );
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

    fireEvent.change(await screen.findByRole("textbox", { name: "Title" }), {
      target: { value: "My draft" },
    });
    serverRecord = { ...serverRecord, title: "Remote edit", revision: 2 };
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: collectionKeys.rows("ws-1", "collection-1"),
      });
    });
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
      "My draft",
    );

    fireEvent.click(
      screen
        .getByRole("textbox", { name: "Title" })
        .closest("form")!
        .querySelector('button[type="submit"]')!,
    );
    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledOnce());
    expect(updateCollectionRecord.mock.calls[0]?.[2].expectedRevision).toBe(1);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "revision conflict",
    );
    await waitFor(() =>
      expect(queryCollectionRecords.mock.calls.length).toBeGreaterThanOrEqual(3),
    );
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
      "My draft",
    );

    fireEvent.click(
      screen
        .getByRole("textbox", { name: "Title" })
        .closest("form")!
        .querySelector('button[type="submit"]')!,
    );
    await waitFor(() => expect(updateCollectionRecord).toHaveBeenCalledTimes(2));
    expect(updateCollectionRecord.mock.calls[1]?.[2].expectedRevision).toBe(2);
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue(
        "My draft",
      ),
    );
  });

  it.each(["success", "failure"] as const)(
    "fences late record create %s after the production revocation event",
    async (outcome) => {
      const response = deferred<{
        record: CollectionRecord;
        replayed: boolean;
      }>();
      const createCollectionRecord = vi.fn(() => response.promise);
      const queryCollectionRecords = vi.fn(async () => ({
        records: [],
        total: 0,
        nextCursor: null,
      }));
      setApiInstance({
        getCollection: vi.fn(async () => detail),
        queryCollectionRecords,
        createCollectionRecord,
      } as unknown as ApiClient);
      const queryClient = new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      });
      authorizeWorkspace(queryClient);
      const invalidate = vi.spyOn(queryClient, "invalidateQueries");
      const realtime = createCollectionTestWs();
      renderWithI18n(
        <QueryClientProvider client={queryClient}>
          <CollectionRealtimeHarness ws={realtime.ws} />
          <CollectionDetailPage collectionId="collection-1" />
        </QueryClientProvider>,
      );

      const input = await screen.findByRole("textbox", {
        name: "Record title",
      });
      fireEvent.change(input, { target: { value: "Private record" } });
      fireEvent.click(screen.getByRole("button", { name: "Add record" }));
      await waitFor(() => expect(createCollectionRecord).toHaveBeenCalledOnce());
      const readsBeforeRevoke = queryCollectionRecords.mock.calls.length;
      vi.spyOn(queryClient, "fetchQuery").mockReturnValue(new Promise(() => {}));
      invalidate.mockClear();

      act(() => {
        realtime.emit("member:removed", {
          member_id: "member-1",
          user_id: "u1",
          workspace_id: "ws-1",
        });
      });
      expect(isClientWorkspaceAccessAllowed(queryClient, "ws-1")).toBe(false);

      await act(async () => {
        if (outcome === "success") {
          response.resolve({
            record: createdRecord("collection-1"),
            replayed: false,
          });
        } else {
          response.reject(new Error("late failure"));
        }
        await response.promise.catch(() => undefined);
        await Promise.resolve();
      });

      await waitFor(() =>
        expect(
          screen.getByText("Workspace access was revoked"),
        ).toBeInTheDocument(),
      );
      expect(screen.queryByText("late failure")).not.toBeInTheDocument();
      expect(queryCollectionRecords).toHaveBeenCalledTimes(readsBeforeRevoke);
      expect(invalidate).not.toHaveBeenCalled();
      expect(createCollectionRecord).toHaveBeenCalledOnce();
    },
  );

  it.each(["success", "failure"] as const)(
    "does not carry an old collection create %s into a new source instance",
    async (outcome) => {
      const response = deferred<{
        record: CollectionRecord;
        replayed: boolean;
      }>();
      const createCollectionRecord = vi.fn(() => response.promise);
      setApiInstance({
        getCollection: vi.fn(async (collectionId: string) =>
          collectionId === "collection-a"
            ? detailFor("collection-a", "Alpha")
            : detailFor("collection-b", "Beta"),
        ),
        queryCollectionRecords: vi.fn(async () => ({
          records: [],
          total: 0,
          nextCursor: null,
        })),
        createCollectionRecord,
      } as unknown as ApiClient);
      const queryClient = new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      });
      authorizeWorkspace(queryClient);
      const view = renderWithI18n(
        <QueryClientProvider client={queryClient}>
          <CollectionDetailPage collectionId="collection-a" />
        </QueryClientProvider>,
      );

      fireEvent.change(
        await screen.findByRole("textbox", { name: "Record title" }),
        { target: { value: "Alpha draft" } },
      );
      fireEvent.click(screen.getByRole("button", { name: "Add record" }));
      await waitFor(() => expect(createCollectionRecord).toHaveBeenCalledOnce());

      view.rerender(
        <QueryClientProvider client={queryClient}>
          <CollectionDetailPage collectionId="collection-b" />
        </QueryClientProvider>,
      );
      expect(await screen.findByText("Beta")).toBeInTheDocument();
      const betaInput = screen.getByRole("textbox", { name: "Record title" });
      expect(betaInput).toHaveValue("");
      expect(betaInput).toBeEnabled();

      await act(async () => {
        if (outcome === "success") {
          response.resolve({
            record: createdRecord("collection-a"),
            replayed: false,
          });
        } else {
          response.reject(new Error("Alpha failed"));
        }
        await response.promise.catch(() => undefined);
        await Promise.resolve();
      });

      expect(betaInput).toHaveValue("");
      expect(betaInput).toBeEnabled();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    },
  );

  it("keeps a fresh A draft when an older A request finishes after A to B to A", async () => {
    const response = deferred<{
      record: CollectionRecord;
      replayed: boolean;
    }>();
    const createCollectionRecord = vi.fn(() => response.promise);
    setApiInstance({
      getCollection: vi.fn(async (collectionId: string) =>
        detailFor(
          collectionId,
          collectionId === "collection-a" ? "Alpha" : "Beta",
        ),
      ),
      queryCollectionRecords: vi.fn(async () => ({
        records: [],
        total: 0,
        nextCursor: null,
      })),
      createCollectionRecord,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    authorizeWorkspace(queryClient);
    const view = renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-a" />
      </QueryClientProvider>,
    );

    fireEvent.change(
      await screen.findByRole("textbox", { name: "Record title" }),
      { target: { value: "Old Alpha draft" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Add record" }));
    await waitFor(() => expect(createCollectionRecord).toHaveBeenCalledOnce());

    view.rerender(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-b" />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("Beta")).toBeInTheDocument();
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <CollectionDetailPage collectionId="collection-a" />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("Alpha")).toBeInTheDocument();
    const freshInput = screen.getByRole("textbox", { name: "Record title" });
    fireEvent.change(freshInput, { target: { value: "Fresh Alpha draft" } });

    await act(async () => {
      response.reject(new Error("Old Alpha failed"));
      await response.promise.catch(() => undefined);
      await Promise.resolve();
    });

    expect(freshInput).toHaveValue("Fresh Alpha draft");
    expect(freshInput).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
