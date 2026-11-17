import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { CollectionField } from "@multica/core/collections";
import {
  DropdownMenu,
  DropdownMenuContent,
} from "@multica/ui/components/ui/dropdown-menu";
import { renderWithI18n } from "../test/i18n";
import { CollectionFieldMenu, CollectionTitleMenu } from "./collection-field-menu";
import { CollectionTable, type CollectionTableActions } from "./collection-table";
import type { CollectionCommands } from "./use-collection-commands";

const api = vi.hoisted(() => ({ listCollectionRecords: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
// jsdom has no layout, so the real row virtualizer would render no rows.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (options: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: options.count }, (_, index) => ({
        index,
        key: index,
        start: index * 41,
        end: (index + 1) * 41,
        size: 41,
        lane: 0,
      })),
    getTotalSize: () => options.count * 41,
    measureElement: () => {},
  }),
}));

const stage: CollectionField = {
  id: "stage",
  name: "Stage",
  type: "select",
  position: 0,
  config: {
    options: [
      { id: "11111111-1111-4111-8111-111111111111", name: "Todo", color: "#dc2626" },
      { id: "22222222-2222-4222-8222-222222222222", name: "Done", color: "#2563eb" },
    ],
  },
};

const commands = {
  createRecord: { mutateAsync: vi.fn(), isPending: false },
  deleteRecord: { mutate: vi.fn() },
  setField: vi.fn(),
  setTitle: vi.fn(),
} as unknown as CollectionCommands;

function withQuery(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  api.listCollectionRecords.mockResolvedValue({
    records: [],
    total: 0,
    groups: [],
    next_cursor: null,
  });
});
afterEach(cleanup);

