import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act, fireEvent, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import { issueStatusKeys } from "@multica/core/issue-statuses/queries";
import type { Issue, IssueStatus, IssueStatusEntry } from "@multica/core/types";
import type { IssueProperty } from "@multica/core/types";
import type { IssueGrouping } from "@multica/core/issues/stores/view-store";
import type { IssueGroupBranches } from "../surface/use-issue-group-branches";
import { propertyGroupId } from "../utils/drag-utils";
import { ListView } from "./list-view";
import { IssueContextMenuProvider } from "../actions";
import { ScrollRestorationProvider, type ScrollRestorationAdapter } from "../../platform";
import type { IssueStatusPagination } from "../surface/use-issue-status-branches";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";

const TEST_RESOURCES = { en: { common: enCommon, issues: enIssues } };

const mockToastInfo = vi.hoisted(() => vi.fn());
vi.mock("sonner", () => ({ toast: { info: mockToastInfo } }));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

const mockGetAgentTaskSnapshot = vi.hoisted(() => vi.fn().mockResolvedValue([]));
vi.mock("@multica/core/api", () => ({
  api: { getAgentTaskSnapshot: mockGetAgentTaskSnapshot },
  getApi: () => ({ getAgentTaskSnapshot: mockGetAgentTaskSnapshot }),
  setApiInstance: vi.fn(),
}));

vi.mock("@multica/core/paths", async () => {
  const actual =
    await vi.importActual<typeof import("@multica/core/paths")>("@multica/core/paths");
  return {
    ...actual,
    useWorkspaceSlug: () => "acme",
    useRequiredWorkspaceSlug: () => "acme",
    useWorkspacePaths: () => actual.paths.workspace("acme"),
  };
});

vi.mock("../../navigation", () => ({
  AppLink: ({ children, href, ...props }: any) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
  useNavigation: () => ({ push: vi.fn(), pathname: "/issues" }),
  resolveClickIntent: () => "push",
  useIntentNavigate: () => () => {},
  NavigationProvider: ({ children }: { children: React.ReactNode }) => children,
}));

const mockAuthUser = { id: "user-1", email: "test@test.com", name: "Test User" };
vi.mock("@multica/core/auth", () => ({
  useAuthStore: Object.assign(
    (selector?: any) => {
      const state = { user: mockAuthUser, isAuthenticated: true };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ user: mockAuthUser, isAuthenticated: true }) },
  ),
  registerAuthStore: vi.fn(),
  createAuthStore: vi.fn(),
}));

let mockProperties: IssueProperty[] = [];
const mockSetProperty = vi.fn();
const mockUnsetProperty = vi.fn();
const getActorName = () => "Alex";
vi.mock("@multica/core/properties", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/properties")>()),
  propertyListOptions: () => ({ queryKey: ["properties"], queryFn: async () => mockProperties, initialData: mockProperties }),
  useSetIssueProperty: () => ({ mutate: mockSetProperty }),
  useUnsetIssueProperty: () => ({ mutate: mockUnsetProperty }),
}));
vi.mock("@multica/core/workspace/hooks", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/workspace/hooks")>()),
  useActorName: () => ({ getActorName }),
}));

// View store. `listCollapsedStatuses` is mutable so `toggleListCollapsed`
// models the real store: the accordion's expanded set is derived from it.
const mockViewState: {
  grouping: IssueGrouping;
  listCollapsedGroups: string[];
  toggleListGroupCollapsed: (key: string) => void;
  sortBy: string;
  sortDirection: string;
  cardProperties: Record<string, boolean>;
  cardPropertyIds: string[];
  listCollapsedStatuses: IssueStatus[];
  toggleListCollapsed: (status: IssueStatus) => void;
  showStatus: (status: IssueStatus) => void;
} = {
  grouping: "status",
  listCollapsedGroups: [],
  toggleListGroupCollapsed: vi.fn((key: string) => {
    mockViewState.listCollapsedGroups = mockViewState.listCollapsedGroups.includes(key)
      ? mockViewState.listCollapsedGroups.filter((id) => id !== key)
      : [...mockViewState.listCollapsedGroups, key];
  }),
  sortBy: "position",
  sortDirection: "asc",
  cardProperties: {},
  cardPropertyIds: [],
  listCollapsedStatuses: [],
  toggleListCollapsed: vi.fn((status: IssueStatus) => {
    mockViewState.listCollapsedStatuses =
      mockViewState.listCollapsedStatuses.includes(status)
        ? mockViewState.listCollapsedStatuses.filter((s) => s !== status)
        : [...mockViewState.listCollapsedStatuses, status];
  }),
  showStatus: vi.fn(),
};

