"use client";

import { useEffect, useMemo, useState } from "react";
import { BarChart3, CalendarRange, Flag, FolderKanban, ListTodo, Pencil, Plus } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { milestoneListOptions } from "@multica/core/milestones";
import { useCreateMilestone, useUpdateMilestone } from "@multica/core/milestones/mutations";
import { projectListOptions } from "@multica/core/projects/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import { useModalStore } from "@multica/core/modals";
import { cn } from "@multica/ui/lib/utils";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect } from "@multica/ui/components/ui/native-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { PageHeader } from "../../layout/page-header";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Milestone, MilestoneStatus, Project } from "@multica/core/types";

const planStatuses: MilestoneStatus[] = ["planned", "in_progress", "paused", "completed", "cancelled"];
const ALL_PLANS = "__all__";
const ACTIVE_PLANS = "__active__";
type PlanStatusFilter = typeof ACTIVE_PLANS | "all" | MilestoneStatus;

function StatCard({
  icon: Icon,
  label,
  value,
  detail,
}: {
  icon: typeof FolderKanban;
  label: string;
  value: string;
  detail: string;
}) {
  return (
    <div className="rounded-md border bg-card p-4">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        <Icon className="size-4 text-muted-foreground" />
      </div>
      <div className="mt-3 text-2xl font-semibold tabular-nums">{value}</div>
      <div className="mt-1 text-xs text-muted-foreground">{detail}</div>
    </div>
  );
}

