import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ApiError } from "@multica/core/api";
import { renderWithI18n } from "../test/i18n";
import { OfficePreview } from "./office-preview";

const getPreview = vi.hoisted(() => vi.fn());
vi.mock("@multica/core/api", async (original) => ({
  ...await original<typeof import("@multica/core/api")>(),
  api: { getAttachmentOfficePreview: getPreview },
}));
vi.mock("@multica/core/paths", () => ({ useCurrentWorkspace: () => ({ id: "ws-1" }) }));
afterEach(() => { cleanup(); vi.restoreAllMocks(); });
function mount() {
  const client = new QueryClient();
  return renderWithI18n(<QueryClientProvider client={client}><OfficePreview attachmentId="att-1" filename="Slides.pptx" onDownload={vi.fn()} /></QueryClientProvider>);
}
it("loads authenticated PDF data and releases its object URL on close", async () => {
  const blob = new Blob(["%PDF-1.7"]);
  getPreview.mockResolvedValue(blob);
  URL.createObjectURL = vi.fn(() => "blob:office-preview");
  URL.revokeObjectURL = vi.fn();
  const view = mount();
  expect(await screen.findByTitle("Slides.pptx")).toHaveAttribute("src", "blob:office-preview");
  expect(getPreview).toHaveBeenCalledWith("att-1", expect.objectContaining({ workspaceId: "ws-1" }));
  view.unmount();
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:office-preview");
});
it("keeps retry and download available when the converter is unavailable", async () => {
  getPreview.mockRejectedValue(new ApiError("unavailable", 503, "Service Unavailable"));
  mount();
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("busy or unavailable"));
  expect(screen.getByRole("button", { name: "Retry" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Download" })).toBeEnabled();
});
