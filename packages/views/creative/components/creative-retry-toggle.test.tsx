import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CreativeRetryToggle } from "./creative-retry-toggle";
import copy from "../../locales/zh-Hans/creative.json";

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), error: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api: { getCreativeRetrySettings: mocks.get, updateCreativeRetrySettings: mocks.update } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("@multica/core/creative", async () => {
  const queries = await import("@multica/core/creative/queries");
  return { creativeKeys: queries.creativeKeys, creativeRetrySettingsOptions: queries.creativeRetrySettingsOptions };
});
vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (value: typeof copy) => string) => selector(copy) }) }));
vi.mock("sonner", () => ({ toast: { error: mocks.error } }));

beforeEach(() => { vi.resetAllMocks(); mocks.get.mockResolvedValue({ automatic_retry_enabled: true, can_manage: true }); });
afterEach(cleanup);
function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><CreativeRetryToggle /></QueryClientProvider>);
}

it("saves a disabled switch and displays the paused state", async () => {
  mocks.update.mockImplementation(async () => { mocks.get.mockResolvedValue({ automatic_retry_enabled: false, can_manage: true }); return { automatic_retry_enabled: false, can_manage: true }; });
  mount();
  const control = screen.getByRole("switch", { name: "自动重试" });
  await waitFor(() => expect(control).toBeChecked());
  fireEvent.click(control);
  await waitFor(() => expect(mocks.update).toHaveBeenCalledWith(false));
  await waitFor(() => expect(control).not.toBeChecked());
  expect(screen.getByText("已暂停")).toBeInTheDocument();
});

it("restores the server value when saving fails", async () => {
  mocks.update.mockRejectedValue(new Error("failed"));
  mount();
  const control = screen.getByRole("switch", { name: "自动重试" });
  await waitFor(() => expect(control).toBeChecked());
  fireEvent.click(control);
  await waitFor(() => expect(mocks.error).toHaveBeenCalled());
  await waitFor(() => expect(control).toBeChecked());
});

it("does not allow members without management permission to toggle", async () => {
  mocks.get.mockResolvedValue({ automatic_retry_enabled: true, can_manage: false });
  mount();
  const control = screen.getByRole("switch", { name: "自动重试" });
  await waitFor(() => expect(control).toBeChecked());
  expect(control).toHaveAttribute("aria-disabled", "true");
  fireEvent.click(control);
  expect(mocks.update).not.toHaveBeenCalled();
});
