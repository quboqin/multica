"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, CheckCircle2, ImageIcon, KeyRound, LoaderCircle, Play, RefreshCw, Settings2 } from "lucide-react";
import { api } from "@multica/core/api";
import { autopilotListOptions, autopilotRunsOptions, useTriggerAutopilot } from "@multica/core/autopilots";
import { creativeKeys, creativeMaterialLibraryOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Autopilot, AutopilotRun, CreativeMaterialCandidate, CreativeMaterialCrawlRun } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { toast } from "sonner";
import { useNavigation } from "../../navigation";

export interface CollectionPlanMatch {
  plan: Autopilot;
  crawlRun?: CreativeMaterialCrawlRun;
  lastImportedCrawlRun?: CreativeMaterialCrawlRun;
  activeAutopilotRun?: AutopilotRun;
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
    const planRuns = runsByPlan.get(plan.id) ?? [];
    const latestAutopilotRun = planRuns.reduce<AutopilotRun | undefined>((latest, run) => (
      !latest || compareAutopilotRuns(run, latest) > 0 ? run : latest
    ), undefined);
    const activeAutopilotRun = latestAutopilotRun
      && (latestAutopilotRun.status === "issue_created" || latestAutopilotRun.status === "running")
      ? latestAutopilotRun
      : undefined;
    const latestCrawlRun = latestCrawlRunByPlan.get(plan.id);
    // A newly triggered autopilot has not necessarily reached the browser crawl
    // yet. Do not show a previous run's terminal result as the new run's result.
    const crawlRun = activeAutopilotRun && latestCrawlRun?.autopilot_run_id !== activeAutopilotRun.id
      ? undefined
      : latestCrawlRun;
    const lastImportedCrawlRun = lastImportedCrawlRunByPlan.get(plan.id);
    return {
      plan,
      crawlRun,
      ...(lastImportedCrawlRun && lastImportedCrawlRun.id !== crawlRun?.id ? { lastImportedCrawlRun } : {}),
      ...(activeAutopilotRun ? { activeAutopilotRun } : {}),
    };
  });
}

export function resolveCollectionCrawlRunId(
  inspectedCrawlRunId: string,
  collectionCrawlRuns: CreativeMaterialCrawlRun[],
): string {
  return collectionCrawlRuns.some((crawlRun) => crawlRun.id === inspectedCrawlRunId)
    ? inspectedCrawlRunId
    : "";
}

export function latestCollectionCrawlRunId(collectionCrawlRuns: CreativeMaterialCrawlRun[]): string {
  return collectionCrawlRuns.reduce<CreativeMaterialCrawlRun | undefined>((latest, crawlRun) => (
    !latest || compareCrawlRuns(crawlRun, latest) > 0 ? crawlRun : latest
  ), undefined)?.id ?? "";
}

export function crawlRunForAutopilotRun(
  crawlRuns: CreativeMaterialCrawlRun[],
  autopilotRunId: string,
): CreativeMaterialCrawlRun | undefined {
  if (!autopilotRunId) return undefined;
  return crawlRuns.find((run) => run.autopilot_run_id === autopilotRunId);
}

function compareCrawlRuns(left: CreativeMaterialCrawlRun, right: CreativeMaterialCrawlRun): number {
  const leftTime = Date.parse(left.created_at || left.started_at || "") || 0;
  const rightTime = Date.parse(right.created_at || right.started_at || "") || 0;
  return leftTime !== rightTime ? leftTime - rightTime : left.id.localeCompare(right.id);
}

function compareAutopilotRuns(left: AutopilotRun, right: AutopilotRun): number {
  const leftTime = Date.parse(left.created_at || left.triggered_at || "") || 0;
  const rightTime = Date.parse(right.created_at || right.triggered_at || "") || 0;
  return leftTime !== rightTime ? leftTime - rightTime : left.id.localeCompare(right.id);
}

