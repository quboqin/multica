"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Plus, FolderKanban, Rows3, LayoutGrid, Search } from "lucide-react";
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
import { projectListOptions } from "@multica/core/projects/queries";
import { useUpdateProject } from "@multica/core/projects/mutations";
import { PROJECT_STATUS_CONFIG, PROJECT_STATUS_ORDER } from "@multica/core/projects/config";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { useModalStore } from "@multica/core/modals";
import { AppLink } from "../../navigation";
import { ActorAvatar } from "../../common/actor-avatar";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { cn } from "@multica/ui/lib/utils";
import type { Project, ProjectStatus, UpdateProjectRequest } from "@multica/core/types";
import { PageHeader } from "../../layout/page-header";
import { ProjectIcon } from "./project-icon";
import { useT } from "../../i18n";
import { matchesPinyin } from "../../editor/extensions/pinyin-match";
import { useFormatRelativeDate, useProjectStatusLabels } from "./labels";
import { useProjectViewStore } from "@multica/core/projects";
import { ProjectStatusBadge, ProjectPriorityBadge } from "./project-badge";
import { ProjectLeadPicker } from "./project-lead-picker";

const COMPACT_GRID = "grid w-full min-w-[740px] grid-cols-[24px_minmax(200px,1fr)_96px_96px_80px_80px_80px]";
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
      const items = pointer.filter((collision) => !columnIds.has(collision.id as string));
      if (items.length > 0) return items;
      return pointer;
    }

    return closestCenter(args);
  };
}

