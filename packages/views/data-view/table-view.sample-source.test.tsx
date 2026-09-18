/** @vitest-environment jsdom */

import { useMemo, useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef, ColumnSizingState } from "@tanstack/react-table";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  dataSourceGroupQueryKey,
  dataSourceIdentityKey,
  dataSourceRowQueryKey,
  type DataSource,
  type DataSourceCellCommand,
  type DataSourceField,
  type DataSourceGroupPage,
  type DataSourcePageRequest,
} from "@multica/core/data-source";
import { DataViewCellEditor } from "./cell-editor";
import { useDataViewController } from "./controller";
import { useDataViewSelection } from "./selection";
import type {
  DataViewGroupBy,
  DataViewQueryBinding,
} from "./query-binding";
import type { DataViewStructuralRow } from "./table-rows";
import { TableView } from "./table-view";

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 41,
        end: (index + 1) * 41,
        size: 41,
      })),
    getTotalSize: () => count * 41,
    measureElement: () => {},
  }),
}));

type SampleRow = {
  key: string;
  caption: string;
  amount: number;
  bucket?: string;
  attributes: { flag: boolean };
};

type SampleQuery = {
  search: string;
  sort: { fieldId: "caption" | "amount"; direction: "asc" | "desc" };
};

const sampleLibraryKey = (workspaceId: string, sourceId: string) =>
  JSON.stringify([workspaceId, sourceId]);

function structuralActionLabel(key: string) {
  if (key.includes("groups")) return "Load more groups";
  if (!key.startsWith("activate:")) return "Load more rows";
  const [groupKey] = JSON.parse(key.slice("activate:".length)) as [
    string | null,
    string | null,
  ];
  const label = groupKey?.replace(/^opaque:/, "") ?? "ungrouped";
  return `Load rows for ${label}`;
}
type SampleReadQuery = {
  view: SampleQuery;
  groupBy: DataViewGroupBy;
  branch: { groupKey: string | null; parentRowId: string | null };
  hierarchy: boolean;
};
type SampleField = DataSourceField<SampleRow>;
type SampleCommand = DataSourceCellCommand<SampleRow>;
type SampleRawRowPage = {
  rows: SampleRow[];
  total: number;
  branchTotal: number;
  nextCursor: string | null;
};
type SampleDisplayRow =
  | { kind: "sample"; key: string; sourceRow: SampleRow }
  | DataViewStructuralRow;
type SampleSource = DataSource<
  SampleRow,
  SampleReadQuery,
  SampleCommand,
  SampleField,
  { branchTotal: number }
> & {
  readGroups(
    query: SampleQuery,
    groupBy: { fieldId: string },
    page: DataSourcePageRequest,
  ): Promise<DataSourceGroupPage>;
};

const sampleFields: SampleField[] = [
  {
    id: "caption",
    label: "Caption",
    kind: "text",
    value: (row) => row.caption,
    sortable: true,
    filterable: false,
    groupable: false,
    canSet: () => true,
    canClear: () => true,
  },
  {
    id: "amount",
    label: "Amount",
    kind: "number",
    value: (row) => row.amount,
    sortable: true,
    filterable: false,
    groupable: false,
    canSet: () => true,
    canClear: () => false,
  },
  {
    id: "bucket",
    label: "Bucket",
    kind: "select",
    value: (row) => row.bucket,
    sortable: true,
    filterable: false,
    groupable: true,
    canSet: () => true,
    canClear: (row) => row.bucket !== undefined,
    options: [
      { id: "alpha", label: "Alpha", value: "alpha" },
      { id: "beta", label: "Beta", value: "beta" },
    ],
  },
  {
    id: "flag",
    label: "Flag",
    kind: "checkbox",
    value: (row) => row.attributes.flag,
    sortable: false,
    filterable: false,
    groupable: true,
    canSet: () => true,
    canClear: () => false,
  },
];

class SampleLibrary {
  readonly rows = new Map<string, SampleRow[]>();
  readonly reads: string[] = [];
  readonly writes: string[] = [];
  failNext = false;

