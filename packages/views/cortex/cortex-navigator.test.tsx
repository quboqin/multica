import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useDocumentPreferences } from "@multica/core/documents";
import type { Issue } from "@multica/core/types";
import { SidebarProvider } from "@multica/ui/components/ui/sidebar";
import { NavigationProvider } from "../navigation/context";
import type { NavigationAdapter } from "../navigation";
import { renderWithI18n } from "../test/i18n";
import { CollectionNavigator, DocumentNavigator } from "./cortex-navigator";

const api = vi.hoisted(() => ({
  listDocuments: vi.fn(),
  listCollections: vi.fn(),
  listProjects: vi.fn(),
  listMembers: vi.fn(),
  createCollection: vi.fn(),
  updateCollection: vi.fn(),
  createIssue: vi.fn(),
  deleteIssue: vi.fn(),
  moveDocument: vi.fn(),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("sonner", () => ({ toast }));
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
// Partial: deleting a document runs the shared issue mutation, which reads the
// real project keys to invalidate.
vi.mock("@multica/core/projects/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/projects/queries")>()),
  projectListOptions: () => ({
    queryKey: ["projects"],
    queryFn: async () => [],
  }),
}));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
  useWorkspacePaths: () => ({
    documents: () => "/acme/documents",
    documentDetail: (id: string) => `/acme/documents/${id}`,
    collections: () => "/acme/collections",
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
    { id: "c1", workspace_id: "ws-1", name: "Backlog", created_by: "user-1", project_id: null, revision: 1, record_count: 96 },
    { id: "c2", workspace_id: "ws-1", name: "Assets", created_by: "someone-else", project_id: null, revision: 1 },
  ]);
  api.listMembers.mockResolvedValue([{ user_id: "user-1", role: "member" }]);
  api.updateCollection.mockResolvedValue({});
  api.deleteIssue.mockResolvedValue(undefined);
  api.createCollection.mockResolvedValue({
    id: "c3", workspace_id: "ws-1", name: "Leads", created_by: "", project_id: null, revision: 1,
  });
});
afterEach(cleanup);

function mount(ui: ReactNode) {
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
        {ui}
      </QueryClientProvider>
    </NavigationProvider>,
  );
}

it("opens documents in a column of their own, without the tables", async () => {
  mount(<DocumentNavigator activeDocumentId="child" />);
  const column = screen.getByRole("complementary", { name: "Documents" });
  expect(await screen.findByRole("link", { name: "Spec" })).toHaveAttribute("aria-current", "page");
  expect(column).toContainElement(screen.getByRole("tree", { name: "Documents" }));
  expect(api.listCollections).not.toHaveBeenCalled();
  expect(screen.queryByRole("heading", { name: "Tables" })).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "Collapse" }));
  expect(screen.queryByRole("link", { name: "Spec" })).not.toBeInTheDocument();
  expect(useDocumentPreferences.getState().navigator["ws-1"]?.collapsed).toEqual(["root"]);
});

it("opens tables in a column of their own, without the document tree", async () => {
  mount(<CollectionNavigator activeCollectionId="c1" />);
  const column = screen.getByRole("complementary", { name: "Tables" });
  const backlog = await screen.findByRole("link", { name: /Backlog/ });
  expect(column).toContainElement(backlog);
  expect(backlog).toHaveTextContent("96");
  expect(backlog).toHaveAttribute("aria-current", "page");
  // Record count is optional in the schema: no number when it is missing.
  expect(screen.getByRole("link", { name: "Assets" })).toHaveTextContent(/^Assets$/);
  expect(api.listDocuments).not.toHaveBeenCalled();
  expect(screen.queryByRole("tree")).not.toBeInTheDocument();
});

it("narrows the tables by name", async () => {
  mount(<CollectionNavigator />);
  await screen.findByRole("link", { name: /Backlog/ });
  fireEvent.change(screen.getByRole("textbox", { name: "Search" }), {
    target: { value: "  asse" },
  });
  expect(screen.getByRole("link", { name: "Assets" })).toBeInTheDocument();
  expect(screen.queryByRole("link", { name: /Backlog/ })).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("textbox", { name: "Search" }), {
    target: { value: "nothing" },
  });
  expect(screen.getByText("No tables")).toBeInTheDocument();
});