describe("field entry points", () => {
  const actions = (overrides: Partial<CollectionTableActions> = {}): CollectionTableActions => ({
    canManage: true,
    fieldCount: 0,
    sortBy: "",
    sortDir: "asc",
    onSort: vi.fn(),
    onGroup: vi.fn(),
    onFilter: vi.fn(),
    onHide: vi.fn(),
    onEditField: vi.fn(),
    onEditTitle: vi.fn(),
    onArchiveField: vi.fn(),
    onAddField: vi.fn(),
    onReorderField: vi.fn(),
    onOpenRecord: vi.fn(),
    ...overrides,
  });

  it("loads expanded branches independently and collapses nested rows", async () => {
    const record = (id: string, title: string, child_count = 0) => ({ id, title, child_count, workspace_id: "ws-1", collection_id: "c-1", fields: {}, links: {}, revision: 1, created_at: "" });
    api.listCollectionRecords.mockImplementation(async (_id, query, cursor) => ({
      records: query.parent_id === "parent"
        ? cursor ? [record("sibling", "Sibling")] : [record("child", "Child", 1)]
        : query.parent_id === "child" ? [record("grandchild", "Grandchild")]
          : [record("parent", "Parent", 2)],
      total: 2, groups: [], next_cursor: query.parent_id === "parent" && !cursor ? "page-2" : null,
    }));
    withQuery(<CollectionTable collectionId="c-1" titleName="Name" fields={[]} query={{}} commands={commands} actions={actions()} selectedRecordId={null} />);
    await screen.findByRole("button", { name: "Rename Parent" });
    expect(api.listCollectionRecords).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Expand children of Parent" }));
    await screen.findByRole("button", { name: "Rename Child" });
    fireEvent.click(screen.getByRole("button", { name: "Load more children of Parent" }));
    await screen.findByRole("button", { name: "Rename Sibling" });
    fireEvent.click(screen.getByRole("button", { name: "Expand children of Child" }));
    await screen.findByRole("button", { name: "Rename Grandchild" });
    fireEvent.click(screen.getByRole("button", { name: "Collapse children of Parent" }));
    expect(screen.queryByRole("button", { name: "Rename Child" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Rename Grandchild" })).not.toBeInTheDocument();
    expect(api.listCollectionRecords).toHaveBeenCalledWith("c-1", { parent_id: "parent" }, "page-2", expect.anything(), "ws-1");
  });

  it("preserves a new child's title on failure and expands its parent after success", async () => {
    const user = userEvent.setup();
    api.listCollectionRecords.mockResolvedValue({ records: [{ id: "parent", title: "Parent", fields: {}, links: {}, revision: 1, created_at: "" }], total: 1, groups: [], next_cursor: null });
    vi.mocked(commands.createRecord.mutateAsync).mockRejectedValueOnce(new Error("Could not save")).mockResolvedValueOnce({} as never);
    withQuery(<CollectionTable collectionId="c-1" titleName="Name" fields={[]} query={{}} commands={commands} actions={actions()} selectedRecordId={null} />);
    await user.click(await screen.findByRole("button", { name: "Add child row to Parent" }));
    await user.type(screen.getByLabelText("Record title"), "New child");
    await user.click(screen.getByRole("button", { name: "Add child row" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not save");
    expect(screen.getByLabelText("Record title")).toHaveValue("New child");
    await user.click(screen.getByRole("button", { name: "Add child row" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(commands.createRecord.mutateAsync).toHaveBeenLastCalledWith({ title: "New child", fields: undefined, parentId: "parent" });
    await waitFor(() => expect(api.listCollectionRecords).toHaveBeenCalledWith("c-1", { parent_id: "parent" }, null, expect.anything(), "ws-1"));
  });

  it("pins selection before the title in both the header and record rows", async () => {
    const record = {
      id: "r-1", workspace_id: "ws-1", collection_id: "c-1",
      title: "Oracle", fields: {}, links: {}, revision: 1, created_at: "",
    };
    api.listCollectionRecords.mockResolvedValue({
      records: [record], total: 1, groups: [], next_cursor: null,
    });
    const toggle = vi.fn();
    withQuery(
      <CollectionTable
        collectionId="c-1" titleName="Name" fields={[stage]} query={{}}
        commands={commands} actions={actions({ selection: { rows: [], toggle } })}
        selectedRecordId={null}
      />,
    );
    const checkbox = await screen.findByRole("checkbox", { name: "Select Oracle" });
    const row = checkbox.closest("tr")!;
    const header = screen.getAllByRole("columnheader");
    expect(header.map((cell) => cell.dataset.columnId)).toEqual([
      "__select", "title", "stage", "__add",
    ]);
    expect(within(row).getAllByRole("cell").map((cell) => cell.dataset.columnId))
      .toEqual(["__select", "title", "stage", "__add"]);
    expect(header[0]).toHaveStyle({ position: "sticky", left: "0px" });
    expect(header[1]).toHaveStyle({ position: "sticky", left: "44px" });
    fireEvent.click(checkbox);
    expect(toggle).toHaveBeenCalledWith(record, true);
  });

  it("spells out New field on a table with no fields and hands over its header cell", async () => {
    const tableActions = actions();
    withQuery(
      <CollectionTable
        collectionId="c-1"
        titleName="Name"
        fields={[]}
        query={{}}
        commands={commands}
        actions={tableActions}
        selectedRecordId={null}
      />,
    );
    const add = await screen.findByRole("button", { name: "New field" });
    expect(add).toHaveTextContent("New field");
    fireEvent.click(add);
    expect(tableActions.onAddField).toHaveBeenCalledTimes(1);
    const anchor = vi.mocked(tableActions.onAddField).mock.calls[0]![0];
    expect(anchor).toBe(add.closest("th"));
  });

  it("shrinks to a plus once the table has fields, and hides it from non-managers", async () => {
    const { unmount } = withQuery(
      <CollectionTable
        collectionId="c-1"
        titleName="Name"
        fields={[stage]}
        query={{}}
        commands={commands}
        actions={actions({ fieldCount: 1 })}
        selectedRecordId={null}
      />,
    );
    expect(await screen.findByRole("button", { name: "New field" })).toHaveTextContent(/^$/);
    unmount();
    withQuery(
      <CollectionTable
        collectionId="c-1"
        titleName="Name"
        fields={[stage]}
        query={{}}
        commands={commands}
        actions={actions({ fieldCount: 1, canManage: false })}
        selectedRecordId={null}
      />,
    );
    await screen.findByRole("button", { name: "Stage" });
    expect(screen.queryByRole("button", { name: "New field" })).not.toBeInTheDocument();
  });

  it("renames the title column from its own header menu", async () => {
    const user = userEvent.setup();
    const tableActions = actions();
    withQuery(
      <CollectionTable
        collectionId="c-1"
        titleName="Customer"
        fields={[stage]}
        query={{}}
        commands={commands}
        actions={tableActions}
        selectedRecordId={null}
      />,
    );
    const header = await screen.findByRole("button", { name: "Customer" });
    await user.click(header);
    await user.click(await screen.findByRole("menuitem", { name: /Edit field/ }));
    expect(tableActions.onEditTitle).toHaveBeenCalledWith(header.closest("th"));
  });

  it("keeps the title column to renaming and sorting", () => {
    const onSort = vi.fn();
    const { unmount } = renderWithI18n(
      <DropdownMenu open>
        <DropdownMenuContent>
          <CollectionTitleMenu name="Customer" canManage onEdit={vi.fn()} onSort={onSort} />
        </DropdownMenuContent>
      </DropdownMenu>,
    );
    expect(screen.getByRole("menuitem", { name: /Edit field/ })).toHaveTextContent("Text");
    expect(screen.queryByRole("menuitem", { name: /Archive|Hide|Filter|Group/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("menuitem", { name: "Descending" }));
    expect(onSort).toHaveBeenCalledWith("desc");
    unmount();
    renderWithI18n(
      <DropdownMenu open>
        <DropdownMenuContent>
          <CollectionTitleMenu name="Customer" canManage={false} onEdit={vi.fn()} onSort={vi.fn()} />
        </DropdownMenuContent>
      </DropdownMenu>,
    );
    expect(screen.queryByRole("menuitem", { name: /Edit field/ })).not.toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Ascending" })).toBeInTheDocument();
  });

  it("allows grouping relations while keeping scalar query controls hidden", async () => {
    const user = userEvent.setup();
    const relation: CollectionField = {
      id: "tasks",
      name: "Implementation",
      type: "relation",
      position: 1,
      config: { options: [], relation: { to_type: "issue", collection_id: "" } },
    };
    const tableActions = actions({ fieldCount: 1 });
    withQuery(
      <CollectionTable
        collectionId="c-1"
        titleName="Name"
        fields={[relation]}
        query={{}}
        commands={commands}
        actions={tableActions}
        selectedRecordId={null}
      />,
    );
    await user.click(await screen.findByRole("button", { name: "Implementation" }));
    expect(await screen.findByRole("menuitem", { name: /Group by this field/ })).toBeVisible();
    // Relation grouping resolves link targets; scalar sort/filter stay unavailable.
    expect(await screen.findByRole("menuitem", { name: /Edit field/ })).toHaveTextContent("Relation");
    expect(screen.queryByRole("menuitem", { name: /Ascending|Descending|Filter/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Hide in this view" }));
    expect(tableActions.onHide).toHaveBeenCalledWith("tasks");
  });

  it("opens one Edit field entry from the column menu", () => {
    const onEdit = vi.fn();
    renderWithI18n(
      <DropdownMenu open>
        <DropdownMenuContent>
          <CollectionFieldMenu
            field={stage}
            fieldCount={1}
            canManage
            onEdit={onEdit}
            onSort={vi.fn()}
            onFilter={vi.fn()}
            onHide={vi.fn()}
            onArchive={vi.fn()}
          />
        </DropdownMenuContent>
      </DropdownMenu>,
    );
    const item = screen.getByRole("menuitem", { name: /Edit field/ });
    expect(item).toHaveTextContent("Select · 2 options");
    fireEvent.click(item);
    expect(onEdit).toHaveBeenCalledTimes(1);
  });
});
