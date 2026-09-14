/** @vitest-environment jsdom */

import type { PropsWithChildren } from "react";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import {
  isClientWorkspaceAccessAllowed,
  setCurrentWorkspace,
} from "@multica/core/platform";
import { workspaceKeys } from "@multica/core/workspace/queries";
import { renderWithI18n } from "../test/i18n";
import { CollectionsPage } from "./collections-page";
import {
  CollectionRealtimeHarness,
  createCollectionTestWs,
} from "./realtime-test-harness";

const { mockPush } = vi.hoisted(() => ({ mockPush: vi.fn() }));

vi.mock("@multica/core", async () => ({
  ...(await vi.importActual<typeof import("@multica/core")>("@multica/core")),
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/paths", async () => ({
  ...(await vi.importActual<typeof import("@multica/core/paths")>(
    "@multica/core/paths",
  )),
  useRequiredWorkspaceSlug: () => "alpha",
  useWorkspacePaths: () => ({
    collectionDetail: (id: string) => `/alpha/collections/${id}`,
  }),
}));

vi.mock("@multica/core/config", async () => ({
  ...(await vi.importActual<typeof import("@multica/core/config")>(
    "@multica/core/config",
  )),
  useFeatureEnabled: () => true,
}));

vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({ role: "owner" }),
}));

vi.mock("../navigation", async () => ({
  ...(await vi.importActual<typeof import("../navigation")>("../navigation")),
  AppLink: ({ children }: PropsWithChildren) => <>{children}</>,
  useNavigation: () => ({ push: mockPush }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function makeQueryClient() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  queryClient.setQueryData(workspaceKeys.list(), [
    { id: "ws-1", slug: "alpha" },
  ]);
  return queryClient;
}

describe("CollectionsPage", () => {
  beforeEach(() => {
    setCurrentWorkspace("alpha", "ws-1");
    mockPush.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    setCurrentWorkspace(null, null);
    vi.restoreAllMocks();
  });

  it("recovers a missed collection event on the visible 30-second poll", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let items = [
      {
        id: "collection-1",
        workspaceId: "ws-1",
        name: "First collection",
        revision: 1,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      },
    ];
    const listCollections = vi.fn(async () => ({
      collections: items,
      total: items.length,
      nextCursor: null,
    }));
    setApiInstance({ listCollections } as unknown as ApiClient);
    const queryClient = makeQueryClient();
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionsPage />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("First collection")).toBeInTheDocument();
    expect(listCollections).toHaveBeenCalledOnce();
    items = [
      ...items,
      {
        ...items[0]!,
        id: "collection-2",
        name: "Missed collection",
      },
    ];

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(await screen.findByText("Missed collection")).toBeInTheDocument();
    expect(listCollections).toHaveBeenCalledTimes(2);
  });

  it("does not navigate when a create finishes after the page unmounts", async () => {
    const response = deferred<{ collection: { id: string } }>();
    const createCollection = vi.fn(() => response.promise);
    setApiInstance({
      listCollections: vi.fn(async () => ({
        collections: [],
        total: 0,
        nextCursor: null,
      })),
      createCollection,
    } as unknown as ApiClient);
    const queryClient = makeQueryClient();
    const view = renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <CollectionsPage />
      </QueryClientProvider>,
    );

    fireEvent.change(
      await screen.findByRole("textbox", { name: "Collection name" }),
      { target: { value: "Orders" } },
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Create collection" }),
    );
    await waitFor(() => expect(createCollection).toHaveBeenCalledOnce());
    view.unmount();

    await act(async () => {
      response.resolve({ collection: { id: "collection-1" } });
      await response.promise;
      await Promise.resolve();
    });

    expect(mockPush).not.toHaveBeenCalled();
  });

  it.each(["success", "failure"] as const)(
    "fences late collection create %s after the production revocation event",
    async (outcome) => {
      const response = deferred<{ collection: { id: string } }>();
      const createCollection = vi.fn(() => response.promise);
      const listCollections = vi.fn(async () => ({
        collections: [],
        total: 0,
        nextCursor: null,
      }));
      setApiInstance({ listCollections, createCollection } as unknown as ApiClient);
      const queryClient = makeQueryClient();
      const invalidate = vi.spyOn(queryClient, "invalidateQueries");
      const realtime = createCollectionTestWs();
      renderWithI18n(
        <QueryClientProvider client={queryClient}>
          <CollectionRealtimeHarness ws={realtime.ws} />
          <CollectionsPage />
        </QueryClientProvider>,
      );

      const input = await screen.findByRole("textbox", {
        name: "Collection name",
      });
      fireEvent.change(input, { target: { value: "Private" } });
      fireEvent.click(
        screen.getByRole("button", { name: "Create collection" }),
      );
      await waitFor(() => expect(createCollection).toHaveBeenCalledOnce());
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
          response.resolve({ collection: { id: "new-private" } });
        } else {
          response.reject(new Error("late failure"));
        }
        await response.promise.catch(() => undefined);
        await Promise.resolve();
      });

      expect(mockPush).not.toHaveBeenCalled();
      expect(input).toHaveValue("Private");
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      expect(listCollections).toHaveBeenCalledOnce();
      expect(invalidate).not.toHaveBeenCalled();

      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: "Create collection" }),
        ).toBeEnabled(),
      );
      fireEvent.click(
        screen.getByRole("button", { name: "Create collection" }),
      );
      await waitFor(() => expect(createCollection).toHaveBeenCalledOnce());
      expect(listCollections).toHaveBeenCalledOnce();
      expect(invalidate).not.toHaveBeenCalled();
    },
  );
});
