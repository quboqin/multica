import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";
import { DocumentSharing, ResourceSharing } from "./document-sharing";
import {
  DocumentVersionList,
  DocumentVersionPreview,
  useDocumentHistory,
} from "./document-history";
import type { Issue } from "@multica/core/types";

const api = vi.hoisted(() => ({
  updateDocumentAccess: vi.fn(),
  updateCollectionAccess: vi.fn(),
  listMembers: vi.fn(),
  listProjects: vi.fn(),
  listIssueAttachments: vi.fn().mockResolvedValue([]),
  listDocumentVersions: vi.fn(),
  getDocumentVersion: vi.fn(),
  restoreDocumentVersion: vi.fn(),
}));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: () => "Owner" }),
}));
vi.mock("../editor/readonly-content", () => ({
  ReadonlyContent: ({ content }: { content: string }) => <div>{content}</div>,
}));
afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  api.listMembers.mockResolvedValue([
    { user_id: "owner", name: "Owner", email: "owner@test.local" },
    { user_id: "reader", name: "Reader", email: "reader@test.local" },
  ]);
  api.listProjects.mockResolvedValue([]);
});
function mount(child: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrap = (node: React.ReactNode) => (
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  );
  const result = renderWithI18n(wrap(child));
  return {
    ...result,
    rerender: (node: React.ReactNode) => result.rerender(wrap(node)),
  };
}
it("adds a reader and keeps the sharing dialog and choices on failure", async () => {
  api.updateDocumentAccess.mockRejectedValue(
    new Error("Sharing changed; reload"),
  );
  mount(
    <DocumentSharing
      id="doc"
      wsId="ws"
      access={{
        owner_id: "owner",
        scope: "private",
        scope_role: "view",
        project_id: null,
        revision: 3,
        can_edit: true,
        can_manage: true,
        collaborators: [],
      }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Publish" }));
  fireEvent.click(
    await screen.findByRole("button", { name: /reader@test.local/ }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Save sharing" }));
  await screen.findByRole("alert");
  expect(api.updateDocumentAccess).toHaveBeenCalledWith(
    "doc",
    expect.objectContaining({
      collaborators: [{ user_id: "reader", role: "view" }],
      expected_revision: 3,
    }),
    "ws",
  );
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(screen.getByText("Reader")).toBeTruthy();
});
it("does not adopt a background sharing revision while editing the form", async () => {
  api.updateDocumentAccess.mockRejectedValue(
    new Error("Sharing changed; reload"),
  );
  const sharing = (revision: number) => (
    <DocumentSharing
      id="doc"
      wsId="ws"
      access={{
        owner_id: "owner",
        scope: "private",
        scope_role: "view",
        project_id: null,
        revision,
        can_edit: true,
        can_manage: true,
        collaborators: [],
      }}
    />
  );
  const { rerender } = mount(sharing(3));
  fireEvent.click(screen.getByRole("button", { name: "Publish" }));
  fireEvent.click(
    await screen.findByRole("button", { name: /reader@test.local/ }),
  );
  rerender(sharing(4));
  fireEvent.click(screen.getByRole("button", { name: "Save sharing" }));
  await screen.findByRole("alert");
  expect(api.updateDocumentAccess).toHaveBeenCalledWith(
    "doc",
    expect.objectContaining({
      expected_revision: 3,
      collaborators: [{ user_id: "reader", role: "view" }],
    }),
    "ws",
  );
});
function HistoryPage({
  issue,
  enabled = true,
}: {
  issue: Issue;
  enabled?: boolean;
}) {
  const history = useDocumentHistory(issue, enabled);
  return (
    <>
      <DocumentVersionPreview
        issue={issue}
        history={history}
        onClose={() => {}}
      />
      <DocumentVersionList history={history} />
    </>
  );
}
it("previews history then restores with the currently read issue revision", async () => {
  const snapshot = {
    version: 1,
    title: "Old title",
    body: "Old body",
    actor_type: "member",
    actor_id: "owner",
    action: "create",
    restored_from: null,
    created_at: "2026-09-22T10:00:00Z",
  };
  api.listDocumentVersions.mockResolvedValue({
    versions: [{ ...snapshot, version: 2 }, { ...snapshot }],
    next_cursor: null,
  });
  api.getDocumentVersion.mockResolvedValue(snapshot);
  api.restoreDocumentVersion.mockRejectedValue(new Error("Document changed"));
  const history = (revision: number) => (
    <HistoryPage
      issue={
        {
          id: "doc",
          workspace_id: "ws",
          title: "Current title",
          description: "Current body",
          revision,
        } as Issue
      }
    />
  );
  const { rerender } = mount(history(8));
  fireEvent.click(await screen.findByRole("button", { name: /v1/ }));
  await screen.findByText("Old body");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByText("Current body")).toBeNull();
  fireEvent.click(screen.getByRole("switch", { name: "Compare with current" }));
  expect(screen.getByText("Current body")).toBeTruthy();
  fireEvent.click(screen.getByRole("switch", { name: "Compare with current" }));
  expect(screen.queryByText("Current body")).toBeNull();
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "Restore this version" })
        .hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "Restore this version" }));
  expect(api.restoreDocumentVersion).not.toHaveBeenCalled();
  rerender(history(9));
  fireEvent.click(screen.getByRole("button", { name: "Confirm restore" }));
  await screen.findByRole("alert");
  expect(api.restoreDocumentVersion).toHaveBeenCalledWith("doc", 1, 8);
  expect(screen.getByText("Old body")).toBeTruthy();
});

it("does not fetch history when the Versions tab is inactive or access is read-only", () => {
  mount(
    <HistoryPage
      enabled={false}
      issue={
        {
          id: "doc",
          workspace_id: "ws",
          title: "Title",
          description: "Body",
          revision: 1,
        } as Issue
      }
    />,
  );
  expect(api.listDocumentVersions).not.toHaveBeenCalled();
  expect(api.getDocumentVersion).not.toHaveBeenCalled();
});

it("uses the collection endpoint for named collaborators and a shared edit audience", async () => {
 api.updateCollectionAccess.mockResolvedValue({});
 mount(<ResourceSharing kind="collection" id="table" wsId="ws" access={{owner_id:"owner",scope:"workspace",scope_role:"edit",project_id:null,revision:4,can_edit:true,can_manage:true,collaborators:[]}}/>);
 fireEvent.click(screen.getByRole("button",{name:"Share"}));
 expect(await screen.findByRole("dialog",{name:"Share table"})).toBeTruthy();
 fireEvent.click(await screen.findByRole("button",{name:/reader@test.local/}));
 fireEvent.click(screen.getByRole("button",{name:"Save sharing"}));
 await waitFor(()=>expect(api.updateCollectionAccess).toHaveBeenCalledWith("table",{scope:"workspace",scope_role:"edit",project_id:null,collaborators:[{user_id:"reader",role:"view"}],expected_revision:4},"ws"));
 expect(api.updateDocumentAccess).not.toHaveBeenCalled();
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
});