function ProjectCard({ project }: { project: Project }) {
  const { t } = useT("projects");
  const wsPaths = useWorkspacePaths();
  const formatRelativeDate = useFormatRelativeDate();
  const updateProject = useUpdateProject();

  const handleUpdate = useCallback(
    (data: UpdateProjectRequest) => {
      updateProject.mutate({ id: project.id, ...data });
    },
    [project.id, updateProject],
  );

  const progressPercent = project.issue_count > 0 ? Math.round((project.done_count / project.issue_count) * 100) : 0;

  return (
    <div className="group/card flex flex-col rounded-md border bg-card hover:border-primary/50 transition-colors">
      <div className="p-3 pb-2">
        <div className="flex items-center gap-2">
          <AppLink
            href={wsPaths.projectDetail(project.id)}
            className="flex items-center gap-2 min-w-0 flex-1"
          >
            <ProjectIcon project={project} size="sm" />
            <h3 className="font-medium text-sm truncate">{project.title}</h3>
          </AppLink>
          <ProjectStatusBadge project={project} handleUpdate={handleUpdate} triggerClassName="shrink-0" />
        </div>

        {project.issue_count > 0 ? (
          <div className="flex justify-end items-center gap-1.5 pt-2">
            <div className="relative h-4 w-4">
              <svg className="h-4 w-4 -rotate-90" viewBox="0 0 16 16">
                <circle
                  className="text-muted"
                  strokeWidth="2"
                  stroke="currentColor"
                  fill="none"
                  r="6"
                  cx="8"
                  cy="8"
                />
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
            <span className="text-[10px] text-muted-foreground tabular-nums">
              {project.done_count}/{project.issue_count}
            </span>
          </div>
        ) : (
          <span className="text-[10px] text-muted-foreground pt-2 flex justify-end">{t(($) => $.detail.no_issues_yet)}</span>
        )}
      </div>

      <div className="flex items-center justify-between px-3 pb-3 border-t mt-0 pt-2">
        <ProjectLeadPicker
          project={project}
          handleUpdate={handleUpdate}
          renderTrigger={(leadName) => (
            <button type="button" className="flex items-center gap-1.5 rounded px-1.5 py-0.5 -mx-1.5 hover:bg-accent/60 transition-colors cursor-pointer">
              {project.lead_type && project.lead_id ? (
                <ActorAvatar actorType={project.lead_type} actorId={project.lead_id} size={20} enableHoverCard />
              ) : (
                <span className="inline-flex h-5 w-5 rounded-full border border-dashed border-muted-foreground/30" />
              )}
              <span className="text-[10px] text-muted-foreground truncate max-w-[60px]">
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

function ProjectCardCompact({ project }: { project: Project }) {
  const wsPaths = useWorkspacePaths();
  const formatRelativeDate = useFormatRelativeDate();
  const updateProject = useUpdateProject();

  const handleUpdate = useCallback(
    (data: UpdateProjectRequest) => {
      updateProject.mutate({ id: project.id, ...data });
    },
    [project.id, updateProject],
  );

  return (
    <div className={cn(COMPACT_GRID, "h-10 items-center gap-2 px-4 text-sm transition-colors hover:bg-accent/40 border-b")}>
      <ProjectIcon project={project} size="sm" />
      <AppLink
        href={wsPaths.projectDetail(project.id)}
        className="flex items-center justify-start gap-2 min-w-0 overflow-hidden"
      >
        <span className="font-medium truncate text-left">{project.title}</span>
      </AppLink>

      <div className="flex items-center justify-start">
        <ProjectPriorityBadge project={project} handleUpdate={handleUpdate} align="start" />
      </div>

      <div className="flex items-center justify-start">
        <ProjectStatusBadge project={project} handleUpdate={handleUpdate} align="start" />
      </div>

      <span className="flex items-center justify-start gap-1.5 text-xs text-muted-foreground tabular-nums">
        {project.issue_count > 0 ? `${project.done_count}/${project.issue_count}` : "--"}
      </span>

      <ProjectLeadPicker
        project={project}
        handleUpdate={handleUpdate}
        align="start"
        renderTrigger={(leadName) => (
          <button type="button" className="flex items-center justify-start gap-1.5 rounded px-1 py-0.5 hover:bg-accent/60 transition-colors cursor-pointer">
            <span className="shrink-0">
              {project.lead_type && project.lead_id ? (
                <ActorAvatar actorType={project.lead_type} actorId={project.lead_id} size={20} enableHoverCard />
              ) : (
                <span className="inline-flex h-5 w-5 rounded-full border border-dashed border-muted-foreground/30" />
              )}
            </span>
            <span className="text-xs text-muted-foreground truncate max-w-[50px]">
              {leadName ?? "--"}
            </span>
          </button>
        )}
      />

      <span className="text-left text-xs text-muted-foreground tabular-nums">
        {formatRelativeDate(project.created_at)}
      </span>
    </div>
  );
}

function ProjectStatusBuckets({ projects }: { projects: Project[] }) {
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

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      isDraggingRef.current = true;
      setActiveProject(projectMapRef.current.get(String(event.active.id)) ?? null);
    },
    [],
  );
  const handleDragOver = useCallback(
    (event: DragOverEvent) => {
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
    },
    [],
  );
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
            statusLabel={statusLabels[status]}
            emptyLabel={t(($) => $.page.empty)}
          />
        ))}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeProject && (
          <div className="w-[280px]">
            <ProjectCard project={activeProject} />
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
  statusLabel,
  emptyLabel,
}: {
  status: ProjectStatus;
  projectIds: string[];
  projectMap: Map<string, Project>;
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
          <span className="truncate text-xs font-semibold">
            {statusLabel}
          </span>
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {projects.length}
          </span>
        </div>
      </div>

      <SortableContext
        items={projectIds}
        strategy={verticalListSortingStrategy}
      >
        <div
          ref={setNodeRef}
          className={cn(
            "min-h-[200px] flex-1 space-y-2 overflow-y-auto rounded-lg p-1 transition-colors",
            isOver && "bg-accent/60",
          )}
        >
          {projects.map((project) => (
            <SortableProjectCard key={project.id} project={project} />
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

function SortableProjectCard({ project }: { project: Project }) {
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
      <ProjectCard project={project} />
    </div>
  );
}

export function ProjectsPage() {
  const { t } = useT("projects");
  const wsId = useWorkspaceId();
  const initialMilestoneId =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("milestone_id")
      : null;
  const viewMode = useProjectViewStore((s) => s.viewMode);
  const setViewMode = useProjectViewStore((s) => s.setViewMode);
  const isCompact = viewMode === "compact";
  const { data: projects = [], isLoading } = useQuery(projectListOptions(wsId, initialMilestoneId ? { milestone_id: initialMilestoneId } : undefined));
  const openCreateProject = () =>
    useModalStore.getState().open(
      "create-project",
      initialMilestoneId ? { milestone_id: initialMilestoneId } : null,
    );

  const [search, setSearch] = useState("");
  const filteredProjects = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return projects;
    return projects.filter((p) =>
      p.title.toLowerCase().includes(q) || matchesPinyin(p.title, q)
    );
  }, [projects, search]);

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex items-center gap-2">
          <FolderKanban className="h-4 w-4 text-muted-foreground" />
          <h1 className="text-sm font-medium">{t(($) => $.page.title)}</h1>
          {!isLoading && projects.length > 0 && (
            <span className="text-xs text-muted-foreground tabular-nums">{projects.length}</span>
          )}
        </div>
        <Button size="sm" variant="outline" onClick={openCreateProject}>
          <Plus className="h-3.5 w-3.5 mr-1" />
          {t(($) => $.page.new_project)}
        </Button>
      </PageHeader>

      <div className="flex flex-1 min-h-0 flex-col overflow-hidden">
        {(projects.length > 0 || isLoading) && (
          <div className="flex h-12 shrink-0 items-center justify-between border-b px-4 gap-2 sm:gap-3">
            <div className="relative flex-1 sm:flex-none">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t(($) => $.page.search_placeholder)}
                className="h-8 w-full sm:w-64 pl-8 text-sm"
              />
            </div>

            <div className="flex items-center gap-2 sm:gap-4 shrink-0">
              <span className="hidden sm:inline-block font-mono text-xs tabular-nums text-muted-foreground/70">
                {filteredProjects.length} / {projects.length}
              </span>
              <div className="flex items-center gap-0.5 rounded-md bg-muted p-0.5">
                <button
                  type="button"
                  onClick={() => setViewMode("compact")}
                  className={cn(
                    "inline-flex items-center gap-1.5 rounded p-1 sm:px-2.5 sm:py-1 text-xs font-medium transition-colors",
                    isCompact ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <Rows3 className="size-3.5" />
                  <span className="hidden sm:inline-block">{t(($) => $.page.view_compact)}</span>
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode("comfortable")}
                  className={cn(
                    "inline-flex items-center gap-1.5 rounded p-1 sm:px-2.5 sm:py-1 text-xs font-medium transition-colors",
                    !isCompact ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <LayoutGrid className="size-3.5" />
                  <span className="hidden sm:inline-block">{t(($) => $.page.view_comfortable)}</span>
                </button>
              </div>
            </div>
          </div>
        )}

        <div key={viewMode} className={cn("flex-1 min-h-0", isCompact ? "overflow-hidden flex flex-col" : "overflow-hidden")}>
          {isLoading ? (
            isCompact ? (
              <div className="pt-4 mx-5 overflow-x-auto rounded-md border pb-4 mb-5">
                <div className="min-w-[740px]">
                  <div className={cn(COMPACT_GRID, "h-10 items-center gap-2 px-4 border-b")}>
                    <Skeleton className="h-6 w-6 rounded" />
                    <Skeleton className="h-4 w-48" />
                  </div>
                  {Array.from({ length: 6 }).map((_, i) => (
                    <div key={i} className={cn(COMPACT_GRID, "h-10 items-center gap-2 px-4 border-b")}>
                      <Skeleton className="h-6 w-6 rounded" />
                      <Skeleton className="h-4 w-48" />
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div className="pt-4 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 px-5">
                {Array.from({ length: 8 }).map((_, i) => (
                  <div key={i} className="flex flex-col rounded-md border p-3 gap-2">
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-8 w-8 rounded" />
                      <Skeleton className="h-4 w-3/4" />
                    </div>
                    <div className="flex gap-1.5">
                      <Skeleton className="h-5 w-16 rounded" />
                      <Skeleton className="h-5 w-20 rounded" />
                    </div>
                    <div className="flex items-center justify-between">
                      <Skeleton className="h-5 w-5 rounded-full" />
                      <Skeleton className="h-3 w-12" />
                    </div>
                  </div>
                ))}
              </div>
            )
          ) : projects.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-24 text-muted-foreground">
              <FolderKanban className="h-10 w-10 mb-3 opacity-30" />
              <p className="text-sm">{t(($) => $.page.empty)}</p>
              <Button size="sm" variant="outline" className="mt-3" onClick={openCreateProject}>
                {t(($) => $.page.create_first)}
              </Button>
            </div>
          ) : filteredProjects.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-24 text-muted-foreground">
              <Search className="h-10 w-10 mb-3 opacity-30" />
              <p className="text-sm">{t(($) => $.page.no_search_results)}</p>
            </div>
          ) : isCompact ? (
            <div className="mt-4 mx-5 rounded-md border mb-5 overflow-auto flex-1">
              <div className="min-w-[740px]">
                <div className={cn(COMPACT_GRID, "h-8 shrink-0 items-center gap-2 px-4 text-xs font-medium text-muted-foreground border-b bg-muted/30 sticky top-0 z-10")}>
                  <span />
                  <span className="text-left">{t(($) => $.table.name)}</span>
                  <span className="text-left">{t(($) => $.table.priority)}</span>
                  <span className="text-left">{t(($) => $.table.status)}</span>
                  <span className="text-left">{t(($) => $.table.progress)}</span>
                  <span className="text-left">{t(($) => $.table.lead)}</span>
                  <span className="text-left">{t(($) => $.table.created)}</span>
                </div>
                <div className="pb-4">
                  {filteredProjects.map((project) => (
                    <ProjectCardCompact key={project.id} project={project} />
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <ProjectStatusBuckets projects={filteredProjects} />
          )}
        </div>
      </div>
    </div>
  );
}
