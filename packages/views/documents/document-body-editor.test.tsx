import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useDocumentPreferences } from "@multica/core/documents";
import type { Issue } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { DocumentBodyEditor } from "./document-body-editor";

const api = vi.hoisted(() => ({ updateIssue: vi.fn(), getIssue: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api }));
vi.mock("../editor", () => ({
  ContentEditor: ({
    value,
    onDraftChange,
    onUpdate,
  }: {
    value: string;
    onDraftChange: (body: string, base: string) => void;
    onUpdate: (body: string, base: string) => void;
  }) => (
    <textarea
      aria-label="Body"
      value={value}
      onChange={(event) => onDraftChange(event.target.value, "Original")}
      onBlur={(event) => onUpdate(event.target.value, "Original")}
    />
  ),
}));
const original = {
  id: "doc",
  workspace_id: "ws",
  kind: "doc",
  description: "Original",
  document_revision: 1,
  revision: 1,
} as Issue;
const key = JSON.stringify(["ws", "doc"]);
beforeEach(() => {
  vi.clearAllMocks();
  useDocumentPreferences.setState({ drafts: {} });
});
afterEach(cleanup);
function mount(issue = original) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui = (value: Issue) => (
    <QueryClientProvider client={client}>
      <DocumentBodyEditor issue={value} attachmentIds={() => []} />
    </QueryClientProvider>
  );
  return { ...renderWithI18n(ui(issue)), ui };
}
it("captures the read version before debounce and refuses to adopt a remote version while dirty", async () => {
  api.updateIssue.mockRejectedValue(new Error("conflict"));
  api.getIssue.mockResolvedValue({
    ...original,
    description: "Remote",
    document_revision: 2,
  });
  const view = mount();
  fireEvent.change(screen.getByLabelText("Body"), {
    target: { value: "Local" },
  });
  expect(useDocumentPreferences.getState().drafts[key]?.version).toBe(1);
  view.rerender(
    view.ui({
      ...original,
      description: "Remote",
      document_revision: 2,
      revision: 2,
    }),
  );
  fireEvent.blur(screen.getByLabelText("Body"));
  await waitFor(() =>
    expect(api.updateIssue).toHaveBeenCalledWith(
      "doc",
      expect.objectContaining({
        description: "Local",
        expected_document_revision: 1,
      }),
    ),
  );
  await screen.findByRole("alert");
  expect(useDocumentPreferences.getState().drafts[key]?.body).toBe("Local");
});
it("keeps edits typed during an in-flight save and advances their base only after its own successful save", async () => {
  let resolve!: (value: Issue) => void;
  api.updateIssue
    .mockImplementationOnce(
      () =>
        new Promise<Issue>((done) => {
          resolve = done;
        }),
    )
    .mockResolvedValue({
      ...original,
      description: "Newest",
      document_revision: 3,
      revision: 3,
    });
  mount();
  fireEvent.change(screen.getByLabelText("Body"), {
    target: { value: "First" },
  });
  fireEvent.blur(screen.getByLabelText("Body"));
  fireEvent.change(screen.getByLabelText("Body"), {
    target: { value: "Newest" },
  });
  await act(async () =>
    resolve({
      ...original,
      description: "First",
      document_revision: 2,
      revision: 2,
    }),
  );
  expect(useDocumentPreferences.getState().drafts[key]).toMatchObject({
    body: "Newest",
    version: 2,
  });
  fireEvent.blur(screen.getByLabelText("Body"));
  await waitFor(() =>
    expect(api.updateIssue).toHaveBeenLastCalledWith(
      "doc",
      expect.objectContaining({
        description: "Newest",
        expected_document_revision: 2,
      }),
    ),
  );
  await waitFor(() =>
    expect(useDocumentPreferences.getState().drafts[key]).toBeUndefined(),
  );
});
it("allows an explicit merge retry against the fetched current version", async () => {
  api.updateIssue
    .mockRejectedValueOnce(new Error("conflict"))
    .mockResolvedValue({
      ...original,
      description: "Merged",
      document_revision: 3,
      revision: 3,
    });
  api.getIssue.mockResolvedValue({
    ...original,
    description: "Remote",
    document_revision: 2,
  });
  mount();
  fireEvent.change(screen.getByLabelText("Body"), {
    target: { value: "Local" },
  });
  fireEvent.blur(screen.getByLabelText("Body"));
  await screen.findByText("Remote");
  fireEvent.change(
    screen.getByRole("textbox", { name: "Your draft — edit to merge" }),
    { target: { value: "Merged" } },
  );
  fireEvent.click(screen.getByRole("button", { name: "Save merged draft" }));
  await waitFor(() =>
    expect(api.updateIssue).toHaveBeenLastCalledWith(
      "doc",
      expect.objectContaining({
        description: "Merged",
        expected_document_revision: 2,
        description_base: "Remote",
      }),
    ),
  );
});
