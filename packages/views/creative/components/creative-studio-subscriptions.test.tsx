import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { CreativeOrderSchema, EMPTY_CREATIVE_FEEDBACK_DASHBOARD } from "@multica/core/api/schemas";
import copy from "../../locales/en/creative.json";
import { CreativeStudioPage } from "./creative-studio-page";

const apiMock = vi.hoisted(() => ({
  getBaseUrl: vi.fn(),
  listWorkspaceCapabilities: vi.fn(), getWorkspaceCapabilities: vi.fn(), getCreativeOrder: vi.fn(),
  listCreativeOrders: vi.fn(), listCreativeMaterialLibrary: vi.fn(), listCreativeResources: vi.fn(),
  listCreativeSourceAnalyses: vi.fn(), listCreativeFeedback: vi.fn(), getCreativeFeedbackDashboard: vi.fn(),
  getAttachment: vi.fn(), recoverCreativeOrderCandidates: vi.fn(),
}));
vi.mock("@multica/core/api", () => ({ api: apiMock }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws" }));
vi.mock("../../navigation", () => ({ useNavigation: () => ({ pathname: "/ws/creative", searchParams: new URLSearchParams("tab=orders&order=order"), replace: vi.fn(), push: vi.fn() }) }));
vi.mock("../../layout/page-header", () => ({ PageHeader: ({ children }: { children: ReactNode }) => <header>{children}</header> }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (value: typeof copy) => string, values: Record<string, string | number> = {}) => Object.entries(values).reduce((text, [key, value]) => text.replaceAll(`{{${key}}}`, String(value)), selector(copy)) }) }));
vi.mock("./creative-workbench", () => ({ CreativeWorkbench: () => <div>Workbench content</div> }));
vi.mock("./creative-collection-plans", () => ({ CreativeCollectionPlans: () => null }));

let client: QueryClient;
beforeEach(() => {
  vi.resetAllMocks();
  apiMock.getBaseUrl.mockReturnValue("");
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity, gcTime: Infinity } } });
  const capability = { items: [{ key: "creative_factory", enabled: true }] };
  apiMock.getWorkspaceCapabilities.mockResolvedValue(capability);
  apiMock.listWorkspaceCapabilities.mockResolvedValue(capability);
  apiMock.getCreativeOrder.mockImplementation(() => new Promise(() => {}));
  apiMock.listCreativeOrders.mockResolvedValue({ orders: [] });
  apiMock.listCreativeMaterialLibrary.mockResolvedValue({ candidates: [], crawl_runs: [] });
  apiMock.listCreativeResources.mockResolvedValue({ resources: [] });
  apiMock.listCreativeSourceAnalyses.mockResolvedValue({ analyses: [] });
  apiMock.listCreativeFeedback.mockResolvedValue({ events: [] });
  apiMock.getCreativeFeedbackDashboard.mockResolvedValue(EMPTY_CREATIVE_FEEDBACK_DASHBOARD);
  apiMock.getAttachment.mockImplementation(async (id: string) => ({ id, url: "https://example.test/image.png", filename: `${id}.png` }));
  apiMock.recoverCreativeOrderCandidates.mockResolvedValue({ task_id: "recovery" });
});

