import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";
import { InsertViewDialog, summarizeView } from "./insert-view-dialog";

const api = vi.hoisted(() => ({
  listCollections: vi.fn(),
  listIssueViews: vi.fn(),
  getCollection: vi.fn(),
  getIssueView: vi.fn(),
}));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const collection = {
  id: "col-1",
  workspace_id: "ws-1",
  name: "Backlog",
  created_by: "u1",
  project_id: null,
  revision: 1,
  record_count: 1284,
};
const view = (id: string, name: string, extra: object = {}) => ({
  id,
  name,
  collection_id: "col-1",
  workspace_id: "ws-1",
  owner_id: "u1",
  scope_type: "workspace",
  visibility: "workspace",
  definition_version: 1,
  query: {},
  display: { layout: "table" },
  revision: 1,
  created_at: "",
  updated_at: "",
  ...extra,
});

beforeEach(() => {
  vi.clearAllMocks();
  api.listCollections.mockResolvedValue([collection]);
  api.getCollection.mockResolvedValue({
    collection,
    fields: [{ id: "f-status", name: "State", type: "select", config: { options: [] }, position: 0 }],
  });
  api.listIssueViews.mockImplementation(
    async ({ collection_id }: { collection_id?: string }) =>
      collection_id
        ? [
            view("v-table", "All requests"),
            view("v-cal", "Due dates", {
              visibility: "private",
              display: { layout: "calendar", groupBy: "f-status" },
              query: { properties: { f: ["x"] } },
            }),
          ]
        : [view("v-issues", "Team board", { collection_id: null, display: { viewMode: "board" } })],
  );
});
afterEach(cleanup);

function mount(onInsert = vi.fn(), onOpenChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={client}>
      <InsertViewDialog open onOpenChange={onOpenChange} onInsert={onInsert} />
    </QueryClientProvider>,
  );
  return { onInsert, onOpenChange };
}

it("lists sources, summarizes their saved views and inserts the chosen one", async () => {
  const { onInsert, onOpenChange } = mount();
  expect(await screen.findByRole("button", { name: /Backlog/ })).toHaveTextContent("1284");
  expect(screen.getByRole("button", { name: /Tasks \(built-in collection\)/ })).toBeInTheDocument();
  const first = await screen.findByRole("radio", { name: /All requests/ });
  await waitFor(() => expect(first).toBeChecked());
  expect(screen.getByText(/Calendar · Group: State · 1 filter · Only me/)).toBeInTheDocument();
  expect(screen.getByRole("radio", { name: /Static snapshot/ })).toBeDisabled();

  fireEvent.click(screen.getByRole("radio", { name: /Due dates/ }));
  expect(screen.getByTestId("insert-view-preview")).toHaveTextContent(":::multica-view v-cal");
  fireEvent.click(screen.getByRole("button", { name: "Insert" }));
  expect(onInsert).toHaveBeenCalledWith("v-cal");
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it("switches to built-in task views and filters sources by search", async () => {
  mount();
  fireEvent.click(await screen.findByRole("button", { name: /Tasks/ }));
  expect(await screen.findByRole("radio", { name: /Team board/ })).toBeInTheDocument();
  expect(api.listIssueViews).toHaveBeenCalledWith({ scope_type: "workspace" });
  fireEvent.change(screen.getByRole("textbox", { name: "Search tables…" }), {
    target: { value: "back" },
  });
  expect(screen.queryByRole("button", { name: /Tasks/ })).not.toBeInTheDocument();
});

it("summarizes issue views from their persisted display", () => {
  expect(
    summarizeView(
      view("v", "x", {
        collection_id: null,
        display: { viewMode: "board", grouping: "assignee" },
        query: { status: ["todo"], sort: "position" },
      }) as never,
    ),
  ).toEqual({ layout: "board", grouping: "assignee", filterCount: 1, visibility: "workspace" });
});
