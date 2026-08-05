"use client";

import { useMemo } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { CheckCircle2, KeyRound, LoaderCircle, Play, RefreshCw, Settings2 } from "lucide-react";
import { autopilotListOptions, autopilotRunsOptions, useTriggerAutopilot } from "@multica/core/autopilots";
import { creativeMaterialLibraryOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Autopilot, AutopilotRun, CreativeMaterialCrawlRun } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { toast } from "sonner";
import { useNavigation } from "../../navigation";

export interface CollectionPlanMatch {
  plan: Autopilot;
  crawlRun?: CreativeMaterialCrawlRun;
  lastImportedCrawlRun?: CreativeMaterialCrawlRun;
}

export function matchCollectionPlans(
  plans: Autopilot[],
  runsByPlan: Map<string, AutopilotRun[]>,
  crawlRuns: CreativeMaterialCrawlRun[],
): CollectionPlanMatch[] {
  const planByAutopilotRun = new Map<string, Autopilot>();
  for (const plan of plans) {
    for (const run of runsByPlan.get(plan.id) ?? []) {
      planByAutopilotRun.set(run.id, plan);
    }
  }
  const latestCrawlRunByPlan = new Map<string, CreativeMaterialCrawlRun>();
  const lastImportedCrawlRunByPlan = new Map<string, CreativeMaterialCrawlRun>();
  for (const crawlRun of crawlRuns) {
    const plan = planByAutopilotRun.get(crawlRun.autopilot_run_id);
    if (!plan) continue;
    const current = latestCrawlRunByPlan.get(plan.id);
    if (!current || compareCrawlRuns(crawlRun, current) > 0) {
      latestCrawlRunByPlan.set(plan.id, crawlRun);
    }
    const lastImported = lastImportedCrawlRunByPlan.get(plan.id);
    if (crawlRun.imported_count > 0 && (!lastImported || compareCrawlRuns(crawlRun, lastImported) > 0)) {
      lastImportedCrawlRunByPlan.set(plan.id, crawlRun);
    }
  }
  return plans.map((plan) => {
    const crawlRun = latestCrawlRunByPlan.get(plan.id);
    const lastImportedCrawlRun = lastImportedCrawlRunByPlan.get(plan.id);
    return {
      plan,
      crawlRun,
      ...(lastImportedCrawlRun && lastImportedCrawlRun.id !== crawlRun?.id ? { lastImportedCrawlRun } : {}),
    };
  });
}

function compareCrawlRuns(left: CreativeMaterialCrawlRun, right: CreativeMaterialCrawlRun): number {
  const leftTime = Date.parse(left.created_at || left.started_at || "") || 0;
  const rightTime = Date.parse(right.created_at || right.started_at || "") || 0;
  return leftTime !== rightTime ? leftTime - rightTime : left.id.localeCompare(right.id);
}

