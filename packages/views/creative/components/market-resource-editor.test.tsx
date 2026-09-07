import "@testing-library/jest-dom/vitest";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { creativeKeys, creativeResourcesOptions, useCreativeResourceDraftStore } from "@multica/core/creative";
import type { CreativeResource } from "@multica/core/types";
import { ResourceEditor } from "./creative-studio-page";
import { createDefaultPrimeTemplateSet } from "../lib/prime-template-set";
import { renderWithI18n } from "../../test/i18n";

const mocks = vi.hoisted(() => ({ workspaceId: "workspace", list: vi.fn(), update: vi.fn(), publish: vi.fn() }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => mocks.workspaceId }));
vi.mock("@multica/core/api", () => ({ api: { listCreativeResources: mocks.list, updateCreativeResource: mocks.update, publishCreativeResource: mocks.publish } }));
vi.mock("./market-resource-files", () => ({ MarketResourceFiles: () => <div /> }));

function fixture(): CreativeResource {
  const templates = createDefaultPrimeTemplateSet();
  const config = { brand: "Test", market: "Malaysia", prime_composition_mode: "deterministic", prime_template_set: templates, prime_template_set_validation: { status: "passed", families: templates.families } };
  return { id: "market", workspace_id: "workspace", name: "Market", description: "", kind: "market_pack", status: "published", version: 1, published_version: 1, config, published_config: structuredClone(config), created_by: "user", created_at: "", updated_at: "" };
}
let resource: CreativeResource;
let client: QueryClient;

function Harness() {
  const query = useQuery(creativeResourcesOptions(mocks.workspaceId));
  return <ResourceEditor resources={query.data?.resources ?? []} copyLibraries={[]} onCreate={vi.fn()} onArchive={vi.fn()} />;
}
function mount() { return renderWithI18n(<QueryClientProvider client={client}><Harness /></QueryClientProvider>); }
async function openPrime() {
  fireEvent.click(await screen.findByRole("tab", { name: "品牌与 Prime" }));
  return screen.findByRole("switch", { name: "Integrate Prime template with the model" });
}

beforeEach(() => {
  mocks.workspaceId = "workspace";
  vi.resetAllMocks();
  useCreativeResourceDraftStore.setState({ changes: {} });
  resource = fixture();
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } });
  client.setQueryData(creativeKeys.resources("workspace"), { resources: [resource] });
  mocks.list.mockImplementation(async () => ({ resources: [resource] }));
  mocks.update.mockImplementation(async (_id, request) => {
    const config = { ...request.config };
    delete config.prime_template_set_validation;
    delete config.prime_layout_contract;
    resource = { ...resource, status: "draft", version: resource.version + 1, config };
    return resource;
  });
  mocks.publish.mockImplementation(async () => {
    resource = { ...resource, status: "published", published_version: resource.version, published_config: { ...resource.config } };
    return resource;
  });
});
afterEach(() => { cleanup(); client.clear(); });

it("keeps unsaved mode across server refresh and remount, isolated by workspace", async () => {
  let view = mount();
  fireEvent.click(await openPrime());
  expect(screen.getByRole("switch")).toBeChecked();
  await act(async () => { resource = { ...resource, updated_at: "changed", config: { ...resource.config, currency: "MYR" } }; client.setQueryData(creativeKeys.resources("workspace"), { resources: [resource] }); });
  expect(screen.getByRole("switch")).toBeChecked();
  expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  view.unmount();
  view = mount();
  expect(await openPrime()).toBeChecked();
  expect(screen.getByTestId("prime-published-mode")).toHaveTextContent("Platform overlay");
  view.unmount();
  mocks.workspaceId = "another-workspace";
  client.setQueryData(creativeKeys.resources(mocks.workspaceId), { resources: [resource] });
  mount();
  expect(await openPrime()).not.toBeChecked();
});

