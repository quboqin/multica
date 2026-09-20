import { useInfiniteQuery, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import {
  collectionRecordsOptions,
  type CollectionField,
  type CollectionRecord,
} from "@multica/core/collections";
import type { IssueView } from "@multica/core/api/schemas";
import { renderWithI18n } from "../test/i18n";
import { CollectionFieldEditor } from "./collection-cell";
import { savedViewPreferences } from "./collection-detail-page";
import { useCollectionCommands } from "./use-collection-commands";

const { api, ApiError, toast } = vi.hoisted(() => {
  class ApiError extends Error {
    constructor(
      message: string,
      readonly status: number,
    ) {
      super(message);
    }
  }
  return {
    ApiError,
    api: {
      listCollectionRecords: vi.fn(),
      setCollectionRecordField: vi.fn(),
    },
    toast: { error: vi.fn(), success: vi.fn() },
  };
});
vi.mock("@multica/core/api", () => ({ api, ApiError }));
vi.mock("sonner", () => ({ toast }));

const RED = "11111111-1111-4111-8111-111111111111";
const BLUE = "22222222-2222-4222-8222-222222222222";
const tags: CollectionField = {
  id: "tags",
  name: "Tags",
  type: "multi_select",
  position: 0,
  config: {
    options: [
      { id: RED, name: "Red", color: "#dc2626" },
      { id: BLUE, name: "Blue", color: "#2563eb" },
    ],
  },
};
let server: CollectionRecord;

function Harness() {
  const commands = useCollectionCommands("ws-1", "c-1");
  const { data } = useInfiniteQuery(collectionRecordsOptions("ws-1", "c-1", {}));
  const record = data?.pages[0]?.records[0];
  if (!record) return null;
  return (
    <CollectionFieldEditor
      record={record}
      field={tags}
      open
      onOpenChange={() => undefined}
      onChange={(value) => void commands.setField(record, tags.id, value)}
    />
  );
}

function withQuery(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderWithI18n(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  server = {
    id: "r-1",
    workspace_id: "ws-1",
    collection_id: "c-1",
    title: "Row",
    fields: {},
    revision: 1,
    created_at: "2026-09-19T00:00:00Z",
  };
  api.listCollectionRecords.mockImplementation(async () => ({
    records: [server],
    total: 1,
    groups: [],
    next_cursor: null,
  }));
  api.setCollectionRecordField.mockImplementation(
    async (_c: string, _r: string, field: string, value: unknown, expected: unknown) => {
      const current = server.fields[field] ?? null;
      if (JSON.stringify(current) !== JSON.stringify(expected))
        throw new ApiError("field changed; reload and retry", 409);
      server = {
        ...server,
        revision: server.revision + 1,
        fields: { ...server.fields, [field]: value },
      };
      return server;
    },
  );
});
afterEach(cleanup);

describe("multi-select cells in a collection table", () => {
  it("toggles several options in place, each write expecting the previous one", async () => {
    withQuery(<Harness />);
    fireEvent.click(await screen.findByRole("button", { name: /Red/ }));
    // The optimistic value reaches the open picker before the next toggle.
    await waitFor(() => expect(screen.getByRole("button", { name: "Tags: Row" })).toHaveTextContent("Red"));
    fireEvent.click(screen.getByRole("button", { name: /Blue/ }));
    await waitFor(() => expect(api.setCollectionRecordField).toHaveBeenCalledTimes(2));
    expect(api.setCollectionRecordField).toHaveBeenNthCalledWith(1, "c-1", "r-1", "tags", [RED], null);
    expect(api.setCollectionRecordField).toHaveBeenNthCalledWith(2, "c-1", "r-1", "tags", [RED, BLUE], [RED]);
    await waitFor(() => expect(server.fields.tags).toEqual([RED, BLUE]));
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("keeps the user's value in a conflict toast instead of dropping it", async () => {
    withQuery(<Harness />);
    await screen.findByRole("button", { name: /Red/ });
    // Another writer changes the cell after this client loaded it.
    server = { ...server, fields: { tags: [BLUE] } };
    fireEvent.click(screen.getByRole("button", { name: /Red/ }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    const [, options] = toast.error.mock.calls[0]!;
    expect(options.description).toContain(BLUE);
    options.action.onClick();
    await waitFor(() => expect(server.fields.tags).toEqual([RED]));
  });
});

describe("saved view preferences", () => {
  it("restores filters and sort that earlier builds kept in the query", () => {
    const view = {
      id: "v",
      display: { layout: "board" },
      query: {
        properties: { f1: [{ op: "gte", value: "3" }], f2: ["x"] },
        sort_by: "f1",
        sort_dir: "desc",
      },
    } as unknown as IssueView;
    expect(savedViewPreferences(view)).toMatchObject({
      layout: "board",
      sortBy: "f1",
      sortDir: "desc",
      filters: [
        { field: "f1", op: "gte", value: "3" },
        { field: "f2", op: "exact", value: "x" },
      ],
    });
  });
});
