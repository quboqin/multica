"use client";

import { useEffect, useMemo, useState } from "react";
import { Activity, Gauge, MoreHorizontal, Pencil, Plus, Target, Trash2, TrendingUp } from "lucide-react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
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
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect } from "@multica/ui/components/ui/native-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { kpiMetricListOptions, useCreateKpiMetric, useDeleteKpiMetric, useUpdateKpiMetric } from "@multica/core/kpis";
import { memberListOptions, squadListOptions, workspaceKeys } from "@multica/core/workspace/queries";
import type { KpiMetric, KpiMetricStatus, SquadMember } from "@multica/core/types";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { PageHeader } from "../layout/page-header";

const kpiStatuses: KpiMetricStatus[] = ["pending", "on_track", "at_risk", "missed"];
const NO_OWNER = "__none__";

const kpiStatusClass: Record<KpiMetricStatus, string> = {
  on_track: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  at_risk: "bg-amber-500/10 text-amber-700 dark:text-amber-300",
  missed: "bg-rose-500/10 text-rose-700 dark:text-rose-300",
  pending: "bg-slate-500/10 text-slate-600 dark:text-slate-300",
};

const kpiStatusBarClass: Record<KpiMetricStatus, string> = {
  on_track: "bg-emerald-500",
  at_risk: "bg-amber-500",
  missed: "bg-rose-500",
  pending: "bg-slate-400",
};

function percent(part: number, total: number) {
  return total > 0 ? Math.round((part / total) * 100) : 0;
}

function clampCompletionRate(value: string) {
  const parsed = Number.parseInt(value, 10);
  if (Number.isNaN(parsed)) return 0;
  return Math.min(100, Math.max(0, parsed));
}