export function CreativeCollectionPlans({
  trackedAutopilotRunId,
  onTrackedAutopilotRunIdChange,
  onOpenMaterialLibrary,
}: {
  trackedAutopilotRunId: string;
  onTrackedAutopilotRunIdChange: (runId: string) => void;
  onOpenMaterialLibrary: (crawlRunId: string) => void;
}) {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const autopilots = useQuery(autopilotListOptions(wsId));
  const materials = useQuery({
    ...creativeMaterialLibraryOptions(wsId),
    refetchInterval: trackedAutopilotRunId ? 2500 : false,
  });
  const failedArchiveCandidates = useMemo(
    () => (materials.data?.candidates ?? []).filter((candidate) => !candidate.archived_url && candidate.archive_status === "failed"),
    [materials.data?.candidates],
  );
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
  const collectionCrawlRuns = useMemo(
    () => matches.flatMap((match) => match.crawlRun ? [match.crawlRun] : []),
    [matches],
  );
  const latestCollectionRunId = latestCollectionCrawlRunId(collectionCrawlRuns);
  const trackedCrawlRun = useMemo(
    () => crawlRunForAutopilotRun(materials.data?.crawl_runs ?? [], trackedAutopilotRunId),
    [materials.data?.crawl_runs, trackedAutopilotRunId],
  );
  const displayedCrawlRun = useMemo(() => {
    if (trackedAutopilotRunId) return trackedCrawlRun;
    return collectionCrawlRuns.find((run) => run.id === latestCollectionRunId);
  }, [collectionCrawlRuns, latestCollectionRunId, trackedAutopilotRunId, trackedCrawlRun]);
  const displayedMatches = useMemo(() => {
    const activeMatch = matches.find((match) => Boolean(match.activeAutopilotRun));
    const latestMatch = matches.find((match) => match.crawlRun?.id === latestCollectionRunId);
    const primaryMatch = activeMatch ?? latestMatch ?? matches[0];
    return primaryMatch ? [primaryMatch] : [];
  }, [latestCollectionRunId, matches]);
  const hiddenPlanCount = Math.max(matches.length - displayedMatches.length, 0);
  useEffect(() => {
    if (!trackedCrawlRun) return;
    if (!crawlRunNeedsRefresh(trackedCrawlRun) && collectionCrawlRuns.some((run) => run.id === trackedCrawlRun.id)) {
      onTrackedAutopilotRunIdChange("");
    }
  }, [collectionCrawlRuns, onTrackedAutopilotRunIdChange, trackedCrawlRun]);
  const runMaterials = useQuery({
    queryKey: [...creativeKeys.materials(wsId), "run", displayedCrawlRun?.id ?? ""],
    queryFn: () => api.listCreativeMaterialLibrary({ runId: displayedCrawlRun!.id }),
    enabled: Boolean(wsId && displayedCrawlRun?.id),
    refetchInterval: (query) => {
      const expected = Math.max(displayedCrawlRun?.imported_count ?? 0, 0);
      const loaded = newMaterialsFromCrawlRun(query.state.data?.candidates ?? []).length;
      return crawlRunNeedsRefresh(displayedCrawlRun) || expected > loaded ? 2500 : false;
    },
  });
  const trigger = useTriggerAutopilot();
  const retryArchives = useMutation({
    mutationFn: (candidateIds: string[]) => api.retryCreativeMaterialArchives(candidateIds),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      toast.success(`已重新安排 ${result.scheduled_count} 条素材归档`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法重新归档素材"),
  });
  const openAutopilots = () => navigation.push(paths.autopilots());
  const openCredentials = () => navigation.push(`${paths.settings()}?tab=integrations`);
  const runNow = async (plan: Autopilot) => {
    try {
      const run = await trigger.mutateAsync(plan.id);
      onTrackedAutopilotRunIdChange(run.id);
      toast.success(`已启动 ${plan.title}，正在读取本次运行回执`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法启动采集计划");
    }
  };

  return (
    <section className="border" aria-label="自动采集计划">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold">自动采集计划</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">由 Multica 自动化执行，只采集图片广告并归档到素材库。</p>
        </div>
        <Button size="sm" variant="outline" onClick={openAutopilots}>
          <Settings2 className="h-4 w-4" />管理自动化
        </Button>
      </div>

      {failedArchiveCandidates.length > 0 && <details className="border-b px-4 py-3">
        <summary className="cursor-pointer text-sm font-medium">归档修复 · {failedArchiveCandidates.length} 条素材需要稳定文件</summary>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground"><p>这些素材不会出现在可生产素材中。确认 OSS 网络恢复后，再重新安排归档。</p><Button size="sm" variant="outline" disabled={retryArchives.isPending} onClick={() => retryArchives.mutate(failedArchiveCandidates.map((candidate) => candidate.id))}><RefreshCw className={retryArchives.isPending ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重新安排归档</Button></div>
      </details>}

      {matches.length === 0 && !autopilots.isLoading && (
        <div className="px-4 py-5 text-sm text-muted-foreground">
          暂无采集计划。请在自动化中创建 run_only 计划。
        </div>
      )}
      <div className="divide-y">
        {displayedMatches.map(({ plan, crawlRun, lastImportedCrawlRun, activeAutopilotRun }) => (
          <CollectionPlanRow
            key={plan.id}
            plan={plan}
            crawlRun={crawlRun}
            lastImportedCrawlRun={lastImportedCrawlRun}
            activeAutopilotRun={activeAutopilotRun}
            running={trigger.isPending || Boolean(activeAutopilotRun)}
            onRunNow={() => void runNow(plan)}
            onOpenCredentials={openCredentials}
          />
        ))}
      </div>
      {hiddenPlanCount > 0 && <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-2.5 text-sm text-muted-foreground">
        <span>还有 {hiddenPlanCount} 个采集计划</span>
        <Button size="sm" variant="ghost" onClick={openAutopilots}>管理自动化<ArrowRight className="h-4 w-4" /></Button>
      </div>}
      {(trackedAutopilotRunId || displayedCrawlRun) && <CrawlRunMaterialsPanel
        run={displayedCrawlRun}
        candidates={newMaterialsFromCrawlRun(runMaterials.data?.candidates ?? [])}
        loading={runMaterials.isLoading || runMaterials.isFetching}
        starting={!displayedCrawlRun && Boolean(trackedAutopilotRunId)}
        onOpenMaterialLibrary={onOpenMaterialLibrary}
      />}
    </section>
  );
}

function CollectionPlanRow({
  plan,
  crawlRun,
  lastImportedCrawlRun,
  activeAutopilotRun,
  running,
  onRunNow,
  onOpenCredentials,
}: {
  plan: Autopilot;
  crawlRun?: CreativeMaterialCrawlRun;
  lastImportedCrawlRun?: CreativeMaterialCrawlRun;
  activeAutopilotRun?: AutopilotRun;
  running: boolean;
  onRunNow: () => void;
  onOpenCredentials: () => void;
}) {
  const status = activeAutopilotRun && !crawlRun ? "running" : crawlRun?.status || "not_started";
  const canRun = plan.status === "active";
  const diagnosis = crawlRun ? crawlRunDiagnosis(crawlRun) : null;
  return (
    <div className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">{plan.title}</span>
          <Badge variant={crawlRunStatusVariant(status)}>{crawlRunStatusLabel(status)}</Badge>
        </div>
        {crawlRun ? <>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
            <Badge variant="outline">{crawlRunImportBadgeLabel(crawlRun)}</Badge>
          </div>
          {lastImportedCrawlRun && <div className="mt-2 flex flex-wrap items-center gap-2 border-l-2 border-emerald-600 pl-2 text-xs">
            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-700" />
            <span className="font-medium">最近入库批次 {lastImportedCrawlRun.imported_count} 张素材</span>
          </div>}
        </> : <p className="mt-1 text-xs text-muted-foreground">尚未运行，可直接从这里启动首次采集</p>}
        {crawlRun?.error_message && <p className="mt-1 break-words text-xs text-destructive">{crawlRun.error_message}</p>}
        {diagnosis && <p className={diagnosis.tone === "error" ? "mt-1 break-words text-xs text-destructive" : "mt-1 break-words text-xs text-muted-foreground"}>{diagnosis.label}</p>}
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

export function newMaterialsFromCrawlRun(candidates: CreativeMaterialCandidate[]): CreativeMaterialCandidate[] {
  return candidates.filter((candidate) => candidate.is_new_in_run);
}

export function crawlRunImportBadgeLabel(run: CreativeMaterialCrawlRun): string {
  const importedCount = Math.max(run.imported_count ?? 0, 0);
  if (importedCount > 0) return `本次入库 ${importedCount} 张素材`;
  return crawlRunNeedsRefresh(run) ? "正在查找新增素材" : "本次没有新增素材";
}

export function crawlRunNeedsRefresh(run: CreativeMaterialCrawlRun | undefined): boolean {
  if (!run) return false;
  return !["completed", "partial", "failed", "action_required", "cancelled"].includes(run.status);
}

function CrawlRunMaterialsPanel({
  run,
  candidates,
  loading,
  starting,
  onOpenMaterialLibrary,
}: {
  run?: CreativeMaterialCrawlRun;
  candidates: CreativeMaterialCandidate[];
  loading: boolean;
  starting: boolean;
  onOpenMaterialLibrary: (crawlRunId: string) => void;
}) {
  const imageCandidates = candidates.filter((candidate) => candidate.asset_type === "image");
  const visibleCandidates = imageCandidates.slice(0, 6);
  const importedCount = Math.max(run?.imported_count ?? 0, 0);
  const isRefreshing = crawlRunNeedsRefresh(run);
  const isCollectingWithoutNewMaterials = Boolean(run) && candidates.length === 0 && importedCount === 0 && isRefreshing;
  const isTerminalEmptyResult = Boolean(run) && !loading && importedCount === 0 && !isRefreshing;
  const collectingWithoutNewMaterialsLabel = "本次运行仍在采集中，发现新增素材后会自动显示。";

  return (
    <section className="border-t bg-muted/10 px-4 py-3" aria-label="本次运行回执" aria-live="polite">
      {!run && (
        <div className="flex min-h-14 items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircle className="h-4 w-4 animate-spin" />{starting ? "正在连接采集浏览器，新增素材会自动显示。" : "正在读取本次运行。"}
        </div>
      )}
      {run && <>
        {loading && candidates.length === 0 && importedCount > 0 && (
          <div className="flex min-h-14 items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="h-4 w-4 animate-spin" />正在读取本次新增素材...
          </div>
        )}
        {isCollectingWithoutNewMaterials && (
          <div className="flex min-h-14 items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="h-4 w-4 animate-spin" />{collectingWithoutNewMaterialsLabel}
          </div>
        )}
        {isTerminalEmptyResult && (
          <div className="flex min-h-14 items-center gap-2 text-sm text-muted-foreground">
            <CheckCircle2 className="h-4 w-4" />本次运行没有新增素材，历史素材不会混入本次结果。
          </div>
        )}
        {!loading && importedCount > 0 && candidates.length === 0 && (
          <div className="flex min-h-14 items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="h-4 w-4 animate-spin" />本次已登记 {importedCount} 张新增素材，正在读取本次素材。
          </div>
        )}
        {!loading && candidates.length > 0 && imageCandidates.length === 0 && (
          <div className="flex min-h-14 items-center gap-2 text-sm text-muted-foreground">
            <ImageIcon className="h-4 w-4" />此历史批次没有可选图片，非图片素材已按当前规则排除。
          </div>
        )}
        {visibleCandidates.length > 0 && (
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
            <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
              {visibleCandidates.map((candidate) => <CrawlRunMaterialTile key={candidate.id} candidate={candidate} />)}
            </div>
            <div className="flex flex-wrap items-center gap-2 lg:justify-end">
              {imageCandidates.length > visibleCandidates.length && <span className="text-xs text-muted-foreground">还有 {imageCandidates.length - visibleCandidates.length} 张</span>}
              <Button size="sm" variant="outline" onClick={() => onOpenMaterialLibrary(run.id)}>
                查看本次图片
                <ArrowRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </>}
    </section>
  );
}

function CrawlRunMaterialTile({
  candidate,
}: {
  candidate: CreativeMaterialCandidate;
}) {
  const source = resolvePublicFileUrl(candidate.archived_url || candidate.poster_url || candidate.preview_url) ?? "";
  const isImage = candidate.asset_type === "image";

  return (
    <article className="min-w-0 border bg-background" data-testid="crawl-run-material-tile" data-candidate-id={candidate.id}>
      <div className="relative aspect-square bg-muted/40">
        {source ? <AuthenticatedMaterialImage source={source} alt={candidate.title || "本次新增素材"} /> : <div className="flex h-full items-center justify-center"><ImageIcon className="h-5 w-5 text-muted-foreground" /></div>}
      </div>
      <div className="border-t px-2 py-1.5">
        <p className="truncate text-xs font-medium" title={candidate.title}>{candidate.title || "未命名素材"}</p>
        <p className="flex items-center gap-1 truncate text-[10px] text-muted-foreground">
          {isImage ? <CheckCircle2 className="h-3 w-3 shrink-0 text-emerald-700" /> : <ImageIcon className="h-3 w-3 shrink-0" />}
          {candidate.competitor || candidate.connector_id || "本次采集"}
        </p>
      </div>
    </article>
  );
}

function AuthenticatedMaterialImage({ source, alt }: { source: string; alt: string }) {
  const [objectURL, setObjectURL] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    let nextObjectURL = "";
    let active = true;
    setObjectURL("");
    setFailed(false);
    const sameOrigin = new URL(source, window.location.href).origin === window.location.origin;
    const token = sameOrigin ? window.localStorage.getItem("multica_token") : "";
    const headers = token ? { Authorization: `Bearer ${token}` } : undefined;
    void fetch(source, { credentials: sameOrigin ? "include" : "omit", headers, signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`image request failed: ${response.status}`);
        return response.blob();
      })
      .then((blob) => {
        nextObjectURL = URL.createObjectURL(blob);
        if (!active) {
          URL.revokeObjectURL(nextObjectURL);
          return;
        }
        setObjectURL(nextObjectURL);
      })
      .catch((error: unknown) => {
        if (active && !(error instanceof DOMException && error.name === "AbortError")) setFailed(true);
      });
    return () => {
      active = false;
      controller.abort();
      if (nextObjectURL) URL.revokeObjectURL(nextObjectURL);
    };
  }, [source]);

  if (objectURL) return <img src={objectURL} alt={alt} width={640} height={480} className="h-full w-full object-contain" />;
  if (failed) return <div className="flex h-full flex-col items-center justify-center gap-1 text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />图片暂时无法加载</div>;
  return <div className="flex h-full items-center justify-center"><LoaderCircle className="h-5 w-5 animate-spin text-muted-foreground" /></div>;
}

function crawlRunDiagnosis(run: CreativeMaterialCrawlRun): { label: string; tone: "neutral" | "error" } | null {
  const diagnosis = run.diagnostics?.diagnosis;
  if (!diagnosis?.summary) return null;
  const sourceTotal = typeof diagnosis.matched_total === "number" ? ` · AppGrowing 查询命中 ${diagnosis.matched_total} 条` : "";
  const imported = Number.isFinite(run.imported_count) ? ` · 本轮入库 ${run.imported_count} 条` : "";
  const counts = `${sourceTotal}${imported}`;
  if (diagnosis.state === "needs_agent_diagnosis") {
    const state = run.diagnostics.agent_diagnosis_state;
    if (state === "queued") return { label: `AI 正在诊断采集异常${counts}`, tone: "error" };
    if (state === "unconfigured") return { label: `采集异常待 AI 诊断：尚未配置诊断智能体${counts}`, tone: "error" };
    return { label: `采集异常待 AI 诊断${counts}`, tone: "error" };
  }
  if (diagnosis.state === "recovered") return { label: `AI 已自动修复：${diagnosis.summary}${counts}`, tone: "neutral" };
  if (diagnosis.state === "needs_user_action") return { label: `${diagnosis.summary}${counts}`, tone: "error" };
  return counts ? { label: `采集诊断：${diagnosis.summary}${counts}`, tone: "neutral" } : null;
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

function crawlRunStatusVariant(status: string): "default" | "secondary" | "destructive" | "outline" {
  if (status === "completed") return "default";
  if (status === "failed" || status === "action_required") return "destructive";
  return "outline";
}
