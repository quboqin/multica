import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CreativeResource } from "@multica/core/types";
import copy from "../../locales/en/creative.json";
import { CopyLibraryOrderDialog } from "./copy-library-order-dialog";

const apiMock = vi.hoisted(() => ({ listCreativeResources: vi.fn(), listSquads: vi.fn(), listCreativeResourceFiles: vi.fn(), createIssue: vi.fn(), createCreativeOrder: vi.fn(), setIssueMetadataKey: vi.fn(), updateIssue: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api: apiMock }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (value: typeof copy) => string) => selector(copy) }) }));
vi.mock("./creative-material-library", () => ({ creativeSubmissionKey: async (value: unknown) => JSON.stringify(value), EMPTY_VISUAL_DIRECTION: { schema_version: 1, theme: "", style_tags: [], must_preserve: [], avoid: [] }, VisualDirectionEditor: () => null, visualDirectionSummary: () => "" }));

const library = { id: "library", name: "Approved library", published_version: 2, published_config: { fragments: [
  { id: "headline", key: "headline", name: "Headline", role: "headline", text: "Approved headline", status: "approved", creative_types: ["num"] },
  { id: "draft", key: "draft", role: "benefit", text: "Unapproved draft", status: "draft", creative_types: ["num"] },
] } } as unknown as CreativeResource;

beforeEach(() => {
  vi.resetAllMocks();
  apiMock.listCreativeResources.mockResolvedValue({ resources: [{ id: "market", name: "Market", kind: "market_pack", published_version: 1, published_config: { copy_library_id: library.id } }] });
  apiMock.listSquads.mockResolvedValue([{ id: "squad", name: "Team" }]);
  apiMock.listCreativeResourceFiles.mockResolvedValue({ files: [] });
  apiMock.createIssue.mockResolvedValue({ id: "issue" });
  apiMock.createCreativeOrder.mockResolvedValue({ id: "order" });
  apiMock.setIssueMetadataKey.mockResolvedValue({});
  apiMock.updateIssue.mockResolvedValue({});
});
afterEach(cleanup);

function mount(onCreated = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={queryClient}><CopyLibraryOrderDialog library={library} onClose={vi.fn()} onCreated={onCreated} /></QueryClientProvider>);
}

describe("copy library order", () => {
  it("submits an explicit visual-only order with all slots optional", async () => {
    const onCreated = vi.fn();
    mount(onCreated);
    const submit = await screen.findByRole("button", { name: "Start visual exploration" });
    await waitFor(() => expect(submit).toBeEnabled());
    fireEvent.click(submit);
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("order"));
    expect(apiMock.createCreativeOrder).toHaveBeenCalledWith(expect.objectContaining({ items: [expect.objectContaining({ source_kind: "copy_library", candidate_id: "", source_analysis_id: "", copy_snapshot: expect.objectContaining({ slots: {}, repayment_plan_keys: [], visual_only: true }) })] }));
    expect(apiMock.updateIssue).toHaveBeenCalledWith("issue", { assignee_type: "squad", assignee_id: "squad" });
  });

  it("includes approved selections and retries handoff without duplicating the order", async () => {
    apiMock.updateIssue.mockRejectedValueOnce(new Error("Handoff failed"));
    mount();
    expect(screen.queryByText("Unapproved draft")).not.toBeInTheDocument();
    const headlineSection = screen.getByText("Headline", { selector: "summary" }).closest("details")!;
    fireEvent.click(within(headlineSection).getByRole("checkbox"));
    const submit = screen.getByRole("button", { name: "Create images" });
    await waitFor(() => expect(submit).toBeEnabled());
    fireEvent.click(submit);
    await screen.findByText("Handoff failed");
    fireEvent.click(submit);
    await waitFor(() => expect(apiMock.updateIssue).toHaveBeenCalledTimes(2));
    expect(apiMock.createIssue).toHaveBeenCalledTimes(1);
    expect(apiMock.createCreativeOrder).toHaveBeenCalledTimes(1);
    expect(apiMock.createCreativeOrder.mock.calls[0]?.[0].items[0].copy_snapshot).toMatchObject({ slots: { headline: ["headline"] }, visual_only: false });
  });
});
