import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import copy from "../../locales/en/creative.json";
import { CreativeGalleryConfirmationDialog } from "./creative-gallery-confirmation-dialog";

const mocks = vi.hoisted(() => ({ getCreativeOrder: vi.fn(), confirmCreativeGalleryDelivery: vi.fn(), getBaseUrl: () => "" }));
vi.mock("@multica/core/api", () => ({ api: mocks }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (value: typeof copy) => string) => selector(copy) }) }));
afterEach(cleanup);
beforeEach(() => {
  vi.resetAllMocks();
  mocks.confirmCreativeGalleryDelivery.mockResolvedValue({ id: "membership", subject_type: "variant", subject_id: "variant", event_type: "decision", decision: "accepted", undo_of_id: "", context_snapshot: { revision: 1 } });
});

function mount(status: string, onConfirmed = vi.fn()) {
  mocks.getCreativeOrder.mockResolvedValue({ id: "order", issue_id: "issue", items: [{ id: "item", variants: [{ id: "variant", variant_key: "C01", revision: 1, active_revision: 0, assets: [], qc_reports: [{ lane: "visual", attempt: 1, revision: 1, status, findings: {} }] }] }] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><CreativeGalleryConfirmationDialog orderId="order" variantId="variant" revision={1} onClose={vi.fn()} onConfirmed={onConfirmed} /></QueryClientProvider>);
}

describe("gallery delivery confirmation", () => {
  it("confirms a failed QC result without requiring acknowledgement", async () => {
    mocks.confirmCreativeGalleryDelivery.mockRejectedValueOnce(new Error("Prime package incomplete"));
    const confirmed = vi.fn();
    mount("failed", confirmed);
    const confirm = screen.getByRole("button", { name: "Confirm delivery" });
    await waitFor(() => expect(confirm).toBeEnabled());
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    fireEvent.click(confirm);
    await screen.findByRole("alert");
    fireEvent.click(confirm);
    await waitFor(() => expect(confirmed).toHaveBeenCalledOnce());
    expect(mocks.confirmCreativeGalleryDelivery).toHaveBeenLastCalledWith(expect.objectContaining({ variant_id: "variant", revision: 1, qc_risk_acknowledged: false, qc_risk_reason: "" }));
  });

  it("confirms a passing historical package without inventing risk acknowledgement", async () => {
    const confirmed = vi.fn();
    mount("passed", confirmed);
    const confirm = screen.getByRole("button", { name: "Confirm delivery" });
    await waitFor(() => expect(confirm).toBeEnabled());
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    fireEvent.click(confirm);
    await waitFor(() => expect(confirmed).toHaveBeenCalledOnce());
    expect(mocks.confirmCreativeGalleryDelivery).toHaveBeenCalledWith(expect.objectContaining({ qc_risk_acknowledged: false, qc_risk_reason: "" }));
  });
});