  seed(workspaceId: string, sourceId: string, caption: string) {
    this.rows.set(sampleLibraryKey(workspaceId, sourceId), [
      {
        key: "same-row",
        caption,
        amount: 1,
        bucket: "alpha",
        attributes: { flag: false },
      },
      {
        key: "second",
        caption: `${caption} 2`,
        amount: 2,
        bucket: "beta",
        attributes: { flag: true },
      },
      {
        key: "third",
        caption: `${caption} 3`,
        amount: 3,
        attributes: { flag: false },
      },
      {
        key: "fourth",
        caption: `${caption} 4`,
        amount: 4,
        bucket: "alpha",
        attributes: { flag: false },
      },
      {
        key: "fifth",
        caption: `${caption} 5`,
        amount: 5,
        bucket: "alpha",
        attributes: { flag: true },
      },
    ]);
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function filteredRows(rows: SampleRow[], query: SampleQuery) {
  const direction = query.sort.direction === "asc" ? 1 : -1;
  return rows
    .filter((row) =>
      row.caption.toLowerCase().includes(query.search.toLowerCase()),
    )
    .sort((left, right) => {
      const a = left[query.sort.fieldId];
      const b = right[query.sort.fieldId];
      return String(a).localeCompare(String(b), undefined, { numeric: true }) * direction;
    });
}

function createSampleSource(
  library: SampleLibrary,
  workspaceId: string,
  sourceId: string,
  writable = true,
): SampleSource {
  const libraryKey = sampleLibraryKey(workspaceId, sourceId);
  return {
    identity: { workspaceId, namespace: "sample", sourceId },
    key: sourceId,
    fields: sampleFields,
    capabilities: {
      layouts: ["table"],
      grouping: true,
      hierarchy: false,
      writable,
      maxPageSize: 2,
    },
    rowId: (row) => row.key,
    read: async (request, page) => {
      library.reads.push(
        JSON.stringify({ libraryKey, request, cursor: page.cursor ?? null }),
      );
      const all = filteredRows(library.rows.get(libraryKey) ?? [], request.view);
      const branchRows = request.groupBy
        ? all.filter(
            (row) =>
              `opaque:${row.bucket ?? "unset"}` === request.branch.groupKey,
          )
        : all;
      const start = Number(page.cursor ?? 0);
      const limit = Math.min(page.limit ?? 2, 2);
      return {
        rows: branchRows
          .slice(start, start + limit)
          .map((row) => structuredClone(row)),
        total: all.length,
        nextCursor:
          start + limit < branchRows.length ? String(start + limit) : null,
        metadata: { branchTotal: branchRows.length },
      };
    },
    readGroups: async (query, groupBy, page) => {
      if (groupBy.fieldId !== "bucket") throw new Error("unsupported group");
      const rows = filteredRows(library.rows.get(libraryKey) ?? [], query);
      const groups = ["alpha", "beta", "unset"].flatMap((value) => {
        const count = rows.filter(
          (row) => (row.bucket ?? "unset") === value,
        ).length;
        return count === 0
          ? []
          : [
              {
                key: `opaque:${value}`,
                label:
                  value === "alpha"
                    ? "Alpha"
                    : value === "beta"
                      ? "Beta"
                      : "No bucket",
                count,
                valueState: value === "unset" ? "unset" as const : "value" as const,
                ...(value === "unset" ? {} : { value }),
              },
            ];
      });
      const start = Number(page.cursor ?? 0);
      const limit = Math.min(page.limit ?? 1, 1);
      return {
        groups: groups.slice(start, start + limit),
        total: rows.length,
        nextCursor: start + limit < groups.length ? String(start + limit) : null,
      };
    },
    execute: async (command) => {
      const field = sampleFields.find(
        (candidate) => candidate.id === command.fieldId,
      );
      const allowed =
        writable &&
        field &&
        (command.change.op === "clear"
          ? field.canClear(command.row)
          : field.canSet(command.row));
      if (!allowed) {
        return { status: "failed", error: new Error("read-only") };
      }
      if (library.failNext) {
        library.failNext = false;
        return { status: "failed", error: new Error("retry me") };
      }
      const row = (library.rows.get(libraryKey) ?? []).find(
        (candidate) => candidate.key === command.row.key,
      );
      if (!row) return { status: "failed", error: new Error("missing row") };
      const value =
        command.change.op === "set" ? command.change.value : undefined;
      if (command.fieldId === "caption") row.caption = String(value ?? "");
      else if (command.fieldId === "amount") row.amount = Number(value);
      else if (command.fieldId === "bucket") {
        if (typeof value === "string") row.bucket = value;
        else delete row.bucket;
      } else if (command.fieldId === "flag") {
        row.attributes.flag = value === true;
      }
      library.writes.push(`${libraryKey}:${row.key}:${command.fieldId}`);
      return { status: "accepted", row: structuredClone(row) };
    },
  };
}

function createSampleBinding(
  source: SampleSource,
): DataViewQueryBinding<
  SampleRow,
  SampleQuery,
  SampleRawRowPage,
  DataSourceGroupPage
> {
  return {
    identity: source.identity,
    rowPageKey: ({ query, groupBy, branch, hierarchy, page }) =>
      dataSourceRowQueryKey({
        identity: source.identity,
        query,
        groupBy,
        branch,
        hierarchy,
        page,
      }),
    rowBranchKey: ({ query, groupBy, branch, hierarchy }) => [
      ...dataSourceIdentityKey(source.identity),
      "rows",
      query,
      groupBy,
      branch,
      hierarchy,
    ],
    readRowPage: async ({ query, groupBy, branch, hierarchy, page }, signal) => {
      const result = await source.read(
        { view: query, groupBy, branch, hierarchy },
        page,
        signal,
      );
      return {
        rows: result.rows,
        total: result.total,
        branchTotal: result.metadata.branchTotal,
        nextCursor: result.nextCursor,
      };
    },
    mapRowPage: (page) => page,
    groupPagesKey: ({ query, groupBy }) =>
      dataSourceGroupQueryKey({
        identity: source.identity,
        query,
        groupBy,
        page: { limit: 1, cursor: null },
      }).slice(0, -2),
    readGroupPage: ({ query, groupBy, page }) =>
      source.readGroups(query, groupBy, page),
    mapGroupPage: (page) => page,
  };
}

function SampleTable({ source }: { source: SampleSource }) {
  const queryClient = useQueryClient();
  const [groupBy, setGroupBy] = useState<DataViewGroupBy>(null);
  const [sort, setSort] = useState<SampleQuery["sort"]>({
    fieldId: "amount",
    direction: "asc",
  });
  const [collapsedGroups, setCollapsedGroups] = useState(new Set<string>());
  const [selected, setSelected] = useState(new Set<string>());
  const binding = useMemo(() => createSampleBinding(source), [source]);
  const query = useMemo(() => ({ search: "", sort }), [sort]);
  const dataView = useDataViewController({
    binding,
    query,
    groupBy,
    hierarchy: false,
    collapsedGroupKeys: collapsedGroups,
    collapsedRowIds: new Set(),
    rowId: source.rowId,
    directChildCount: () => 0,
    projectRow: ({ row }) => ({
      kind: "sample" as const,
      key: row.key,
      sourceRow: row,
    }),
    rowPageSize: 2,
    groupPageSize: 1,
    skeletonCount: 2,
  });
  const visibleRowIds = dataView.rows.flatMap((row) =>
    row.kind === "sample" ? [row.key] : [],
  );
  const handleSelection = useDataViewSelection({
    sourceIdentity: source.identity,
    rowIds: visibleRowIds,
    selectedIds: selected,
    select: (ids) => setSelected((previous) => new Set([...previous, ...ids])),
    deselect: (ids) =>
      setSelected((previous) => {
        const next = new Set(previous);
        for (const id of ids) next.delete(id);
        return next;
      }),
    toggle: (id) =>
      setSelected((previous) => {
        const next = new Set(previous);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      }),
    clear: () => setSelected(new Set()),
  });
  const columns = useMemo<ColumnDef<SampleDisplayRow>[]>(
    () => [
      {
        id: "select",
        header: "Select",
        cell: ({ row }) =>
          row.original.kind === "sample" ? (
            <input
              type="checkbox"
              aria-label={`Select ${row.original.sourceRow.caption}`}
              checked={selected.has(row.original.key)}
              onClick={(event) =>
                handleSelection(row.original.key, event.shiftKey)
              }
              onChange={() => {}}
            />
          ) : null,
      },
      ...source.fields.map(
        (field): ColumnDef<SampleDisplayRow> => ({
          id: field.id,
          header: field.label,
          cell: ({ row }) =>
            row.original.kind === "sample" ? (
              <DataViewCellEditor
                source={source}
                field={field}
                row={row.original.sourceRow}
                saveLabel="Save"
                clearLabel="Clear"
                onAccepted={() =>
                  queryClient.invalidateQueries({
                    queryKey: dataSourceIdentityKey(source.identity),
                  })
                }
              />
            ) : null,
        }),
      ),
    ],
    [handleSelection, queryClient, selected, source],
  );

  return (
    <>
      <button type="button" onClick={() => setGroupBy({ fieldId: "bucket" })}>
        Group by bucket
      </button>
      <button type="button" onClick={() => setGroupBy(null)}>
        Ungroup
      </button>
      <button
        type="button"
        onClick={() => setSort({ fieldId: "amount", direction: "desc" })}
      >
        Sort amount descending
      </button>
      <output aria-label="Selected count">{selected.size}</output>
      <TableView
        sourceIdentity={source.identity}
        writable={source.capabilities.writable}
        rows={dataView.rows}
        columns={columns}
        rowId={(row) => row.key}
        visibleColumnIds={["select", ...source.fields.map((field) => field.id)]}
        columnSizing={{} as ColumnSizingState}
        onColumnSizingChange={() => {}}
        onReorderColumn={() => {}}
        renderStructuralRow={(row) => {
          const value = row.original;
          if (value.kind === "group") {
            return {
              content: (
                <button
                  type="button"
                  onClick={() =>
                    setCollapsedGroups((previous) => {
                      const next = new Set(previous);
                      if (next.has(value.key)) next.delete(value.key);
                      else next.add(value.key);
                      return next;
                    })
                  }
                >
                  {value.label} ({value.count})
                </button>
              ),
            };
          }
          if (value.kind === "load_more") {
            return {
              content: (
                <button
                  type="button"
                  disabled={value.state === "loading"}
                  onClick={value.onLoad}
                >
                  {structuralActionLabel(value.key)}
                </button>
              ),
            };
          }
          return value.kind === "skeleton" ? { content: "Loading" } : null;
        }}
      />
    </>
  );
}

function renderSample(
  source: SampleSource,
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  }),
) {
  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <SampleTable source={source} />
      </QueryClientProvider>,
    ),
  };
}

