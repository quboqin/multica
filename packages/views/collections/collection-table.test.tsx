import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen } from "@testing-library/react";
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

  it("leaves sort, group and filter out of a relation column's menu", async () => {
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
    // Its cells are links in their own table; the record query cannot read them.
    expect(await screen.findByRole("menuitem", { name: /Edit field/ })).toHaveTextContent("Relation");
    expect(screen.queryByRole("menuitem", { name: /Ascending|Descending|Filter|Group/ })).not.toBeInTheDocument();
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
