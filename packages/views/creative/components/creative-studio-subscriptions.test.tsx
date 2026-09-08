import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { EMPTY_CREATIVE_FEEDBACK_DASHBOARD } from "@multica/core/api/schemas";
import copy from "../../locales/en/creative.json";
import { CreativeStudioPage } from "./creative-studio-page";

const apiMock = vi.hoisted(() => ({
  getBaseUrl: vi.fn(),
  listWorkspaceCapabilities: vi.fn(), getWorkspaceCapabilities: vi.fn(), getCreativeOrder: vi.fn(),
  listCreativeOrders: vi.fn(), listCreativeMaterialLibrary: vi.fn(), listCreativeResources: vi.fn(),
  listCreativeSourceAnalyses: vi.fn(), listCreativeFeedback: vi.fn(), getCreativeFeedbackDashboard: vi.fn(),
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
