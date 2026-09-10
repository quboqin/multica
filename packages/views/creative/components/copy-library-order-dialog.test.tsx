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
vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (value: typeof copy) => string, values: Record<string, string | number> = {}) => Object.entries(values).reduce((text, [key, value]) => text.replaceAll(`{{${key}}}`, String(value)), selector(copy)) }) }));
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

function mount(onCreated = vi.fn(), selectedLibrary = library) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={queryClient}><CopyLibraryOrderDialog library={selectedLibrary} onClose={vi.fn()} onCreated={onCreated} /></QueryClientProvider>);
}

describe("copy library order", () => {
  it("previews and submits any selected column using the published market currency", async () => {
    apiMock.listCreativeResources.mockResolvedValue({ resources: [{ id: "market", name: "Malaysia", kind: "market_pack", published_version: 1, published_config: { copy_library_id: library.id, currency: "MYR", locale: "ms-MY" } }] });
    const selectedLibrary = { ...library, published_config: { ...library.published_config, repayment_plan: {
      labels: { principal: "Principal", tenor: "Tenor", monthly_installment: "Monthly", total_interest: "Interest", total_repayment: "Total" },
      entries: [{ id: "plan", key: "plan", principal: 1000, tenor_months: 3, monthly_installment: 350, total_interest: 50, total_repayment: 1050, source: "Approved", status: "approved" }],
    } } } as unknown as CreativeResource;
    mount(vi.fn(), selectedLibrary);
    const columns = screen.getByRole("group", { name: copy.copyOrder.columns });
    for (const name of ["Principal", "Tenor", "Monthly", "Interest"]) fireEvent.click(within(columns).getByRole("checkbox", { name }));
    fireEvent.click(await screen.findByRole("checkbox", { name: /Principal: RM1,000/ }));
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("columnheader")).toHaveLength(1);
    expect(within(table).getByText("Interest")).toBeInTheDocument();
    expect(within(table).getByText("RM50")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create images" }));
    await waitFor(() => expect(apiMock.createCreativeOrder).toHaveBeenCalledOnce());
    expect(apiMock.createCreativeOrder.mock.calls[0]?.[0].items[0].copy_snapshot).toMatchObject({ repayment_plan_keys: ["plan"], repayment_plan_columns: ["total_interest"] });
  });
  it.each([1, 3, 10])("freezes %i sets and displays the corresponding image count", async (count) => {
    mount();
    const control = screen.getByRole("combobox", { name: "Number of sets" });
    expect(control).toHaveValue("3");
    expect(within(control).getAllByRole("option")).toHaveLength(10);
    fireEvent.change(control, { target: { value: String(count) } });
    expect(screen.getByText(`${count} sets · ${count * 3} images`)).toBeInTheDocument();
    const submit = screen.getByRole("button", { name: "Start visual exploration" });
    await waitFor(() => expect(submit).toBeEnabled());
    fireEvent.click(submit);
    await waitFor(() => expect(apiMock.createCreativeOrder).toHaveBeenCalledOnce());
    expect(apiMock.createCreativeOrder.mock.calls[0]?.[0].input_snapshot.target_variant_count).toBe(count);
  });

  it("starts a distinct submission when the requested set count changes after a handoff failure", async () => {
    apiMock.updateIssue.mockRejectedValueOnce(new Error("Handoff failed"));
    mount();
    const submit = screen.getByRole("button", { name: "Start visual exploration" });
    await waitFor(() => expect(submit).toBeEnabled());
    fireEvent.click(submit);
    await screen.findByText("Handoff failed");
    fireEvent.change(screen.getByRole("combobox", { name: "Number of sets" }), { target: { value: "10" } });
    fireEvent.click(submit);
    await waitFor(() => expect(apiMock.createCreativeOrder).toHaveBeenCalledTimes(2));
    const first = apiMock.createCreativeOrder.mock.calls[0]?.[0];
    const second = apiMock.createCreativeOrder.mock.calls[1]?.[0];
    expect(first.submission_key).not.toBe(second.submission_key);
    expect(first.input_snapshot.target_variant_count).toBe(3);
    expect(second.input_snapshot.target_variant_count).toBe(10);
  });
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
