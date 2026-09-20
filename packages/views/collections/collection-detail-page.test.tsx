import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CollectionField } from "@multica/core/collections";
import {
  dataSourceIdentityKey,
  defaultDataViewPreferences,
  useDataViewPreferences,
  type DataViewLayout,
} from "@multica/core/data-source";
import { NavigationProvider } from "../navigation/context";
import type { NavigationAdapter } from "../navigation";
import { renderWithI18n } from "../test/i18n";
import { CollectionDetailPage } from "./collection-detail-page";

const { api, ApiError } = vi.hoisted(() => {
  class ApiError extends Error {}
  return {
    ApiError,
    api: {
      getCollection: vi.fn(),
      listCollections: vi.fn(),
      listCollectionRecords: vi.fn(),
      listCollectionTrash: vi.fn(),
      listIssueViews: vi.fn(),
      listMembers: vi.fn(),
    },
  };
});
vi.mock("@multica/core/api", () => ({ api, ApiError }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
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
    collections: () => "/acme/collections",
    collectionDetail: (id: string) => `/acme/collections/${id}`,
    projectDetail: (id: string) => `/acme/projects/${id}`,
  }),
}));
vi.mock("@multica/core/projects/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/projects/queries")>()),
  projectListOptions: () => ({ queryKey: ["projects"], queryFn: async () => [] }),
}));

const field = (id: string, name: string, type: string): CollectionField => ({
  id,
  name,
  type,
  position: 0,
  config: {
    options:
      type === "select"
        ? [{ id: "11111111-1111-4111-8111-111111111111", name: "Todo", color: "#dc2626" }]
        : [],
  },
});

function mount({
  layout,
  fields,
  role = "owner",
}: {
  layout: DataViewLayout;
  fields: CollectionField[];
  role?: string;
}) {
  api.getCollection.mockResolvedValue({
    collection: {
      id: "c-1",
      workspace_id: "ws-1",
      name: "Table1",
      created_by: "someone-else",
      project_id: null,
      revision: 1,
    },
    fields,
  });
  api.listMembers.mockResolvedValue([{ user_id: "user-1", role }]);
  // The layout a table was left in is a stored preference of this browser.
  useDataViewPreferences.setState({
    bySource: {
      [dataSourceIdentityKey({
        workspaceId: "ws-1",
        namespace: "collections",
        sourceId: "c-1",
      })]: { ...defaultDataViewPreferences, layout },
    },
  });
  const adapter: NavigationAdapter = {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/acme/collections/c-1",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (path) => path,
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <NavigationProvider value={adapter}>
      <QueryClientProvider client={client}>
        <CollectionDetailPage id="c-1" />
      </QueryClientProvider>
    </NavigationProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.listCollections.mockResolvedValue([]);
  api.listIssueViews.mockResolvedValue([]);
  api.listCollectionTrash.mockResolvedValue({ records: [], total: 0, retention_days: 30 });
  api.listCollectionRecords.mockResolvedValue({
    records: [],
    total: 0,
    groups: [],
    next_cursor: null,
  });
});
afterEach(cleanup);

// A table left in a layout without column headers used to offer no way to add
// or edit a field: the board only said which field it was missing.
describe("fields in layouts without column headers", () => {
  it("offers the single-select field a board is missing", async () => {
    const user = userEvent.setup();
    mount({ layout: "board", fields: [field("notes", "Notes", "text")] });
    const main = await screen.findByRole("main");
    expect(
      await within(main).findByText("Add a single-select field to use the board layout."),
    ).toBeInTheDocument();
    await user.click(within(main).getByRole("button", { name: "New field" }));
    const panel = await screen.findByRole("dialog", { name: "New field" });
    expect(within(panel).getByRole("combobox", { name: "Field type" })).toHaveTextContent("Select");
    expect(within(panel).getByLabelText("Option 1")).toBeInTheDocument();
  });

  it("offers the date field a calendar is missing", async () => {
    const user = userEvent.setup();
    mount({ layout: "calendar", fields: [] });
    const main = await screen.findByRole("main");
    await within(main).findByText("Add a date field to use the calendar layout.");
    await user.click(within(main).getByRole("button", { name: "New field" }));
    const panel = await screen.findByRole("dialog", { name: "New field" });
    expect(within(panel).getByRole("combobox", { name: "Field type" })).toHaveTextContent("Date");
  });

  it("edits and adds fields from Display on a board", async () => {
    const user = userEvent.setup();
    mount({
      layout: "board",
      fields: [field("stage", "Stage", "select"), field("notes", "Notes", "text")],
    });
    await user.click(await screen.findByRole("button", { name: /Display/ }));
    await user.click(await screen.findByRole("button", { name: "Edit field Notes" }));
    const panel = await screen.findByRole("dialog", { name: "Edit field" });
    expect(within(panel).getByLabelText("Name")).toHaveValue("Notes");
    await user.click(within(panel).getByRole("button", { name: "Cancel" }));

    await user.click(screen.getByRole("button", { name: /Display/ }));
    await user.click(await screen.findByRole("button", { name: "New field" }));
    expect(await screen.findByRole("dialog", { name: "New field" })).toBeInTheDocument();
  });

  it("lists the title column first in Display, where it can be renamed", async () => {
    const user = userEvent.setup();
    mount({ layout: "board", fields: [field("stage", "Stage", "select")] });
    await user.click(await screen.findByRole("button", { name: /Display/ }));
    // Always shown, so it is not one of the toggles.
    expect(screen.queryByRole("menuitemcheckbox", { name: "Name" })).not.toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Edit field Name" }));
    const panel = await screen.findByRole("dialog", { name: "Edit field" });
    expect(within(panel).getByLabelText("Name")).toHaveValue("Name");
    expect(within(panel).getByRole("combobox", { name: "Field type" })).toBeDisabled();
  });

  it("keeps a calendar's field list for the people who manage the table", async () => {
    const user = userEvent.setup();
    const { unmount } = mount({
      layout: "calendar",
      fields: [field("due", "Due", "date")],
    });
    await user.click(await screen.findByRole("button", { name: /^Fields/ }));
    expect(await screen.findByRole("button", { name: "Edit field Due" })).toBeInTheDocument();
    // Nothing on a calendar can be shown or hidden.
    expect(screen.queryByRole("menuitemcheckbox")).not.toBeInTheDocument();
    unmount();

    mount({ layout: "calendar", fields: [field("due", "Due", "date")], role: "member" });
    await screen.findByRole("button", { name: /Sort/ });
    expect(screen.queryByRole("button", { name: /^Fields/ })).not.toBeInTheDocument();
  });

  it("leaves field management out for members who cannot change the table", async () => {
    const user = userEvent.setup();
    mount({ layout: "board", fields: [field("notes", "Notes", "text")], role: "member" });
    const main = await screen.findByRole("main");
    await within(main).findByText("Add a single-select field to use the board layout.");
    expect(within(main).queryByRole("button", { name: "New field" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Display/ }));
    await screen.findByRole("menuitemcheckbox", { name: "Notes" });
    expect(screen.queryByRole("button", { name: "Edit field Notes" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit field Name" })).not.toBeInTheDocument();
  });
});