vi.mock("@multica/core/issues/stores/view-store-context", () => ({
  ViewStoreProvider: ({ children }: { children: React.ReactNode }) => children,
  useViewStore: (selector?: any) => (selector ? selector(mockViewState) : mockViewState),
  useViewStoreApi: () => ({
    getState: () => mockViewState,
    setState: vi.fn(),
    subscribe: vi.fn(),
  }),
}));

vi.mock("@multica/core/modals", () => ({
  useModalStore: Object.assign(
    () => ({ open: vi.fn() }),
    { getState: () => ({ open: vi.fn() }) },
  ),
}));

vi.mock("./priority-icon", () => ({
  PriorityIcon: () => <span data-testid="priority-icon" />,
}));

// Capture the DndContext callbacks so a test can drive dnd-kit's real
// lifecycle — including the cancel path, which never calls onDragEnd.
let lastOnDragStart: any = null;
let lastOnDragCancel: any = null;
let lastOnDragEnd: any = null;
let lastOnDragOver: any = null;
const stableSetNodeRef = () => {};

vi.mock("@dnd-kit/core", () => ({
  DndContext: ({ children, onDragStart, onDragCancel, onDragEnd, onDragOver }: any) => {
    lastOnDragStart = onDragStart;
    lastOnDragCancel = onDragCancel;
    lastOnDragEnd = onDragEnd;
    lastOnDragOver = onDragOver;
    return children;
  },
  DragOverlay: () => null,
  PointerSensor: class {},
  useSensor: () => ({}),
  useSensors: () => [],
  useDroppable: () => ({ setNodeRef: stableSetNodeRef, isOver: false }),
  pointerWithin: vi.fn(),
  closestCenter: vi.fn(),
}));

vi.mock("@dnd-kit/sortable", () => ({
  SortableContext: ({ children }: any) => children,
  verticalListSortingStrategy: {},
  arrayMove: <T,>(arr: T[]) => arr,
  useSortable: () => ({
    attributes: {},
    listeners: {},
    setNodeRef: vi.fn(),
    transform: null,
    transition: null,
    isDragging: false,
  }),
}));

vi.mock("@dnd-kit/utilities", () => ({
  CSS: { Transform: { toString: () => undefined } },
}));

// jsdom has no layout, so the real Virtuoso measures a 0-height viewport and
// renders nothing. Render rows inline so the panel's contents are assertable.
vi.mock("react-virtuoso", () => ({
  Virtuoso: ({ data, itemContent, defaultItemHeight, components }: any) => (
    <div data-testid="virtuoso-mock" data-estimated-height={defaultItemHeight}>
      {(data ?? []).map((item: any, i: number) => (
        <div key={i}>{itemContent(i, item)}</div>
      ))}
      {components?.Footer && <components.Footer />}
    </div>
  ),
}));