it("saves the current mode before publishing and shows the effective published mode", async () => {
  mount();
  fireEvent.click(await openPrime());
  fireEvent.click(screen.getByRole("button", { name: "Save and publish" }));
  await waitFor(() => expect(mocks.publish).toHaveBeenCalledOnce());
  expect(mocks.update.mock.calls[0]?.[1].config).toMatchObject({ prime_composition_mode: "model_integrated", prime_model_template_family: "light_background" });
  expect(mocks.update.mock.invocationCallOrder[0]).toBeLessThan(mocks.publish.mock.invocationCallOrder[0]!);
  await waitFor(() => expect(screen.getByTestId("prime-published-mode")).toHaveTextContent("Model integration"));
  expect(resource.published_config?.prime_composition_mode).toBe("model_integrated");
  expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();
});

it("keeps drafts separate when switching between market resources", async () => {
  const second = { ...fixture(), id: "second", name: "Second", config: { ...fixture().config, brand: "Second" } };
  second.published_config = second.config;
  client.setQueryData(creativeKeys.resources("workspace"), { resources: [resource, second] });
  mount();
  fireEvent.click(await openPrime());
  fireEvent.click(screen.getByRole("button", { name: /Second/ }));
  expect(screen.getByRole("switch")).not.toBeChecked();
  fireEvent.click(screen.getByRole("button", { name: /Test/ }));
  expect(screen.getByRole("switch")).toBeChecked();
});

it("keeps QR-bearing templates ineligible for enabling model integration", async () => {
  const templateSet = createDefaultPrimeTemplateSet();
  resource.config.prime_template_set_validation = { status: "passed", families: templateSet.families.map((family) => ({ ...family, templates: Object.fromEntries(Object.entries(family.templates).map(([size, template]) => [size, { ...template, qr_payload: "https://example.test/qr" }])) })) };
  mount();
  const control = await openPrime();
  expect(control).not.toBeChecked();
  expect(control).toHaveAttribute("aria-disabled", "true");
  expect(screen.getByText("All template families contain QR codes; model integration is unavailable")).toBeInTheDocument();
});

it("preserves unsaved input when saving fails and does not publish stale settings", async () => {
  mocks.update.mockRejectedValueOnce(new Error("Save failed"));
  mount();
  fireEvent.click(await openPrime());
  fireEvent.click(screen.getByRole("button", { name: "Save and publish" }));
  await screen.findByRole("alert");
  expect(screen.getByRole("switch")).toBeChecked();
  expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  expect(mocks.publish).not.toHaveBeenCalled();
});

it("allows turning the saved mode off even after draft validation is cleared", async () => {
  mount();
  fireEvent.click(await openPrime());
  fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
  await screen.findByText("Draft saved, not published");
  const control = screen.getByRole("switch");
  expect(control).toBeChecked();
  expect(control).not.toHaveAttribute("aria-disabled", "true");
  expect(screen.getByTestId("prime-published-mode")).toHaveTextContent("Platform overlay");
  fireEvent.click(control);
  expect(control).not.toBeChecked();
  fireEvent.click(screen.getByRole("button", { name: "Save and publish" }));
  await waitFor(() => expect(mocks.publish).toHaveBeenCalledOnce());
  expect(resource.config.prime_composition_mode).toBe("deterministic");
  expect(resource.config.prime_model_template_family).toBeUndefined();
});

it("keeps a saved draft after publication failure and retries without resaving", async () => {
  mocks.publish.mockRejectedValueOnce(new Error("Template validation failed"));
  mount();
  fireEvent.click(await openPrime());
  fireEvent.click(screen.getByRole("button", { name: "Save and publish" }));
  await screen.findByRole("alert");
  expect(screen.getByRole("switch")).toBeChecked();
  expect(screen.getByRole("switch")).not.toHaveAttribute("aria-disabled", "true");
  expect(screen.getByTestId("prime-published-mode")).toHaveTextContent("Platform overlay");
  fireEvent.click(screen.getByRole("button", { name: "Publish" }));
  await waitFor(() => expect(screen.getByTestId("prime-published-mode")).toHaveTextContent("Model integration"));
  expect(mocks.update).toHaveBeenCalledOnce();
});