function KpiMetricDialog({
  open,
  onOpenChange,
  metric,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  metric: KpiMetric | null;
}) {
  const { t } = useT("plans");
  const wsId = useWorkspaceId();
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const createMetric = useCreateKpiMetric();
  const updateMetric = useUpdateKpiMetric();
  const [name, setName] = useState("");
  const [owner, setOwner] = useState("");
  const [target, setTarget] = useState("");
  const [current, setCurrent] = useState("");
  const [completionRate, setCompletionRate] = useState(0);
  const [status, setStatus] = useState<KpiMetricStatus>("pending");
  const [note, setNote] = useState("");
  const isEditing = Boolean(metric);
  const pending = createMetric.isPending || updateMetric.isPending;

  useEffect(() => {
    if (!open) return;
    setName(metric?.name ?? "");
    setOwner(metric?.owner ?? "");
    setTarget(metric?.target ?? "");
    setCurrent(metric?.current ?? "");
    setCompletionRate(metric?.completion_rate ?? 0);
    setStatus(metric?.status ?? "pending");
    setNote(metric?.note ?? "");
  }, [metric, open]);

  const reset = () => {
    setName("");
    setOwner("");
    setTarget("");
    setCurrent("");
    setCompletionRate(0);
    setStatus("pending");
    setNote("");
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !pending) reset();
    onOpenChange(nextOpen);
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName || pending) return;
    const payload = {
      name: trimmedName,
      owner: owner.trim(),
      target: target.trim(),
      current: current.trim(),
      completion_rate: completionRate,
      status,
      note: note.trim(),
    };
    try {
      if (metric) {
        await updateMetric.mutateAsync({ id: metric.id, ...payload });
        toast.success(t(($) => $.kpi.toast_updated));
      } else {
        await createMetric.mutateAsync(payload);
        toast.success(t(($) => $.kpi.toast_created));
      }
      reset();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error && err.message ? err.message : t(($) => $.kpi.toast_failed));
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={handleSubmit} className="contents">
          <DialogHeader>
            <DialogTitle>{isEditing ? t(($) => $.kpi.dialog.edit_title) : t(($) => $.kpi.dialog.create_title)}</DialogTitle>
            <DialogDescription>{t(($) => $.kpi.dialog.description)}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.fields.name)}</span>
              <Input value={name} onChange={(event) => setName(event.target.value)} placeholder={t(($) => $.kpi.placeholders.name)} autoFocus />
            </label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.fields.owner)}</span>
                <Select items={[]} value={owner || NO_OWNER} onValueChange={(value) => setOwner(!value || value === NO_OWNER ? "" : value)}>
                  <SelectTrigger className="w-full">
                    <SelectValue>
                      {() => <span className="truncate">{owner || t(($) => $.kpi.no_owner)}</span>}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_OWNER}>{t(($) => $.kpi.no_owner)}</SelectItem>
                    {members.map((member) => (
                      <SelectItem key={member.user_id} value={member.name}>
                        <span className="truncate">{member.name}</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </label>
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.fields.status)}</span>
                <NativeSelect value={status} onChange={(event) => setStatus(event.target.value as KpiMetricStatus)}>
                  {kpiStatuses.map((item) => (
                    <option key={item} value={item}>
                      {t(($) => $.kpi.status[item])}
                    </option>
                  ))}
                </NativeSelect>
              </label>
            </div>
            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.completion_rate)}</span>
              <Input
                type="number"
                min={0}
                max={100}
                value={completionRate}
                onChange={(event) => setCompletionRate(clampCompletionRate(event.target.value))}
                placeholder={t(($) => $.kpi.placeholders.completion_rate)}
              />
            </label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.target)}</span>
                <Input value={target} onChange={(event) => setTarget(event.target.value)} placeholder={t(($) => $.kpi.placeholders.target)} />
              </label>
              <label className="block space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.current)}</span>
                <Input value={current} onChange={(event) => setCurrent(event.target.value)} placeholder={t(($) => $.kpi.placeholders.current)} />
              </label>
            </div>
            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">{t(($) => $.kpi.fields.note)}</span>
              <Textarea value={note} onChange={(event) => setNote(event.target.value)} placeholder={t(($) => $.kpi.placeholders.note)} className="min-h-20 resize-none" />
            </label>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={() => handleOpenChange(false)}>
              {t(($) => $.kpi.dialog.cancel)}
            </Button>
            <Button type="submit" disabled={!name.trim() || pending}>
              {pending ? t(($) => $.kpi.dialog.saving) : t(($) => $.kpi.dialog.save)}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function KpiPage() {
  const { t } = useT("plans");
  const wsId = useWorkspaceId();
  const { data: metrics = [] } = useQuery(kpiMetricListOptions(wsId));
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: squads = [] } = useQuery(squadListOptions(wsId));
  const squadMemberQueries = useQueries({
    queries: squads.map((squad) => ({
      queryKey: [...workspaceKeys.squads(wsId), squad.id, "members"] as const,
      queryFn: () => api.listSquadMembers(squad.id),
      enabled: Boolean(wsId && squad.id),
      staleTime: 30 * 1000,
    })),
  });
  const deleteMetric = useDeleteKpiMetric();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingMetric, setEditingMetric] = useState<KpiMetric | null>(null);

  const metricSquadLabels = useMemo(() => {
    const ownerNameToUserId = new Map<string, string>();
    for (const member of members) {
      ownerNameToUserId.set(member.name.trim().toLocaleLowerCase(), member.user_id);
    }

    const squadNamesByMemberId = new Map<string, string[]>();
    squads.forEach((squad, index) => {
      const queryMembers = squadMemberQueries[index]?.data as SquadMember[] | undefined;
      const squadMembers = queryMembers ?? squad.member_preview ?? [];
      for (const squadMember of squadMembers) {
        if (squadMember.member_type !== "member") continue;
        const existing = squadNamesByMemberId.get(squadMember.member_id) ?? [];
        existing.push(squad.name);
        squadNamesByMemberId.set(squadMember.member_id, existing);
      }
    });

    const noSquad = t(($) => $.kpi.no_squad);
    const labels = new Map<string, string>();
    for (const metric of metrics) {
      const owner = metric.owner.trim();
      const memberId = owner ? ownerNameToUserId.get(owner.toLocaleLowerCase()) : undefined;
      const squadNames = memberId ? squadNamesByMemberId.get(memberId) : undefined;
      labels.set(metric.id, squadNames && squadNames.length > 0 ? squadNames.join(" / ") : noSquad);
    }
    return labels;
  }, [members, metrics, squadMemberQueries, squads, t]);

  const summary = useMemo(() => {
    const counts = Object.fromEntries(kpiStatuses.map((status) => [status, 0])) as Record<KpiMetricStatus, number>;
    const squadMap = new Map<string, number>();
    for (const metric of metrics) {
      counts[metric.status] += 1;
      const squad = metricSquadLabels.get(metric.id) ?? t(($) => $.kpi.no_squad);
      squadMap.set(squad, (squadMap.get(squad) ?? 0) + 1);
    }
    const squads = [...squadMap.entries()]
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
      .slice(0, 5);
    return {
      counts,
      squads,
      total: metrics.length,
      active: metrics.filter((metric) => metric.status !== "pending").length,
      healthy: counts.on_track,
      watch: counts.at_risk + counts.missed,
      averageCompletion: metrics.length > 0 ? Math.round(metrics.reduce((sum, metric) => sum + metric.completion_rate, 0) / metrics.length) : 0,
    };
  }, [metricSquadLabels, metrics, t]);

  const openCreate = () => {
    setEditingMetric(null);
    setDialogOpen(true);
  };

  const openEdit = (metric: KpiMetric) => {
    setEditingMetric(metric);
    setDialogOpen(true);
  };

  const handleDelete = (metric: KpiMetric) => {
    deleteMetric.mutate(metric.id, {
      onSuccess: () => toast.success(t(($) => $.kpi.toast_deleted)),
      onError: (err) => toast.error(err instanceof Error && err.message ? err.message : t(($) => $.kpi.toast_failed)),
    });
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex items-center gap-2">
          <Gauge className="size-4 text-muted-foreground" />
          <h1 className="text-sm font-medium">{t(($) => $.kpi.title)}</h1>
        </div>
        <Button size="sm" onClick={openCreate}>
          <Plus className="size-4" />
          {t(($) => $.kpi.create_action)}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-auto p-5">
        <div className="space-y-5">
          <section className="rounded-md border bg-card p-5 shadow-sm">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <h2 className="text-lg font-semibold">{t(($) => $.kpi.title)}</h2>
                  <span className="rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">{t(($) => $.kpi.independent_badge)}</span>
                </div>
                <p className="mt-2 max-w-3xl text-sm text-muted-foreground">{t(($) => $.kpi.workspace_description)}</p>
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:w-[480px]">
                <div className="rounded border bg-background px-3 py-2">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground"><Target className="size-3.5" />{t(($) => $.kpi.board.total)}</div>
                  <div className="mt-1 text-xl font-semibold">{summary.total}</div>
                </div>
                <div className="rounded border bg-background px-3 py-2">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground"><Gauge className="size-3.5" />{t(($) => $.kpi.board.completion)}</div>
                  <div className="mt-1 text-xl font-semibold text-sky-600">{summary.averageCompletion}%</div>
                </div>
                <div className="rounded border bg-background px-3 py-2">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground"><TrendingUp className="size-3.5" />{t(($) => $.kpi.board.healthy)}</div>
                  <div className="mt-1 text-xl font-semibold text-emerald-600">{summary.healthy}</div>
                </div>
                <div className="rounded border bg-background px-3 py-2">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground"><Activity className="size-3.5" />{t(($) => $.kpi.board.watch)}</div>
                  <div className="mt-1 text-xl font-semibold text-amber-600">{summary.watch}</div>
                </div>
              </div>
            </div>
          </section>

          <section className="grid grid-cols-1 gap-5 xl:grid-cols-[1.25fr_0.75fr]">
            <div className="rounded-md border bg-card shadow-sm">
              <div className="border-b px-4 py-3">
                <h2 className="text-sm font-medium">{t(($) => $.kpi.board.status_title)}</h2>
                <p className="mt-1 text-xs text-muted-foreground">{t(($) => $.kpi.board.status_subtitle)}</p>
              </div>
              <div className="space-y-4 p-4">
                {kpiStatuses.map((status) => {
                  const count = summary.counts[status];
                  const value = percent(count, summary.total);
                  return (
                    <div key={status}>
                      <div className="mb-1.5 flex items-center justify-between text-xs">
                        <span className="font-medium">{t(($) => $.kpi.status[status])}</span>
                        <span className="text-muted-foreground">{count} / {summary.total}</span>
                      </div>
                      <div className="h-2 overflow-hidden rounded-full bg-muted">
                        <div className={cn("h-full rounded-full", kpiStatusBarClass[status])} style={{ width: `${value}%` }} />
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            <div className="rounded-md border bg-card shadow-sm">
              <div className="border-b px-4 py-3">
                <h2 className="text-sm font-medium">{t(($) => $.kpi.board.squad_title)}</h2>
                <p className="mt-1 text-xs text-muted-foreground">{t(($) => $.kpi.board.squad_subtitle)}</p>
              </div>
              <div className="space-y-3 p-4">
                {summary.squads.length === 0 ? (
                  <div className="py-8 text-center text-sm text-muted-foreground">{t(($) => $.kpi.empty)}</div>
                ) : (
                  summary.squads.map((squad) => (
                    <div key={squad.name} className="flex items-center justify-between rounded border bg-background px-3 py-2">
                      <span className="truncate text-sm font-medium">{squad.name}</span>
                      <span className="text-xs text-muted-foreground">{squad.count}</span>
                    </div>
                  ))
                )}
              </div>
            </div>
          </section>

          <section className="rounded-md border bg-card shadow-sm">
            <div className="flex items-center justify-between border-b px-4 py-3">
              <div className="flex items-center gap-2">
                <Gauge className="size-4 text-muted-foreground" />
                <h2 className="text-sm font-medium">{t(($) => $.kpi.metrics_title)}</h2>
              </div>
              <span className="text-xs text-muted-foreground">{t(($) => $.kpi.subtitle)}</span>
            </div>
            {metrics.length === 0 ? (
              <div className="flex flex-col items-center justify-center px-4 py-14 text-center text-sm text-muted-foreground">
                <Gauge className="mb-3 size-8 opacity-30" />
                <p>{t(($) => $.kpi.empty)}</p>
                <Button size="sm" className="mt-4" onClick={openCreate}>
                  <Plus className="size-4" />
                  {t(($) => $.kpi.create_action)}
                </Button>
              </div>
            ) : (
              <div className="grid grid-cols-1 divide-y lg:grid-cols-2 lg:divide-x lg:divide-y-0">
                {metrics.map((metric) => (
                  <div key={metric.id} className="p-4">
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium">{metric.name}</div>
                        <div className="mt-1 flex flex-wrap gap-x-2 gap-y-1 text-xs text-muted-foreground">
                          <span className="truncate">{t(($) => $.kpi.fields.owner)}: {metric.owner || t(($) => $.kpi.no_owner)}</span>
                          <span className="truncate">{t(($) => $.kpi.fields.squad)}: {metricSquadLabels.get(metric.id) ?? t(($) => $.kpi.no_squad)}</span>
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        <span className={cn("rounded px-2 py-1 text-xs", kpiStatusClass[metric.status])}>
                          {t(($) => $.kpi.status[metric.status])}
                        </span>
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button variant="ghost" size="icon-sm" className="text-muted-foreground">
                                <MoreHorizontal />
                              </Button>
                            }
                          />
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onClick={() => openEdit(metric)}>
                              <Pencil className="h-3.5 w-3.5" />
                              {t(($) => $.kpi.edit_action)}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem variant="destructive" onClick={() => handleDelete(metric)}>
                              <Trash2 className="h-3.5 w-3.5" />
                              {t(($) => $.kpi.delete_action)}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </div>
                    <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
                      <div>
                        <div className="text-muted-foreground">{t(($) => $.kpi.target)}</div>
                        <div className="mt-1 font-medium">{metric.target || t(($) => $.kpi.pending)}</div>
                      </div>
                      <div>
                        <div className="text-muted-foreground">{t(($) => $.kpi.current)}</div>
                        <div className="mt-1 font-medium">{metric.current || t(($) => $.kpi.pending)}</div>
                      </div>
                    </div>
                    <div className="mt-3">
                      <div className="mb-1.5 flex items-center justify-between text-xs">
                        <span className="text-muted-foreground">{t(($) => $.kpi.completion_rate)}</span>
                        <span className="font-medium">{metric.completion_rate}%</span>
                      </div>
                      <div className="h-2 overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full bg-sky-500" style={{ width: `${metric.completion_rate}%` }} />
                      </div>
                    </div>
                    {metric.note && <div className="mt-3 rounded bg-muted/40 px-3 py-2 text-xs text-muted-foreground">{metric.note}</div>}
                  </div>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>

      <KpiMetricDialog open={dialogOpen} onOpenChange={setDialogOpen} metric={editingMetric} />
    </div>
  );
}

export const PlansKpiPage = KpiPage;