const ISSUES: Issue[] = [
  {
    id: "issue-1",
    identifier: "MUL-1",
    title: "First todo issue",
    status: "todo",
    priority: "none",
    position: 1,
    workspace_id: "ws-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } as Issue,
  {
    id: "issue-2",
    identifier: "MUL-2",
    title: "Second todo issue",
    status: "todo",
    priority: "none",
    position: 2,
    workspace_id: "ws-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } as Issue,
];

const emptyPage = {
  hasMore: false,
  isLoading: false,
  isFetching: false,
  isError: false,
  loadMore: vi.fn(),
  retry: vi.fn(),
};

const PAGINATION = {
  todo: { ...emptyPage, total: 2 },
  in_progress: { ...emptyPage, total: 1 },
  cancelled: { ...emptyPage, total: 1 },
} as unknown as IssueStatusPagination;

function renderListView(
  issues: Issue[] = ISSUES,
  visibleStatuses: IssueStatus[] = ["todo"],
  hiddenStatuses: IssueStatus[] = [],
  onMoveIssue = vi.fn(),
  statuses: IssueStatusEntry[] = [],
  scrollAdapter: ScrollRestorationAdapter = { get: () => undefined },
  groupBranches?: IssueGroupBranches,
  onCreateIssue?: () => void,
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  queryClient.setQueryData(issueStatusKeys.list("ws-1"), { statuses });
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider resources={TEST_RESOURCES} locale="en">
        <IssueContextMenuProvider>
          <ScrollRestorationProvider adapter={scrollAdapter}>
            <ListView
              issues={issues}
              visibleStatuses={visibleStatuses}
              hiddenStatuses={hiddenStatuses}
              statusPagination={PAGINATION}
              groupBranches={groupBranches}
              onCreateIssue={onCreateIssue}
              onMoveIssue={onMoveIssue}
            />
          </ScrollRestorationProvider>
        </IssueContextMenuProvider>
      </I18nProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockViewState.grouping = "status";
  mockViewState.listCollapsedGroups = [];
  mockProperties = [];
  mockSetProperty.mockClear();
  mockUnsetProperty.mockClear();
});

describe("ListView status header collapse", () => {
  beforeEach(() => {
    mockViewState.sortBy = "position";
    mockViewState.sortDirection = "asc";
    mockViewState.listCollapsedStatuses = [];
    mockViewState.cardProperties = { priority: true };
    vi.mocked(mockViewState.showStatus).mockClear();
    mockToastInfo.mockClear();
    lastOnDragStart = null;
    lastOnDragCancel = null;
    lastOnDragEnd = null;
  });

  it("honors the priority card-property toggle", () => {
    mockViewState.cardProperties = { priority: false };
    renderListView();

    expect(screen.queryByTestId("priority-icon")).not.toBeInTheDocument();
  });

  it.each([32, 36, 48])("reserves token-sized rows before restoring scroll and hands %ipx rows to Virtuoso", (height) => {
    // jsdom has no layout engine or Tailwind compiler. Supply the resolved
    // row height, but inspect the real seed DOM at the restoration boundary.
    const styles = document.createElement("style");
    styles.textContent = `[class~="group/row"] { height: ${height}px; }`;
    document.head.append(styles);
    const issues = Array.from({ length: 100 }, (_, index) => ({
      ...ISSUES[0]!, id: `issue-${index}`, identifier: `MUL-${index}`, position: index,
    }));
    let seedRows = 0;
    let spacerHeight: string | undefined;
    const get = vi.fn(() => {
      const scroller = document.querySelector('[data-tab-scroll-root="list"]')!;
      seedRows = scroller.querySelectorAll('[class~="group/row"]').length;
      spacerHeight = scroller.querySelector<HTMLElement>('div[aria-hidden="true"][style*="height"]')?.style.height;
      return { top: 2400, height: 600 };
    });
    try {
      renderListView(issues, ["todo"], [], vi.fn(), [], { get });
      expect(get).toHaveBeenCalledWith("list");
      expect(seedRows).toBe(30);
      expect(spacerHeight).toBe("calc(70 * var(--issue-row-height))");
      expect(screen.getByTestId("virtuoso-mock")).toHaveAttribute("data-estimated-height", String(height));
    } finally {
      styles.remove();
    }
  });

  it("shows hidden statuses with a recovery action", async () => {
    const user = userEvent.setup();
    renderListView(ISSUES, ["todo"], ["cancelled"]);

    expect(screen.getByText("Hidden columns")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show column" }));
    await user.click(await screen.findByRole("menuitem", { name: "Show column" }));

    expect(mockViewState.showStatus).toHaveBeenCalledWith("cancelled");
  });

  it("explains why same-status reordering is ignored under an automatic sort", () => {
    mockViewState.sortBy = "created_at";
    renderListView();

    act(() => {
      lastOnDragStart({ active: { id: "issue-1" } });
      lastOnDragEnd({
        active: { id: "issue-1" },
        over: { id: "issue-2" },
      });
    });

    expect(mockToastInfo).toHaveBeenCalledWith(
      "Switch to Manual ordering to rearrange issues within a column.",
      { id: "issue-manual-reorder-hint" },
    );
  });

  it("collapses a status group when its header is clicked", async () => {
    const user = userEvent.setup();
    renderListView();

    const trigger = screen.getByRole("button", { expanded: true });
    await user.click(trigger);

    expect(mockViewState.listCollapsedStatuses).toEqual(["todo"]);
  });

  it("still collapses after a drag is cancelled instead of dropped", async () => {
    const user = userEvent.setup();
    renderListView();

    // dnd-kit fires onDragCancel — never onDragEnd — when the browser aborts
    // an active pointer drag. On touch that happens on every scroll gesture
    // that starts on a row: the drag activates past the 5px threshold, then
    // the browser takes the gesture over and fires pointercancel. A cancel
    // that leaves the drag lock engaged makes the header's collapse toggle a
    // permanent no-op (MUL-6240).
    expect(lastOnDragCancel).toBeTypeOf("function");
    act(() => {
      lastOnDragStart({ active: { id: "issue-1" } });
      lastOnDragCancel();
    });

    const trigger = screen.getByRole("button", { expanded: true });
    await user.click(trigger);

    expect(mockViewState.listCollapsedStatuses).toEqual(["todo"]);
  });
});

// Custom statuses remain visible in independent sections.
describe("ListView custom statuses", () => {
  it.each([false, true])("only allows dropping into an active custom status (archived=%s)", (archived) => {
    const onMove = vi.fn();
    const status = {
      key: "awaiting_response", name: "Awaiting Response", category: "started",
      is_system: false, archived_at: archived ? "2026-01-01" : null,
    } as IssueStatusEntry;
    mockViewState.sortBy = "position";
    mockViewState.listCollapsedStatuses = [];
    renderListView(ISSUES, ["todo", "awaiting_response"], [], onMove, [status]);
    act(() => lastOnDragStart({ active: { id: "issue-1" } }));
    act(() => lastOnDragOver({
      active: { id: "issue-1" },
      over: { id: "status:awaiting_response" },
    }));
    act(() => lastOnDragEnd({
      active: { id: "issue-1" },
      over: { id: "status:awaiting_response" },
    }));
    if (archived) expect(onMove).not.toHaveBeenCalled();
    else expect(onMove).toHaveBeenCalledWith("issue-1", expect.objectContaining({ status: "awaiting_response" }), expect.any(Function));
  });

  it("renders a custom-status issue in its own status section", () => {
    const custom = {
      ...ISSUES[0]!,
      id: "issue-custom",
      identifier: "MUL-3",
      title: "Waiting on the reporter",
      status: "awaiting_response",
      status_category: "started",
    } as Issue;

    renderListView([custom], ["awaiting_response"]);

    expect(screen.getByText("Waiting on the reporter")).toBeInTheDocument();
  });
  it.each([
    { type: "number", value: 0, label: "0" },
    { type: "multi_select", value: ["beta", "alpha"], label: "Alpha, Beta" },
  ])("renders $type groups, collapses independently and creates with the typed value", async ({ type, value, label }) => {
    mockViewState.grouping = "property:field-1";
    mockViewState.sortBy = "created_at";
    mockProperties = [{ id: "field-1", name: "Field", type, config: { options: [
      { id: "alpha", name: "Alpha" }, { id: "beta", name: "Beta" },
    ] } } as IssueProperty];
    const issue = { ...ISSUES[0]!, properties: { "field-1": value } };
    const groupKey = propertyGroupId("field-1", value);
    const noneKey = propertyGroupId("field-1", null);
    const branches: IssueGroupBranches = {
      enabled: true, issues: [issue], total: 1, isLoading: false, isRefreshing: false, isError: false,
      hasMoreGroups: false, isLoadingMoreGroups: false, loadMoreGroups: vi.fn(), retryGroups: vi.fn(),
      descriptors: [
        { key: groupKey, count: 1, value: { kind: "property", property_id: "field-1", value_state: "value", value } },
        { key: noneKey, count: 0, value: { kind: "property", property_id: "field-1", value_state: "unset" } },
      ],
      pagination: {
        [groupKey]: { ...emptyPage, total: 1, loaded: 1 },
        [noneKey]: { ...emptyPage, total: 0, loaded: 0 },
      },
    };
    const create = vi.fn();
    const move = vi.fn();
    renderListView([], ["todo"], [], move, [], undefined, branches, create);
    const heading = screen.getByRole("button", { name: `${label} 1` });
    expect(screen.getByText(issue.title)).toBeVisible();
    expect(screen.getByRole("button", { name: "No value 0" })).toBeVisible();
    await userEvent.click(within(heading.parentElement!).getByRole("button", { name: "Add issue" }));
    expect(create).toHaveBeenCalledWith({ properties: { "field-1": value } });
    await userEvent.click(heading);
    expect(mockViewState.listCollapsedGroups).toEqual([groupKey]);
    expect(mockViewState.listCollapsedStatuses).toEqual([]);
    act(() => {
      lastOnDragStart({ active: { id: issue.id } });
      lastOnDragEnd({ active: { id: issue.id }, over: { id: noneKey } });
    });
    expect(mockUnsetProperty).toHaveBeenCalledWith({ issueId: issue.id, propertyId: "field-1" }, expect.anything());
    expect(move).toHaveBeenCalledWith(issue.id, expect.not.objectContaining({ status: expect.anything() }), expect.any(Function));
  });

});