it("keeps the way back to a collapsed app sidebar at the top of the column", async () => {
  mount(
    <SidebarProvider>
      <CollectionNavigator activeCollectionId="c1" />
    </SidebarProvider>,
  );
  const column = screen.getByRole("complementary", { name: "Tables" });
  await screen.findByRole("link", { name: /Backlog/ });
  expect(column.querySelector("[data-slot='sidebar-trigger']")).not.toBeNull();
});

it("filters to favorites through the filter menu state", async () => {
  useDocumentPreferences.setState({
    favorites: { "ws-1": ["child"] },
    navigator: { "ws-1": { filter: "favorites", projectId: "", collapsed: [] } },
  });
  mount(<DocumentNavigator />);
  expect(await screen.findByRole("link", { name: "Spec" })).toBeInTheDocument();
  expect(screen.queryByRole("link", { name: "Product" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Clear filter Favorites" }));
  expect(await screen.findByRole("link", { name: "Product" })).toBeInTheDocument();
});

it("creates a table from the popover and opens it after the server confirms", async () => {
  mount(<CollectionNavigator />);
  fireEvent.click(await screen.findByRole("button", { name: "New table" }));
  fireEvent.change(await screen.findByRole("textbox", { name: "Table name" }), {
    target: { value: "Leads" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(push).toHaveBeenCalledWith("/acme/collections/c3"));
  expect(api.createCollection).toHaveBeenCalledWith("Leads");
});

it("deletes a document from its row after confirming, and says where its child pages go", async () => {
  const user = userEvent.setup();
  mount(<DocumentNavigator activeDocumentId="root" />);
  await screen.findByRole("link", { name: "Product" });
  await user.click(screen.getByRole("button", { name: "Actions for Product" }));
  await user.click(await screen.findByRole("menuitem", { name: "Delete document" }));

  const dialog = await screen.findByRole("alertdialog", { name: "Delete “Product”?" });
  expect(dialog).toHaveTextContent("Its 1 child page moves to the top level.");
  expect(api.deleteIssue).not.toHaveBeenCalled();

  api.listDocuments.mockResolvedValue([doc("child", "Spec")]);
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  // Leaves the deleted document only once the server has confirmed.
  await waitFor(() => expect(push).toHaveBeenCalledWith("/acme/documents"));
  expect(api.deleteIssue).toHaveBeenCalledWith("root");
  await waitFor(() =>
    expect(screen.queryByRole("link", { name: "Product" })).not.toBeInTheDocument(),
  );
  expect(screen.getByRole("link", { name: "Spec" })).toBeInTheDocument();
});

it("keeps the document and the dialog when the server refuses the delete", async () => {
  const user = userEvent.setup();
  api.deleteIssue.mockRejectedValue(new Error("forbidden"));
  mount(<DocumentNavigator />);
  await screen.findByRole("link", { name: "Spec" });
  await user.click(screen.getByRole("button", { name: "Actions for Spec" }));
  await user.click(await screen.findByRole("menuitem", { name: "Delete document" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Delete “Spec”?" });
  expect(dialog).not.toHaveTextContent("top level");
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("forbidden"));
  expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  expect(push).not.toHaveBeenCalled();
});

it("lets the people who manage a table delete it from its row", async () => {
  const user = userEvent.setup();
  mount(<CollectionNavigator activeCollectionId="c1" />);
  await screen.findByRole("link", { name: /Backlog/ });
  // Assets belongs to someone else and this member is no admin.
  expect(screen.queryByRole("button", { name: "Actions for Assets" })).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Actions for Backlog" }));
  await user.click(await screen.findByRole("menuitem", { name: "Delete table" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Delete “Backlog”?" });
  expect(dialog).toHaveTextContent("This cannot be undone.");

  api.listCollections.mockResolvedValue([
    { id: "c2", workspace_id: "ws-1", name: "Assets", created_by: "someone-else", project_id: null, revision: 1 },
  ]);
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(push).toHaveBeenCalledWith("/acme/collections"));
  expect(api.updateCollection).toHaveBeenCalledWith("c1", { archived: true });
  await waitFor(() =>
    expect(screen.queryByRole("link", { name: /Backlog/ })).not.toBeInTheDocument(),
  );
});

it("lets workspace admins delete tables they did not create", async () => {
  api.listMembers.mockResolvedValue([{ user_id: "user-1", role: "admin" }]);
  mount(<CollectionNavigator />);
  expect(await screen.findByRole("button", { name: "Actions for Assets" })).toBeInTheDocument();
});
