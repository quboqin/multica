import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { CellContext, ColumnDef } from "@tanstack/react-table";
import { DataViewTable } from "./table-view";

// jsdom has no layout, so the real row virtualizer sees a 0-height viewport
// and renders nothing. Render every row inline instead (mirrors the
// react-virtuoso mock in issue-surface.test.tsx).
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (options: {
    count: number;
    getItemKey?: (index: number) => unknown;
  }) => ({
    getVirtualItems: () =>
      Array.from({ length: options.count }, (_, index) => ({
        index,
        key: options.getItemKey?.(index) ?? index,
        start: index * 41,
        end: (index + 1) * 41,
        size: 41,
        lane: 0,
      })),
    getTotalSize: () => options.count * 41,
    measureElement: () => {},
  }),
}));

type SampleRow = { id: string; title: string };
const capabilities = {
  layouts: ["table"],
  editing: "adapter",
  sideEffects: "none",
  sorting: "none",
} as const;

function SampleCell({ row }: CellContext<SampleRow, unknown>) {
  const [draft, setDraft] = useState(row.original.title);
  return (
    <input
      aria-label="Title draft"
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
    />
  );
}

const columns: ColumnDef<SampleRow>[] = [
  { id: "title", header: "Title", cell: SampleCell },
];

afterEach(cleanup);

describe("shared table with independent sample sources", () => {
  it.each(["sample-documents", "sample-records"])(
    "renders %s without a task DTO or task actions",
    (namespace) => {
      const onRowClick = vi.fn();
      render(
        <DataViewTable
          sourceIdentity={{ workspaceId: "ws", namespace, sourceId: "one" }}
          capabilities={capabilities}
          rows={[{ id: "one", title: "Example" }]}
          rowId={(row) => row.id}
          columns={columns}
          visibleColumnIds={["title"]}
          columnSizing={{ title: 320 }}
          onColumnSizingChange={vi.fn()}
          onReorderColumn={vi.fn()}
          onRowClick={onRowClick}
        />,
      );
      expect(screen.getByRole("columnheader", { name: /Title/ })).toBeVisible();
      expect(screen.getByRole("textbox", { name: "Title draft" })).toHaveValue(
        "Example",
      );
      fireEvent.click(screen.getByRole("textbox").closest("tr")!);
      expect(onRowClick).toHaveBeenCalledOnce();
      expect(onRowClick.mock.calls[0]?.[0].original).toEqual({
        id: "one",
        title: "Example",
      });
    },
  );

  it("preserves a cell draft on data refresh but resets it on a source transition", () => {
    const draw = (workspaceId: string, title: string) => (
      <DataViewTable
        sourceIdentity={{
          workspaceId,
          namespace: "sample-records",
          sourceId: "one",
        }}
        capabilities={capabilities}
        rows={[{ id: "same", title }]}
        rowId={(row) => row.id}
        columns={columns}
        visibleColumnIds={["title"]}
        columnSizing={{}}
        onColumnSizingChange={vi.fn()}
        onReorderColumn={vi.fn()}
        meta={{}}
      />
    );
    const { rerender } = render(draw("first", "Original"));
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "Unsaved draft" },
    });
    rerender(draw("first", "Updated on server"));
    expect(screen.getByRole("textbox")).toHaveValue("Unsaved draft");
    rerender(draw("second", "Different workspace"));
    expect(screen.getByRole("textbox")).toHaveValue("Different workspace");
  });

  it("does not mount a layout the source does not support", () => {
    render(
      <DataViewTable
        sourceIdentity={{
          workspaceId: "ws",
          namespace: "sample",
          sourceId: "one",
        }}
        capabilities={{
          layouts: [],
          editing: "none",
          sideEffects: "none",
          sorting: "none",
        }}
        rows={[]}
        rowId={(row: SampleRow) => row.id}
        columns={columns}
        visibleColumnIds={["title"]}
        columnSizing={{}}
        onColumnSizingChange={vi.fn()}
        onReorderColumn={vi.fn()}
      />,
    );
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
