import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NavigationProvider } from "../../navigation";
import type { NavigationAdapter } from "../../navigation";
import { renderWithI18n } from "../../test/i18n";
import { LinkedRecordsSection } from "./linked-records-section";

const api = vi.hoisted(() => ({ listIssueRecordLinks: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
  useWorkspacePaths: () => ({
    collectionDetail: (id: string) => `/acme/collections/${id}`,
  }),
}));

function mount() {
  const adapter: NavigationAdapter = {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/acme/issues/i-1",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (path) => path,
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <NavigationProvider value={adapter}>
      <QueryClientProvider client={client}>
        <LinkedRecordsSection issueId="i-1" />
      </QueryClientProvider>
    </NavigationProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

// AC-8: the issue side of a relation field.
describe("linked records on an issue", () => {
  it("lists the records that link here, each opening its table on that record", async () => {
    api.listIssueRecordLinks.mockResolvedValue([
      { id: "l-1", collection_id: "c-1", collection_name: "Requirements", record_id: "r-1", record_title: "Embed live views", field_id: "f-1", field_name: "Implementation" },
      { id: "l-2", collection_id: "c-2", collection_name: "Bugs", record_id: "r-2", record_title: "", field_id: "f-2", field_name: "Fix" },
    ]);
    mount();
    const toggle = await screen.findByRole("button", { name: /Linked records/ });
    expect(toggle).toHaveTextContent("2");
    expect(api.listIssueRecordLinks).toHaveBeenCalledWith("i-1", expect.objectContaining({ workspaceId: "ws-1" }));
    expect(screen.getByRole("link", { name: /Requirements\s*Embed live views/ })).toHaveAttribute(
      "href",
      "/acme/collections/c-1?record=r-1",
    );
    // A record without a title is still a row someone can open.
    expect(screen.getByRole("link", { name: /Bugs\s*Untitled/ })).toHaveAttribute(
      "href",
      "/acme/collections/c-2?record=r-2",
    );
    fireEvent.click(toggle);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("renders nothing for an issue no record links to", async () => {
    api.listIssueRecordLinks.mockResolvedValue([]);
    const { container } = mount();
    await waitFor(() => expect(api.listIssueRecordLinks).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
