import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useDocumentPreferences } from "@multica/core/documents";
import type { Issue } from "@multica/core/types";
import { NavigationProvider } from "../navigation/context";
import type { NavigationAdapter } from "../navigation";
import { renderWithI18n } from "../test/i18n";
import { CortexNavigator } from "./cortex-navigator";

const api = vi.hoisted(() => ({
  listDocuments: vi.fn(),
  listCollections: vi.fn(),
  listProjects: vi.fn(),
  createCollection: vi.fn(),
  createIssue: vi.fn(),
  moveDocument: vi.fn(),
}));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/projects/queries", () => ({
  projectListOptions: () => ({
    queryKey: ["projects"],
    queryFn: async () => [],
  }),
}));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
  useWorkspacePaths: () => ({
    documentDetail: (id: string) => `/acme/documents/${id}`,
    collectionDetail: (id: string) => `/acme/collections/${id}`,
  }),
}));

const doc = (id: string, title: string, parent: string | null = null) =>
  ({
    id,
    title,
    description: "",
    parent_issue_id: parent,
    project_id: null,
    position: 0,
  }) as Issue;

let push: ReturnType<typeof vi.fn<(path: string) => void>>;
beforeEach(() => {
  vi.clearAllMocks();
  useDocumentPreferences.setState({ navigator: {}, favorites: {}, recent: {} });
  api.listDocuments.mockResolvedValue([
    doc("root", "Product"),
    doc("child", "Spec", "root"),
  ]);
  api.listCollections.mockResolvedValue([
    { id: "c1", workspace_id: "ws-1", name: "Backlog", created_by: "", project_id: null, revision: 1, record_count: 96 },
    { id: "c2", workspace_id: "ws-1", name: "Assets", created_by: "", project_id: null, revision: 1 },
  ]);
  api.createCollection.mockResolvedValue({
    id: "c3", workspace_id: "ws-1", name: "Leads", created_by: "", project_id: null, revision: 1,
  });
});
afterEach(cleanup);

function mount(props: { activeDocumentId?: string; activeCollectionId?: string }) {
  push = vi.fn<(path: string) => void>();
  const adapter: NavigationAdapter = {
    push,
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (path) => path,
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <NavigationProvider value={adapter}>
      <QueryClientProvider client={client}>
        <CortexNavigator {...props} />
      </QueryClientProvider>
    </NavigationProvider>,
  );
}

it("renders the document tree and tables with counts and active state", async () => {
  mount({ activeDocumentId: "child", activeCollectionId: "c1" });
  expect(await screen.findByRole("link", { name: "Spec" })).toHaveAttribute("aria-current", "page");
  const backlog = await screen.findByRole("link", { name: /Backlog/ });
  expect(backlog).toHaveTextContent("96");
  expect(backlog).toHaveAttribute("aria-current", "page");
  // Record count is optional in the schema: no number when it is missing.
  expect(screen.getByRole("link", { name: "Assets" })).toHaveTextContent(/^Assets$/);

  fireEvent.click(screen.getByRole("button", { name: "Collapse" }));
  expect(screen.queryByRole("link", { name: "Spec" })).not.toBeInTheDocument();
  expect(useDocumentPreferences.getState().navigator["ws-1"]?.collapsed).toEqual(["root"]);
});

it("filters to favorites through the filter menu state", async () => {
  useDocumentPreferences.setState({
    favorites: { "ws-1": ["child"] },
    navigator: { "ws-1": { filter: "favorites", projectId: "", collapsed: [] } },
  });
  mount({});
  expect(await screen.findByRole("link", { name: "Spec" })).toBeInTheDocument();
  expect(screen.queryByRole("link", { name: "Product" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Clear filter Favorites" }));
  expect(await screen.findByRole("link", { name: "Product" })).toBeInTheDocument();
});

it("creates a table from the popover and opens it after the server confirms", async () => {
  mount({});
  fireEvent.click(await screen.findByRole("button", { name: "New table" }));
  fireEvent.change(await screen.findByRole("textbox", { name: "Table name" }), {
    target: { value: "Leads" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(push).toHaveBeenCalledWith("/acme/collections/c3"));
  expect(api.createCollection).toHaveBeenCalledWith("Leads");
});