afterEach(cleanup);

describe("shared TableView with a non-Issue source", () => {
  it("reads independent pages and maps text, number, select and checkbox writes", async () => {
    const user = userEvent.setup();
    const library = new SampleLibrary();
    library.seed("ws-a", "sample-a", "Alpha row");
    const source = createSampleSource(library, "ws-a", "sample-a");
    renderSample(source);

    await screen.findByDisplayValue("Alpha row");
    const caption = screen.getAllByLabelText("Caption")[0]!;
    await user.clear(caption);
    await user.type(caption, "Updated caption");
    fireEvent.submit(caption.closest("form")!);
    await waitFor(() =>
      expect(
        library.rows.get(sampleLibraryKey("ws-a", "sample-a"))?.[0]
          ?.caption,
      ).toBe("Updated caption"),
    );
    await screen.findByDisplayValue("Updated caption");

    const amount = screen.getAllByLabelText("Amount")[0]!;
    fireEvent.change(amount, { target: { value: "1.5" } });
    fireEvent.submit(amount.closest("form")!);
    await waitFor(() =>
      expect(
        library.rows.get(sampleLibraryKey("ws-a", "sample-a"))?.[0]
          ?.amount,
      ).toBe(1.5),
    );
    fireEvent.change(screen.getAllByLabelText("Bucket")[0]!, {
      target: { value: "beta" },
    });
    await waitFor(() =>
      expect(
        library.rows.get(sampleLibraryKey("ws-a", "sample-a"))?.[0]
          ?.bucket,
      ).toBe("beta"),
    );
    await user.click(screen.getAllByLabelText("Flag")[0]!);
    await waitFor(() => {
      const row = library.rows.get(
        sampleLibraryKey("ws-a", "sample-a"),
      )?.[0];
      expect(row?.attributes.flag).toBe(true);
    });
    fireEvent.change(screen.getAllByLabelText("Bucket")[0]!, {
      target: { value: "" },
    });
    await waitFor(() =>
      expect(
        library.rows.get(sampleLibraryKey("ws-a", "sample-a"))?.[0]
          ?.bucket,
      ).toBeUndefined(),
    );

    await user.click(screen.getByRole("button", { name: "Load more rows" }));
    expect(await screen.findByDisplayValue("Alpha row 3")).toBeInTheDocument();
    await user.click(screen.getByLabelText("Select Alpha row 3"));
    expect(screen.getByLabelText("Selected count")).toHaveTextContent("1");
    expect(library.reads.length).toBeGreaterThanOrEqual(6);
  });

  it("pages groups and rows independently and applies source-side sorting", async () => {
    const user = userEvent.setup();
    const library = new SampleLibrary();
    library.seed("ws-a", "sample-a", "Grouped row");
    renderSample(createSampleSource(library, "ws-a", "sample-a"));
    await screen.findByDisplayValue("Grouped row");

    await user.click(screen.getByRole("button", { name: "Group by bucket" }));
    expect(await screen.findByRole("button", { name: "Alpha (3)" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Load more groups" }));
    expect(await screen.findByRole("button", { name: "Beta (1)" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Load rows for alpha" }));
    expect(await screen.findByDisplayValue("Grouped row")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Load more rows" }));
    expect(await screen.findByDisplayValue("Grouped row 5")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Ungroup" }));
    await user.click(
      screen.getByRole("button", { name: "Sort amount descending" }),
    );
    expect((await screen.findAllByLabelText("Amount"))[0]).toHaveValue(5);
  });

  it("retains a failed draft, retries explicitly, and blocks a read-only source", async () => {
    const user = userEvent.setup();
    const library = new SampleLibrary();
    library.seed("ws-a", "sample-a", "Original");
    library.failNext = true;
    const source = createSampleSource(library, "ws-a", "sample-a");
    const rendered = renderSample(source);

    const caption = await screen.findByDisplayValue("Original");
    await user.clear(caption);
    await user.type(caption, "Draft survives");
    fireEvent.submit(caption.closest("form")!);
    expect(await screen.findByRole("alert")).toHaveTextContent("retry me");
    expect(caption).toHaveValue("Draft survives");
    fireEvent.submit(caption.closest("form")!);
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(library.writes).toHaveLength(1);

    rendered.rerender(
      <QueryClientProvider client={rendered.queryClient}>
        <SampleTable
          source={createSampleSource(library, "ws-a", "sample-a", false)}
        />
      </QueryClientProvider>,
    );
    await screen.findByText("Draft survives");
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(library.writes).toHaveLength(1);
  });

  it("isolates equal row, field and query ids across sources, workspaces and remounts", async () => {
    const user = userEvent.setup();
    const library = new SampleLibrary();
    // These two identities collided under the former `parts.join(":")` key.
    library.seed("ws:sample", "same", "Workspace A");
    library.seed("ws", "sample:same", "Source B");
    library.seed("ws-b", "same", "Workspace B");
    const first = renderSample(
      createSampleSource(library, "ws:sample", "same"),
    );
    expect(await screen.findByDisplayValue("Workspace A")).toBeInTheDocument();
    await user.click(screen.getByLabelText("Select Workspace A"));
    expect(screen.getByLabelText("Selected count")).toHaveTextContent("1");
    const caption = screen.getAllByLabelText("Caption")[0]!;
    await user.clear(caption);
    await user.type(caption, "Persisted A");
    fireEvent.submit(caption.closest("form")!);
    await waitFor(() =>
      expect(
        library.rows.get(sampleLibraryKey("ws:sample", "same"))?.[0]
          ?.caption,
      ).toBe("Persisted A"),
    );

    first.rerender(
      <QueryClientProvider client={first.queryClient}>
        <SampleTable
          source={createSampleSource(library, "ws", "sample:same")}
        />
      </QueryClientProvider>,
    );
    expect(await screen.findByDisplayValue("Source B")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Persisted A")).toBeNull();
    await waitFor(() =>
      expect(screen.getByLabelText("Selected count")).toHaveTextContent("0"),
    );

    first.rerender(
      <QueryClientProvider client={first.queryClient}>
        <SampleTable source={createSampleSource(library, "ws-b", "same")} />
      </QueryClientProvider>,
    );
    expect(await screen.findByDisplayValue("Workspace B")).toBeInTheDocument();

    first.unmount();
    renderSample(
      createSampleSource(library, "ws:sample", "same"),
      new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    );
    expect(await screen.findByDisplayValue("Persisted A")).toBeInTheDocument();
  });

  it("keeps a late write and its invalidation scoped to the source that started it", async () => {
    const user = userEvent.setup();
    const library = new SampleLibrary();
    library.seed("ws-a", "source-a", "Source A");
    library.seed("ws-a", "source-b", "Source B");
    const sourceA = createSampleSource(library, "ws-a", "source-a");
    const originalExecute = sourceA.execute;
    const release = deferred<void>();
    sourceA.execute = async (command) => {
      await release.promise;
      return originalExecute(command);
    };
    const rendered = renderSample(sourceA);
    const caption = await screen.findByDisplayValue("Source A");
    await user.clear(caption);
    await user.type(caption, "Late A");
    fireEvent.submit(caption.closest("form")!);

    rendered.rerender(
      <QueryClientProvider client={rendered.queryClient}>
        <SampleTable
          source={createSampleSource(library, "ws-a", "source-b")}
        />
      </QueryClientProvider>,
    );
    expect(await screen.findByDisplayValue("Source B")).toBeInTheDocument();
    await act(async () => {
      release.resolve();
      await release.promise;
    });
    await waitFor(() =>
      expect(
        library.rows.get(sampleLibraryKey("ws-a", "source-a"))?.[0]
          ?.caption,
      ).toBe("Late A"),
    );
    expect(
      library.rows.get(sampleLibraryKey("ws-a", "source-b"))?.[0]?.caption,
    ).toBe("Source B");
    expect(screen.getByDisplayValue("Source B")).toBeInTheDocument();
  });
});
