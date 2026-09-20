import { QueryClient, QueryClientProvider, useInfiniteQuery } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import {
  collectionRecordsOptions,
  type CollectionField,
  type CollectionRecord,
  type RecordLink,
} from "@multica/core/collections";
import { NavigationProvider } from "../navigation";
import type { NavigationAdapter } from "../navigation";
import { renderWithI18n } from "../test/i18n";
import { CollectionFieldEditor, RecordValue } from "./collection-cell";
import { RelationList } from "./collection-relation";
import { useCollectionCommands } from "./use-collection-commands";

const { api, toast } = vi.hoisted(() => ({
  api: {
    listCollectionRecords: vi.fn(),
    listCollections: vi.fn(),
    listIssueStatuses: vi.fn(),
    searchIssues: vi.fn(),
    linkCollectionRecord: vi.fn(),
    unlinkCollectionRecord: vi.fn(),
  },
  toast: { error: vi.fn(), success: vi.fn() },
}));
vi.mock("@multica/core/api", () => ({ api, ApiError: class ApiError extends Error {} }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useWorkspacePaths: () => ({
    issueDetail: (id: string) => `/acme/issues/${id}`,
    collectionDetail: (id: string) => `/acme/collections/${id}`,
  }),
}));
vi.mock("sonner", () => ({ toast }));

const tasks: CollectionField = {
  id: "tasks",
  name: "Tasks",
  type: "relation",
  position: 0,
  config: { options: [], relation: { to_type: "issue", collection_id: "" } },
};
const customer: CollectionField = {
  id: "customer",
  name: "Customer",
  type: "relation",
  position: 1,
  config: { options: [], relation: { to_type: "record", collection_id: "c-2" } },
};
const link = (patch: Partial<RecordLink> & Pick<RecordLink, "id" | "to_id">): RecordLink => ({
  to_type: "issue",
  title: "",
  identifier: "",
  status: "",
  collection_id: "",
  missing: false,
  ...patch,
});

let server: CollectionRecord;
const row = (title: string, id: string): CollectionRecord => ({
  id,
  workspace_id: "ws-1",
  collection_id: "c-2",
  title,
  fields: {},
  links: {},
  revision: 1,
  created_at: "2026-09-20T00:00:00Z",
});

function Cell({ field }: { field: CollectionField }) {
  const commands = useCollectionCommands("ws-1", "c-1");
  const { data } = useInfiniteQuery(collectionRecordsOptions("ws-1", "c-1", {}));
  const record = data?.pages[0]?.records[0];
  if (!record) return null;
  return (
    <CollectionFieldEditor
      record={record}
      field={field}
      open
      onOpenChange={() => undefined}
      onChange={() => undefined}
      relation={{ link: commands.linkRecord, unlink: commands.unlinkRecord }}
    />
  );
}

function mount(ui: ReactNode) {
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
      <QueryClientProvider client={client}>{ui}</QueryClientProvider>
    </NavigationProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  server = {
    ...row("Embed live views", "r-1"),
    collection_id: "c-1",
    links: {
      tasks: [link({ id: "l-1", to_id: "i-1", identifier: "MUL-31", title: "Data model", status: "in_progress" })],
    },
  };
  api.listIssueStatuses.mockResolvedValue({ statuses: [] });
  api.listCollections.mockResolvedValue([{ id: "c-2", workspace_id: "ws-1", name: "Customers" }]);
  api.listCollectionRecords.mockImplementation(async (id: string) =>
    id === "c-1"
      ? { records: [server], total: 1, groups: [], next_cursor: null }
      : { records: [row("ACME", "r-9"), row("Globex", "r-8")], total: 2, groups: [], next_cursor: null },
  );
  api.searchIssues.mockResolvedValue({
    issues: [
      { id: "i-1", identifier: "MUL-31", title: "Data model", status: "in_progress" },
      { id: "i-2", identifier: "MUL-60", title: "Calendar layout", status: "todo" },
    ],
  });
  api.linkCollectionRecord.mockImplementation(
    async (_c: string, _r: string, field: string, toId: string) => {
      const added =
        field === "tasks"
          ? link({ id: `l-${toId}`, to_id: toId, identifier: "MUL-60", title: "Calendar layout", status: "todo" })
          : link({ id: `l-${toId}`, to_id: toId, to_type: "record", title: "ACME", collection_id: "c-2" });
      server = {
        ...server,
        revision: server.revision + 1,
        links: { ...server.links, [field]: [...(server.links[field] ?? []), added] },
      };
      return server;
    },
  );
  api.unlinkCollectionRecord.mockImplementation(async (_c: string, _r: string, linkId: string) => {
    server = {
      ...server,
      revision: server.revision + 1,
      links: Object.fromEntries(
        Object.entries(server.links).map(([field, links]) => [
          field,
          links.filter((item) => item.id !== linkId),
        ]),
      ),
    };
    return server;
  });
});
afterEach(cleanup);