function PlanDialog({
  open,
  onOpenChange,
  milestone,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  milestone: Milestone | null;
  onSaved: (milestoneId: string) => void;
}) {
  const { t } = useT("plans");
  const createMilestone = useCreateMilestone();
  const updateMilestone = useUpdateMilestone();
  const isEditing = Boolean(milestone);
  const isPending = createMilestone.isPending || updateMilestone.isPending;
  const [title, setTitle] = useState(milestone?.title ?? "");
  const [description, setDescription] = useState(milestone?.description ?? "");
  const [startDate, setStartDate] = useState(milestone?.start_date ?? "");
  const [endDate, setEndDate] = useState(milestone?.end_date ?? "");
  const [status, setStatus] = useState<MilestoneStatus>(milestone?.status ?? "planned");

  useEffect(() => {
    if (!open) return;
    setTitle(milestone?.title ?? "");
    setDescription(milestone?.description ?? "");
    setStartDate(milestone?.start_date ?? "");
    setEndDate(milestone?.end_date ?? "");
    setStatus(milestone?.status ?? "planned");
  }, [milestone, open]);

  const reset = () => {
    setTitle(milestone?.title ?? "");
    setDescription(milestone?.description ?? "");
    setStartDate(milestone?.start_date ?? "");
    setEndDate(milestone?.end_date ?? "");
    setStatus(milestone?.status ?? "planned");
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !isPending) reset();
    onOpenChange(nextOpen);
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedTitle = title.trim();
    if (!trimmedTitle || isPending) return;

    try {
      const payload = {
        title: trimmedTitle,
        description: description.trim() || undefined,
        start_date: startDate || null,
        end_date: endDate || null,
        status,
      };
      const savedMilestone = milestone
        ? await updateMilestone.mutateAsync({ id: milestone.id, ...payload })
        : await createMilestone.mutateAsync(payload);
      toast.success(t(($) => (isEditing ? $.dialog.toast_updated : $.dialog.toast_created)));
      reset();
      onSaved(savedMilestone.id);
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error && err.message ? err.message : t(($) => (isEditing ? $.dialog.toast_update_failed : $.dialog.toast_failed)));
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={handleSubmit} className="contents">
          <DialogHeader>
            <DialogTitle>{t(($) => (isEditing ? $.dialog.edit_title : $.dialog.title))}</DialogTitle>
            <DialogDescription>
              {t(($) => $.dialog.description)}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t(($) => $.dialog.fields.title)}</span>
              <Input
                autoFocus
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder={t(($) => $.dialog.placeholders.title)}
              />
            </label>

            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t(($) => $.dialog.fields.description)}</span>
              <Textarea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder={t(($) => $.dialog.placeholders.description)}
                className="min-h-24 resize-none"
              />
            </label>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t(($) => $.dialog.fields.start_date)}</span>
                <Input type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} />
              </label>
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t(($) => $.dialog.fields.end_date)}</span>
                <Input type="date" value={endDate} onChange={(event) => setEndDate(event.target.value)} />
              </label>
            </div>

            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t(($) => $.dialog.fields.status)}</span>
              <NativeSelect
                className="w-full"
                value={status}
                onChange={(event) => setStatus(event.target.value as MilestoneStatus)}
              >
                {planStatuses.map((item) => (
                  <option key={item} value={item}>
                    {t(($) => $.status[item])}
                  </option>
                ))}
              </NativeSelect>
            </label>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={isPending}
              onClick={() => handleOpenChange(false)}
            >
              {t(($) => $.dialog.cancel)}
            </Button>
            <Button type="submit" disabled={!title.trim() || isPending}>
              {isPending ? t(($) => (isEditing ? $.dialog.saving : $.dialog.creating)) : t(($) => (isEditing ? $.dialog.save : $.dialog.submit))}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function PlanRow({
  milestone,
  projects,
  onEdit,
}: {
  milestone: Milestone;
  projects: Project[];
  onEdit: (milestone: Milestone) => void;
}) {
  const { t } = useT("plans");
  const wsPaths = useWorkspacePaths();
  const total = projects.length;
  const done = projects.filter((p) => p.status === "completed" || p.status === "cancelled").length;
  const blocked = projects.filter((p) => p.status === "paused").length;
  const progress = total > 0 ? Math.round((done / total) * 100) : 0;

  return (
    <div className="grid grid-cols-[minmax(220px,1fr)_120px_120px_120px] items-center gap-4 border-b px-4 py-3 last:border-b-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <CalendarRange className="size-4 text-muted-foreground" />
          <div className="truncate text-sm font-medium">{milestone.title}</div>
          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{t(($) => $.status[milestone.status as MilestoneStatus])}</span>
        </div>
        <div className="mt-1 truncate text-xs text-muted-foreground">
          {milestone.start_date ?? t(($) => $.row.no_start)} - {milestone.end_date ?? t(($) => $.row.no_end)}
        </div>
      </div>
      <div>
        <div className="h-2 overflow-hidden rounded-full bg-muted">
          <div className="h-full rounded-full bg-primary" style={{ width: `${progress}%` }} />
        </div>
        <div className="mt-1 text-xs text-muted-foreground tabular-nums">{t(($) => $.row.complete, { progress })}</div>
      </div>
      <div className="text-xs text-muted-foreground tabular-nums">
        {t(($) => $.row.projects_count, { done, total })}
      </div>
      <div className="flex items-center justify-between gap-2">
        <span className={cn("text-xs tabular-nums", blocked > 0 ? "text-warning" : "text-muted-foreground")}>
          {t(($) => $.row.paused_count, { count: blocked })}
        </span>
        <Button size="icon-sm" variant="ghost" aria-label={t(($) => $.row.edit)} onClick={() => onEdit(milestone)}>
          <Pencil className="size-4" />
        </Button>
        <Button
          size="sm"
          variant="ghost"
          nativeButton={false}
          render={<AppLink href={`${wsPaths.projects()}?milestone_id=${encodeURIComponent(milestone.id)}`} />}
        >
          {t(($) => $.row.open)}
        </Button>
      </div>
    </div>
  );
}

