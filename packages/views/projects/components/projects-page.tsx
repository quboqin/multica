"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  Filter,
  FolderKanban,
  LayoutGrid,
  MoreHorizontal,
  Pin,
  PinOff,
  Plus,
  Rows3,
  Search,
  Trash2,
  X,
} from "lucide-react";
import {
  closestCenter,
  DndContext,
  DragOverlay,
  PointerSensor,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  defaultAnimateLayoutChanges,
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
  type AnimateLayoutChanges,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  projectListOptions,
  useUpdateProject,
  useDeleteProject,
  useProjectViewStore,
  type ProjectColumnKey,
  type ProjectListFilters,
  type ProjectSortField,
} from "@multica/core/projects";
import {
  pinListOptions,
  useCreatePin,
  useDeletePin,
} from "@multica/core/pins";
import { milestoneListOptions } from "@multica/core/milestones";
import {
  PROJECT_PRIORITY_CONFIG,
  PROJECT_STATUS_CONFIG,
  PROJECT_STATUS_ORDER,
} from "@multica/core/projects/config";
import { formatDateOnly, isPastDateOnly } from "@multica/core/issues/date";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { useAuthStore } from "@multica/core/auth";
import { useActorName } from "@multica/core/workspace/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import { useModalStore } from "@multica/core/modals";
import { AppLink } from "../../navigation";
import { ActorAvatar } from "../../common/actor-avatar";
import { FILTER_ITEM_CLASS, HoverCheck } from "../../common/hover-check";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { cn } from "@multica/ui/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import {
  ListGrid,
  ListGridCell,
  ListGridHeader,
  ListGridHeaderCell,
  ListGridRow,
  LIST_GRID_BOTTOM_CLEARANCE,
  type ListGridSortDirection,
} from "@multica/ui/components/ui/list-grid";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import type {
  Milestone,
  MemberWithUser,
  Project,
  ProjectPriority,
  ProjectStatus,
  UpdateProjectRequest,
} from "@multica/core/types";
import { PageHeader } from "../../layout/page-header";
import { ProjectIcon } from "./project-icon";
import { useT } from "../../i18n";
import { matchesPinyin } from "../../editor/extensions/pinyin-match";
import { useFormatRelativeDate, useProjectPriorityLabels, useProjectStatusLabels } from "./labels";
import { ProjectStatusBadge, ProjectPriorityBadge } from "./project-badge";
import { ProjectLeadPicker } from "./project-lead-picker";
import { LabelChip } from "../../labels/label-chip";
import { Markdown } from "../../common/markdown";

// Sort order maps for the enum columns (header sort needs a total order).
const PRIORITY_ORDER: Record<ProjectPriority, number> = {
  urgent: 4,
  high: 3,
  medium: 2,
  low: 1,
  none: 0,
};
const STATUS_ORDER: Record<ProjectStatus, number> = {
  planned: 0,
  in_progress: 1,
  paused: 2,
  completed: 3,
  cancelled: 4,
};

const PROJECT_STATUS_COLUMN_BG: Record<ProjectStatus, string> = {
  planned: "bg-muted/35",
  in_progress: "bg-warning/5",
  paused: "bg-muted/25",
  completed: "bg-info/5",
  cancelled: "bg-destructive/5",
};

const animateLayoutChanges: AnimateLayoutChanges = (args) => {
  const { isSorting, wasDragging } = args;
  if (isSorting || wasDragging) return false;
  return defaultAnimateLayoutChanges(args);
};

type ProjectColumns = Record<ProjectStatus, string[]>;

function projectStatusColumnId(status: ProjectStatus): string {
  return `project-status:${status}`;
}

function projectStatusFromColumnId(id: string): ProjectStatus | null {
  const status = id.startsWith("project-status:")
    ? id.slice("project-status:".length)
    : id;
  return PROJECT_STATUS_ORDER.includes(status as ProjectStatus)
    ? (status as ProjectStatus)
    : null;
}

function buildProjectColumns(projects: Project[]): ProjectColumns {
  const columns: ProjectColumns = {
    planned: [],
    in_progress: [],
    paused: [],
    completed: [],
    cancelled: [],
  };

  for (const project of projects) {
    columns[project.status].push(project.id);
  }

  return columns;
}

function findProjectColumn(
  columns: ProjectColumns,
  id: string,
): ProjectStatus | null {
  const status = projectStatusFromColumnId(id);
  if (status) return status;

  for (const columnStatus of PROJECT_STATUS_ORDER) {
    if (columns[columnStatus].includes(id)) return columnStatus;
  }

  return null;
}

function makeProjectKanbanCollision(columnIds: Set<string>): CollisionDetection {
  return (args) => {
    const pointer = pointerWithin(args);
    if (pointer.length > 0) {
      const items = pointer.filter(
        (collision) => !columnIds.has(collision.id as string),
      );
      if (items.length > 0) return items;
      return pointer;
    }

    return closestCenter(args);
  };
}

const progressOf = (p: Project) =>
  p.issue_count > 0 ? p.done_count / p.issue_count : -1;

function formatProjectDate(date: string | null | undefined): string {
  return formatDateOnly(date, { month: "short", day: "numeric" }, "en-US");
}

function milestoneStatusDotClass(status: string): string {
  if (status === "completed" || status === "cancelled") {
    return "bg-muted-foreground";
  }
  if (status === "in_progress") return "bg-warning";
  if (status === "paused") return "bg-muted-foreground";
  return "bg-info";
}

// Composite "type:id" lead value so the string[] filter holds member/agent
// refs alike.
function leadFilterValue(p: Project): string | null {
  return p.lead_type && p.lead_id ? `${p.lead_type}:${p.lead_id}` : null;
}

// ---------------------------------------------------------------------------
// Table (compact) view — ListGrid. Name + status are the core columns;
// progress/lead/plan/due/updated/issues/created collapse below @2xl, with min-width
// + the wrapper's overflow as the escape valve. The title link keeps direct
// navigation; non-control row surface opens the quick project preview.
// ---------------------------------------------------------------------------

const COLUMN_WIDTHS: Record<ProjectColumnKey, number> = {
  progress: 88,
  lead: 132,
  plan: 148,
  due_date: 104,
  updated: 104,
  issues: 80,
  created: 104,
};

// Fixed tracks: edges 12+12, checkbox 16, name min 240, status 116,
// kebab 28 = 424, plus the 12 gap-x-3 gaps between the wide template's
// 13 tracks.
const FIXED_TRACKS_WIDTH = 424 + 12 * 12;

// Render/track order: checkbox, name, status (core, fixed 116px),
// progress, lead, plan, due, updated, issues, created, kebab. MUST be a literal string —
// Tailwind can't see interpolated `grid-cols-[...]` arbitrary values, so an
// interpolated width silently drops the whole template and the grid
// collapses to one column.
const GRID_COLS =
  "grid-cols-[0.75rem_1rem_minmax(120px,1fr)_116px_1.75rem_0.75rem] " +
  "@2xl:grid-cols-[0.75rem_1rem_minmax(240px,1fr)_116px_var(--pjc-progress)_var(--pjc-lead)_var(--pjc-plan)_var(--pjc-due)_var(--pjc-updated)_var(--pjc-issues)_var(--pjc-created)_1.75rem_0.75rem]";