export function CreativeCollectionPlans() {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const autopilots = useQuery(autopilotListOptions(wsId));
  const materials = useQuery(creativeMaterialLibraryOptions(wsId));
  const plans = useMemo(
    () => (autopilots.data ?? []).filter((autopilot) => autopilot.execution_mode === "run_only"),
    [autopilots.data],
  );
  const planRuns = useQueries({
    queries: plans.map((plan) => autopilotRunsOptions(wsId, plan.id)),
  });
  const runsByPlan = useMemo(
    () => new Map(plans.map((plan, index) => [plan.id, planRuns[index]?.data ?? []])),
    [planRuns, plans],
  );
  const matches = useMemo(
    () => matchCollectionPlans(plans, runsByPlan, materials.data?.crawl_runs ?? []),
    [materials.data?.crawl_runs, plans, runsByPlan],
  );
  const trigger = useTriggerAutopilot();
  const openAutopilots = () => navigation.push(paths.autopilots());
  const openCredentials = () => navigation.push(`${paths.settings()}?tab=integrations`);
  const runNow = async (plan: Autopilot) => {
    try {
      await trigger.mutateAsync(plan.id);
      toast.success(`已启动 ${plan.title}`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法启动采集计划");
    }
  };

  return (
    <section className="border" aria-label="采集计划">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold">采集计划</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">由 Multica 自动化执行，采集结果直接归档到素材库。</p>
        </div>
        <Button size="sm" variant="outline" onClick={openAutopilots}>
          <Settings2 className="h-4 w-4" />管理自动化
        </Button>
      </div>

      {matches.length === 0 && !autopilots.isLoading && (
        <div className="px-4 py-5 text-sm text-muted-foreground">
          暂无采集计划。请在自动化中创建 run_only 计划。
        </div>
      )}
      <div className="divide-y">
        {matches.map(({ plan, crawlRun, lastImportedCrawlRun }) => (
          <CollectionPlanRow
            key={plan.id}
            plan={plan}
            crawlRun={crawlRun}
            lastImportedCrawlRun={lastImportedCrawlRun}
            running={trigger.isPending}
            onRunNow={() => void runNow(plan)}
            onOpenCredentials={openCredentials}
          />
        ))}
      </div>
    </section>
  );
}

function CollectionPlanRow({
  plan,
  crawlRun,
  lastImportedCrawlRun,
  running,
  onRunNow,
  onOpenCredentials,
}: {
  plan: Autopilot;
  crawlRun?: CreativeMaterialCrawlRun;
  lastImportedCrawlRun?: CreativeMaterialCrawlRun;
  running: boolean;
  onRunNow: () => void;
  onOpenCredentials: () => void;
}) {
  const status = crawlRun?.status || "not_started";
  const canRun = plan.status === "active";
  const analysis = crawlRun ? crawlRunAnalysisProgress(crawlRun) : null;
  const lastImportedAnalysis = lastImportedCrawlRun ? crawlRunAnalysisProgress(lastImportedCrawlRun) : null;
  return (
    <div className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">{plan.title}</span>
          <Badge variant={crawlRunStatusVariant(status)}>{crawlRunStatusLabel(status)}</Badge>
        </div>
        {crawlRun ? <>
          <p className="mt-1 truncate text-xs text-muted-foreground">{crawlRun.query_summary || "最近一次采集"}</p>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
            <Badge variant="outline">{crawlRun.imported_count > 0 ? `新增 ${crawlRun.imported_count}` : "本轮无新增"}</Badge>
            <Badge variant={analysis?.variant}>{analysis?.label}</Badge>
            {crawlRun.existing_count > 0 && <span className="text-muted-foreground">历史复用 {crawlRun.existing_count}</span>}
          </div>
          {lastImportedCrawlRun && lastImportedAnalysis && <div className="mt-2 flex flex-wrap items-center gap-2 border-l-2 border-emerald-600 pl-2 text-xs">
            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-700" />
            <span className="font-medium">最近新增批次 {lastImportedCrawlRun.imported_count} 张</span>
            <span className="text-muted-foreground">{lastImportedAnalysis.label} · 历史复用 {lastImportedCrawlRun.existing_count}</span>
          </div>}
        </> : <p className="mt-1 text-xs text-muted-foreground">尚未运行，可直接从这里启动首次采集</p>}
        {crawlRun?.error_message && <p className="mt-1 break-words text-xs text-destructive">{crawlRun.error_message}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {status === "action_required" && (
          <Button size="sm" variant="outline" onClick={onOpenCredentials}>
            <KeyRound className="h-4 w-4" />重新登录/检查凭证
          </Button>
        )}
        {status === "failed" && (
          <Button size="sm" variant="outline" disabled={!canRun || running} onClick={onRunNow}>
            {running ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            重新运行
          </Button>
        )}
        {status !== "failed" && status !== "action_required" && (
          <Button size="sm" variant="outline" disabled={!canRun || running} onClick={onRunNow}>
            {running ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
            立即运行
          </Button>
        )}
      </div>
    </div>
  );
}

function crawlRunStatusLabel(status: string) {
  switch (status) {
    case "running": return "采集中";
    case "completed": return "采集完成";
    case "partial": return "部分完成";
    case "failed": return "失败";
    case "action_required": return "需要处理";
    case "cancelled": return "已取消";
    case "not_started": return "未运行";
    default: return "排队中";
  }
}

export function crawlRunAnalysisProgress(run: CreativeMaterialCrawlRun): {
  label: string;
  variant: "default" | "destructive" | "outline";
} {
  const expected = Math.max(run.imported_count, 0);
  const analyzed = Math.max(run.candidate_metrics?.analyzed ?? 0, 0);
  const failed = Math.max(run.candidate_metrics?.analysis_failed ?? 0, 0);
  if (expected === 0) return { label: "无新增待分析", variant: "outline" };
  if (failed > 0) return { label: `新增分析 ${analyzed}/${expected} · 失败 ${failed}`, variant: "destructive" };
  if (analyzed >= expected) return { label: `新增分析完成 ${analyzed}/${expected}`, variant: "default" };
  return { label: `新增分析 ${analyzed}/${expected}`, variant: "outline" };
}

function crawlRunStatusVariant(status: string): "default" | "secondary" | "destructive" | "outline" {
  if (status === "completed") return "default";
  if (status === "failed" || status === "action_required") return "destructive";
  return "outline";
}
