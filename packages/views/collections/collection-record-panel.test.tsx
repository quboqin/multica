import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CollectionField, CollectionRecord } from "@multica/core/collections";
import { NavigationProvider } from "../navigation";
import type { NavigationAdapter } from "../navigation";
import { renderWithI18n } from "../test/i18n";
import { CollectionRecordPanel } from "./collection-record-panel";
import type { CollectionCommands } from "./use-collection-commands";

const { api, toast, createIssue } = vi.hoisted(() => ({
  api: {
    listCollectionRecords: vi.fn(),
    listCollectionRecordBacklinks: vi.fn(),
    listCollections: vi.fn(),
    listIssueStatuses: vi.fn(),
    searchIssues: vi.fn(),
  },
  toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  createIssue: vi.fn(),
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
vi.mock("@multica/core/issues/mutations", () => ({
  useCreateIssue: () => ({ mutateAsync: createIssue }),
}));
vi.mock("sonner", () => ({ toast }));

const notes: CollectionField = {
  id: "notes",
  name: "Notes",
  type: "text",
  position: 0,
  config: { options: [] },
};
const tasks: CollectionField = {
  id: "tasks",
  name: "Implementation",
  type: "relation",
  position: 1,
  config: { options: [], relation: { to_type: "issue", collection_id: "" } },
};
const record: CollectionRecord = {
  id: "r-1",
  workspace_id: "ws-1",
  collection_id: "c-1",
  title: "Embed live views",
  fields: { notes: "Inline editing" },
  links: {
    tasks: [
      { id: "l-1", to_type: "issue", to_id: "i-1", title: "Data model", identifier: "MUL-31", status: "in_progress", collection_id: "", missing: false },
    ],
  },
  revision: 3,
  created_at: "2026-09-20T00:00:00Z",
};

function commandsWith() {
  return {
    setField: vi.fn(),
    setTitle: vi.fn(),
    linkRecord: vi.fn().mockResolvedValue(true),
    unlinkRecord: vi.fn().mockResolvedValue(true),
    deleteRecord: { mutate: vi.fn(), isPending: false },
    createField: {
      mutateAsync: vi.fn().mockResolvedValue({ ...tasks, id: "new-field", name: "Linked issues" }),
      isPending: false,
    },
  } as unknown as CollectionCommands;
}

function mount({
  fields = [notes, tasks],
  canManage = true,
  projectId = null,
  commands = commandsWith(),
}: {
  fields?: CollectionField[];
  canManage?: boolean;
  projectId?: string | null;
  commands?: CollectionCommands;
} = {}) {
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
  renderWithI18n(
    <NavigationProvider value={adapter}>
      <QueryClientProvider client={client}>
        <CollectionRecordPanel
          wsId="ws-1"
          collectionId="c-1"
          projectId={projectId}
          recordId="r-1"
          fields={fields}
          canManage={canManage}
          commands={commands}
          onClose={vi.fn()}
        />
      </QueryClientProvider>
    </NavigationProvider>,
  );
  return { adapter, commands };
}

beforeEach(() => {
  vi.clearAllMocks();
  api.listCollectionRecords.mockResolvedValue({ records: [record], total: 1, groups: [], next_cursor: null });
  api.listCollectionRecordBacklinks.mockResolvedValue([]);
  api.listCollections.mockResolvedValue([]);
  api.listIssueStatuses.mockResolvedValue({ statuses: [] });
  api.searchIssues.mockResolvedValue({ issues: [] });
  createIssue.mockResolvedValue({ id: "i-9", identifier: "MUL-90", title: "Embed live views" });
});
afterEach(cleanup);

describe("relations in the record panel", () => {
  it("gives a relation its own section, with links that open and unlink", async () => {
    const { commands } = mount();
    const section = await screen.findByRole("region", { name: "Implementation" });
    // Value fields keep their rows; the relation is not one of them.
    expect(screen.getByRole("button", { name: "Notes: Embed live views" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Implementation: Embed live views" })).not.toBeInTheDocument();

    expect(within(section).getByRole("link", { name: /MUL-31\s*Data model/ })).toHaveAttribute(
      "href",
      "/acme/issues/i-1",
    );
    expect(within(section).getByRole("button", { name: "Link existing issue" })).toBeInTheDocument();
    fireEvent.click(within(section).getByRole("button", { name: "Unlink MUL-31" }));
    expect(commands.unlinkRecord).toHaveBeenCalledWith("r-1", "tasks", "l-1");
  });

  it("lists the records that point at this one, and nothing when none do", async () => {
    mount();
    await screen.findByRole("region", { name: "Implementation" });
    await waitFor(() => expect(api.listCollectionRecordBacklinks).toHaveBeenCalled());
    expect(screen.queryByText("Referenced by")).not.toBeInTheDocument();
    cleanup();

    api.listCollectionRecordBacklinks.mockResolvedValue([
      { id: "b-1", collection_id: "c-7", collection_name: "Roadmap", record_id: "r-7", record_title: "Q4", field_id: "f", field_name: "Requirements" },
    ]);
    mount();
    expect(await screen.findByText("Referenced by")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Roadmap.*Q4/ })).toHaveAttribute(
      "href",
      "/acme/collections/c-7?record=r-7",
    );
  });
});

describe("convert to issue", () => {
  const openDialog = async () => {
    fireEvent.click(await screen.findByRole("button", { name: "Convert to issue" }));
    return screen.findByRole("dialog", { name: "Convert to issue" });
  };

  it("creates the issue in the table's project and links it from the record", async () => {
    const { adapter, commands } = mount({ projectId: "p-1" });
    const dialog = await openDialog();
    const title = within(dialog).getByLabelText("Issue title");
    expect(title).toHaveValue("Embed live views");
    // The table already has a relation to issues, so nothing is added to it.
    expect(within(dialog).queryByText(/field to hold the link/)).not.toBeInTheDocument();
    fireEvent.change(title, { target: { value: " Build the embed block " } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Create issue" }));

    await waitFor(() =>
      expect(commands.linkRecord).toHaveBeenCalledWith("r-1", "tasks", "i-9"),
    );
    expect(createIssue).toHaveBeenCalledWith({ title: "Build the embed block", project_id: "p-1" });
    expect(commands.createField.mutateAsync).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Convert to issue" })).not.toBeInTheDocument(),
    );
    const [message, options] = toast.success.mock.calls[0]!;
    expect(message).toContain("MUL-90");
    options.action.onClick();
    expect(adapter.push).toHaveBeenCalledWith("/acme/issues/i-9");
  });

  it("adds the relation first when a manager converts in a table without one", async () => {
    const { commands } = mount({ fields: [notes] });
    const dialog = await openDialog();
    expect(within(dialog).getByText(/gets a “Linked issues” field/)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Create issue" }));
    await waitFor(() =>
      expect(commands.linkRecord).toHaveBeenCalledWith("r-1", "new-field", "i-9"),
    );
    expect(commands.createField.mutateAsync).toHaveBeenCalledWith({
      name: "Linked issues",
      type: "relation",
      config: { relation: { to_type: "issue" } },
    });
    // The field exists before the issue does: an issue nobody can find is worse.
    expect(
      vi.mocked(commands.createField.mutateAsync).mock.invocationCallOrder[0]!,
    ).toBeLessThan(createIssue.mock.invocationCallOrder[0]!);
  });

  it("tells a member who cannot manage fields what the table is missing", async () => {
    mount({ fields: [notes], canManage: false });
    const dialog = await openDialog();
    expect(within(dialog).getByText(/no field that links to issues yet/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Create issue" })).toBeDisabled();
    expect(createIssue).not.toHaveBeenCalled();
  });

  it("stays open with the error when the issue cannot be created", async () => {
    createIssue.mockRejectedValueOnce(new Error("issue limit reached"));
    const { commands } = mount();
    const dialog = await openDialog();
    fireEvent.click(within(dialog).getByRole("button", { name: "Create issue" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("issue limit reached");
    expect(commands.linkRecord).not.toHaveBeenCalled();
    expect(within(dialog).getByLabelText("Issue title")).toHaveValue("Embed live views");
  });

  it("says so when the issue exists but the link did not land", async () => {
    const commands = commandsWith();
    vi.mocked(commands.linkRecord).mockResolvedValue(false);
    mount({ commands });
    const dialog = await openDialog();
    fireEvent.click(within(dialog).getByRole("button", { name: "Create issue" }));
    await waitFor(() => expect(toast.warning).toHaveBeenCalled());
    expect(toast.warning.mock.calls[0]![0]).toContain("MUL-90");
    expect(toast.success).not.toHaveBeenCalled();
  });
});