const stopRowNavigation = (e: MouseEvent) => e.stopPropagation();

function columnTrackVars(
  isVisible: (key: ProjectColumnKey) => boolean,
): React.CSSProperties {
  const width = (key: ProjectColumnKey) =>
    isVisible(key) ? `${COLUMN_WIDTHS[key]}px` : "0px";
  const minWidth =
    FIXED_TRACKS_WIDTH +
    (Object.keys(COLUMN_WIDTHS) as ProjectColumnKey[]).reduce(
      (sum, key) => sum + (isVisible(key) ? COLUMN_WIDTHS[key] : 0),
      0,
    );
  return {
    "--pjc-progress": width("progress"),
    "--pjc-lead": width("lead"),
    "--pjc-plan": width("plan"),
    "--pjc-due": width("due_date"),
    "--pjc-updated": width("updated"),
    "--pjc-issues": width("issues"),
    "--pjc-created": width("created"),
    "--pjc-minw": `${minWidth}px`,
  } as React.CSSProperties;
}

function ProgressRing({ project }: { project: Project }) {
  if (project.issue_count === 0) {
    return <span className="text-xs text-muted-foreground/40">—</span>;
  }
  const pct = Math.round((project.done_count / project.issue_count) * 100);
  return (
    <span className="flex items-center gap-1.5">
      <span className="relative h-3.5 w-3.5">
        <svg className="h-3.5 w-3.5 -rotate-90" viewBox="0 0 16 16">
          <circle className="text-muted" strokeWidth="2" stroke="currentColor" fill="none" r="6" cx="8" cy="8" />
          <circle
            className="text-emerald-500"
            strokeWidth="2"
            stroke="currentColor"
            fill="none"
            r="6"
            cx="8"
            cy="8"
            strokeDasharray={`${pct * 0.377} 37.7`}
            strokeLinecap="round"
          />
        </svg>
      </span>
      <span className="text-xs tabular-nums text-muted-foreground">
        {project.done_count}/{project.issue_count}
      </span>
    </span>
  );
}