it("keeps mixed batch progress and recovery inside each material card", async () => {
  const candidateItems = Array.from({ length: 5 }, (_, index) => ({
    id: `item-${index}`, source_kind: index === 1 ? "copy_library" : "material", candidate_id: index === 1 ? "" : `source-${index}`, direction: "", copy_snapshot: { library_name: "Approved copy" },
    candidate_progress: { state: index === 1 ? "planning_incomplete" : "generating", target: 3, expected: 5, planned: index === 1 ? 4 : 5,
      generated: 0, primed: 0, settled: 0, plan_task_id: `plan-${index}`, plan_status: "completed" },
    variants: Array.from({ length: index === 1 ? 4 : 5 }, (_, variantIndex) => ({ id: `candidate-${index}-${variantIndex}`, candidate_state: "candidate", variant_key: `C0${variantIndex + 1}`, status: "running" })),
  }));
  const completedItem = { id: "completed", source_kind: "copy_library", copy_snapshot: { library_name: "Completed copy" }, variants: Array.from({ length: 3 }, (_, index) => ({
    id: `selected-${index}`, variant_key: `V0${index + 1}`, candidate_state: "selected", revision: 1, active_revision: 1, status: "completed", qc_status: "passed",
    assets: ["1080x1080", "1200x628", "800x1000"].flatMap((size) => ["primed", "delivered"].map((stage) => ({ id: `${index}-${size}-${stage}`, variant_id: `selected-${index}`, attachment_id: `${index}-${size}-${stage}`, revision: 1, size_key: size, stage, status: "completed" }))),
  })) };
  apiMock.listCreativeMaterialLibrary.mockResolvedValue({ candidates: candidateItems.filter((item) => item.candidate_id).map((item, index) => ({ id: item.candidate_id, title: `Original material ${index + 1}` })), crawl_runs: [] });
  apiMock.getCreativeOrder.mockResolvedValue(CreativeOrderSchema.parse({ id: "order", status: "partial", input_snapshot: { target_variant_count: 3 }, items: [...candidateItems, completedItem] }));
  render(<QueryClientProvider client={client}><WorkspaceSlugProvider slug="ws"><CreativeStudioPage /></WorkspaceSlugProvider></QueryClientProvider>);
  await screen.findByTestId("creative-order-item-item-0");
  expect(screen.getAllByTestId(/^creative-order-item-/)).toHaveLength(6);
  await waitFor(() => expect(screen.getByTestId("creative-order-item-item-0").querySelector("summary")).toHaveTextContent("Original material 1"));
  expect(screen.queryByRole("region", { name: copy.candidateProgress.title })).not.toBeInTheDocument();
  for (const label of screen.getAllByText(copy.candidateProgress.generating)) {
    expect(label.closest('[data-testid^="creative-order-item-"]')).not.toBeNull();
  }
  const completed = screen.getByTestId("creative-order-item-completed");
  expect(completed.querySelector("summary")).toHaveTextContent("3/3");
  expect(within(completed).queryByTestId("creative-order-candidate-selection-pending")).not.toBeInTheDocument();
  const incomplete = screen.getByTestId("creative-order-item-item-1");
  expect(incomplete.querySelector("summary")).toHaveTextContent("Approved copy");
  fireEvent.click(incomplete.querySelector("summary")!);
  fireEvent.click(within(incomplete).getByRole("button", { name: copy.candidateProgress.recoverPlan }));
  await waitFor(() => expect(apiMock.recoverCreativeOrderCandidates).toHaveBeenCalledWith("order", "item-1"));
});
afterEach(() => { cleanup(); client.clear(); });

it("opens order details without loading the hidden order list or feedback dashboard", async () => {
  render(<QueryClientProvider client={client}><WorkspaceSlugProvider slug="ws"><CreativeStudioPage /></WorkspaceSlugProvider></QueryClientProvider>);
  await waitFor(() => expect(apiMock.getCreativeOrder).toHaveBeenCalledTimes(1));
  expect(apiMock.listCreativeOrders).not.toHaveBeenCalled();
  expect(apiMock.getCreativeFeedbackDashboard).not.toHaveBeenCalled();
  expect(apiMock.listCreativeResources).not.toHaveBeenCalled();
  expect(apiMock.listCreativeSourceAnalyses).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("tab", { name: copy.studio.workbench }));
  await screen.findByText("Workbench content");
  await waitFor(() => expect(apiMock.listCreativeOrders).toHaveBeenCalledTimes(1));
  expect(apiMock.getCreativeFeedbackDashboard).toHaveBeenCalledTimes(1);
});
