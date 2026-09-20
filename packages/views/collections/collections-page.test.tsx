import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NavigationProvider } from "../navigation/context";
import type { NavigationAdapter } from "../navigation";
import { renderWithI18n } from "../test/i18n";
import { CollectionsPage } from "./collections-page";

const api = vi.hoisted(() => ({
  listCollections: vi.fn(),
  listDocuments: vi.fn(),
  listMembers: vi.fn(),
  createCollection: vi.fn(),
}));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/auth", () => {
  const state = () => ({ user: { id: "user-1" } });
  return {
    useAuthStore: Object.assign(
      (selector?: (s: ReturnType<typeof state>) => unknown) =>
        selector ? selector(state()) : state(),
      { getState: state },
    ),
  };
});
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
  useWorkspacePaths: () => ({
    collectionDetail: (id: string) => `/acme/collections/${id}`,
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  api.listCollections.mockResolvedValue([
    { id: "c1", workspace_id: "ws-1", name: "Backlog", created_by: "", project_id: null, revision: 1 },
  ]);
  api.listMembers.mockResolvedValue([]);
});
afterEach(cleanup);

it("lists the tables in their own column beside an empty state", async () => {
  const adapter: NavigationAdapter = {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/acme/collections",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (path) => path,
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  renderWithI18n(
    <NavigationProvider value={adapter}>
      <QueryClientProvider client={client}>
        <CollectionsPage />
      </QueryClientProvider>
    </NavigationProvider>,
  );

  const column = screen.getByRole("complementary", { name: "Tables" });
  expect(await within(column).findByRole("link", { name: "Backlog" })).toHaveAttribute(
    "href",
    "/acme/collections/c1",
  );
  // The column belongs to tables alone: documents open from their own entry.
  expect(api.listDocuments).not.toHaveBeenCalled();

  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { name: "Select or create a table" })).toBeInTheDocument();
  fireEvent.click(within(main).getByRole("button", { name: "New table" }));
  expect(await screen.findByRole("textbox", { name: "Table name" })).toBeInTheDocument();
});