// Compact rows open a preview from the non-control row surface; callers stop
// propagation around this menu so action clicks stay local.
function ProjectRowActions({
  project,
  pinned,
  canDelete,
}: {
  project: Project;
  pinned: boolean;
  canDelete: boolean;
}) {
  const { t } = useT("projects");
  const createPin = useCreatePin();
  const deletePin = useDeletePin();
  const deleteProject = useDeleteProject();
  const [deleteOpen, setDeleteOpen] = useState(false);

  const togglePin = () => {
    if (pinned) deletePin.mutate({ itemType: "project", itemId: project.id });
    else createPin.mutate({ item_type: "project", item_id: project.id });
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              aria-label={t(($) => $.page.row_menu)}
              className="flex size-7 items-center justify-center rounded-md text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-accent-foreground group-hover/row:opacity-100 data-popup-open:bg-accent data-popup-open:opacity-100 data-popup-open:text-accent-foreground"
            >
              <MoreHorizontal className="size-4" />
            </button>
          }
        />
        <DropdownMenuContent align="end" className="w-44">
          <DropdownMenuItem onClick={togglePin}>
            {pinned ? (
              <PinOff className="size-3.5" />
            ) : (
              <Pin className="size-3.5" />
            )}
            {pinned ? t(($) => $.page.unpin) : t(($) => $.page.pin)}
          </DropdownMenuItem>
          {canDelete && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 className="size-3.5" />
                {t(($) => $.page.delete)}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t(($) => $.delete_dialog.title)}</DialogTitle>
            <DialogDescription>
              {t(($) => $.delete_dialog.description)}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setDeleteOpen(false)}
            >
              {t(($) => $.delete_dialog.cancel)}
            </Button>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={() => {
                deleteProject.mutate(project.id, {
                  onError: (err) =>
                    toast.error(
                      err instanceof Error ? err.message : String(err),
                    ),
                });
                setDeleteOpen(false);
              }}
            >
              {t(($) => $.delete_dialog.confirm)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function CheckboxCell({
  checked,
  onToggle,
}: {
  checked: boolean;
  onToggle: () => void;
}) {
  return (
    <ListGridCell className="justify-center px-0">
      <button
        type="button"
        aria-pressed={checked}
        onClick={(e) => {
          stopRowNavigation(e);
          onToggle();
        }}
        onAuxClick={stopRowNavigation}
        className={`-m-1.5 flex items-center p-1.5 ${
          checked ? "" : "opacity-0 transition-opacity group-hover/row:opacity-100"
        }`}
      >
        <Checkbox checked={checked} tabIndex={-1} className="pointer-events-none" />
      </button>
    </ListGridCell>
  );
}

function ProjectTableRow({
  project,
  milestone,
  pinned,
  canDelete,
  isColVisible,
  selected,
  onToggleSelect,
  rowHref,
  onOpenPreview,
}: {
  project: Project;
  milestone: Milestone | null;
  pinned: boolean;
  canDelete: boolean;
  isColVisible: (key: ProjectColumnKey) => boolean;
  selected: boolean;
  onToggleSelect: () => void;
  rowHref: string;
  onOpenPreview: (project: Project) => void;
}) {
  const formatRelativeDate = useFormatRelativeDate();
  const updateProject = useUpdateProject();
  const handleUpdate = useCallback(
    (data: UpdateProjectRequest) => updateProject.mutate({ id: project.id, ...data }),
    [project.id, updateProject],
  );

  return (
    <ListGridRow
      className={`h-auto min-h-14 cursor-pointer py-1.5 ${selected ? "bg-accent/30" : ""}`}
      onClick={() => onOpenPreview(project)}
    >
      <CheckboxCell checked={selected} onToggle={onToggleSelect} />
      <ListGridCell className="items-start gap-2">
        <ProjectIcon project={project} size="sm" className="mt-0.5" />
        <AppLink
          href={rowHref}
          className="min-w-0 flex-1 break-words text-sm font-medium leading-snug line-clamp-2 hover:text-primary"
          title={project.title}
          onClickCapture={stopRowNavigation}
          onAuxClick={stopRowNavigation}
        >
          {project.title}
        </AppLink>
      </ListGridCell>

      {/* status — core column, always visible */}
      <ListGridCell onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
        <ProjectStatusBadge project={project} handleUpdate={handleUpdate} align="start" />
      </ListGridCell>

      {isColVisible("progress") ? (
        <ListGridCell className="hidden @2xl:flex">
          <ProgressRing project={project} />
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      {isColVisible("lead") ? (
        <ListGridCell className="hidden @2xl:flex" onClick={stopRowNavigation} onAuxClick={stopRowNavigation}>
          <ProjectLeadPicker
            project={project}
            handleUpdate={handleUpdate}
            align="start"
            renderTrigger={(leadName) => (
              <button
                type="button"
                className="flex min-w-0 items-center gap-1.5 rounded px-1 py-0.5 transition-colors hover:bg-accent/60"
              >
                {project.lead_type && project.lead_id ? (
                  <ActorAvatar actorType={project.lead_type} actorId={project.lead_id} size={18} enableHoverCard />
                ) : (
                  <span className="inline-flex h-[18px] w-[18px] rounded-full border border-dashed border-muted-foreground/30" />
                )}
                <span className="min-w-0 truncate text-xs text-muted-foreground">
                  {leadName ?? "—"}
                </span>
              </button>
            )}
          />
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      {isColVisible("plan") ? (
        <ListGridCell className="hidden @2xl:flex">
          {milestone ? (
            <span className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
              <span className={cn("size-2 shrink-0 rounded-full", milestoneStatusDotClass(milestone.status))} />
              <span className="min-w-0 truncate">{milestone.title}</span>
            </span>
          ) : (
            <span className="text-xs text-muted-foreground/50">—</span>
          )}
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      {isColVisible("due_date") ? (
        <ListGridCell className="hidden whitespace-nowrap text-xs tabular-nums @2xl:flex">
          {project.due_date ? (
            <span className={cn(isPastDateOnly(project.due_date) ? "text-destructive" : "text-muted-foreground")}>
              {formatProjectDate(project.due_date)}
            </span>
          ) : (
            <span className="text-muted-foreground/50">—</span>
          )}
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      {isColVisible("updated") ? (
        <ListGridCell className="hidden whitespace-nowrap text-xs tabular-nums text-muted-foreground @2xl:flex">
          {formatRelativeDate(project.updated_at)}
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      {isColVisible("issues") ? (
        <ListGridCell className="hidden justify-end font-mono text-xs tabular-nums text-muted-foreground @2xl:flex">
          {project.issue_count}
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      {isColVisible("created") ? (
        <ListGridCell className="hidden whitespace-nowrap text-xs tabular-nums text-muted-foreground @2xl:flex">
          {formatRelativeDate(project.created_at)}
        </ListGridCell>
      ) : (
        <ListGridCell className="hidden px-0 @2xl:flex" />
      )}

      <ListGridCell className="justify-end px-0">
        <span onClick={stopRowNavigation} onAuxClick={stopRowNavigation} className="flex items-center">
          <ProjectRowActions project={project} pinned={pinned} canDelete={canDelete} />
        </span>
      </ListGridCell>
    </ListGridRow>
  );
}

function ProjectTableHeader({
  sortField,
  sortDirection,
  onSort,
  isColVisible,
  allSelected,
  someSelected,
  onToggleAll,
}: {
  sortField: ProjectSortField;
  sortDirection: ListGridSortDirection;
  onSort: (field: ProjectSortField) => void;
  isColVisible: (key: ProjectColumnKey) => boolean;
  allSelected: boolean;
  someSelected: boolean;
  onToggleAll: () => void;
}) {
  const { t } = useT("projects");
  const sorted = (field: ProjectSortField) =>
    sortField === field ? sortDirection : false;
  const anySelected = allSelected || someSelected;
  return (
    <ListGridHeader>
      <div className="flex items-center justify-center">
        <button
          type="button"
          aria-pressed={allSelected}
          onClick={onToggleAll}
          className={`-m-1.5 flex items-center p-1.5 ${
            anySelected ? "" : "opacity-0 transition-opacity group-hover/header:opacity-100"
          }`}
        >
          <Checkbox
            checked={allSelected}
            indeterminate={someSelected && !allSelected}
            tabIndex={-1}
            className="pointer-events-none"
          />
        </button>
      </div>
      <ListGridHeaderCell sorted={sorted("name")} onSort={() => onSort("name")}>
        {t(($) => $.table.name)}
      </ListGridHeaderCell>
      <ListGridHeaderCell sorted={sorted("status")} onSort={() => onSort("status")}>
        {t(($) => $.table.status)}
      </ListGridHeaderCell>
      {isColVisible("progress") ? (
        <ListGridHeaderCell
          className="hidden @2xl:flex"
          sorted={sorted("progress")}
          onSort={() => onSort("progress")}
        >
          {t(($) => $.table.progress)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      {isColVisible("lead") ? (
        <ListGridHeaderCell className="hidden @2xl:flex">
          {t(($) => $.table.lead)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      {isColVisible("plan") ? (
        <ListGridHeaderCell className="hidden @2xl:flex">
          {t(($) => $.table.plan)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      {isColVisible("due_date") ? (
        <ListGridHeaderCell
          className="hidden @2xl:flex"
          sorted={sorted("due_date")}
          onSort={() => onSort("due_date")}
        >
          {t(($) => $.table.due_date)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      {isColVisible("updated") ? (
        <ListGridHeaderCell
          className="hidden @2xl:flex"
          sorted={sorted("updated")}
          onSort={() => onSort("updated")}
        >
          {t(($) => $.table.updated)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      {isColVisible("issues") ? (
        <ListGridHeaderCell className="hidden justify-end @2xl:flex" align="right">
          {t(($) => $.table.issues)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      {isColVisible("created") ? (
        <ListGridHeaderCell
          className="hidden @2xl:flex"
          sorted={sorted("created")}
          onSort={() => onSort("created")}
        >
          {t(($) => $.table.created)}
        </ListGridHeaderCell>
      ) : (
        <ListGridHeaderCell className="hidden px-0 @2xl:flex" />
      )}
      <span aria-hidden="true" />
    </ListGridHeader>
  );
}

// ---------------------------------------------------------------------------
// Card (comfortable) view — kept from the prior page.
// ---------------------------------------------------------------------------

function ProjectCard({
  project,
  pinned,
  canDelete,
}: {
  project: Project;
  pinned: boolean;
  canDelete: boolean;
}) {
  const { t } = useT("projects");
  const wsPaths = useWorkspacePaths();
  const formatRelativeDate = useFormatRelativeDate();
  const updateProject = useUpdateProject();
  const handleUpdate = useCallback(
    (data: UpdateProjectRequest) => updateProject.mutate({ id: project.id, ...data }),
    [project.id, updateProject],
  );
  const progressPercent =
    project.issue_count > 0
      ? Math.round((project.done_count / project.issue_count) * 100)
      : 0;

  return (
    <div className="group/card group/row flex flex-col rounded-md border bg-card transition-colors hover:border-primary/50">
      <div className="p-3 pb-2">
        <div className="flex items-start gap-2">
          <AppLink
            href={wsPaths.projectDetail(project.id)}
            className="flex min-w-0 flex-1 items-start gap-2"
          >
            <ProjectIcon project={project} size="sm" className="mt-0.5" />
            <h3
              title={project.title}
              className="min-w-0 flex-1 break-words text-sm font-medium leading-snug line-clamp-2"
            >
              {project.title}
            </h3>
          </AppLink>
          <ProjectRowActions project={project} pinned={pinned} canDelete={canDelete} />
        </div>

        <div className="mt-2 flex min-h-5 items-center justify-between gap-2">
          <ProjectStatusBadge project={project} handleUpdate={handleUpdate} triggerClassName="shrink-0" align="start" />
          {project.issue_count > 0 ? (
            <div className="flex items-center justify-end gap-1.5">
              <div className="relative h-4 w-4">
                <svg className="h-4 w-4 -rotate-90" viewBox="0 0 16 16">
                  <circle className="text-muted" strokeWidth="2" stroke="currentColor" fill="none" r="6" cx="8" cy="8" />
                  <circle
                    className="text-emerald-500"
                    strokeWidth="2"
                    stroke="currentColor"
                    fill="none"
                    r="6"
                    cx="8"
                    cy="8"
                    strokeDasharray={`${progressPercent * 0.377} 37.7`}
                    strokeLinecap="round"
                  />
                </svg>
              </div>
              <span className="text-[10px] tabular-nums text-muted-foreground">
                {project.done_count}/{project.issue_count}
              </span>
            </div>
          ) : (
            <span className="text-[10px] text-muted-foreground">
              {t(($) => $.detail.no_issues_yet)}
            </span>
          )}
        </div>
      </div>

      <div className="mt-0 flex items-center justify-between border-t px-3 pb-3 pt-2">
        <ProjectLeadPicker
          project={project}
          handleUpdate={handleUpdate}
          renderTrigger={(leadName) => (
            <button type="button" className="-mx-1.5 flex items-center gap-1.5 rounded px-1.5 py-0.5 transition-colors hover:bg-accent/60">
              {project.lead_type && project.lead_id ? (
                <ActorAvatar actorType={project.lead_type} actorId={project.lead_id} size={20} enableHoverCard />
              ) : (
                <span className="inline-flex h-5 w-5 rounded-full border border-dashed border-muted-foreground/30" />
              )}
              <span className="max-w-[60px] truncate text-[10px] text-muted-foreground">
                {leadName ?? t(($) => $.lead.no_lead)}
              </span>
            </button>
          )}
        />
        <div className="flex items-center gap-2">
          <ProjectPriorityBadge project={project} handleUpdate={handleUpdate} align="start" />
          <span className="text-[10px] text-muted-foreground">
            {formatRelativeDate(project.created_at)}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Toolbar — search + result count + filter + display (compact only) + view
// toggle.
// ---------------------------------------------------------------------------

const STATUS_VALUES: ProjectStatus[] = [
  "planned",
  "in_progress",
  "paused",
  "completed",
  "cancelled",
];
const PRIORITY_VALUES: ProjectPriority[] = ["urgent", "high", "medium", "low", "none"];
const COLUMN_KEYS: ProjectColumnKey[] = [
  "progress",
  "lead",
  "plan",
  "due_date",
  "updated",
  "issues",
  "created",
];
const SORT_FIELDS: ProjectSortField[] = [
  "name",
  "priority",
  "status",
  "progress",
  "due_date",
  "updated",
  "created",
];

function countActiveFilters(f: ProjectListFilters): number {
  let c = 0;
  if (f.statuses.length) c++;
  if (f.priorities.length) c++;
  if (f.leads.length) c++;
  return c;
}

// Batch toolbar — page-anchored (not viewport). Pin all selected (any
// member) + Delete (workspace admin). Mirrors the other lists.
function ProjectBatchToolbar({
  rows,
  pinnedIds,
  canDelete,
  onClear,
}: {
  rows: Project[];
  pinnedIds: Set<string>;
  canDelete: boolean;
  onClear: () => void;
}) {
  const { t } = useT("projects");
  const createPin = useCreatePin();
  const deleteProject = useDeleteProject();
  const [confirmDelete, setConfirmDelete] = useState(false);

  if (rows.length === 0) return null;
  const anyUnpinned = rows.some((p) => !pinnedIds.has(p.id));

  return (
    <>
      <div className="absolute bottom-6 left-1/2 z-50 flex -translate-x-1/2 items-center gap-1 rounded-lg border bg-background px-2 py-1.5 shadow-lg">
        <div className="mr-1 flex items-center gap-1.5 border-r pl-1 pr-2">
          <span className="text-sm font-medium">
            {t(($) => $.page.selected, { count: rows.length })}
          </span>
          <button
            type="button"
            aria-label={t(($) => $.page.clear_selection)}
            onClick={onClear}
            className="rounded p-0.5 transition-colors hover:bg-accent"
          >
            <X className="size-3.5 text-muted-foreground" />
          </button>
        </div>
        {anyUnpinned && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              for (const p of rows) {
                if (!pinnedIds.has(p.id)) {
                  createPin.mutate({ item_type: "project", item_id: p.id });
                }
              }
              onClear();
            }}
          >
            <Pin className="mr-1 size-3.5" />
            {t(($) => $.page.pin)}
          </Button>
        )}
        {canDelete && (
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={() => setConfirmDelete(true)}
          >
            <Trash2 className="mr-1 size-3.5" />
            {t(($) => $.page.delete)}
          </Button>
        )}
      </div>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t(($) => $.delete_dialog.title)}</DialogTitle>
            <DialogDescription>{t(($) => $.delete_dialog.description)}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" size="sm" onClick={() => setConfirmDelete(false)}>
              {t(($) => $.delete_dialog.cancel)}
            </Button>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={() => {
                for (const p of rows) deleteProject.mutate(p.id);
                setConfirmDelete(false);
                onClear();
              }}
            >
              {t(($) => $.delete_dialog.confirm)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function ProjectPreviewProp({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] items-start gap-3 py-1.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      <div className="min-w-0 text-xs text-foreground">{children}</div>
    </div>
  );
}

function ProjectQuickViewDialog({
  project,
  milestone,
  open,
  onOpenChange,
  getActorName,
}: {
  project: Project | null;
  milestone: Milestone | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  getActorName: (type: string, id: string) => string;
}) {
  const { t } = useT("projects");
  const statusLabels = useProjectStatusLabels();
  const priorityLabels = useProjectPriorityLabels();

  if (!project) return null;

  const statusCfg = PROJECT_STATUS_CONFIG[project.status];
  const priorityCfg = PROJECT_PRIORITY_CONFIG[project.priority];
  const leadName =
    project.lead_type && project.lead_id
      ? getActorName(project.lead_type, project.lead_id)
      : t(($) => $.lead.no_lead);
  const progress =
    project.issue_count > 0
      ? `${project.done_count}/${project.issue_count}`
      : t(($) => $.detail.no_issues_yet);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[min(720px,calc(100vh-2rem))] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <div className="flex min-w-0 items-start gap-2 pr-6">
            <ProjectIcon project={project} size="sm" className="mt-0.5" />
            <DialogTitle className="min-w-0 break-words leading-snug">
              {project.title}
            </DialogTitle>
          </div>
          <DialogDescription className="sr-only">
            {t(($) => $.preview.description)}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-5 md:grid-cols-[minmax(0,1fr)_14rem]">
          <section className="min-w-0">
            <h3 className="mb-2 text-xs font-medium text-muted-foreground">
              {t(($) => $.detail.section_description)}
            </h3>
            {project.description ? (
              <div className="max-h-80 overflow-y-auto rounded-md border bg-background/60 px-3 py-2">
                <Markdown className="prose-sm max-w-none">
                  {project.description}
                </Markdown>
              </div>
            ) : (
              <div className="rounded-md border border-dashed px-3 py-6 text-center text-xs text-muted-foreground">
                {t(($) => $.preview.no_description)}
              </div>
            )}
          </section>

          <aside className="min-w-0 rounded-md border bg-muted/25 px-3 py-2">
            <ProjectPreviewProp label={t(($) => $.table.status)}>
              <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 font-medium", statusCfg.badgeBg, statusCfg.badgeText)}>
                {statusLabels[project.status]}
              </span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.priority)}>
              <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 font-medium", priorityCfg.badgeBg, priorityCfg.badgeText)}>
                {priorityLabels[project.priority]}
              </span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.lead)}>
              <span className="truncate text-muted-foreground">{leadName}</span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.plan)}>
              {milestone ? (
                <span className="flex min-w-0 items-center gap-1.5">
                  <span className={cn("size-2 shrink-0 rounded-full", milestoneStatusDotClass(milestone.status))} />
                  <span className="min-w-0 truncate">{milestone.title}</span>
                </span>
              ) : (
                <span className="text-muted-foreground">{t(($) => $.detail.no_plan)}</span>
              )}
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.progress)}>
              <span className="tabular-nums text-muted-foreground">{progress}</span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.start_date)}>
              <span className="tabular-nums text-muted-foreground">
                {formatProjectDate(project.start_date) || "—"}
              </span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.due_date)}>
              {project.due_date ? (
                <span className={cn("tabular-nums", isPastDateOnly(project.due_date) ? "text-destructive" : "text-muted-foreground")}>
                  {formatProjectDate(project.due_date)}
                </span>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.updated)}>
              <span className="tabular-nums text-muted-foreground">
                {formatProjectDate(project.updated_at) || project.updated_at}
              </span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.created)}>
              <span className="tabular-nums text-muted-foreground">
                {formatProjectDate(project.created_at) || project.created_at}
              </span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.issues)}>
              <span className="tabular-nums text-muted-foreground">{project.issue_count}</span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.preview.resources)}>
              <span className="tabular-nums text-muted-foreground">{project.resource_count}</span>
            </ProjectPreviewProp>
            <ProjectPreviewProp label={t(($) => $.table.labels)}>
              {project.labels && project.labels.length > 0 ? (
                <span className="flex flex-wrap gap-1">
                  {project.labels.map((label) => (
                    <LabelChip key={label.id} label={label} fullName />
                  ))}
                </span>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </ProjectPreviewProp>
          </aside>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ProjectStatusBuckets({
  projects,
  pinnedProjectIds,
  canDelete,
}: {
  projects: Project[];
  pinnedProjectIds: Set<string>;
  canDelete: boolean;
}) {
  const { t } = useT("projects");
  const statusLabels = useProjectStatusLabels();
  const updateProject = useUpdateProject();
  const [activeProject, setActiveProject] = useState<Project | null>(null);
  const [columns, setColumns] = useState<ProjectColumns>(() =>
    buildProjectColumns(projects),
  );
  const columnsRef = useRef(columns);
  columnsRef.current = columns;
  const isDraggingRef = useRef(false);
  const recentlyMovedRef = useRef(false);
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 6 },
    }),
  );
  const projectMap = useMemo(
    () => new Map(projects.map((project) => [project.id, project])),
    [projects],
  );
  const projectMapRef = useRef(projectMap);
  if (!isDraggingRef.current) {
    projectMapRef.current = projectMap;
  }
  const columnIds = useMemo(
    () => new Set(PROJECT_STATUS_ORDER.map(projectStatusColumnId)),
    [],
  );
  const collisionDetection = useMemo(
    () => makeProjectKanbanCollision(columnIds),
    [columnIds],
  );

  useEffect(() => {
    if (!isDraggingRef.current) {
      setColumns(buildProjectColumns(projects));
    }
  }, [projects]);

  useEffect(() => {
    const id = requestAnimationFrame(() => {
      recentlyMovedRef.current = false;
    });
    return () => cancelAnimationFrame(id);
  }, [columns]);

  const handleDragStart = useCallback((event: DragStartEvent) => {
    isDraggingRef.current = true;
    setActiveProject(projectMapRef.current.get(String(event.active.id)) ?? null);
  }, []);

  const handleDragOver = useCallback((event: DragOverEvent) => {
    const { active, over } = event;
    if (!over || recentlyMovedRef.current) return;

    const activeId = String(active.id);
    const overId = String(over.id);

    setColumns((prev) => {
      const activeCol = findProjectColumn(prev, activeId);
      const overCol = findProjectColumn(prev, overId);
      if (!activeCol || !overCol || activeCol === overCol) return prev;

      recentlyMovedRef.current = true;
      const oldIds = prev[activeCol].filter((id) => id !== activeId);
      const newIds = prev[overCol].filter((id) => id !== activeId);
      const overIndex = newIds.indexOf(overId);
      const insertIndex = overIndex >= 0 ? overIndex : newIds.length;
      newIds.splice(insertIndex, 0, activeId);
      return { ...prev, [activeCol]: oldIds, [overCol]: newIds };
    });
  }, []);

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      if (!event.over) {
        isDraggingRef.current = false;
        setActiveProject(null);
        setColumns(buildProjectColumns(projects));
        return;
      }

      const activeId = String(event.active.id);
      const overId = String(event.over.id);
      const project = activeProject ?? projectMapRef.current.get(activeId);
      const activeStatus = findProjectColumn(columnsRef.current, activeId);
      const overStatus = findProjectColumn(columnsRef.current, overId);
      const nextStatus = overStatus ?? activeStatus;
      isDraggingRef.current = false;
      setActiveProject(null);

      if (!project || !nextStatus) {
        setColumns(buildProjectColumns(projects));
        return;
      }

      if (nextStatus === project.status) {
        setColumns(buildProjectColumns(projects));
        return;
      }

      updateProject.mutate({ id: project.id, status: nextStatus });
    },
    [activeProject, projects, updateProject],
  );

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collisionDetection}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
      onDragCancel={() => {
        isDraggingRef.current = false;
        setActiveProject(null);
        setColumns(buildProjectColumns(projects));
      }}
    >
      <div className="flex h-full min-h-0 gap-3 overflow-x-auto px-5 py-4 pb-5">
        {PROJECT_STATUS_ORDER.map((status) => (
          <ProjectStatusColumn
            key={status}
            status={status}
            projectIds={columns[status]}
            projectMap={projectMapRef.current}
            pinnedProjectIds={pinnedProjectIds}
            canDelete={canDelete}
            statusLabel={statusLabels[status]}
            emptyLabel={t(($) => $.page.empty)}
          />
        ))}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeProject && (
          <div className="w-[280px]">
            <ProjectCard
              project={activeProject}
              pinned={pinnedProjectIds.has(activeProject.id)}
              canDelete={canDelete}
            />
          </div>
        )}
      </DragOverlay>
    </DndContext>
  );
}

function ProjectStatusColumn({
  status,
  projectIds,
  projectMap,
  pinnedProjectIds,
  canDelete,
  statusLabel,
  emptyLabel,
}: {
  status: ProjectStatus;
  projectIds: string[];
  projectMap: Map<string, Project>;
  pinnedProjectIds: Set<string>;
  canDelete: boolean;
  statusLabel: string;
  emptyLabel: string;
}) {
  const statusCfg = PROJECT_STATUS_CONFIG[status];
  const { setNodeRef, isOver } = useDroppable({
    id: projectStatusColumnId(status),
    data: { status },
  });
  const projects = useMemo(
    () =>
      projectIds.flatMap((id) => {
        const project = projectMap.get(id);
        return project ? [project] : [];
      }),
    [projectIds, projectMap],
  );

  return (
    <section
      className={cn(
        "flex h-full min-h-[320px] w-[280px] shrink-0 flex-col rounded-xl p-2 transition-colors",
        PROJECT_STATUS_COLUMN_BG[status],
      )}
    >
      <div className="mb-2 flex items-center justify-between px-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <span className={cn("size-2 rounded-full", statusCfg.dotColor)} />
          <span className="truncate text-xs font-semibold">{statusLabel}</span>
          <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
            {projects.length}
          </span>
        </div>
      </div>

      <SortableContext items={projectIds} strategy={verticalListSortingStrategy}>
        <div
          ref={setNodeRef}
          className={cn(
            "min-h-[200px] flex-1 space-y-2 overflow-y-auto rounded-lg p-1 transition-colors",
            isOver && "bg-accent/60",
          )}
        >
          {projects.map((project) => (
            <SortableProjectCard
              key={project.id}
              project={project}
              pinned={pinnedProjectIds.has(project.id)}
              canDelete={canDelete}
            />
          ))}
          {projects.length === 0 && (
            <p className="py-8 text-center text-xs text-muted-foreground">
              {emptyLabel}
            </p>
          )}
        </div>
      </SortableContext>
    </section>
  );
}

function SortableProjectCard({
  project,
  pinned,
  canDelete,
}: {
  project: Project;
  pinned: boolean;
  canDelete: boolean;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: project.id,
    data: { status: project.status },
    animateLayoutChanges,
  });

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      {...attributes}
      {...listeners}
      className={cn("touch-none", isDragging && "pointer-events-none opacity-30")}
    >
      <ProjectCard project={project} pinned={pinned} canDelete={canDelete} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function ProjectsPage() {
  const { t } = useT("projects");
  const wsId = useWorkspaceId();
  const wsPaths = useWorkspacePaths();
  const currentUser = useAuthStore((s) => s.user);
  const { getActorName } = useActorName();
  const initialMilestoneId =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("milestone_id")
      : null;
  const viewMode = useProjectViewStore((s) => s.viewMode);
  const setViewMode = useProjectViewStore((s) => s.setViewMode);
  const sortField = useProjectViewStore((s) => s.sortField);
  const sortDirection = useProjectViewStore((s) => s.sortDirection);
  const hiddenColumns = useProjectViewStore((s) => s.hiddenColumns);
  const filters = useProjectViewStore((s) => s.filters);
  const toggleSort = useProjectViewStore((s) => s.toggleSort);
  const setSortField = useProjectViewStore((s) => s.setSortField);
  const setSortDirection = useProjectViewStore((s) => s.setSortDirection);
  const toggleColumn = useProjectViewStore((s) => s.toggleColumn);
  const toggleFilter = useProjectViewStore((s) => s.toggleFilter);
  const clearFilters = useProjectViewStore((s) => s.clearFilters);
  const isCompact = viewMode === "compact";
  const isColVisible = (key: ProjectColumnKey) => !hiddenColumns.includes(key);

  const { data: projects = [], isLoading } = useQuery(
    projectListOptions(
      wsId,
      initialMilestoneId ? { milestone_id: initialMilestoneId } : undefined,
    ),
  );
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: milestones = [] } = useQuery(milestoneListOptions(wsId));
  const { data: pins = [] } = useQuery({
    ...pinListOptions(wsId, currentUser?.id ?? ""),
    enabled: !!wsId && !!currentUser?.id,
  });
  const openCreateProject = () =>
    useModalStore.getState().open(
      "create-project",
      initialMilestoneId ? { milestone_id: initialMilestoneId } : null,
    );

  const isWorkspaceAdmin = useMemo(() => {
    if (!currentUser) return false;
    const me = members.find((m: MemberWithUser) => m.user_id === currentUser.id);
    return me?.role === "owner" || me?.role === "admin";
  }, [members, currentUser]);

  const pinnedProjectIds = useMemo(() => {
    const s = new Set<string>();
    for (const pin of pins) if (pin.item_type === "project") s.add(pin.item_id);
    return s;
  }, [pins]);

  const milestoneById = useMemo(() => {
    const map = new Map<string, Milestone>();
    for (const milestone of milestones) map.set(milestone.id, milestone);
    return map;
  }, [milestones]);

  const [search, setSearch] = useState("");
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(new Set());
  const [previewProject, setPreviewProject] = useState<Project | null>(null);
  const toggleSelected = (id: string) =>
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const activeFilterCount = countActiveFilters(filters);
  const hasActiveFilters = activeFilterCount > 0;

  // Filter option counts derive from the full set so toggling one dimension
  // doesn't make the others vanish.
  const leadOptions = useMemo(() => {
    const m = new Map<string, { type: string; id: string; count: number }>();
    for (const p of projects) {
      const v = leadFilterValue(p);
      if (!v || !p.lead_type || !p.lead_id) continue;
      const e = m.get(v);
      if (e) e.count += 1;
      else m.set(v, { type: p.lead_type, id: p.lead_id, count: 1 });
    }
    return m;
  }, [projects]);

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    const filtered = projects.filter((p) => {
      if (q && !p.title.toLowerCase().includes(q) && !matchesPinyin(p.title, q)) {
        return false;
      }
      if (filters.statuses.length && !filters.statuses.includes(p.status)) return false;
      if (filters.priorities.length && !filters.priorities.includes(p.priority)) {
        return false;
      }
      if (filters.leads.length) {
        const v = leadFilterValue(p);
        if (!v || !filters.leads.includes(v)) return false;
      }
      return true;
    });
    const dir = sortDirection === "asc" ? 1 : -1;
    const sorted = [...filtered];
    sorted.sort((a, b) => {
      if (sortField === "name") return a.title.localeCompare(b.title) * dir;
      if (sortField === "priority") {
        return (
          (PRIORITY_ORDER[a.priority] - PRIORITY_ORDER[b.priority]) * dir ||
          a.title.localeCompare(b.title)
        );
      }
      if (sortField === "status") {
        return (
          (STATUS_ORDER[a.status] - STATUS_ORDER[b.status]) * dir ||
          a.title.localeCompare(b.title)
        );
      }
      if (sortField === "progress") {
        return (progressOf(a) - progressOf(b)) * dir || a.title.localeCompare(b.title);
      }
      if (sortField === "due_date") {
        if (!a.due_date && !b.due_date) return a.title.localeCompare(b.title);
        if (!a.due_date) return 1;
        if (!b.due_date) return -1;
        return (
          (Date.parse(a.due_date) - Date.parse(b.due_date)) * dir ||
          a.title.localeCompare(b.title)
        );
      }
      if (sortField === "updated") {
        return (
          (Date.parse(a.updated_at) - Date.parse(b.updated_at)) * dir ||
          a.title.localeCompare(b.title)
        );
      }
      return (Date.parse(a.created_at) - Date.parse(b.created_at)) * dir;
    });
    return sorted;
  }, [projects, search, filters, sortField, sortDirection]);

  const selectedProjects = visible.filter((p) => selectedIds.has(p.id));
  const allSelected = visible.length > 0 && selectedProjects.length === visible.length;
  const someSelected = selectedProjects.length > 0 && !allSelected;
  const handleToggleAll = () =>
    setSelectedIds(allSelected ? new Set() : new Set(visible.map((p) => p.id)));

  const sortLabel = (f: ProjectSortField) =>
    f === "name"
      ? t(($) => $.table.name)
      : f === "priority"
        ? t(($) => $.table.priority)
        : f === "status"
          ? t(($) => $.table.status)
          : f === "progress"
            ? t(($) => $.table.progress)
            : f === "due_date"
              ? t(($) => $.table.due_date)
              : f === "updated"
                ? t(($) => $.table.updated)
                : t(($) => $.table.created);
  const columnLabel = (k: ProjectColumnKey) =>
    k === "progress"
      ? t(($) => $.table.progress)
      : k === "lead"
        ? t(($) => $.table.lead)
        : k === "plan"
          ? t(($) => $.table.plan)
          : k === "due_date"
            ? t(($) => $.table.due_date)
            : k === "updated"
              ? t(($) => $.table.updated)
              : k === "issues"
                ? t(($) => $.table.issues)
                : t(($) => $.table.created);

  const showEmpty = !isLoading && projects.length === 0;
  const countBadge = (n: number) => (
    <span className="ml-auto pl-3 text-xs text-muted-foreground">{n}</span>
  );

  return (
    // relative: positioning anchor for the page-centered batch toolbar.
    <div className="relative flex flex-1 min-h-0 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex items-center gap-2">
          <FolderKanban className="h-4 w-4 text-muted-foreground" />
          <h1 className="text-sm font-medium">{t(($) => $.page.title)}</h1>
          {projects.length > 0 && (
            <span className="font-mono text-xs tabular-nums text-muted-foreground/70">
              {projects.length}
            </span>
          )}
        </div>
        <Button
          size="sm"
          variant="outline"
          className="h-8 w-8 gap-1 px-0 md:w-auto md:px-2.5"
          aria-label={t(($) => $.page.new_project)}
          onClick={openCreateProject}
        >
          <Plus className="h-3.5 w-3.5" />
          <span className="hidden md:inline">{t(($) => $.page.new_project)}</span>
        </Button>
      </PageHeader>

      {showEmpty ? (
        <div className="flex flex-1 flex-col items-center justify-center py-24 text-muted-foreground">
          <FolderKanban className="mb-3 h-10 w-10 opacity-30" />
          <p className="text-sm">{t(($) => $.page.empty)}</p>
          <Button size="sm" variant="outline" className="mt-3" onClick={openCreateProject}>
            {t(($) => $.page.create_first)}
          </Button>
        </div>
      ) : (
        <>
          <ProjectQuickViewDialog
            project={previewProject}
            milestone={
              previewProject?.milestone_id
                ? (milestoneById.get(previewProject.milestone_id) ?? null)
                : null
            }
            open={previewProject != null}
            onOpenChange={(open) => {
              if (!open) setPreviewProject(null);
            }}
            getActorName={getActorName}
          />

          {/* Toolbar */}
          <div className="flex h-12 shrink-0 items-center justify-between gap-2 px-5">
            <div className="flex min-w-0 items-center gap-2">
              <div className="relative hidden md:block">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={t(($) => $.page.search_placeholder)}
                  className="h-8 w-56 pl-8 text-sm"
                />
              </div>
              {(hasActiveFilters || search.trim().length > 0) && (
                <span
                  title={t(($) => $.toolbar.result_count_title)}
                  className="hidden shrink-0 text-xs tabular-nums text-muted-foreground md:inline"
                >
                  {visible.length} / {projects.length}
                </span>
              )}
            </div>

            <div className="flex shrink-0 items-center gap-1">
              {/* Filter */}
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant={hasActiveFilters ? "default" : "outline"}
                      size="sm"
                      className={
                        hasActiveFilters
                          ? "h-8 w-8 gap-1 bg-brand px-0 text-white hover:bg-brand/90 md:w-auto md:px-2.5"
                          : "h-8 w-8 gap-1 px-0 text-muted-foreground md:w-auto md:px-2.5"
                      }
                    >
                      <Filter className="size-3.5" />
                      {hasActiveFilters ? (
                        <>
                          <span className="hidden md:inline">
                            {t(($) => $.toolbar.filter_active_count, { count: activeFilterCount })}
                          </span>
                          <span className="tabular-nums md:hidden">{activeFilterCount}</span>
                        </>
                      ) : (
                        <span className="hidden md:inline">{t(($) => $.toolbar.filter_label)}</span>
                      )}
                      {hasActiveFilters && (
                        <span
                          role="button"
                          tabIndex={-1}
                          aria-label={t(($) => $.toolbar.clear_filters)}
                          className="-mr-1 ml-0.5 hidden rounded-sm p-0.5 hover:bg-white/20 md:inline-flex"
                          onClick={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            clearFilters();
                          }}
                          onPointerDown={(e) => e.stopPropagation()}
                        >
                          <X className="size-3" />
                        </span>
                      )}
                    </Button>
                  }
                />
                <DropdownMenuContent align="end" className="w-auto">
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <span className="flex-1">{t(($) => $.toolbar.section_status)}</span>
                      {filters.statuses.length > 0 && (
                        <span className="text-xs font-medium text-primary">{filters.statuses.length}</span>
                      )}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="w-auto min-w-44">
                      {STATUS_VALUES.map((s) => (
                        <DropdownMenuCheckboxItem
                          key={s}
                          checked={filters.statuses.includes(s)}
                          onCheckedChange={() => toggleFilter("statuses", s)}
                          className={FILTER_ITEM_CLASS}
                        >
                          <HoverCheck checked={filters.statuses.includes(s)} />
                          {t(($) => $.status[s])}
                        </DropdownMenuCheckboxItem>
                      ))}
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <span className="flex-1">{t(($) => $.toolbar.section_priority)}</span>
                      {filters.priorities.length > 0 && (
                        <span className="text-xs font-medium text-primary">{filters.priorities.length}</span>
                      )}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="w-auto min-w-44">
                      {PRIORITY_VALUES.map((pr) => (
                        <DropdownMenuCheckboxItem
                          key={pr}
                          checked={filters.priorities.includes(pr)}
                          onCheckedChange={() => toggleFilter("priorities", pr)}
                          className={FILTER_ITEM_CLASS}
                        >
                          <HoverCheck checked={filters.priorities.includes(pr)} />
                          {t(($) => $.priority[pr])}
                        </DropdownMenuCheckboxItem>
                      ))}
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>
                      <span className="flex-1">{t(($) => $.toolbar.section_lead)}</span>
                      {filters.leads.length > 0 && (
                        <span className="text-xs font-medium text-primary">{filters.leads.length}</span>
                      )}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="max-h-72 w-auto min-w-48 overflow-y-auto">
                      {[...leadOptions.entries()].map(([value, { type, id, count }]) => (
                        <DropdownMenuCheckboxItem
                          key={value}
                          checked={filters.leads.includes(value)}
                          onCheckedChange={() => toggleFilter("leads", value)}
                          className={FILTER_ITEM_CLASS}
                        >
                          <HoverCheck checked={filters.leads.includes(value)} />
                          <ActorAvatar actorType={type} actorId={id} size={16} />
                          <span className="min-w-0 truncate">{getActorName(type, id)}</span>
                          {countBadge(count)}
                        </DropdownMenuCheckboxItem>
                      ))}
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                </DropdownMenuContent>
              </DropdownMenu>

              {/* Display (sort + columns). Always present — view mode is a
                  pure presentation choice and must not reshape the toolbar.
                  Sort applies to both views; the columns section is shown
                  only in the table view (cards have no columns). */}
              <Popover>
                  <Tooltip>
                    <PopoverTrigger
                      render={
                        <TooltipTrigger
                          render={
                            <Button variant="outline" size="sm" className="h-8 w-8 gap-1 px-0 text-muted-foreground md:w-auto md:px-2.5">
                              {sortDirection === "asc" ? <ArrowUp className="size-3.5" /> : <ArrowDown className="size-3.5" />}
                              <span className="hidden md:inline">{sortLabel(sortField)}</span>
                            </Button>
                          }
                        />
                      }
                    />
                    <TooltipContent side="bottom">{t(($) => $.toolbar.display)}</TooltipContent>
                  </Tooltip>
                  <PopoverContent align="end" className="w-64 p-0">
                    <div className="border-b px-3 py-2.5">
                      <span className="text-xs font-medium text-muted-foreground">{t(($) => $.toolbar.sort_by)}</span>
                      <div className="mt-2 flex items-center gap-1.5">
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button variant="outline" size="sm" className="flex-1 justify-between text-xs">
                                {sortLabel(sortField)}
                                <ChevronDown className="size-3 text-muted-foreground" />
                              </Button>
                            }
                          />
                          <DropdownMenuContent align="start" className="w-auto">
                            <DropdownMenuRadioGroup
                              value={sortField}
                              onValueChange={(v) => setSortField(v as ProjectSortField)}
                            >
                              {SORT_FIELDS.map((f) => (
                                <DropdownMenuRadioItem key={f} value={f}>
                                  {sortLabel(f)}
                                </DropdownMenuRadioItem>
                              ))}
                            </DropdownMenuRadioGroup>
                          </DropdownMenuContent>
                        </DropdownMenu>
                        <Button
                          variant="outline"
                          size="icon-sm"
                          onClick={() => setSortDirection(sortDirection === "asc" ? "desc" : "asc")}
                          title={sortDirection === "asc" ? t(($) => $.toolbar.direction_asc) : t(($) => $.toolbar.direction_desc)}
                        >
                          {sortDirection === "asc" ? <ArrowUp className="size-3.5" /> : <ArrowDown className="size-3.5" />}
                        </Button>
                      </div>
                    </div>
                    {isCompact && (
                      <div className="px-3 py-2.5">
                        <span className="text-xs font-medium text-muted-foreground">{t(($) => $.toolbar.section_columns)}</span>
                        <div className="mt-2 space-y-2">
                          {COLUMN_KEYS.map((key) => (
                            <label key={key} className="flex cursor-pointer items-center justify-between">
                              <span className="text-sm">{columnLabel(key)}</span>
                              <Switch size="sm" checked={!hiddenColumns.includes(key)} onCheckedChange={() => toggleColumn(key)} />
                            </label>
                          ))}
                        </div>
                      </div>
                    )}
                  </PopoverContent>
                </Popover>

              {/* View toggle — a single button that flips table ⇄ cards.
                  Pure presentation; coupled to nothing else. */}
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 w-8 gap-1 px-0 text-muted-foreground md:w-auto md:px-2.5"
                      onClick={() => setViewMode(isCompact ? "comfortable" : "compact")}
                    >
                      {isCompact ? (
                        <Rows3 className="size-3.5" />
                      ) : (
                        <LayoutGrid className="size-3.5" />
                      )}
                      <span className="hidden md:inline">
                        {isCompact ? t(($) => $.page.view_table) : t(($) => $.page.view_cards)}
                      </span>
                    </Button>
                  }
                />
                <TooltipContent side="bottom">
                  {isCompact ? t(($) => $.page.view_cards) : t(($) => $.page.view_table)}
                </TooltipContent>
              </Tooltip>
            </div>
          </div>

          {/* Body */}
          {isLoading ? (
            <LoadingState isCompact={isCompact} />
          ) : visible.length === 0 ? (
            <div className="flex flex-1 flex-col items-center justify-center py-24 text-muted-foreground">
              <Search className="mb-3 h-10 w-10 opacity-30" />
              <p className="text-sm">{t(($) => $.page.no_matches)}</p>
            </div>
          ) : isCompact ? (
            <div className="min-h-0 flex-1 overflow-auto @container">
              <ListGrid
                className={`${GRID_COLS} @2xl:min-w-[var(--pjc-minw)]`}
                style={{
                  ...columnTrackVars(isColVisible),
                  paddingBottom: LIST_GRID_BOTTOM_CLEARANCE,
                }}
              >
                <ProjectTableHeader
                  sortField={sortField}
                  sortDirection={sortDirection}
                  onSort={toggleSort}
                  isColVisible={isColVisible}
                  allSelected={allSelected}
                  someSelected={someSelected}
                  onToggleAll={handleToggleAll}
                />
                {visible.map((project) => (
                  <ProjectTableRow
                    key={project.id}
                    project={project}
                    milestone={
                      project.milestone_id
                        ? (milestoneById.get(project.milestone_id) ?? null)
                        : null
                    }
                    pinned={pinnedProjectIds.has(project.id)}
                    canDelete={isWorkspaceAdmin}
                    isColVisible={isColVisible}
                    selected={selectedIds.has(project.id)}
                    onToggleSelect={() => toggleSelected(project.id)}
                    rowHref={wsPaths.projectDetail(project.id)}
                    onOpenPreview={setPreviewProject}
                  />
                ))}
              </ListGrid>
            </div>
          ) : (
            <ProjectStatusBuckets
              projects={visible}
              pinnedProjectIds={pinnedProjectIds}
              canDelete={isWorkspaceAdmin}
            />
          )}

          <ProjectBatchToolbar
            rows={selectedProjects}
            pinnedIds={pinnedProjectIds}
            canDelete={isWorkspaceAdmin}
            onClear={() => setSelectedIds(new Set())}
          />
        </>
      )}
    </div>
  );
}

function LoadingState({ isCompact }: { isCompact: boolean }) {
  if (isCompact) {
    return (
      <div className="min-h-0 flex-1 overflow-auto px-5 pt-4">
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full rounded-md" />
          ))}
        </div>
      </div>
    );
  }
  return (
    <div className="grid grid-cols-1 gap-3 px-5 pt-4 sm:grid-cols-2 lg:grid-cols-4">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="flex flex-col gap-2 rounded-md border p-3">
          <div className="flex items-center gap-2">
            <Skeleton className="h-8 w-8 rounded" />
            <Skeleton className="h-4 w-3/4" />
          </div>
          <div className="flex gap-1.5">
            <Skeleton className="h-5 w-16 rounded" />
            <Skeleton className="h-5 w-20 rounded" />
          </div>
        </div>
      ))}
    </div>
  );
}
