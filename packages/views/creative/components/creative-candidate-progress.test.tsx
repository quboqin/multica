import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { CreativeCandidateProgress } from "@multica/core/types";
import { CreativeOrderItemSchema } from "@multica/core/api/schemas";
import { CreativeOrderDeliveryCandidates } from "./creative-order-delivery";
import { renderWithI18n } from "../../test/i18n";

const recover = vi.hoisted(() => vi.fn());
vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "", recoverCreativeOrderCandidates: recover } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
beforeEach(() => { recover.mockReset(); recover.mockResolvedValue({ task_id: "recovery" }); });
afterEach(cleanup);
function mount(progress?: Partial<CreativeCandidateProgress>, disabled = false) {
  const p = { state: "planning_incomplete", target: 6, expected: 8, planned: 6, generated: 6, primed: 6, settled: 6, plan_task_id: "plan", plan_status: "completed", selection_task_id: "", selection_status: "", ...progress };
  const item = CreativeOrderItemSchema.parse({ id: "item", source_kind: "copy_library", candidate_progress: p });
  return renderWithI18n(<QueryClientProvider client={new QueryClient()}><CreativeOrderDeliveryCandidates orderId="order" item={item} source={{ label: "Selected copy", url: "" }} attachments={new Map()} onAssetSelect={vi.fn()} disabled={disabled} /></QueryClientProvider>);
}
it("explains six of eight candidates without claiming selection is running", async () => {
  mount();
  const card = screen.getByTestId("creative-order-item-item");
  expect(card.querySelector("summary")).toHaveTextContent("Selected copy");
  expect(card.querySelector("summary")).toHaveTextContent("Candidate plans or production tasks are incomplete");
  expect(screen.getByText("Plans 6/8 · Images 6 · Overlays 6 · Target 6 sets")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Complete candidate planning" }));
  await waitFor(() => expect(recover).toHaveBeenCalledWith("order", "item"));
});
it("keeps recovery disabled when the order is ended", () => {
  mount({}, true);
  const button = within(screen.getByTestId("creative-order-item-item")).getByRole("button", { name: "Complete candidate planning" });
  expect(button).toBeDisabled();
  fireEvent.click(button);
  expect(recover).not.toHaveBeenCalled();
});
it("offers selection recovery only when the backend reports ready candidates", async () => {
  recover.mockRejectedValue(new Error("Selection retry budget exhausted"));
  mount({ state: "selection_ready", planned: 8, generated: 8, primed: 8, settled: 8 });
  fireEvent.click(screen.getByRole("button", { name: "Continue candidate selection" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Selection retry budget exhausted");
});
it.each(["generating", "priming", "settling", "selection_queued", "selecting", "cancelled", "future"])("does not offer duplicate recovery in %s", (state) => {
  mount({ state });
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
});
