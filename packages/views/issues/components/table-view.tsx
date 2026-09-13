"use client";

import { useStatusLabel } from "../utils/status-label";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useDndContext } from "@dnd-kit/core";
import { useSortable } from "@dnd-kit/sortable";
import {
  type CellContext,
  type ColumnDef,
  type ColumnSizingState,
  type HeaderContext,
  type OnChangeFn,
  type Table as TanstackTable,
  type TableMeta,
} from "@tanstack/react-table";
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  ChevronRight,
  Download,
  EyeOff,
  GripVertical,
  Loader2,
  Pencil,
  Plus,
  Search,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { cn } from "@multica/ui/lib/utils";
import { ApiError } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import { useModalStore } from "@multica/core/modals";
import {
  issueKeys,
} from "@multica/core/issues/queries";
import { createIssueTableDataSource } from "@multica/core/issues/table-data-source";
import type { IssueTableField } from "@multica/core/issues/table-data-source";
import {
  dataSourceIdentityString,
  type DataSourceCellChange,
} from "@multica/core/data-source";
import {
  assertWorkspaceRequestContext,
  getCurrentSlug,
  type WorkspaceRequestContext,
} from "@multica/core/platform";
import {
  TABLE_SYSTEM_COLUMNS,
  propertyIdFromViewKey,
  type SortField,
  type TableColumnKey,
  type TableSystemColumnKey,
} from "@multica/core/issues/stores/view-store";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import {
  propertyListOptions,
  useSetIssueProperty,
  useUnsetIssueProperty,
} from "@multica/core/properties";
import { projectListOptions } from "@multica/core/projects/queries";
import {
  useAttachLabelToIssue,
  useDetachLabelFromIssue,
} from "@multica/core/labels";
import { useWorkspacePaths } from "@multica/core/paths";
import { buildActorNameResolver, useActorName } from "@multica/core/workspace/hooks";
import {
  agentListOptions,
  memberListOptions,
  squadListOptions,
} from "@multica/core/workspace/queries";
import type {
  Issue,
  IssueProperty,
  IssuePropertyValue,
  IssueTableGroupDescriptor,
  IssueTableGroupsResponse,
  IssueTableGroupSpec,
  IssueTableQuerySpec,
  IssueTableRowsResponse,
  IssueTableRow,
  Project,
  UpdateIssueRequest,
} from "@multica/core/types";
import {
  actorRefsFromValue,
  formatActorRef,
  isActorPropertyType,
} from "@multica/core/types";
import {
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { createIssueTableCommandExecutor } from "../actions/table-command-executor";
import { ActorAvatar } from "../../common/actor-avatar";
import { LabelChip } from "../../labels/label-chip";
import { resolveClickIntent, useIntentNavigate } from "../../navigation";
import { ProjectPicker } from "../../projects/components/project-picker";
import { useT } from "../../i18n";
import { useIssueSurfaceActionsOptional } from "../surface/actions-context";
import { useIssueSurfaceSelection } from "../surface/selection-context";
import type { IssueCreateDefaults } from "../surface/types";
import { ProgressRing } from "./progress-ring";
import {
  AssigneePicker,
  DueDatePicker,
  LabelPicker,
  PriorityPicker,
  StartDatePicker,
  StatusPicker,
} from "./pickers";
import { CustomPropertyValueInput } from "./pickers/custom-property-picker";
import {
  buildIssueTableCsv,
  IssueTableExportIntegrityError,
  refreshFrozenTableRows,
  type IssueTableDisplayRow,
} from "./table-view-model";
import type { ChildProgress } from "./list-row";
import { ListLoadMoreFooter } from "./list-load-more-footer";
import { IssueAgentActivityIndicator } from "./issue-agent-activity-indicator";
import {
  TableView as SharedTableView,
  useDataViewController,
  useDataViewSelection,
  type DataViewQueryBinding,
} from "../../data-view";

// Enough placeholder rows to cover a typical viewport; the virtualizer only
// mounts what fits, so overshooting costs nothing.
const SKELETON_ROW_COUNT = 12;

const SELECT_COLUMN_ID = "__select";
const ADD_COLUMN_ID = "__add";

type TableViewProps = {
  serverQuery: IssueTableQuerySpec;
  childProgressMap: Map<string, ChildProgress>;
  search: string;
  onSearchChange: (query: string) => void;
  onLoadedIssuesChange: (issues: Issue[]) => void;
  onCreateIssue: (defaults: IssueCreateDefaults) => void;
  exportIssues: () => Promise<Issue[]>;
  resolveExportLookups: (needs: {
    projects: boolean;
    childProgress: boolean;
  }) => Promise<{
    projectMap: Map<string, Project>;
    childProgressMap: Map<string, ChildProgress>;
  }>;
};

type LoadedIssueState = {
  membershipIdentity: string;
  issues: Map<string, Issue>;
};

function tableGroupSpec(grouping: string): IssueTableGroupSpec {
  if (grouping === "status") return { kind: "status" };
  if (grouping === "assignee") return { kind: "assignee" };
  if (grouping === "project") return { kind: "project" };
  const propertyId = propertyIdFromViewKey(grouping);
  if (propertyId) return { kind: "property", property_id: propertyId };
  return { kind: "none" };
}

type ColumnLabelKey =
  | "title"
  | "identifier"
  | "status"
  | "priority"
  | "assignee"
  | "labels"
  | "project"
  | "start_date"
  | "due_date"
  | "created_at"
  | "updated_at"
  | "child_progress"
  | "creator";

const SORTABLE_COLUMNS: Partial<Record<TableSystemColumnKey, SortField>> = {
  title: "title",
  status: "status",
  priority: "priority",
  start_date: "start_date",
  due_date: "due_date",
  created_at: "created_at",
  updated_at: "updated_at",
};

function stopRowNavigation(event: React.SyntheticEvent) {
  event.stopPropagation();
}

function SelectAllCheckbox({
  issueIds,
  label,
}: {
  issueIds: string[];
  label: string;
}) {
  const selection = useIssueSurfaceSelection();
  const ref = useRef<HTMLInputElement>(null);
  const selectedCount = issueIds.filter((id) => selection.selectedIds.has(id)).length;
  const checked = issueIds.length > 0 && selectedCount === issueIds.length;

  useEffect(() => {
    if (ref.current) {
      ref.current.indeterminate = selectedCount > 0 && !checked;
    }
  }, [checked, selectedCount]);

  return (
    <input
      ref={ref}
      type="checkbox"
      aria-label={label}
      checked={checked}
      onChange={() =>
        checked ? selection.deselect(issueIds) : selection.select(issueIds)
      }
      className="size-3.5 cursor-pointer accent-primary"
    />
  );
}

function IssueCheckbox({
  checked,
  label,
  onToggle,
}: {
  checked: boolean;
  label: string;
  onToggle: (shiftKey: boolean) => void;
}) {
  return (
    <input
      type="checkbox"
      aria-label={label}
      checked={checked}
      onClick={(event) => {
        event.stopPropagation();
        onToggle(event.shiftKey);
      }}
      onAuxClick={stopRowNavigation}
      onChange={() => undefined}
      className="size-3.5 cursor-pointer accent-primary"
    />
  );
}

function SortableColumnHeader({
  columnKey,
  label,
  sortField,
  sortBy,
  sortDirection,
  onSort,
  onHide,
  ascendingLabel,
  descendingLabel,
  hideLabel,
  reorderLabel,
}: {
  columnKey: TableColumnKey;
  label: string;
  sortField?: SortField;
  sortBy: SortField;
  sortDirection: "asc" | "desc";
  onSort: (field: SortField, direction: "asc" | "desc") => void;
  onHide?: () => void;
  ascendingLabel: string;
  descendingLabel: string;
  hideLabel: string;
  reorderLabel: string;
}) {
  const sortable = columnKey !== "title";
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: columnKey, disabled: !sortable });
  const active = sortField === sortBy;
  // Any column in flight, not only this one: the neighbours shift to open a
  // gap, and each is clipped by its own cell just the same.
  const isReordering = useDndContext().active != null;
  const nodeRef = useRef<HTMLDivElement | null>(null);

  // The cell clips its own content, which is what made a dragged column look
  // like it vanished rather than travelled. The clip only earns its keep at
  // rest, capping a label wider than its column, so it is lifted for the length
  // of a reorder and the column in hand is raised over its neighbours.
  //
  // Only overflow and stacking are touched. Transforming the <th> itself would
  // carry the header's full height along, but `transform` on a table cell is a
  // corner of the spec browsers take liberties with — Chromium lifts the cell
  // out of the table's box model and its geometry stops matching the row. The
  // wrapper below is padded out to the cell's size instead.
  useLayoutEffect(() => {
    const cell = nodeRef.current?.closest("th");
    if (!cell || !isReordering) return;
    cell.style.overflow = "visible";
    if (isDragging) cell.style.zIndex = "20";
    return () => {
      cell.style.removeProperty("overflow");
      cell.style.removeProperty("z-index");
    };
  }, [isDragging, isReordering]);

  return (
    <div
      ref={(node) => {
        nodeRef.current = node;
        setNodeRef(node);
      }}
      // Horizontal travel only. dnd-kit's layout animation also hands back
      // scaleX/scaleY — old rect over new rect — to tween an item into the
      // shape of the slot it landed in. Between two tabs of equal width that
      // ratio is 1 and never shows; between two columns it is not, so a 174px
      // column swapping with a 96px one gets stretched to 1.8x on the way.
      // Reordering columns changes no column's width, so there is nothing for
      // a shape tween to say here. The move and the settle stay animated
      // through `transition`.
      style={{
        transform: transform ? `translate3d(${transform.x}px, 0, 0)` : undefined,
        transition,
      }}
      // The wrapper spans the cell's own box — the negative margins undo the
      // <th>'s padding and put it back inside — so it renders exactly as at
      // rest while being what travels: a header-sized block rather than the
      // line of text in it. Height is derived rather than fixed at h-8: the
      // strip is taller than the cell's nominal height once row borders are in.
      className={cn(
        "group/header -mx-4 -my-2 flex h-[calc(100%+1rem)] min-w-0 items-center px-4",
        isDragging && "opacity-60",
      )}
    >
      {sortable && (
        <button
          type="button"
          aria-label={reorderLabel}
          className={cn(
            "-ml-2 mr-0.5 rounded p-0.5 text-muted-foreground opacity-0 hover:bg-accent hover:text-muted-foreground group-hover/header:opacity-100 focus-visible:opacity-100",
            isDragging ? "cursor-grabbing opacity-100" : "cursor-grab",
          )}
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-3" />
        </button>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger className="flex min-w-0 items-center gap-1 rounded px-1.5 py-1 hover:bg-accent">
          <span className="truncate">{label}</span>
          {active &&
            (sortDirection === "asc" ? (
              <ArrowUp className="size-3 shrink-0" />
            ) : (
              <ArrowDown className="size-3 shrink-0" />
            ))}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-40">
          {sortField && (
            <>
              <DropdownMenuItem onClick={() => onSort(sortField, "asc")}>
                <ArrowUp />
                {ascendingLabel}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => onSort(sortField, "desc")}>
                <ArrowDown />
                {descendingLabel}
              </DropdownMenuItem>
            </>
          )}
          {sortField && onHide && <DropdownMenuSeparator />}
          {onHide && (
            <DropdownMenuItem onClick={onHide}>
              <EyeOff />
              {hideLabel}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

export function TableColumnPicker({
  properties,
  trigger,
}: {
  properties: IssueProperty[];
  trigger: React.ReactElement;
}) {
  const { t } = useT("issues");
  const [search, setSearch] = useState("");
  const tableColumns = useViewStore((state) => state.tableColumns);
  const toggleTableColumn = useViewStore((state) => state.toggleTableColumn);
  const selected = useMemo(
    () => new Set(tableColumns.map((column) => column.key)),
    [tableColumns],
  );
  const query = search.trim().toLocaleLowerCase();
  const systemColumns = TABLE_SYSTEM_COLUMNS.filter((key) =>
    t(($) => $.table.columns[key as ColumnLabelKey])
      .toLocaleLowerCase()
      .includes(query),
  );
  const visibleProperties = properties.filter((property) =>
    property.name.toLocaleLowerCase().includes(query),
  );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={trigger} />
      <DropdownMenuContent align="end" className="w-64 p-0">
        <div className="border-b p-2">
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            onKeyDown={(event) => {
              if (event.key !== "Escape") event.stopPropagation();
            }}
            placeholder={t(($) => $.table.columns.search_placeholder)}
            className="h-7"
          />
        </div>
        <div className="max-h-80 overflow-y-auto p-1">
          {systemColumns.length > 0 && (
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                {t(($) => $.table.columns.system_section)}
              </DropdownMenuLabel>
              {systemColumns.map((key) => (
                <DropdownMenuCheckboxItem
                  key={key}
                  disabled={key === "title"}
                  checked={selected.has(key)}
                  onCheckedChange={() => toggleTableColumn(key)}
                >
                  {t(($) => $.table.columns[key as ColumnLabelKey])}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuGroup>
          )}
          {visibleProperties.length > 0 && (
            <>
              {systemColumns.length > 0 && <DropdownMenuSeparator />}
              <DropdownMenuGroup>
                <DropdownMenuLabel>
                  {t(($) => $.table.columns.property_section)}
                </DropdownMenuLabel>
                {visibleProperties.map((property) => {
                  const key = `property:${property.id}` as const;
                  return (
                    <DropdownMenuCheckboxItem
                      key={property.id}
                      checked={selected.has(key)}
                      onCheckedChange={() => toggleTableColumn(key)}
                    >
                      <span className="truncate">{property.name}</span>
                    </DropdownMenuCheckboxItem>
                  );
                })}
              </DropdownMenuGroup>
            </>
          )}
          {systemColumns.length === 0 && visibleProperties.length === 0 && (
            <p className="px-2 py-6 text-center text-caption text-muted-foreground">
              {t(($) => $.table.columns.no_results)}
            </p>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function TableIssueSearch({
  value,
  onChange,
  placeholder,
  clearLabel,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  clearLabel: string;
}) {
  return (
    <div className="relative w-56 shrink-0">
      <Search
        aria-hidden
        className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        type="text"
        role="searchbox"
        inputMode="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        aria-label={placeholder}
        placeholder={placeholder}
        className="h-7 pl-7 pr-7 text-caption"
      />
      {value && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={clearLabel}
          onClick={() => onChange("")}
          className="absolute right-0.5 top-0.5 text-muted-foreground"
        >
          <X className="size-3" />
        </Button>
      )}
    </div>
  );
}

export function InlineTitle({
  row,
  editing,
  onEditingChange,
  onUpdate,
  onOpen,
  onCreateSubIssue,
  onToggleParent,
  toggleLabel,
  renameLabel,
  createSubIssueLabel,
  writable = true,
}: {
  row: Extract<IssueTableDisplayRow, { kind: "issue" }>;
  /** Rename state is owned by the table (one editor at a time) so it also
   *  survives cell remounts and drives the structure freeze. */
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onUpdate: (updates: Partial<UpdateIssueRequest>) => Promise<boolean>;
  /** Navigate to the issue — clicking the title is the primary way IN. */
  onOpen: (event: React.MouseEvent) => void;
  onCreateSubIssue: () => void;
  onToggleParent: () => void;
  toggleLabel: string;
  renameLabel: string;
  createSubIssueLabel: string;
  writable?: boolean;
}) {
  const [draft, setDraft] = useState(row.issue.title);
  const [pending, setPending] = useState(false);
  const submittingRef = useRef(false);
  const editingRef = useRef(editing);
  editingRef.current = editing;
  // True between the mousedown and the click of ONE gesture when that gesture
  // began while the rename input was up. onBlur commits and flips `editing`
  // off synchronously, before the click that caused the blur lands — so a
  // guard keyed only on the current `editing` value is already gone by click
  // time, and the commit-click bubbles into row navigation (and could hit the
  // title's own open handler): clicking away to save a rename would also open
  // the issue (MUL-5108 review R1#2).
  const gestureStartedWhileEditingRef = useRef(false);

  useEffect(() => {
    // Realtime/cache snapshots should refresh the passive label, but must not
    // overwrite text the user is actively composing.
    if (!editingRef.current) setDraft(row.issue.title);
  }, [row.issue.title]);

  const commit = async () => {
    if (submittingRef.current) return;
    const title = draft.trim();
    if (!title || title === row.issue.title) {
      setDraft(row.issue.title);
      onEditingChange(false);
      return;
    }
    submittingRef.current = true;
    setPending(true);
    try {
      if (await onUpdate({ title })) onEditingChange(false);
    } finally {
      submittingRef.current = false;
      setPending(false);
    }
  };

  return (
    <div
      className="group/title relative flex min-w-0 items-center gap-1.5"
      style={{ paddingLeft: row.depth * 18 }}
      // Record whether the gesture began while editing (mousedown fires before
      // the blur that commits), then swallow that click in the capture phase —
      // before it can reach the row (navigation) or the title's open handler.
      // A gesture that began while NOT editing passes through untouched, so
      // clicking dead space still opens the issue.
      onMouseDownCapture={() => {
        gestureStartedWhileEditingRef.current = editingRef.current;
      }}
      onClickCapture={(event) => {
        if (editing || gestureStartedWhileEditingRef.current) {
          event.stopPropagation();
        }
        gestureStartedWhileEditingRef.current = false;
      }}
      onAuxClickCapture={(event) => {
        if (editing) event.stopPropagation();
      }}
    >
      {row.hasChildren ? (
        <button
          type="button"
          aria-label={toggleLabel}
          className="rounded p-0.5 text-muted-foreground hover:bg-accent"
          onClick={(event) => {
            event.stopPropagation();
            onToggleParent();
          }}
          onAuxClick={stopRowNavigation}
        >
          {row.collapsed ? (
            <ChevronRight className="size-3.5" />
          ) : (
            <ChevronDown className="size-3.5" />
          )}
        </button>
      ) : (
        <span className="w-4 shrink-0" />
      )}
      <span className="min-w-16 shrink-0 text-caption text-muted-foreground">
        {row.issue.identifier}
      </span>
      <IssueAgentActivityIndicator issueId={row.issue.id} />
      {editing && writable ? (
        <Input
          autoFocus
          value={draft}
          disabled={pending}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={() => void commit()}
          onKeyDown={(event) => {
            if (event.key === "Enter") void commit();
            if (event.key === "Escape" && !pending) {
              setDraft(row.issue.title);
              onEditingChange(false);
            }
          }}
          className="h-7 min-w-0 flex-1 px-2"
        />
      ) : (
        <>
          <button
            type="button"
            className="min-w-0 flex-1 truncate text-left hover:underline"
            onClick={(event) => {
              event.stopPropagation();
              onOpen(event);
            }}
          >
            {row.issue.title}
          </button>
          {/* Lifted out of the flex flow, the way SidebarMenuAction is. Laid
            * out inline these two reserved ~40px of the title column for
            * buttons that are invisible until hovered — and title is the
            * column with the least room to spare. The gradient fades the text
            * running underneath rather than letting the icons sit on top of
            * it; the sidebar has no need for one because its labels are short,
            * but a title runs to the cell's edge. focus-within keeps them
            * reachable by keyboard, where hover never fires. */}
          {/* The fade has to be whatever the cell is painted with at the
            * moment the actions show, and a hovered row is not the resting
            * background — pinned cells switch to the muted mix on hover, so
            * the gradient follows. */}
          {/* Keyed to the title cell, not the row: these act on the title,
            * and offering them from anywhere along a row puts them under the
            * pointer while it is somewhere else entirely. The fade still
            * follows the row's hover colour, since that is what the cell is
            * painted with when they appear. */}
          {writable && (
            <span className="pointer-events-none absolute inset-y-0 right-0 flex items-center gap-0.5 bg-gradient-to-l from-background from-70% to-transparent pr-1 pl-8 opacity-0 transition-opacity group-hover:from-[color-mix(in_oklab,var(--muted)_50%,var(--background))] group-hover/title:pointer-events-auto group-hover/title:opacity-100 focus-within:pointer-events-auto focus-within:opacity-100">
              <button
                type="button"
                aria-label={createSubIssueLabel}
                className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                onClick={(event) => {
                  event.stopPropagation();
                  onCreateSubIssue();
                }}
                onAuxClick={stopRowNavigation}
              >
                <Plus className="size-3" />
              </button>
              <button
                type="button"
                aria-label={renameLabel}
                className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                onClick={(event) => {
                  event.stopPropagation();
                  setDraft(row.issue.title);
                  onEditingChange(true);
                }}
                onAuxClick={stopRowNavigation}
              >
                <Pencil className="size-3" />
              </button>
            </span>
          )}
        </>
      )}
    </div>
  );
}

function LazyLabelCell({
  issue,
  open,
  onOpenChange,
  writable,
  onChange,
}: {
  issue: Issue;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  writable: boolean;
  onChange: (labelIds: string[]) => Promise<void>;
}) {
  const { t } = useT("issues");
  const labels = issue.labels ?? [];
  if (open && writable) {
    return (
      <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
        <LabelPicker
          selectedIds={labels.map((label) => label.id)}
          onSelectedIdsChange={(labelIds) => {
            void onChange(labelIds);
          }}
          open
          onOpenChange={(next) => {
            if (!next) onOpenChange(false);
          }}
          triggerRender={<button type="button" className="flex max-w-full gap-1" />}
        />
      </div>
    );
  }
  if (!writable) {
    return labels.length > 0 ? (
      <span className="flex max-w-full items-center gap-1 overflow-hidden">
        {labels.slice(0, 2).map((label) => (
          <LabelChip key={label.id} label={label} />
        ))}
      </span>
    ) : (
      <span className="text-muted-foreground">
        {t(($) => $.table.empty_value)}
      </span>
    );
  }
  return (
    <button
      type="button"
      className="flex max-w-full items-center gap-1 overflow-hidden rounded px-1 py-0.5 hover:bg-accent"
      onClick={(event) => {
        event.stopPropagation();
        onOpenChange(true);
      }}
      onAuxClick={stopRowNavigation}
    >
      {labels.length > 0 ? (
        <>
          {labels.slice(0, 2).map((label) => (
            <LabelChip key={label.id} label={label} />
          ))}
          {labels.length > 2 && (
            <span className="text-caption text-muted-foreground">+{labels.length - 2}</span>
          )}
        </>
      ) : (
        <span className="text-muted-foreground">{t(($) => $.table.empty_value)}</span>
      )}
    </button>
  );
}

type IssueTableGroupContentProps = {
  group: Extract<IssueTableDisplayRow, { kind: "group" }>;
  onToggle: () => void;
};

export function IssueTableGroupContent({
  group,
  onToggle,
}: IssueTableGroupContentProps) {
  return (
    <button
      type="button"
      className="sticky left-4 flex w-fit items-center gap-2 text-caption font-medium"
      onClick={onToggle}
    >
      {group.collapsed ? (
        <ChevronRight className="size-3.5" />
      ) : (
        <ChevronDown className="size-3.5" />
      )}
      {group.label}
      <span className="font-normal tabular-nums text-muted-foreground">
        {group.count}
      </span>
    </button>
  );
}

function propertyDisplayValue(
  property: IssueProperty,
  value: IssuePropertyValue | undefined,
  // Actor values are "<kind>:<uuid>" references; without a resolver they would
  // export as raw ids, so callers that can export an actor column must pass one.
  getActorName?: (type: string, id: string) => string,
) {
  if (value === undefined) return "";
  const options = property.config.options ?? [];
  if (property.type === "select") {
    return options.find((option) => option.id === value)?.name ?? "";
  }
  if (property.type === "multi_select") {
    const ids = Array.isArray(value) ? value : [];
    return options
      .filter((option) => ids.includes(option.id))
      .map((option) => option.name)
      .join(", ");
  }
  if (isActorPropertyType(property.type)) {
    return actorRefsFromValue(value)
      .map((ref) => (getActorName ? getActorName(ref.kind, ref.id) : formatActorRef(ref.kind, ref.id)))
      .join(", ");
  }
  return String(value);
}

/**
 * Render-time context for the module-level cell/header components below,
 * carried on `table.options.meta`. The renderers MUST be module-level
 * components with stable identities: TanStack's flexRender mounts a
 * function-typed `cell`/`header` as a React component, so a renderer closure
 * rebuilt when any lookup changed identity (childProgressMap on every
 * realtime refetch, propertyById, actor names…) was a NEW element type and
 * React remounted every cell — closing any open picker popup and dropping
 * in-progress drafts the moment workspace activity refreshed the window
 * (MUL-5108). Data flows through meta instead so the element types never
 * change.
 */
type TableViewMeta = {
  childProgressMap: Map<string, ChildProgress>;
  propertyById: Map<string, IssueProperty>;
  properties: IssueProperty[];
  fieldById: Map<string, IssueTableField>;
  visibleIssueIds: string[];
  /** `${row.key}:${column.id}` of the cell whose editor popup / rename input
   *  is open, or null. Owned by TableView so the open editor survives cell
   *  remounts and freezes the table structure while it is up. */
  editingCellSession: EditingCellSession | null;
  openEditingCell: (cellKey: string) => void;
  closeEditingCell: (instanceId: number) => void;
  restoreEditingCell: (session: EditingCellSession) => void;
  /** Takes the ISSUE, not its id: the run-confirm gate reads its status
   *  category and owner to decide whether the write needs confirming first. */
  updateField: (
    row: IssueTableRow,
    fieldId: string,
    change: DataSourceCellChange,
  ) => ReturnType<ReturnType<typeof createIssueTableDataSource>["execute"]>;
  writable: boolean;
  openIssue: (issue: Issue, event?: React.MouseEvent) => void;
  createSubIssue: (issue: Issue) => void;
  toggleTableParentCollapsed: (issueId: string) => void;
  handleIssueSelection: (issueId: string, shiftKey: boolean) => void;
  getActorName: (actorType: string, actorId: string) => string;
  columnLabel: (key: TableColumnKey) => string;
  sortBy: SortField;
  sortDirection: "asc" | "desc";
  onSort: (field: SortField, direction: "asc" | "desc") => void;
  toggleTableColumn: (key: TableColumnKey) => void;
};

type EditingCellSession = {
  cellKey: string;
  instanceId: number;
  sourceIdentity: string;
};

function closePendingTableRunConfirm(
  sourceIdentity: string,
  ownerIdentity: string,
) {
  const modal = useModalStore.getState();
  if (
    modal.modal === "issue-run-confirm" &&
    modal.data?.sourceIdentity === sourceIdentity &&
    modal.data?.ownerIdentity === ownerIdentity
  ) {
    modal.close(modal.modalInstanceId ?? undefined);
  }
}

function getTableViewMeta(
  table: TanstackTable<IssueTableDisplayRow>,
): TableViewMeta {
  return table.options.meta as unknown as TableViewMeta;
}

/**
 * Release the hoisted editing key when the cell that owns it unmounts.
 *
 * Row virtualization (see data-table.tsx) unmounts a cell as its row scrolls
 * out of the rendered window. Base UI does NOT call onOpenChange(false) on
 * unmount, so without this the open picker's key — and the frozen row
 * structure keyed off it — would persist after the anchor row leaves the
 * viewport: the table would stay frozen, and scrolling the row back would
 * silently reopen the picker and discard any in-progress rename draft
 * (MUL-5108 review R1#3). Clearing the key iff this unmounting cell still owns
 * it thaws the structure and closes the editor.
 *
 * Live values are read through refs so the empty-dep cleanup always sees the
 * current key/setter. At initial mount a cell is never yet the active editor
 * (the editor is opened by a later interaction, which does not remount the
 * cell), so this never fires spuriously — including under StrictMode's
 * mount → unmount → mount probe, whose first cleanup sees `editingCellKey`
 * still unequal to this cell's key.
 */
export function useReleaseEditingCellOnUnmount(
  cellKey: string | null,
  editingCellSession: EditingCellSession | null,
  closeEditingCell: (instanceId: number) => void,
) {
  const editingCellSessionRef = useRef(editingCellSession);
  editingCellSessionRef.current = editingCellSession;
  const closeEditingCellRef = useRef(closeEditingCell);
  closeEditingCellRef.current = closeEditingCell;
  useEffect(() => {
    return () => {
      const session = editingCellSessionRef.current;
      if (cellKey !== null && session?.cellKey === cellKey) {
        closeEditingCellRef.current(session.instanceId);
      }
    };
  }, [cellKey]);
}

function IssueTableSelectHeader({
  table,
}: HeaderContext<IssueTableDisplayRow, unknown>) {
  const meta = getTableViewMeta(table);
  const { t } = useT("issues");
  return (
    <SelectAllCheckbox
      issueIds={meta.visibleIssueIds}
      label={t(($) => $.table.select_all)}
    />
  );
}

function IssueTableSelectCell({
  row,
  table,
}: CellContext<IssueTableDisplayRow, unknown>) {
  const meta = getTableViewMeta(table);
  const selection = useIssueSurfaceSelection();
  const { t } = useT("issues");
  if (row.original.kind !== "issue") return null;
  const issue = row.original.issue;
  return (
    <IssueCheckbox
      checked={selection.selectedIds.has(issue.id)}
      label={t(($) => $.table.select_issue, { identifier: issue.identifier })}
      onToggle={(shiftKey) => meta.handleIssueSelection(issue.id, shiftKey)}
    />
  );
}

function IssueTableAddColumnHeader({
  table,
}: HeaderContext<IssueTableDisplayRow, unknown>) {
  const meta = getTableViewMeta(table);
  const { t } = useT("issues");
  return (
    <TableColumnPicker
      properties={meta.properties}
      trigger={
        <button
          type="button"
          aria-label={t(($) => $.table.columns.add)}
          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <Plus className="size-3.5" />
        </button>
      }
    />
  );
}

function IssueTableEmptyCell() {
  return null;
}

function IssueTableHeaderCell({
  column,
  table,
}: HeaderContext<IssueTableDisplayRow, unknown>) {
  const meta = getTableViewMeta(table);
  const { t } = useT("issues");
  const key = column.id as TableColumnKey;
  const propertyId = propertyIdFromViewKey(key);
  const property = propertyId ? meta.propertyById.get(propertyId) : undefined;
  const staticSort = propertyId
    ? property &&
      !["multi_select", "checkbox", "actor", "multi_actor"].includes(property.type)
      ? (`property:${propertyId}` as SortField)
      : undefined
    : SORTABLE_COLUMNS[key as TableSystemColumnKey];
  const label = meta.columnLabel(key);
  return (
    <SortableColumnHeader
      columnKey={key}
      label={label}
      sortField={staticSort}
      sortBy={meta.sortBy}
      sortDirection={meta.sortDirection}
      onSort={meta.onSort}
      onHide={key === "title" ? undefined : () => meta.toggleTableColumn(key)}
      ascendingLabel={t(($) => $.table.sort_ascending)}
      descendingLabel={t(($) => $.table.sort_descending)}
      hideLabel={t(($) => $.table.columns.hide)}
      reorderLabel={t(($) => $.table.columns.reorder, { column: label })}
    />
  );
}

function IssueTableBodyCell({
  row,
  column,
  table,
}: CellContext<IssueTableDisplayRow, unknown>) {
  const meta = getTableViewMeta(table);
  const { t, i18n } = useT("issues");
  const statusLabel = useStatusLabel(useWorkspaceId());
  // Computed (and the unmount responder registered) before the early return so
  // the hook order is stable across issue/group rows.
  const cellKey =
    row.original.kind === "issue"
      ? JSON.stringify([row.original.key, column.id])
      : null;
  useReleaseEditingCellOnUnmount(
    cellKey,
    meta.editingCellSession,
    meta.closeEditingCell,
  );
  // Placeholder rows go through the ordinary cell renderer so they inherit the
  // real column widths, pinning and borders — the grid is already correct
  // before any data arrives, so the rows swap in without shifting anything.
  if (row.original.kind === "skeleton") {
    return <Skeleton className="h-3.5 w-full" />;
  }
  if (row.original.kind !== "issue") return null;
  const issueRow = row.original;
  const issue = issueRow.issue;
  const sourceRow = issueRow.sourceRow ?? {
    issue,
    direct_child_count: issueRow.hasChildren ? 1 : 0,
  };
  const key = column.id as TableColumnKey;
  const editorSession =
    meta.editingCellSession?.cellKey === cellKey
      ? meta.editingCellSession
      : null;
  const editorOpen = editorSession !== null;
  const setEditorOpen = (open: boolean) => {
    if (open && cellKey !== null) meta.openEditingCell(cellKey);
    else if (!open && editorSession) {
      meta.closeEditingCell(editorSession.instanceId);
    }
  };
  const field = meta.fieldById.get(key);
  const canSet = field?.canSet(sourceRow) ?? false;
  const canClear = field?.canClear(sourceRow) ?? false;
  const onUpdate = async (updates: Partial<UpdateIssueRequest>) => {
    let change: DataSourceCellChange | null = null;
    switch (key) {
      case "title":
        if (updates.title !== undefined) {
          change = { op: "set", value: updates.title };
        }
        break;
      case "status":
        if (updates.status !== undefined) {
          change = { op: "set", value: updates.status };
        }
        break;
      case "priority":
        if (updates.priority !== undefined) {
          change = { op: "set", value: updates.priority };
        }
        break;
      case "assignee":
        change =
          updates.assignee_type && updates.assignee_id
            ? {
                op: "set",
                value: {
                  type: updates.assignee_type,
                  id: updates.assignee_id,
                },
              }
            : { op: "clear" };
        break;
      case "project":
        change = updates.project_id
          ? { op: "set", value: updates.project_id }
          : { op: "clear" };
        break;
      case "start_date":
        change = updates.start_date
          ? { op: "set", value: updates.start_date }
          : { op: "clear" };
        break;
      case "due_date":
        change = updates.due_date
          ? { op: "set", value: updates.due_date }
          : { op: "clear" };
        break;
    }
    if (!change) return false;
    const result = await meta.updateField(sourceRow, key, change);
    if (result.status === "failed") {
      toast.error(result.error.message);
      if (editorSession) meta.restoreEditingCell(editorSession);
    }
    return result.status !== "failed";
  };

  const propertyId = propertyIdFromViewKey(key);
  if (propertyId) {
    const property = meta.propertyById.get(propertyId);
    if (!property) return null;
    return (
      <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
        <CustomPropertyValueInput
          property={property}
          value={issue.properties[property.id]}
          open={editorOpen}
          onOpenChange={setEditorOpen}
          canSet={canSet}
          canClear={canClear}
          editorSessionKey={editorSession?.instanceId}
          onChange={async (value) => {
            const result = await meta.updateField(
              sourceRow,
              key,
              value === undefined
                ? { op: "clear" }
                : { op: "set", value },
            );
            if (result.status === "failed") {
              toast.error(result.error.message);
              if (editorSession) meta.restoreEditingCell(editorSession);
            }
            return result.status === "accepted";
          }}
        />
      </div>
    );
  }
  if (
    !meta.writable &&
    [
      "status",
      "priority",
      "assignee",
      "project",
      "start_date",
      "due_date",
    ].includes(key)
  ) {
    let value: React.ReactNode = field?.value(sourceRow) as React.ReactNode;
    if (key === "status") value = statusLabel(issue.status);
    else if (key === "priority") value = t(($) => $.priority[issue.priority]);
    else if (key === "assignee") {
      value =
        issue.assignee_type && issue.assignee_id
          ? meta.getActorName(issue.assignee_type, issue.assignee_id)
          : t(($) => $.table.unassigned);
    }
    return (
      <span className="text-muted-foreground">
        {value == null || value === "" ? t(($) => $.table.empty_value) : value}
      </span>
    );
  }
  switch (key) {
    case "title":
      return (
        <InlineTitle
          row={issueRow}
          editing={editorOpen}
          onEditingChange={setEditorOpen}
          onUpdate={onUpdate}
          onOpen={(event) => meta.openIssue(issue, event)}
          onCreateSubIssue={() => meta.createSubIssue(issue)}
          onToggleParent={() => meta.toggleTableParentCollapsed(issue.id)}
          toggleLabel={t(($) => $.table.toggle_sub_issues)}
          renameLabel={t(($) => $.table.rename_title)}
          createSubIssueLabel={t(($) => $.actions.create_sub_issue)}
          writable={canSet}
        />
      );
    case "identifier":
      return (
        <span className="text-caption text-muted-foreground">{issue.identifier}</span>
      );
    case "status":
      return (
        <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <StatusPicker
            status={issue.status}
            onUpdate={onUpdate}
            align="start"
            open={editorOpen}
            onOpenChange={setEditorOpen}
          />
        </div>
      );
    case "priority":
      return (
        <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <PriorityPicker
            priority={issue.priority}
            onUpdate={onUpdate}
            align="start"
            open={editorOpen}
            onOpenChange={setEditorOpen}
          />
        </div>
      );
    case "assignee":
      return (
        <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <AssigneePicker
            assigneeType={issue.assignee_type}
            assigneeId={issue.assignee_id}
            onUpdate={onUpdate}
            align="start"
            open={editorOpen}
            onOpenChange={setEditorOpen}
          />
        </div>
      );
    case "labels":
      return (
        <LazyLabelCell
          issue={issue}
          open={editorOpen}
          onOpenChange={setEditorOpen}
          writable={canSet || canClear}
          onChange={async (labelIds) => {
            const result = await meta.updateField(
              sourceRow,
              key,
              labelIds.length === 0
                ? { op: "clear" }
                : { op: "set", value: labelIds },
            );
            if (result.status === "failed") {
              toast.error(result.error.message);
              if (editorSession) meta.restoreEditingCell(editorSession);
            }
          }}
        />
      );
    case "project":
      return (
        <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <ProjectPicker
            projectId={issue.project_id}
            onUpdate={onUpdate}
            open={editorOpen}
            onOpenChange={setEditorOpen}
            triggerRender={
              <button
                type="button"
                className="flex max-w-full items-center gap-1.5 rounded px-1 py-0.5 hover:bg-accent"
              />
            }
          />
        </div>
      );
    case "start_date":
      return (
        <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <StartDatePicker
            startDate={issue.start_date}
            onUpdate={onUpdate}
            open={editorOpen}
            onOpenChange={setEditorOpen}
          />
        </div>
      );
    case "due_date":
      return (
        <div onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <DueDatePicker
            dueDate={issue.due_date}
            onUpdate={onUpdate}
            open={editorOpen}
            onOpenChange={setEditorOpen}
          />
        </div>
      );
    case "created_at":
    case "updated_at":
      return (
        <span className="text-caption text-muted-foreground">
          {new Intl.DateTimeFormat(i18n.language, {
            month: "short",
            day: "numeric",
            year: "numeric",
          }).format(new Date(issue[key]))}
        </span>
      );
    case "child_progress": {
      const progress = meta.childProgressMap.get(issue.id);
      return progress ? (
        <span className="inline-flex items-center gap-1.5 text-caption text-muted-foreground">
          <ProgressRing done={progress.done} total={progress.total} size={15} />
          {progress.done}/{progress.total}
        </span>
      ) : (
        <span className="text-muted-foreground">{t(($) => $.table.empty_value)}</span>
      );
    }
    case "creator":
      return (
        <span className="flex min-w-0 items-center gap-1.5">
          <ActorAvatar
            actorType={issue.creator_type}
            actorId={issue.creator_id}
            size="sm"
          />
          <span className="truncate">
            {meta.getActorName(issue.creator_type, issue.creator_id)}
          </span>
        </span>
      );
  }
  return null;
}

export function TableView({
  serverQuery,
  childProgressMap,
  search,
  onSearchChange,
  onLoadedIssuesChange,
  onCreateIssue,
  exportIssues,
  resolveExportLookups,
}: TableViewProps) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const workspaceSlug = getCurrentSlug();
  const tableOwnerIdentity = useId();
  const resolveStatusLabel = useStatusLabel(wsId);
  const { entryOf } = useIssueStatuses(wsId);
  const openModal = useModalStore((s) => s.open);
  const queryClient = useQueryClient();
  const intentNavigate = useIntentNavigate();
  const paths = useWorkspacePaths();
  const actions = useIssueSurfaceActionsOptional();
  const writeCapabilityRef = useRef(actions !== null);
  writeCapabilityRef.current = actions !== null;
  useEffect(
    () => () => {
      writeCapabilityRef.current = false;
    },
    [],
  );
  const workspaceContext = useMemo<WorkspaceRequestContext | undefined>(
    () =>
      workspaceSlug
        ? {
            workspaceId: wsId,
            workspaceSlug,
            isActive: () => writeCapabilityRef.current,
          }
        : undefined,
    [workspaceSlug, wsId],
  );
  const issueSourceIdentity = useMemo(
    () =>
      dataSourceIdentityString({
        workspaceId: wsId,
        namespace: "issues",
        sourceId: "tasks",
      }),
    [wsId],
  );
  const canSubmit = useCallback(() => {
    if (!workspaceContext) return writeCapabilityRef.current;
    try {
      assertWorkspaceRequestContext(workspaceContext);
      return true;
    } catch {
      return false;
    }
  }, [workspaceContext]);
  const { mutateAsync: setPropertyAsync } = useSetIssueProperty();
  const { mutateAsync: clearPropertyAsync } = useUnsetIssueProperty();
  const { mutateAsync: attachLabelAsync } = useAttachLabelToIssue();
  const { mutateAsync: detachLabelAsync } = useDetachLabelFromIssue();
  const selection = useIssueSurfaceSelection();
  const { getActorName } = useActorName();
  const {
    data: properties = [],
    isSuccess: propertyCatalogSettled,
  } = useQuery(propertyListOptions(wsId));
  const dataSource = useMemo(
    () => {
      const execute = createIssueTableCommandExecutor({
        actions,
        statusCatalog: { entryOf },
        openRunConfirm: (data) => openModal("issue-run-confirm", data),
        sourceIdentity: issueSourceIdentity,
        ownerIdentity: tableOwnerIdentity,
        workspaceContext,
        canSubmit,
      });
      return createIssueTableDataSource({
        workspaceId: wsId,
        workspaceSlug: workspaceSlug ?? undefined,
        fields: properties,
        execute,
        setProperty: async ({ issueId, propertyId, value }) => {
          await setPropertyAsync({
            issueId,
            propertyId,
            value: value as IssuePropertyValue,
            workspaceContext,
          });
        },
        clearProperty: async ({ issueId, propertyId }) => {
          await clearPropertyAsync({ issueId, propertyId, workspaceContext });
        },
        setLabels: async ({ issue, labelIds }) => {
          const previous = new Set((issue.labels ?? []).map((label) => label.id));
          const next = new Set(labelIds);
          await Promise.all([
            ...labelIds
              .filter((labelId) => !previous.has(labelId))
              .map((labelId) =>
                attachLabelAsync({
                  issueId: issue.id,
                  labelId,
                  workspaceContext,
                }),
              ),
            ...(issue.labels ?? [])
              .map((label) => label.id)
              .filter((labelId) => !next.has(labelId))
              .map((labelId) =>
                detachLabelAsync({
                  issueId: issue.id,
                  labelId,
                  workspaceContext,
                }),
              ),
          ]);
        },
      });
    },
    [
      actions,
      attachLabelAsync,
      canSubmit,
      clearPropertyAsync,
      detachLabelAsync,
      entryOf,
      issueSourceIdentity,
      openModal,
      properties,
      setPropertyAsync,
      tableOwnerIdentity,
      workspaceContext,
      workspaceSlug,
      wsId,
    ],
  );
  const fieldById = useMemo(
    () => new Map(dataSource.fields.map((field) => [field.id, field])),
    [dataSource.fields],
  );
  const propertyById = useMemo(
    () => new Map(properties.map((property) => [property.id, property])),
    [properties],
  );
  const activePropertyIds = useMemo(
    () => new Set(properties.map((property) => property.id)),
    [properties],
  );
  const groupablePropertyIds = useMemo(
    () =>
      new Set(
        properties
          .filter((property) => ["select", "checkbox"].includes(property.type))
          .map((property) => property.id),
      ),
    [properties],
  );
  const tableColumns = useViewStore((state) => state.tableColumns);
  const toggleTableColumn = useViewStore((state) => state.toggleTableColumn);
  const reorderTableColumn = useViewStore((state) => state.reorderTableColumn);
  const setTableColumnWidth = useViewStore((state) => state.setTableColumnWidth);
  const tableGrouping = useViewStore((state) => state.tableGrouping);
  const setTableGrouping = useViewStore((state) => state.setTableGrouping);
  const tableCollapsedGroups = useViewStore((state) => state.tableCollapsedGroups);
  const toggleTableGroupCollapsed = useViewStore(
    (state) => state.toggleTableGroupCollapsed,
  );
  const tableCollapsedParents = useViewStore((state) => state.tableCollapsedParents);
  const toggleTableParentCollapsed = useViewStore(
    (state) => state.toggleTableParentCollapsed,
  );
  const tableHierarchy = useViewStore((state) => state.tableHierarchy);
  const sortBy = useViewStore((state) => state.sortBy);
  const setSortBy = useViewStore((state) => state.setSortBy);
  const sortDirection = useViewStore((state) => state.sortDirection);
  const setSortDirection = useViewStore((state) => state.setSortDirection);
  const [exporting, setExporting] = useState<"all" | "selected" | null>(null);
  // The one cell whose editor (picker popup / rename input) is open — see
  // TableViewMeta.editingCellKey.
  const sourceIdentity = useMemo(
    () => dataSourceIdentityString(dataSource.identity),
    [dataSource.identity],
  );
  const editorInstanceRef = useRef(0);
  const sourceIdentityRef = useRef(sourceIdentity);
  sourceIdentityRef.current = sourceIdentity;
  const [editingCellSession, setEditingCellSession] =
    useState<EditingCellSession | null>(null);
  const editingCellKey = editingCellSession?.cellKey ?? null;
  const openEditingCell = useCallback(
    (cellKey: string) => {
      const session: EditingCellSession = {
        cellKey,
        instanceId: ++editorInstanceRef.current,
        sourceIdentity,
      };
      setEditingCellSession(session);
    },
    [sourceIdentity],
  );
  const closeEditingCell = useCallback((instanceId: number) => {
    setEditingCellSession((current) =>
      current?.instanceId === instanceId ? null : current,
    );
  }, []);
  const restoreEditingCell = useCallback((session: EditingCellSession) => {
    if (
      editorInstanceRef.current !== session.instanceId ||
      sourceIdentityRef.current !== session.sourceIdentity
    ) {
      return;
    }
    setEditingCellSession((current) => current ?? session);
  }, []);

  const groupingPropertyId = propertyIdFromViewKey(tableGrouping);
  const effectiveTableGrouping =
    groupingPropertyId &&
    propertyCatalogSettled &&
    !groupablePropertyIds.has(groupingPropertyId)
      ? "none"
      : tableGrouping;
  useEffect(() => {
    if (
      !groupingPropertyId ||
      !propertyCatalogSettled ||
      groupablePropertyIds.has(groupingPropertyId)
    ) {
      return;
    }
    setTableGrouping("none");
    toast.info(t(($) => $.table.group_property_unavailable));
  }, [
    groupablePropertyIds,
    groupingPropertyId,
    propertyCatalogSettled,
    setTableGrouping,
    t,
  ]);

  const serverGroupSpec = useMemo(
    () => tableGroupSpec(effectiveTableGrouping),
    [effectiveTableGrouping],
  );
  const usesServerGrouping = serverGroupSpec.kind !== "none";
  // Project group rows carry only a project id; the title comes from the
  // shared projects query the surface already primes for this grouping.
  //
  // Read `data` rather than defaulting it in the destructure: an un-settled
  // query has no data, so `= []` would hand this memo a fresh array on every
  // render and churn every consumer of the map below (MUL-5477).
  const groupProjectsQuery = useQuery({
    ...projectListOptions(wsId),
    enabled: serverGroupSpec.kind === "project",
  });
  const groupProjectMap = useMemo(
    () =>
      new Map(
        (groupProjectsQuery.data ?? []).map((project) => [project.id, project]),
      ),
    [groupProjectsQuery.data],
  );
  const serverGroupLabel = useCallback(
    (descriptor: IssueTableGroupDescriptor) => {
      const value = descriptor.value;
      if (value.kind === "status") {
        return resolveStatusLabel(value.status);
      }
      if (value.kind === "assignee") {
        return value.actor
          ? getActorName(value.actor.type, value.actor.id)
          : t(($) => $.table.unassigned);
      }
      if (value.kind === "project") {
        if (!value.project_id) return t(($) => $.swimlane.no_project);
        return (
          groupProjectMap.get(value.project_id)?.title ??
          t(($) => $.table.value_unavailable)
        );
      }
      if (value.kind === "parent") {
        if (value.value_state === "unset") {
          return t(($) => $.swimlane.no_parent);
        }
        return value.parent?.title ?? t(($) => $.table.value_unavailable);
      }
      if (value.value_state === "unset") return t(($) => $.table.no_value);
      if (value.value_state === "unavailable") {
        return t(($) => $.table.value_unavailable);
      }
      const property = propertyById.get(value.property_id);
      if (typeof value.value === "boolean") {
        return value.value
          ? t(($) => $.pickers.custom_property.true_label)
          : t(($) => $.pickers.custom_property.false_label);
      }
      return (
        property?.config.options?.find((option) => option.id === value.value)
          ?.name ?? String(value.value ?? "")
      );
    },
    [getActorName, groupProjectMap, propertyById, resolveStatusLabel, t],
  );
  const issueTableBinding = useMemo<
    DataViewQueryBinding<
      IssueTableRow,
      IssueTableQuerySpec,
      IssueTableRowsResponse,
      IssueTableGroupsResponse
    >
  >(
    () => ({
      identity: dataSource.identity,
      rowPageKey: ({ query, groupBy, branch, hierarchy, page }) => {
        const group = tableGroupSpec(groupBy?.fieldId ?? "none");
        return [
          ...issueKeys.tableRows(
            wsId,
            query,
            group,
            branch.groupKey,
            hierarchy,
            branch.parentRowId,
          ),
          "page",
          page.cursor ?? null,
        ] as const;
      },
      rowBranchKey: ({ query, groupBy, branch, hierarchy }) =>
        issueKeys.tableRows(
          wsId,
          query,
          tableGroupSpec(groupBy?.fieldId ?? "none"),
          branch.groupKey,
          hierarchy,
          branch.parentRowId,
        ),
      readRowPage: async (
        { query, groupBy, branch, hierarchy, page },
        signal,
      ) => {
        const result = await dataSource.read(
          {
            query,
            group: tableGroupSpec(groupBy?.fieldId ?? "none"),
            group_key: branch.groupKey,
            hierarchy: { enabled: hierarchy },
            parent_id: branch.parentRowId,
          },
          page,
          signal,
        );
        return {
          query_fingerprint: result.metadata.queryFingerprint,
          group_key: result.metadata.groupKey,
          parent_id: result.metadata.parentId,
          total: result.total,
          rows: result.rows,
          branch_total: result.metadata.branchTotal,
          next_cursor: result.nextCursor,
        };
      },
      mapRowPage: (page) => ({
        rows: page.rows,
        total: page.total,
        branchTotal: page.branch_total,
        nextCursor: page.next_cursor,
      }),
      groupPagesKey: ({ query, groupBy }) => {
        const group = tableGroupSpec(groupBy.fieldId);
        if (group.kind === "none") {
          return [...issueKeys.tableAll(wsId), "groups", "disabled"];
        }
        return issueKeys.tableGroups(wsId, query, group);
      },
      readGroupPage: async ({ query, groupBy, page }, signal) => {
        const group = tableGroupSpec(groupBy.fieldId);
        if (group.kind === "none") {
          throw new Error(`Unsupported group field: ${groupBy.fieldId}`);
        }
        const result = await dataSource.readGroups(query, group, page, signal);
        return {
          query_fingerprint: result.queryFingerprint,
          total: result.total,
          groups: result.groups,
          next_cursor: result.nextCursor,
        };
      },
      mapGroupPage: (page) => ({
        groups: page.groups.map((descriptor) => {
          const value = descriptor.value;
          const valueState =
            value.kind === "parent" || value.kind === "property"
              ? value.value_state
              : value.kind === "assignee"
                ? value.actor
                  ? "value"
                  : "unset"
                : value.kind === "project"
                  ? value.project_id
                    ? "value"
                    : "unset"
                  : "value";
          return {
            key: descriptor.key,
            label: serverGroupLabel(descriptor),
            count: descriptor.count,
            valueState,
            value: descriptor.value,
          };
        }),
        total: page.total,
        nextCursor: page.next_cursor,
      }),
    }),
    [dataSource, serverGroupLabel, wsId],
  );
  const previousSourceIdentityRef = useRef(sourceIdentity);
  useEffect(() => {
    if (previousSourceIdentityRef.current === sourceIdentity) return;
    const previousSourceIdentity = previousSourceIdentityRef.current;
    previousSourceIdentityRef.current = sourceIdentity;
    closePendingTableRunConfirm(previousSourceIdentity, tableOwnerIdentity);
    editorInstanceRef.current += 1;
    setEditingCellSession(null);
  }, [sourceIdentity, tableOwnerIdentity]);
  useEffect(() => {
    if (!dataSource.capabilities.writable && editingCellKey !== null) {
      editorInstanceRef.current += 1;
      setEditingCellSession(null);
    }
    if (!dataSource.capabilities.writable) {
      closePendingTableRunConfirm(sourceIdentity, tableOwnerIdentity);
    }
  }, [
    dataSource.capabilities.writable,
    editingCellKey,
    sourceIdentity,
    tableOwnerIdentity,
  ]);
  useEffect(
    () => () => closePendingTableRunConfirm(sourceIdentity, tableOwnerIdentity),
    [sourceIdentity, tableOwnerIdentity],
  );
  const collapsedGroupSet = useMemo(
    () => new Set(tableCollapsedGroups),
    [tableCollapsedGroups],
  );
  const collapsedParentSet = useMemo(
    () => new Set(tableCollapsedParents),
    [tableCollapsedParents],
  );
  const groupBy = useMemo(
    () =>
      usesServerGrouping ? { fieldId: effectiveTableGrouping } : null,
    [effectiveTableGrouping, usesServerGrouping],
  );
  const projectIssueRow = useCallback(
    ({
      row,
      depth,
      hasChildren,
      collapsed,
    }: {
      row: IssueTableRow;
      depth: number;
      hasChildren: boolean;
      collapsed: boolean;
    }) => ({
      kind: "issue" as const,
      key: row.issue.id,
      issue: row.issue,
      sourceRow: row,
      depth,
      hasChildren,
      collapsed,
    }),
    [],
  );
  const dataView = useDataViewController({
    binding: issueTableBinding,
    query: serverQuery,
    groupBy,
    hierarchy: tableHierarchy,
    collapsedGroupKeys: collapsedGroupSet,
    collapsedRowIds: collapsedParentSet,
    rowId: dataSource.rowId,
    directChildCount: (row) => row.direct_child_count,
    projectRow: projectIssueRow,
    skeletonCount: SKELETON_ROW_COUNT,
  });
  useEffect(() => {
    const body =
      dataView.groupError instanceof ApiError &&
      dataView.groupError.body &&
      typeof dataView.groupError.body === "object"
        ? (dataView.groupError.body as { error?: unknown })
        : null;
    if (
      dataView.groupError instanceof ApiError &&
      dataView.groupError.status === 422 &&
      body?.error === "unsupported_group"
    ) {
      setTableGrouping("none");
      toast.info(t(($) => $.table.group_property_unavailable));
    }
  }, [dataView.groupError, setTableGrouping, t]);
  const serverDisplayRows: IssueTableDisplayRow[] = dataView.rows;
  const tableMembershipIdentity = useMemo(
    () =>
      JSON.stringify([
        dataSource.identity,
        serverQuery.scope,
        serverQuery.filters,
        serverQuery.search ?? "",
      ]),
    [
      dataSource.identity,
      serverQuery.filters,
      serverQuery.scope,
      serverQuery.search,
    ],
  );
  const authoritativeLoadedIssues = useMemo(
    () => dataView.authoritativeRows.map((row) => row.issue),
    [dataView.authoritativeRows],
  );
  const [loadedIssueState, setLoadedIssueState] = useState<LoadedIssueState>({
    membershipIdentity: tableMembershipIdentity,
    issues: new Map(),
  });
  useEffect(() => {
    setLoadedIssueState((previous) => {
      const reset = previous.membershipIdentity !== tableMembershipIdentity;
      const issues = reset
        ? new Map<string, Issue>()
        : new Map(previous.issues);
      let changed = reset;
      for (const issue of authoritativeLoadedIssues) {
        if (issues.get(issue.id) !== issue) {
          issues.set(issue.id, issue);
          changed = true;
        }
      }
      return changed
        ? { membershipIdentity: tableMembershipIdentity, issues }
        : previous;
    });
  }, [authoritativeLoadedIssues, tableMembershipIdentity]);
  const loadedIssues = useMemo(
    () =>
      loadedIssueState.membershipIdentity === tableMembershipIdentity
        ? [...loadedIssueState.issues.values()]
        : [],
    [
      loadedIssueState.issues,
      loadedIssueState.membershipIdentity,
      tableMembershipIdentity,
    ],
  );

  const visibleColumnConfigs = useMemo(
    () =>
      tableColumns.filter((column) => {
        const propertyId = propertyIdFromViewKey(column.key);
        return !propertyId || activePropertyIds.has(propertyId);
      }),
    [activePropertyIds, tableColumns],
  );

  const issueById = useMemo(
    () => new Map(authoritativeLoadedIssues.map((issue) => [issue.id, issue])),
    [authoritativeLoadedIssues],
  );
  const visibleIssueIds = useMemo(
    () =>
      serverDisplayRows
        .filter((row): row is Extract<IssueTableDisplayRow, { kind: "issue" }> => row.kind === "issue")
        .map((row) => row.issue.id),
    [serverDisplayRows],
  );
  useEffect(() => {
    onLoadedIssuesChange(loadedIssues);
  }, [loadedIssues, onLoadedIssuesChange]);
  const selectedIssues = useMemo(
    () => loadedIssues.filter((issue) => selection.selectedIds.has(issue.id)),
    [loadedIssues, selection.selectedIds],
  );
  const handleIssueSelection = useDataViewSelection({
    sourceIdentity: dataSource.identity,
    rowIds: visibleIssueIds,
    selectedIds: selection.selectedIds,
    select: selection.select,
    deselect: selection.deselect,
    toggle: selection.toggle,
    clear: selection.clear,
  });

  const columnLabel = useCallback(
    (key: TableColumnKey) => {
      const propertyId = propertyIdFromViewKey(key);
      if (propertyId) return propertyById.get(propertyId)?.name ?? t(($) => $.table.no_value);
      return t(($) => $.table.columns[key as ColumnLabelKey]);
    },
    [propertyById, t],
  );

  const updateField = useCallback(
    (row: IssueTableRow, fieldId: string, change: DataSourceCellChange) =>
      dataSource.execute({ row, fieldId, change }),
    [dataSource],
  );

  const openIssue = useCallback(
    (issue: Issue, event?: React.MouseEvent) => {
      // Standard link semantics: plain click navigates in place; modifier /
      // middle clicks open tabs. Callbacks without an event (keyboard
      // affordances) count as plain clicks.
      intentNavigate(
        paths.issueDetail(issue.id),
        event ? resolveClickIntent(event) : "push",
        issue.identifier,
      );
    },
    [intentNavigate, paths],
  );

  const createSubIssue = useCallback(
    (issue: Issue) =>
      onCreateIssue({
        parent_issue_id: issue.id,
        parent_issue_identifier: issue.identifier,
        ...(issue.project_id ? { project_id: issue.project_id } : {}),
      }),
    [onCreateIssue],
  );

  const onSort = useCallback(
    (field: SortField, direction: "asc" | "desc") => {
      setSortBy(field);
      setSortDirection(direction);
    },
    [setSortBy, setSortDirection],
  );

  // Fresh object every render is fine — cells read it through
  // table.options.meta at render time. What must NOT change per render are
  // the column defs' component identities below.
  const viewMeta: TableViewMeta = {
    childProgressMap,
    propertyById,
    properties,
    fieldById,
    visibleIssueIds,
    editingCellSession,
    openEditingCell,
    closeEditingCell,
    restoreEditingCell,
    updateField,
    writable: dataSource.capabilities.writable,
    openIssue,
    createSubIssue,
    toggleTableParentCollapsed,
    handleIssueSelection,
    getActorName,
    columnLabel,
    sortBy,
    sortDirection,
    onSort,
    toggleTableColumn,
  };

  const columns = useMemo<ColumnDef<IssueTableDisplayRow>[]>(
    () => [
      {
        id: SELECT_COLUMN_ID,
        size: 44,
        minSize: 44,
        maxSize: 44,
        enableResizing: false,
        header: IssueTableSelectHeader,
        cell: IssueTableSelectCell,
      },
      ...visibleColumnConfigs.map((config): ColumnDef<IssueTableDisplayRow> => {
        const definition: ColumnDef<IssueTableDisplayRow> = {
          id: config.key,
          minSize: config.key === "title" ? 260 : 96,
          maxSize: 640,
          enableResizing: true,
          header: IssueTableHeaderCell,
          cell: IssueTableBodyCell,
        };
        if (config.width !== undefined) definition.size = config.width;
        return definition;
      }),
      {
        id: ADD_COLUMN_ID,
        size: 48,
        minSize: 48,
        maxSize: 48,
        enableResizing: false,
        header: IssueTableAddColumnHeader,
        cell: IssueTableEmptyCell,
      },
    ],
    [visibleColumnConfigs],
  );

  const columnSizing = useMemo<ColumnSizingState>(
    () =>
      Object.fromEntries(
        visibleColumnConfigs
          .filter((column) => column.width !== undefined)
          .map((column) => [column.key, column.width!]),
      ),
    [visibleColumnConfigs],
  );
  const handleColumnSizingChange = useCallback<OnChangeFn<ColumnSizingState>>(
    (updater) => {
      const next = typeof updater === "function" ? updater(columnSizing) : updater;
      for (const column of visibleColumnConfigs) {
        const width = next[column.key];
        if (width !== column.width) setTableColumnWidth(column.key, width);
      }
    },
    [columnSizing, setTableColumnWidth, visibleColumnConfigs],
  );

  const handleColumnReorder = useCallback(
    (activeId: string, overId: string) => {
      reorderTableColumn(activeId as TableColumnKey, overId as TableColumnKey);
    },
    [reorderTableColumn],
  );

  const handleExport = async (mode: "all" | "selected") => {
    setExporting(mode);
    try {
      // Every lookup the file depends on is AWAITED here rather than read
      // from render-time hook state — a cold or errored query must fail the
      // export, not silently degrade it: with an unsettled catalog the
      // user's property columns vanish from the file, and with unsettled
      // directories every actor exports as "Unknown *" (round-2 review
      // P2#4). fetchQuery throws on failure, which lands in the catch below.
      const needsPropertyCatalog = tableColumns.some(
        (column) => propertyIdFromViewKey(column.key) !== null,
      );
      // fetchQuery bypasses the options' `select`, so unwrap the response.
      const exportProperties = needsPropertyCatalog
        ? (await queryClient.fetchQuery(propertyListOptions(wsId))).properties
        : [];
      const exportPropertyById = new Map(
        exportProperties.map((property) => [property.id, property]),
      );
      // Configured columns against the RESOLVED catalog — only a property
      // definition that is genuinely gone (archived/deleted) drops out.
      const csvColumns = tableColumns.filter((column) => {
        const propertyId = propertyIdFromViewKey(column.key);
        return !propertyId || exportPropertyById.has(propertyId);
      });
      const needsActors = csvColumns.some((column) => {
        if (column.key === "assignee" || column.key === "creator") return true;
        const propertyId = propertyIdFromViewKey(column.key);
        const property = propertyId ? exportPropertyById.get(propertyId) : undefined;
        return property ? isActorPropertyType(property.type) : false;
      });
      const [rows, exportLookups, exportActorName] = await Promise.all([
        mode === "all" ? exportIssues() : Promise.resolve(selectedIssues),
        resolveExportLookups({
          projects: csvColumns.some((column) => column.key === "project"),
          childProgress: csvColumns.some(
            (column) => column.key === "child_progress",
          ),
        }),
        needsActors
          ? Promise.all([
              queryClient.fetchQuery(memberListOptions(wsId)),
              queryClient.fetchQuery(agentListOptions(wsId)),
              queryClient.fetchQuery(squadListOptions(wsId)),
            ]).then(([members, agents, squads]) =>
              buildActorNameResolver({ members, agents, squads }),
            )
          : Promise.resolve(getActorName),
      ]);
      const headers = csvColumns.map((column) => {
        const propertyId = propertyIdFromViewKey(column.key);
        if (propertyId) return exportPropertyById.get(propertyId)?.name ?? "";
        return columnLabel(column.key);
      });
      const csvRows = rows.map((issue) =>
        csvColumns.map((column) => {
          const propertyId = propertyIdFromViewKey(column.key);
          if (propertyId) {
            const property = exportPropertyById.get(propertyId);
            return property
              ? propertyDisplayValue(
                  property,
                  issue.properties[propertyId],
                  exportActorName,
                )
              : "";
          }
          switch (column.key) {
            case "title":
              return issue.title;
            case "identifier":
              return issue.identifier;
            case "status":
              return resolveStatusLabel(issue.status);
            case "priority":
              return t(($) => $.priority[issue.priority]);
            case "assignee":
              return issue.assignee_type && issue.assignee_id
                ? exportActorName(issue.assignee_type, issue.assignee_id)
                : "";
            case "labels":
              return issue.labels?.map((label) => label.name).join(", ") ?? "";
            case "project":
              return issue.project_id
                ? exportLookups.projectMap.get(issue.project_id)?.title ?? ""
                : "";
            case "start_date":
            case "due_date":
              return issue[column.key] ?? "";
            case "created_at":
            case "updated_at":
              return issue[column.key];
            case "child_progress": {
              const progress = exportLookups.childProgressMap.get(issue.id);
              return progress ? `${progress.done}/${progress.total}` : "";
            }
            case "creator":
              return exportActorName(issue.creator_type, issue.creator_id);
          }
          return "";
        }),
      );
      const csv = buildIssueTableCsv(headers, csvRows);
      const blob = new Blob(["\uFEFF", csv], { type: "text/csv;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      const filenamePrefix = mode === "all" ? "issues" : "issues-selected";
      anchor.download = `${filenamePrefix}-${new Date().toISOString().slice(0, 10)}.csv`;
      anchor.click();
      URL.revokeObjectURL(url);
      toast.success(t(($) => $.table.export_success, { count: rows.length }));
    } catch (error) {
      toast.error(
        error instanceof Error &&
          !(error instanceof IssueTableExportIntegrityError) &&
          error.message
          ? error.message
          : t(($) => $.table.export_failed),
      );
    } finally {
      setExporting(null);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b px-3 py-1.5">
        <TableIssueSearch
          value={search}
          onChange={onSearchChange}
          placeholder={t(($) => $.table.search_placeholder)}
          clearLabel={t(($) => $.table.search_clear)}
        />
        <span className="mr-auto" />
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="sm"
                className="h-7"
                disabled={exporting !== null}
              >
                {exporting ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Download className="size-3.5" />
                )}
                {t(($) => $.table.export)}
                <ChevronDown className="size-3" />
              </Button>
            }
          />
          <DropdownMenuContent align="end" className="w-52">
            <DropdownMenuItem onClick={() => void handleExport("all")}>
              <Download className="size-3.5" />
              {t(($) => $.table.export_all)}
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={selectedIssues.length === 0}
              onClick={() => void handleExport("selected")}
            >
              <Download className="size-3.5" />
              {t(($) => $.table.export_selected, {
                count: selectedIssues.length,
              })}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <SharedTableView
        sourceIdentity={dataSource.identity}
        writable={dataSource.capabilities.writable}
        rows={serverDisplayRows}
        columns={columns}
        rowId={(row) => row.key}
        visibleColumnIds={visibleColumnConfigs.map((column) => column.key)}
        columnSizing={columnSizing}
        onColumnSizingChange={handleColumnSizingChange}
        onReorderColumn={handleColumnReorder}
        columnPinning={{ left: [SELECT_COLUMN_ID, "title"], right: [] }}
        meta={viewMeta as TableMeta<IssueTableDisplayRow>}
        editingKey={editingCellKey}
        refreshFrozenRows={(snapshot) =>
          refreshFrozenTableRows(snapshot, issueById)
        }
        emptyMessage={t(($) => $.table.empty)}
        onRowClick={(row, event) => {
          if (row.original.kind === "issue") {
            openIssue(row.original.issue, event);
          }
        }}
        renderStructuralRow={(row) => {
          if (row.original.kind === "group") {
            const group = row.original;
            return {
              rowClassName: "bg-muted/40 hover:bg-muted/60",
              cellClassName: "h-9 px-4 py-1.5",
              content: (
                <IssueTableGroupContent
                  group={group}
                  onToggle={() => toggleTableGroupCollapsed(group.key)}
                />
              ),
            };
          }
          if (row.original.kind === "load_more") {
            const loadMoreRow = row.original;
            return {
              rowClassName: "hover:bg-transparent",
              cellClassName: "p-0",
              content: (
                <div className="sticky left-0 w-full">
                  <ListLoadMoreFooter
                    hasMore={
                      loadMoreRow.state === "loading" ||
                      loadMoreRow.state === "has_more"
                    }
                    isLoading={loadMoreRow.state === "loading"}
                    total={loadMoreRow.total}
                    onLoadMore={() => loadMoreRow.onLoad?.()}
                    isError={loadMoreRow.state === "error"}
                    onRetry={loadMoreRow.onLoad}
                  />
                </div>
              ),
            };
          }
          return null;
        }}
        className="min-h-0 flex-1"
      />
    </div>
  );
}