describe("relation cells", () => {
  it("searches tasks, links one and unlinks another without closing", async () => {
    mount(<Cell field={tasks} />);
    const cell = await screen.findByRole("button", { name: "Tasks: Embed live views" });
    expect(cell).toHaveTextContent("MUL-31");
    // Nothing is searched until there is something to search for.
    expect(await screen.findByText("Type to search issues")).toBeInTheDocument();
    expect(api.searchIssues).not.toHaveBeenCalled();

    fireEvent.change(screen.getByPlaceholderText("Search Issues…"), { target: { value: "cal" } });
    const candidate = await screen.findByRole("button", { name: /MUL-60/ });
    expect(api.searchIssues).toHaveBeenCalledWith(
      expect.objectContaining({ q: "cal", kind: "task", include_closed: true }),
    );
    // A search lists matches only: what is linked shows once, checked, among
    // them, so Enter never lands on a link that merely sat at the top.
    expect(screen.getAllByRole("button", { name: /MUL-31/ })).toHaveLength(1);
    expect(screen.queryByText("Linked")).not.toBeInTheDocument();

    fireEvent.click(candidate);
    await waitFor(() =>
      expect(api.linkCollectionRecord).toHaveBeenCalledWith("c-1", "r-1", "tasks", "i-2"),
    );
    await waitFor(() => expect(cell).toHaveTextContent("MUL-60"));

    fireEvent.click(screen.getByRole("button", { name: /MUL-31/ }));
    await waitFor(() =>
      expect(api.unlinkCollectionRecord).toHaveBeenCalledWith("c-1", "r-1", "l-1"),
    );
    await waitFor(() => expect(cell).not.toHaveTextContent("MUL-31"));
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("links the first match on Enter once the search has answered", async () => {
    api.searchIssues.mockResolvedValue({
      issues: [
        { id: "i-2", identifier: "MUL-60", title: "Calendar layout", status: "todo" },
        { id: "i-3", identifier: "MUL-61", title: "Calendar week view", status: "todo" },
      ],
    });
    mount(<Cell field={tasks} />);
    const input = await screen.findByPlaceholderText("Search Issues…");
    fireEvent.change(input, { target: { value: "cal" } });
    // The matches arrive after the keystroke. Enter lands on the first of
    // them, not on the link that led the list before anything was typed.
    await screen.findByRole("button", { name: /MUL-61/ });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(api.linkCollectionRecord).toHaveBeenCalledWith("c-1", "r-1", "tasks", "i-2"),
    );
    expect(api.unlinkCollectionRecord).not.toHaveBeenCalled();
  });

  it("lists the target table's records, and never the row itself", async () => {
    // A relation of a table to itself: the row being edited is among the rows.
    api.listCollectionRecords.mockImplementation(async (id: string) =>
      id === "c-1"
        ? { records: [server], total: 1, groups: [], next_cursor: null }
        : { records: [row("ACME", "r-9"), { ...server }], total: 2, groups: [], next_cursor: null },
    );
    mount(<Cell field={customer} />);
    fireEvent.click(await screen.findByRole("button", { name: /ACME/ }));
    await waitFor(() =>
      expect(api.linkCollectionRecord).toHaveBeenCalledWith("c-1", "r-1", "customer", "r-9"),
    );
    expect(screen.getByPlaceholderText("Search Customers…")).toBeInTheDocument();
    expect(api.searchIssues).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: /^Embed live views/ }),
    ).not.toBeInTheDocument();
  });

  it("reports a failed link and leaves the cell as it was", async () => {
    api.linkCollectionRecord.mockRejectedValueOnce(new Error("a relation cell holds at most 50 links"));
    mount(<Cell field={tasks} />);
    fireEvent.change(await screen.findByPlaceholderText("Search Issues…"), { target: { value: "cal" } });
    fireEvent.click(await screen.findByRole("button", { name: /MUL-60/ }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("a relation cell holds at most 50 links"),
    );
    expect(screen.getByRole("button", { name: "Tasks: Embed live views" })).not.toHaveTextContent("MUL-60");
  });

  it("keeps a link whose target was deleted, struck through", async () => {
    const record: CollectionRecord = {
      ...server,
      links: {
        tasks: [
          link({ id: "l-1", to_id: "i-1", identifier: "MUL-31", title: "Data model", status: "done" }),
          link({ id: "l-2", to_id: "i-gone", missing: true }),
        ],
        customer: [link({ id: "l-3", to_id: "r-9", to_type: "record", title: "ACME", collection_id: "c-2" })],
      },
    };
    mount(
      <>
        <div data-testid="tasks"><RecordValue record={record} field={tasks} /></div>
        <div data-testid="customer"><RecordValue record={record} field={customer} compact /></div>
      </>,
    );
    const cell = screen.getByTestId("tasks");
    expect(within(cell).getByText("MUL-31")).toBeInTheDocument();
    expect(within(cell).getByText("Deleted")).toHaveClass("line-through");
    expect(within(screen.getByTestId("customer")).getByText("ACME")).toBeInTheDocument();
  });
});

describe("relation list in the record panel", () => {
  it("opens live targets and lets a dead link be removed", () => {
    const onUnlink = vi.fn();
    const links = [
      link({ id: "l-1", to_id: "i-1", identifier: "MUL-31", title: "Data model", status: "todo" }),
      link({ id: "l-2", to_id: "r-9", to_type: "record", title: "ACME", collection_id: "c-2" }),
      link({ id: "l-3", to_id: "i-gone", missing: true }),
    ];
    mount(<RelationList links={links} onUnlink={onUnlink} />);
    expect(screen.getByRole("link", { name: /MUL-31\s*Data model/ })).toHaveAttribute("href", "/acme/issues/i-1");
    expect(screen.getByRole("link", { name: "ACME" })).toHaveAttribute(
      "href",
      "/acme/collections/c-2?record=r-9",
    );
    // A deleted target has nowhere to go, but its link can still be cleaned up.
    expect(screen.getAllByRole("link")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Unlink Deleted" }));
    expect(onUnlink).toHaveBeenCalledWith(links[2]);
  });
});
