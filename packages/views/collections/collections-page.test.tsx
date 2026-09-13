/** @vitest-environment jsdom */

import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { setCurrentWorkspace } from "@multica/core/platform";
import { workspaceKeys } from "@multica/core/workspace/queries";
import { renderWithI18n } from "../test/i18n";
import { CollectionsPage } from "./collections-page";

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
  useNavigation: () => ({ push: mockPush }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("CollectionsPage", () => {
  beforeEach(() => {
    setCurrentWorkspace("alpha", "ws-1");
    mockPush.mockReset();
  });

  afterEach(() => {
    cleanup();
    setCurrentWorkspace(null, null);
    vi.restoreAllMocks();
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
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    queryClient.setQueryData(workspaceKeys.list(), [
      { id: "ws-1", slug: "alpha" },
    ]);
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
});
