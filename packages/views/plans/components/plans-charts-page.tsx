"use client";

import { useMemo, useState } from "react";
import { ArrowLeft, BarChart3 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { milestoneListOptions } from "@multica/core/milestones";
import { projectListOptions } from "@multica/core/projects/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { PageHeader } from "../../layout/page-header";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
import type { Label, MilestoneStatus, Project, ProjectStatus } from "@multica/core/types";

const projectStatuses: ProjectStatus[] = ["planned", "in_progress", "paused", "completed", "cancelled"];
const planStatuses: MilestoneStatus[] = ["planned", "in_progress", "paused", "completed", "cancelled"];
const ALL_PLANS = "__all__";
const ACTIVE_PLANS = "__active__";
type PlanStatusFilter = typeof ACTIVE_PLANS | "all" | MilestoneStatus;
type LabelProgressRow = {
  id: string;
  name: string;
  color: string;
  projectCount: number;
  done: number;
  total: number;
  progress: number;
};
const terminalProjectStatuses = new Set<ProjectStatus>(["completed", "cancelled"]);
const priorityRank = { urgent: 0, high: 1, medium: 2, low: 3, none: 4 } as const;
const colors = {
  blue: "#2563eb",
  green: "#16a34a",
  amber: "#f59e0b",
  red: "#ef4444",
  slate: "#94a3b8",
} as const;
const statusColor: Record<ProjectStatus, string> = {
  planned: colors.slate,
  in_progress: colors.blue,
  paused: colors.amber,
  completed: colors.green,
  cancelled: colors.red,
};

function projectProgress(project: Project): number {
  if (project.issue_count > 0) return Math.round((project.done_count / project.issue_count) * 100);
  return terminalProjectStatuses.has(project.status) ? 100 : 0;
}

function ChartCard({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  return (
    <section className="overflow-hidden rounded-md border bg-card shadow-sm">
      <div className="border-b px-5 py-4">
        <h3 className="truncate text-sm font-semibold leading-none">{title}</h3>
        <p className="mt-1.5 truncate text-xs text-muted-foreground">{subtitle}</p>
      </div>
      <div className="p-5">{children}</div>
    </section>
  );
}

function ProgressRing({ value, label }: { value: number; label: string }) {
  return (
    <div className="relative grid size-36 place-items-center rounded-full bg-muted">
      <div
        className="absolute inset-0 rounded-full"
        style={{
          background: `conic-gradient(${colors.blue} ${value * 3.6}deg, color-mix(in oklch, var(--muted-foreground) 16%, transparent) 0deg)`,
        }}
      />
      <div className="absolute inset-3 rounded-full bg-card" />
      <div className="relative text-center">
        <div className="text-3xl font-semibold tabular-nums">{value}%</div>
        <div className="mt-1 text-xs text-muted-foreground">{label}</div>
      </div>
    </div>
  );
}

function MilestoneStatusBars({
  rows,
  statusLabels,
  projectCountLabel,
  progressLabel,
}: {
  rows: Array<Record<ProjectStatus | "id" | "name" | "progress" | "total", string | number>>;
  statusLabels: Record<ProjectStatus, string>;
  projectCountLabel: (count: number) => string;
  progressLabel: string;
}) {
  return (
    <div className="divide-y">
      {rows.map((row) => {
        const total = Number(row.total);
        const progressed = Number(row.in_progress ?? 0) + Number(row.paused ?? 0) + Number(row.completed ?? 0) + Number(row.cancelled ?? 0);
        const progressPct = total > 0 ? Math.round((progressed / total) * 100) : 0;
        return (
          <div key={String(row.id)} className="grid grid-cols-[minmax(180px,1fr)_minmax(280px,1.1fr)] gap-5 py-4 first:pt-0 last:pb-0">
            <div className="min-w-0">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold">{String(row.name)}</div>
                  <div className="mt-1 text-xs text-muted-foreground">{projectCountLabel(total)}</div>
                </div>
                <div className="shrink-0 text-right">
                <div className="bg-gradient-to-r from-sky-500 to-emerald-500 bg-clip-text text-xl font-semibold tabular-nums text-transparent">{row.progress}%</div>
                <div className="text-[11px] text-muted-foreground">{progressLabel}</div>
              </div>
            </div>
            <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full bg-gradient-to-r from-sky-500 via-cyan-500 to-emerald-500"
                  style={{ width: `${progressPct}%` }}
                />
              </div>
            </div>
            <div className="grid grid-cols-5 gap-2">
              {projectStatuses.map((status) => {
                const count = Number(row[status] ?? 0);
                return (
                  <div key={status} className="rounded-md border bg-muted/30 px-2 py-2 text-center">
                    <div className="mx-auto mb-1 size-2 rounded-sm" style={{ backgroundColor: statusColor[status] }} />
                    <div className="text-base font-semibold tabular-nums">{count}</div>
                    <div className="mt-0.5 truncate text-[11px] text-muted-foreground">{statusLabels[status]}</div>
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function LabelProgressChart({
  rows,
  projectCountLabel,
  issueCountLabel,
  progressLabel,
}: {
  rows: LabelProgressRow[];
  projectCountLabel: (count: number) => string;
  issueCountLabel: (done: number, total: number) => string;
  progressLabel: string;
}) {
  return (
    <div className="space-y-3">
      {rows.map((row) => (
        <div key={row.id} className="rounded-md border bg-muted/20 px-4 py-3">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex min-w-0 items-center gap-2">
                <span className="size-2.5 shrink-0 rounded-full" style={{ backgroundColor: row.color }} />
                <span className="truncate text-sm font-semibold">{row.name}</span>
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                {projectCountLabel(row.projectCount)} ? {issueCountLabel(row.done, row.total)}
              </div>
            </div>
            <div className="shrink-0 text-right">
              <div className="text-xl font-semibold tabular-nums">{row.progress}%</div>
              <div className="text-[11px] text-muted-foreground">{progressLabel}</div>
            </div>
          </div>
          <div className="mt-3 h-2.5 overflow-hidden rounded-full bg-background">
            <div
              className="h-full rounded-full"
              style={{ width: `${row.progress}%`, backgroundColor: row.color }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

export function PlansChartsPage() {
  const { t } = useT("plans");
  const wsId = useWorkspaceId();
  const wsPaths = useWorkspacePaths();
  const [selectedMilestoneId, setSelectedMilestoneId] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<PlanStatusFilter>(ACTIVE_PLANS);
  const { data: milestones = [], isLoading: milestonesLoading } = useQuery(milestoneListOptions(wsId));
  const { data: projects = [], isLoading: projectsLoading } = useQuery(projectListOptions(wsId));

  const projectsByMilestone = useMemo(() => {
    const map = new Map<string, Project[]>();
    for (const milestone of milestones) map.set(milestone.id, []);
    for (const project of projects) {
      if (!project.milestone_id) continue;
      map.set(project.milestone_id, [...(map.get(project.milestone_id) ?? []), project]);
    }
    return map;
  }, [milestones, projects]);

  const filteredMilestones = useMemo(
    () =>
      milestones.filter((milestone) => {
        if (statusFilter === "all") return true;
        if (statusFilter === ACTIVE_PLANS) return milestone.status !== "completed";
        return milestone.status === statusFilter;
      }),
    [milestones, statusFilter],
  );
  const filteredMilestoneIds = useMemo(() => new Set(filteredMilestones.map((milestone) => milestone.id)), [filteredMilestones]);
  const selectedMilestoneVisible = selectedMilestoneId ? filteredMilestoneIds.has(selectedMilestoneId) : false;
  const visibleSelectedMilestoneId = selectedMilestoneVisible ? selectedMilestoneId : null;
  const visibleMilestones = useMemo(
    () => (visibleSelectedMilestoneId ? filteredMilestones.filter((m) => m.id === visibleSelectedMilestoneId) : filteredMilestones),
    [filteredMilestones, visibleSelectedMilestoneId],
  );
  const scopedProjects = useMemo(
    () =>
      visibleSelectedMilestoneId
        ? projectsByMilestone.get(visibleSelectedMilestoneId) ?? []
        : projects.filter((p) => p.milestone_id && filteredMilestoneIds.has(p.milestone_id)),
    [filteredMilestoneIds, projects, projectsByMilestone, visibleSelectedMilestoneId],
  );
  const selectedMilestone = selectedMilestoneId ? filteredMilestones.find((m) => m.id === selectedMilestoneId) : null;
  const selectedPlanLabel = selectedMilestone?.title ?? t(($) => $.page.all_plans);
  const statusFilterLabel =
    statusFilter === "all"
      ? t(($) => $.page.all_statuses)
      : statusFilter === ACTIVE_PLANS
        ? t(($) => $.page.active_plans)
        : t(($) => $.status[statusFilter]);
  const doneIssues = scopedProjects.reduce((sum, p) => sum + p.done_count, 0);
  const totalIssues = scopedProjects.reduce((sum, p) => sum + p.issue_count, 0);
  const issueProgress = totalIssues > 0 ? Math.round((doneIssues / totalIssues) * 100) : 0;
  const loading = milestonesLoading || projectsLoading;

  const statusRows = useMemo(
    () =>
      projectStatuses.map((status) => ({
        status,
        name: t(($) => $.status[status]),
        value: scopedProjects.filter((project) => project.status === status).length,
        fill: statusColor[status],
      })),
    [scopedProjects, t],
  );
  const statusLabels = useMemo(
    () =>
      Object.fromEntries(projectStatuses.map((status) => [status, t(($) => $.status[status])])) as Record<ProjectStatus, string>,
    [t],
  );

  const labelRows = useMemo<LabelProgressRow[]>(() => {
    const unlabeled: Label = {
      id: "__unlabeled__",
      workspace_id: wsId,
      name: t(($) => $.charts.label_progress.unlabeled),
      color: colors.slate,
      created_at: "",
      updated_at: "",
    };
    const map = new Map<string, Omit<LabelProgressRow, "progress">>();
    for (const project of scopedProjects) {
      const labels = project.labels && project.labels.length > 0 ? project.labels : [unlabeled];
      for (const label of labels) {
        const current = map.get(label.id) ?? {
          id: label.id,
          name: label.name,
          color: label.color,
          projectCount: 0,
          done: 0,
          total: 0,
        };
        current.projectCount += 1;
        current.done += project.done_count;
        current.total += project.issue_count;
        map.set(label.id, current);
      }
    }
    return Array.from(map.values())
      .map((row) => ({
        ...row,
        progress: row.total > 0 ? Math.round((row.done / row.total) * 100) : 0,
      }))
      .sort((a, b) => b.projectCount - a.projectCount || b.progress - a.progress || a.name.localeCompare(b.name));
  }, [scopedProjects, t, wsId]);

  const milestoneRows = useMemo(
    () =>
      visibleMilestones.map((milestone) => {
        const milestoneProjects = projectsByMilestone.get(milestone.id) ?? [];
        const totalProgress = milestoneProjects.reduce((sum, project) => sum + projectProgress(project), 0);
        return {
          id: milestone.id,
          name: milestone.title,
          planned: milestoneProjects.filter((project) => project.status === "planned").length,
          in_progress: milestoneProjects.filter((project) => project.status === "in_progress").length,
          paused: milestoneProjects.filter((project) => project.status === "paused").length,
          completed: milestoneProjects.filter((project) => project.status === "completed").length,
          cancelled: milestoneProjects.filter((project) => project.status === "cancelled").length,
          total: milestoneProjects.length,
          progress: milestoneProjects.length > 0 ? Math.round(totalProgress / milestoneProjects.length) : 0,
        };
      }),
    [visibleMilestones, projectsByMilestone],
  );

  const keyProjects = useMemo(
    () =>
      scopedProjects
        .filter((project) => project.priority === "urgent" || project.priority === "high")
        .sort((a, b) => priorityRank[a.priority] - priorityRank[b.priority] || projectProgress(b) - projectProgress(a))
        .slice(0, 10),
    [scopedProjects],
  );
  const statusTotal = statusRows.reduce((sum, row) => sum + row.value, 0);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex items-center gap-2">
          <BarChart3 className="size-4 text-muted-foreground" />
          <h1 className="text-sm font-medium">{t(($) => $.charts.title)}</h1>
          {!loading && <span className="text-xs tabular-nums text-muted-foreground">{scopedProjects.length}</span>}
        </div>
        <Button size="sm" variant="outline" nativeButton={false} render={<AppLink href={wsPaths.plans()} />}>
          <ArrowLeft className="size-4" />
          {t(($) => $.charts.back)}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-auto p-5">
        {loading ? (
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
              {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 rounded-md" />)}
            </div>
            <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
              {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-[320px] rounded-md" />)}
            </div>
          </div>
        ) : (
          <div className="space-y-5">
            <div className="flex flex-wrap items-center gap-2">
              <Select
                items={[]}
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
                items={[]}
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

            <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
              <div className="rounded-md border bg-card p-4 shadow-sm"><div className="text-xs text-muted-foreground">{t(($) => $.stats.plans.label)}</div><div className="mt-2 text-2xl font-semibold tabular-nums">{visibleMilestones.length}</div></div>
              <div className="rounded-md border bg-card p-4 shadow-sm"><div className="text-xs text-muted-foreground">{t(($) => $.stats.projects.label)}</div><div className="mt-2 text-2xl font-semibold tabular-nums">{scopedProjects.length}</div></div>
              <div className="rounded-md border bg-card p-4 shadow-sm"><div className="text-xs text-muted-foreground">{t(($) => $.stats.execution.label)}</div><div className="mt-2 text-2xl font-semibold tabular-nums">{issueProgress}%</div></div>
              <div className="rounded-md border bg-card p-4 shadow-sm"><div className="text-xs text-muted-foreground">{t(($) => $.charts.tasks_done)}</div><div className="mt-2 text-2xl font-semibold tabular-nums">{doneIssues}/{totalIssues}</div></div>
            </div>

            {scopedProjects.length === 0 ? (
              <section className="rounded-md border bg-card p-8 text-center text-sm text-muted-foreground">{t(($) => $.charts.empty)}</section>
            ) : (
              <>
                <section className="grid grid-cols-1 gap-5 rounded-md border bg-card p-5 shadow-sm xl:grid-cols-[220px_minmax(0,1fr)]">
                  <div className="flex items-center justify-center">
                    <ProgressRing value={issueProgress} label={t(($) => $.charts.milestone_status.progress)} />
                  </div>
                  <div className="min-w-0 space-y-5">
                    <div>
                      <div className="text-sm font-semibold">{t(($) => $.charts.overview.title)}</div>
                      <div className="mt-1 text-xs text-muted-foreground">{t(($) => $.charts.overview.subtitle)}</div>
                    </div>
                    <div className="space-y-3">
                      {statusRows.filter((row) => row.value > 0).map((row) => {
                        const pct = statusTotal > 0 ? Math.round((row.value / statusTotal) * 100) : 0;
                        return (
                          <div key={row.status} className="grid grid-cols-[92px_minmax(0,1fr)_42px] items-center gap-3 text-xs">
                            <span className="truncate text-muted-foreground">{row.name}</span>
                            <div className="h-2 overflow-hidden rounded-full bg-muted">
                              <div className="h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: row.fill }} />
                            </div>
                            <span className="text-right tabular-nums text-muted-foreground">{row.value}</span>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </section>

                <ChartCard title={t(($) => $.charts.milestone_status.title)} subtitle={t(($) => $.charts.milestone_status.subtitle)}>
                  <MilestoneStatusBars
                    rows={milestoneRows}
                    statusLabels={statusLabels}
                    projectCountLabel={(count) => t(($) => $.charts.milestone_status.project_count, { count })}
                    progressLabel={t(($) => $.charts.milestone_status.progress)}
                  />
                </ChartCard>

                <ChartCard title={t(($) => $.charts.label_progress.title)} subtitle={t(($) => $.charts.label_progress.subtitle)}>
                  <LabelProgressChart
                    rows={labelRows}
                    projectCountLabel={(count) => t(($) => $.charts.label_progress.project_count, { count })}
                    issueCountLabel={(done, total) => t(($) => $.charts.label_progress.issue_count, { done, total })}
                    progressLabel={t(($) => $.charts.label_progress.progress)}
                  />
                </ChartCard>

                <section className="rounded-md border bg-card shadow-sm">
                  <div className="flex items-center justify-between border-b px-4 py-3">
                    <h2 className="text-sm font-semibold">{t(($) => $.charts.key_projects.title)}</h2>
                    <span className="text-xs text-muted-foreground">{t(($) => $.charts.key_projects.subtitle)}</span>
                  </div>
                  <div className="divide-y">
                    {keyProjects.map((project) => {
                      const pct = projectProgress(project);
                      return (
                        <div key={project.id} className="px-4 py-3 text-sm">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <AppLink href={wsPaths.projectDetail(project.id)} className="block truncate font-medium hover:underline">
                                {project.title}
                              </AppLink>
                              <div className="mt-1 text-xs text-muted-foreground">
                                {t(($) => $.charts.key_projects.issue_count, { done: project.done_count, total: project.issue_count })}
                              </div>
                            </div>
                            <span className="shrink-0 rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">{t(($) => $.status[project.status])}</span>
                          </div>
                          <div className="mt-3 flex items-center gap-3">
                            <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted">
                              <div
                                className="h-full rounded-full bg-gradient-to-r from-sky-500 via-cyan-500 to-emerald-500"
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                            <span className="w-10 bg-gradient-to-r from-sky-500 to-emerald-500 bg-clip-text text-right text-xs font-semibold tabular-nums text-transparent">{pct}%</span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </section>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