export function PlansPage() {
  const { t } = useT("plans");
  const wsId = useWorkspaceId();
  const wsPaths = useWorkspacePaths();
  const [selectedMilestoneId, setSelectedMilestoneId] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<PlanStatusFilter>(ACTIVE_PLANS);
  const [createOpen, setCreateOpen] = useState(false);
  const [editingMilestone, setEditingMilestone] = useState<Milestone | null>(null);
  const { data: milestones = [], isLoading: milestonesLoading } = useQuery(milestoneListOptions(wsId));
  const { data: projects = [], isLoading: projectsLoading } = useQuery(projectListOptions(wsId));
  const filteredMilestones = useMemo(
    () =>
      milestones.filter((milestone) => {
        if (statusFilter === "all") return true;
        if (statusFilter === ACTIVE_PLANS) return milestone.status !== "completed";
        return milestone.status === statusFilter;
      }),
    [milestones, statusFilter],
  );

  const projectsByMilestone = useMemo(() => {
    const map = new Map<string, Project[]>();
    for (const milestone of milestones) map.set(milestone.id, []);
    for (const project of projects) {
      if (!project.milestone_id) continue;
      map.set(project.milestone_id, [...(map.get(project.milestone_id) ?? []), project]);
    }
    return map;
  }, [milestones, projects]);

  const filteredMilestoneIds = useMemo(() => new Set(filteredMilestones.map((milestone) => milestone.id)), [filteredMilestones]);
  const selectedMilestoneVisible = selectedMilestoneId ? filteredMilestoneIds.has(selectedMilestoneId) : false;
  const visibleSelectedMilestoneId = selectedMilestoneVisible ? selectedMilestoneId : null;
  const scopedProjects = selectedMilestoneVisible
    ? projectsByMilestone.get(visibleSelectedMilestoneId!) ?? []
    : projects.filter((p) => p.milestone_id && filteredMilestoneIds.has(p.milestone_id));
  const selectedMilestone = selectedMilestoneId
    ? filteredMilestones.find((milestone) => milestone.id === selectedMilestoneId)
    : null;
  const selectedPlanLabel = selectedMilestone?.title ?? t(($) => $.page.all_plans);
  const statusFilterLabel =
    statusFilter === "all"
      ? t(($) => $.page.all_statuses)
      : statusFilter === ACTIVE_PLANS
        ? t(($) => $.page.active_plans)
        : t(($) => $.status[statusFilter]);
  const completedProjects = scopedProjects.filter((p) => p.status === "completed" || p.status === "cancelled").length;
  const totalIssues = scopedProjects.reduce((sum, p) => sum + p.issue_count, 0);
  const doneIssues = scopedProjects.reduce((sum, p) => sum + p.done_count, 0);
  const issueProgress = totalIssues > 0 ? Math.round((doneIssues / totalIssues) * 100) : 0;
  const loading = milestonesLoading || projectsLoading;
  const openCreateProject = () =>
    useModalStore.getState().open(
      "create-project",
      selectedMilestoneId ? { milestone_id: selectedMilestoneId } : null,
    );

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex items-center gap-2">
          <Flag className="size-4 text-muted-foreground" />
          <h1 className="text-sm font-medium">{t(($) => $.page.title)}</h1>
          {!loading && filteredMilestones.length > 0 && (
            <span className="text-xs tabular-nums text-muted-foreground">{filteredMilestones.length}</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" nativeButton={false} render={<AppLink href={wsPaths.plansCharts()} />}>
            <BarChart3 className="size-4" />
            {t(($) => $.page.open_charts)}
          </Button>
          <Button size="sm" variant="outline" onClick={openCreateProject}>
            <Plus className="size-4" />
            {t(($) => $.page.create_project)}
          </Button>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            {t(($) => $.page.create_plan)}
          </Button>
        </div>
      </PageHeader>

      <div className="flex-1 overflow-auto p-5">
        {loading ? (
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
              {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-28 rounded-md" />)}
            </div>
            <Skeleton className="h-72 rounded-md" />
          </div>
        ) : milestones.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-md border py-24 text-muted-foreground">
            <CalendarRange className="mb-3 size-10 opacity-30" />
            <p className="text-sm">{t(($) => $.empty.title)}</p>
            <p className="mt-1 text-xs">{t(($) => $.empty.description)}</p>
            <Button size="sm" className="mt-4" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              {t(($) => $.page.create_plan)}
            </Button>
          </div>
        ) : (
          <div className="space-y-5">
            <div className="flex flex-wrap items-center gap-2">
              <Select
                value={statusFilter}
                onValueChange={(value) => {
                  setStatusFilter(value as PlanStatusFilter);
                  setSelectedMilestoneId(null);
                }}
              >
                <SelectTrigger size="sm" className="min-w-[180px] max-w-full">
                  <SelectValue>{() => <span className="truncate">{statusFilterLabel}</span>}</SelectValue>
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="max-h-72">
                  <SelectItem value={ACTIVE_PLANS}>{t(($) => $.page.active_plans)}</SelectItem>
                  <SelectItem value="all">{t(($) => $.page.all_statuses)}</SelectItem>
                  {planStatuses.map((status) => (
                    <SelectItem key={status} value={status}>
                      {t(($) => $.status[status])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={selectedMilestoneVisible ? selectedMilestoneId! : ALL_PLANS}
                onValueChange={(value) => setSelectedMilestoneId(value === ALL_PLANS ? null : value)}
              >
                <SelectTrigger size="sm" className="min-w-[260px] max-w-full">
                  <SelectValue>{() => <span className="truncate">{selectedPlanLabel}</span>}</SelectValue>
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="max-h-72">
                  <SelectItem value={ALL_PLANS}>{t(($) => $.page.all_plans)}</SelectItem>
                  {filteredMilestones.map((milestone) => (
                    <SelectItem key={milestone.id} value={milestone.id}>
                      <span className="truncate">{milestone.title}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {(selectedMilestoneVisible || statusFilter !== ACTIVE_PLANS) && (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setSelectedMilestoneId(null);
                    setStatusFilter(ACTIVE_PLANS);
                  }}
                >
                  {t(($) => $.page.clear_filter)}
                </Button>
              )}
            </div>

            {filteredMilestones.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-md border py-16 text-muted-foreground">
                <CalendarRange className="mb-3 size-8 opacity-30" />
                <p className="text-sm">{t(($) => $.empty.filtered_title)}</p>
                <Button
                  size="sm"
                  variant="outline"
                  className="mt-4"
                  onClick={() => {
                    setSelectedMilestoneId(null);
                    setStatusFilter("all");
                  }}
                >
                  {t(($) => $.page.all_statuses)}
                </Button>
              </div>
            ) : (
              <>

            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <StatCard icon={CalendarRange} label={t(($) => $.stats.plans.label)} value={String(filteredMilestones.length)} detail={t(($) => $.stats.plans.detail)} />
              <StatCard icon={FolderKanban} label={t(($) => $.stats.projects.label)} value={String(scopedProjects.length)} detail={t(($) => $.stats.projects.detail, { count: completedProjects })} />
              <StatCard icon={ListTodo} label={t(($) => $.stats.execution.label)} value={`${issueProgress}%`} detail={t(($) => $.stats.execution.detail, { done: doneIssues, total: totalIssues })} />
            </div>

            <div className="grid grid-cols-1 gap-5">
              <section className="rounded-md border bg-card">
                <div className="flex items-center justify-between border-b px-4 py-3">
                  <div className="flex items-center gap-2">
                    <BarChart3 className="size-4 text-muted-foreground" />
                    <h2 className="text-sm font-medium">{t(($) => $.progress.title)}</h2>
                  </div>
                  <span className="text-xs text-muted-foreground">{t(($) => $.progress.subtitle)}</span>
                </div>
                <div className="overflow-x-auto">
                  <div className="min-w-[760px]">
                    {filteredMilestones.map((milestone) => (
                      <PlanRow
                        key={milestone.id}
                        milestone={milestone}
                        projects={projectsByMilestone.get(milestone.id) ?? []}
                        onEdit={(item) => setEditingMilestone(item)}
                      />
                    ))}
                  </div>
                </div>
              </section>

            </div>
              </>
            )}
          </div>
        )}
      </div>
      <PlanDialog
        open={createOpen || Boolean(editingMilestone)}
        onOpenChange={(open) => {
          if (!open) {
            setCreateOpen(false);
            setEditingMilestone(null);
          }
        }}
        milestone={editingMilestone}
        onSaved={(milestoneId) => setSelectedMilestoneId(milestoneId)}
      />
    </div>
  );
}

