"use client";

import { cloneElement, useDeferredValue, useEffect, useId, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, BookOpenText, Check, Download, ExternalLink, ImageIcon, LoaderCircle, Pencil, Plus, RefreshCw, RotateCcw, Search, Sparkles, Upload, X } from "lucide-react";
import { api } from "@multica/core/api";
import {
  creativeFeedbackOptions,
  creativeKeys,
  creativeMaterialLibraryOptions,
  creativeOrdersOptions,
  creativeResourceFilesOptions,
  creativeResourcesOptions,
  creativeSourceAnalysesOptions,
  parseCreativeCopyLibraryConfig,
  validateCustomCopyFinancialFacts,
} from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { squadListOptions } from "@multica/core/workspace/queries";
import type { CreateCreativeFeedbackRequest, CreateCreativeOrderRequest, CreativeCopySnapshot, CreativeMaterialCandidate, CreativeMaterialCrawlRun, CreativeMaterialImportResult, CreativeMaterialLibraryQuery, CreativeRepaymentPlanEntry, CreativeResource, CreativeSourceAnalysis, SquadMember, CreativeVisualDirection } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button, buttonVariants } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { candidateDecisionFeedbackInput, latestCandidateFeedback } from "../lib/creative-candidate-feedback";
import { formatCreativeDateTime } from "../lib/creative-time";
import {
  creativeMaterialMatchesFilter,
  creativeMaterialProductionState,
  creativeMaterialSelectionActionLabel,
  canRetryMaterialAnalysis,
  latestCompletedAnalyses,
  materialAnalysisState,
  type CreativeMaterialAnalysisReadiness,
  type CreativeMaterialProductionState,
  type MaterialAnalysisState,
  type MaterialLibraryFilter,
} from "../lib/creative-material-state";

export { candidateDecisionFeedbackInput, latestCandidateFeedback } from "../lib/creative-candidate-feedback";
export {
  creativeMaterialMatchesFilter,
  creativeMaterialProductionState,
  creativeMaterialSelectionActionLabel,
  effectiveMaterialCandidateStatus,
  canRetryMaterialAnalysis,
  latestAnalyses,
  latestCompletedAnalyses,
  materialAnalysisState,
  materialReferenceAnalysisBadgeLabel,
} from "../lib/creative-material-state";
export type {
  CreativeMaterialAnalysisReadiness,
  CreativeMaterialProductionState,
  MaterialAnalysisState,
  MaterialLibraryFilter,
} from "../lib/creative-material-state";

type ImportDraft = {
  mode: "file" | "url";
  sourceUrl: string;
  title: string;
  competitor: string;
  area: string;
  language: string;
  tags: string;
  note: string;
};

const EMPTY_IMPORT: ImportDraft = {
  mode: "file",
  sourceUrl: "",
  title: "",
  competitor: "",
  area: "",
  language: "",
  tags: "",
  note: "",
};

const MATERIAL_FILTER_LABELS: Record<MaterialLibraryFilter, string> = {
  available: "可用素材",
  analyze: "分析中",
  generated: "已生成",
  rejected: "已拒绝",
  all: "全部历史",
};

const MATERIAL_PAGE_SIZE = 60;

const MATERIAL_SORT_LABELS = {
  recent: "最近采集",
  impressions: "曝光估算",
  duration: "投放天数",
} as const;

export function CreativeMaterialLibrary({
  onOrderCreated,
  onOpenCopyLibrary,
  filter = "available",
  onFilterChange,
  onRunChange,
  runId = "",
}: {
  onOrderCreated?: (orderId: string) => void;
  onOpenCopyLibrary?: () => void;
  filter?: MaterialLibraryFilter;
  onFilterChange?: (filter: MaterialLibraryFilter) => void;
  onRunChange?: (runId: string) => void;
  runId?: string;
} = {}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [competitor, setCompetitor] = useState("");
  const [area, setArea] = useState("");
  const [language, setLanguage] = useState("");
  const [media, setMedia] = useState("");
  const [assetType, setAssetType] = useState<"" | "image" | "video" | "unknown">("");
  const [sort, setSort] = useState<keyof typeof MATERIAL_SORT_LABELS>("recent");
  const [offset, setOffset] = useState(0);
  const [preview, setPreview] = useState<CreativeMaterialCandidate | null>(null);
  const [directEditCandidate, setDirectEditCandidate] = useState<CreativeMaterialCandidate | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [orderDraftOpen, setOrderDraftOpen] = useState(false);
  const [rejecting, setRejecting] = useState<CreativeMaterialCandidate | null>(null);
  const [selectedCandidatesById, setSelectedCandidatesById] = useState<Record<string, CreativeMaterialCandidate>>({});
  const deferredQuery = useDeferredValue(query);
  const materialQuery = useMemo<CreativeMaterialLibraryQuery>(() => ({
    limit: MATERIAL_PAGE_SIZE,
    offset,
    query: deferredQuery.trim(),
    competitor,
    area,
    language,
    media,
    assetType: assetType || undefined,
    sort,
    view: filter,
  }), [area, assetType, competitor, deferredQuery, filter, language, media, offset, sort]);
  const scopedMaterialQuery = useMemo<CreativeMaterialLibraryQuery>(() => ({
    ...materialQuery,
    runId: runId || undefined,
  }), [materialQuery, runId]);
  const batchQuery = useMemo<CreativeMaterialLibraryQuery>(() => ({
    includeEmptyRuns: false,
    limit: 1,
    offset: 0,
  }), []);
  const materials = useQuery(creativeMaterialLibraryOptions(wsId, scopedMaterialQuery));
  const materialBatches = useQuery(creativeMaterialLibraryOptions(wsId, batchQuery));
  const analyses = useQuery({
    ...creativeSourceAnalysesOptions(wsId),
    refetchInterval: orderDraftOpen ? 2500 : false,
  });
  const feedback = useQuery(creativeFeedbackOptions(wsId, "candidate"));
  const orders = useQuery(creativeOrdersOptions(wsId));
  const resources = useQuery(creativeResourcesOptions(wsId));
  const candidates = useMemo(() => materials.data?.candidates ?? [], [materials.data?.candidates]);
  const { marketPack, copyLibrary } = useMemo(
    () => defaultPreAdaptationResources(resources.data?.resources ?? []),
    [resources.data?.resources],
  );
  const completedAnalyses = useMemo(() => latestCompletedAnalyses(analyses.data?.analyses ?? []), [analyses.data?.analyses]);
  const latestDecisions = useMemo(() => latestCandidateFeedback(feedback.data?.events ?? []), [feedback.data?.events]);
  const generatedCandidateIds = useMemo(
    () => new Set((orders.data?.orders ?? []).flatMap((order) => order.items.map((item) => item.candidate_id))),
    [orders.data?.orders],
  );
  const displayedCandidates = candidates;
  const analysisStateByCandidateId = useMemo(
    () => new Map(displayedCandidates.map((candidate) => [candidate.id, materialAnalysisState(candidate, analyses.data?.analyses ?? [])])),
    [analyses.data?.analyses, displayedCandidates],
  );
  const productionStateByCandidateId = useMemo(
    () => new Map(displayedCandidates.map((candidate) => {
      const analysisState = analysisStateByCandidateId.get(candidate.id) ?? materialAnalysisState(candidate, analyses.data?.analyses ?? []);
      const analysis = completedAnalyses.get(candidate.id);
      const analysisReadiness = creativeMaterialAnalysisReadiness(analysis, analysisState, marketPack, copyLibrary);
      const decision = latestDecisions.get(candidate.id)?.decision;
      return [candidate.id, creativeMaterialProductionState(candidate, analysisState, decision, generatedCandidateIds.has(candidate.id), analysisReadiness)];
    })),
    [analyses.data?.analyses, analysisStateByCandidateId, completedAnalyses, copyLibrary, displayedCandidates, generatedCandidateIds, latestDecisions, marketPack],
  );
  const selectedCandidates = useMemo(
    () => Object.values(selectedCandidatesById),
    [selectedCandidatesById],
  );
  useEffect(() => {
    if (selectedCandidates.length === 0) setOrderDraftOpen(false);
  }, [selectedCandidates.length]);
  const retryAnalysis = useMutation({
    mutationFn: (candidateId: string) => api.retryCreativeMaterialReferenceAnalysis(candidateId),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.analyses(wsId) });
      const notice = materialImportNotice(result);
      const message = notice.message.replace("素材已导入，", "");
      if (notice.level === "warning") {
        toast.warning(message);
      } else {
        toast.success(message);
      }
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法重新排队参考分析"),
  });
  const decideCandidate = useMutation({
    mutationFn: ({ candidate, reasonCodes, comment, idempotencyKey }: { candidate: CreativeMaterialCandidate; reasonCodes?: string[]; comment?: string; idempotencyKey?: string }) => api.createCreativeFeedback(
      candidateDecisionFeedbackInput(candidate, "rejected", reasonCodes, comment, idempotencyKey),
    ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "candidate", "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法记录素材决定"),
  });
  const undoDecision = useMutation({
    mutationFn: async (candidate: CreativeMaterialCandidate) => {
      const feedback = await api.listCreativeFeedback("candidate", candidate.id);
      const decision = latestCandidateFeedback(feedback.events).get(candidate.id);
      if (decision?.decision !== "selected" && decision?.decision !== "rejected") {
        if (candidate.source_issue_id && candidate.status === "selected") {
          return api.updateCreativeMaterialCandidate(candidate.source_issue_id, candidate.id, {
            status: "new",
            note: candidate.note,
          });
        }
        throw new Error("未找到可撤销的素材选择记录");
      }
      return api.undoCreativeFeedback(decision.id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "candidate", "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法撤销素材决定"),
  });
  const competitors = useMemo(
    () => materialFacetValues(candidates, (candidate) => candidate.competitor),
    [candidates],
  );
  const areas = useMemo(() => materialFacetValues(candidates, (candidate) => candidate.area_names), [candidates]);
  const languages = useMemo(() => materialFacetValues(candidates, (candidate) => candidate.language_names), [candidates]);
  const mediaNames = useMemo(() => materialFacetValues(candidates, (candidate) => candidate.media_names), [candidates]);
  const scopedCandidates = useMemo(() => {
    return displayedCandidates.filter((candidate) => {
      const state = productionStateByCandidateId.get(candidate.id);
      return state ? creativeMaterialMatchesFilter(state, filter) : filter === "all";
    });
  }, [displayedCandidates, filter, productionStateByCandidateId]);
  const visible = useMemo(() => {
    const needle = deferredQuery.trim().toLocaleLowerCase();
    return scopedCandidates.filter((item) => {
      if (competitor && item.competitor !== competitor) return false;
      if (!needle) return true;
      return [item.title, item.competitor, item.connector_id, ...item.tags]
        .join(" ")
        .toLocaleLowerCase()
        .includes(needle);
    });
  }, [competitor, deferredQuery, scopedCandidates]);
  const previewCandidate = displayedCandidates.find((item) => item.id === preview?.id) ?? preview;
  const previewAnalysis = previewCandidate ? completedAnalyses.get(previewCandidate.id) : undefined;
  const previewCanRetryAnalysis = previewCandidate
    ? canRetryMaterialAnalysis(previewCandidate, productionStateByCandidateId.get(previewCandidate.id) ?? { status: "generated" })
    : false;
  const totalCount = materials.data?.total_count || scopedCandidates.length;
  const nextOffset = materials.data?.next_offset ?? null;
  const crawlRuns = useMemo(
    () => [...(materialBatches.data?.crawl_runs ?? [])].sort(compareMaterialCrawlRuns),
    [materialBatches.data?.crawl_runs],
  );
  const activeCrawlRun = crawlRuns.find((run) => run.id === runId);
  const toggleSelection = (candidate: CreativeMaterialCandidate) => {
    setSelectedCandidatesById((current) => {
      if (current[candidate.id]) {
        const { [candidate.id]: _removed, ...remaining } = current;
        return remaining;
      }
      return { ...current, [candidate.id]: candidate };
    });
  };
  const clearSelection = () => setSelectedCandidatesById({});
  const selectRun = (nextRunId: string) => {
    setOffset(0);
    onRunChange?.(nextRunId);
  };

  return <div className="mx-auto w-full min-w-0 max-w-[1440px]">
    <div className="mb-4 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h2 className="text-base font-semibold">素材库</h2>
        <p className="mt-1 text-sm text-muted-foreground">从已分析素材中选出需要出图的一批，再统一确认文案。</p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="outline">{totalCount} 条</Badge>
        {selectedCandidates.length > 0 && <Badge>{selectedCandidates.length} 已选</Badge>}
        <Button size="sm" onClick={() => setImportOpen(true)}><Plus className="h-4 w-4" />导入素材</Button>
      </div>
    </div>
    <div className="grid min-w-0 gap-4 lg:grid-cols-[260px_minmax(0,1fr)]">
      <MaterialBatchSidebar runs={crawlRuns} activeRunId={runId} onSelect={selectRun} />
      <div className="min-w-0">
        <div className="mb-4 flex flex-wrap gap-2 border-y bg-muted/10 py-3">
          <div className="relative min-w-56 flex-1 md:max-w-md"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input className="pl-8" value={query} onChange={(event) => { setQuery(event.target.value); setOffset(0); }} placeholder="搜索标题、竞品或标签" /></div>
          <select id="creative-material-status-filter" name="status" className="h-9 min-w-36 border bg-background px-3 text-sm" value={filter} onChange={(event) => { setOffset(0); onFilterChange?.(event.target.value as MaterialLibraryFilter); }} aria-label="按素材状态筛选">
            {(Object.keys(MATERIAL_FILTER_LABELS) as MaterialLibraryFilter[]).map((key) => <option key={key} value={key}>{runId && key === "all" ? "本次全部" : MATERIAL_FILTER_LABELS[key]}</option>)}
          </select>
          <select id="creative-material-competitor-filter" name="competitor" className="h-9 min-w-40 border bg-background px-3 text-sm" value={competitor} onChange={(event) => { setCompetitor(event.target.value); setOffset(0); }} aria-label="按竞品筛选">
            <option value="">全部竞品</option>
            {competitors.map((name) => <option key={name} value={name}>{name}</option>)}
          </select>
          <select className="h-9 min-w-32 border bg-background px-3 text-sm" value={area} onChange={(event) => { setArea(event.target.value); setOffset(0); }} aria-label="按市场筛选"><option value="">全部市场</option>{areas.map((name) => <option key={name} value={name}>{name}</option>)}</select>
          <select className="h-9 min-w-32 border bg-background px-3 text-sm" value={language} onChange={(event) => { setLanguage(event.target.value); setOffset(0); }} aria-label="按语言筛选"><option value="">全部语言</option>{languages.map((name) => <option key={name} value={name}>{name}</option>)}</select>
          <select className="h-9 min-w-32 border bg-background px-3 text-sm" value={media} onChange={(event) => { setMedia(event.target.value); setOffset(0); }} aria-label="按渠道筛选"><option value="">全部渠道</option>{mediaNames.map((name) => <option key={name} value={name}>{name}</option>)}</select>
          <select className="h-9 min-w-28 border bg-background px-3 text-sm" value={assetType} onChange={(event) => { setAssetType(event.target.value as typeof assetType); setOffset(0); }} aria-label="按素材类型筛选"><option value="">全部类型</option><option value="image">图片</option><option value="video">视频</option><option value="unknown">未知</option></select>
          <select className="h-9 min-w-32 border bg-background px-3 text-sm" value={sort} onChange={(event) => { setSort(event.target.value as keyof typeof MATERIAL_SORT_LABELS); setOffset(0); }} aria-label="排序方式">{Object.entries(MATERIAL_SORT_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>
          {runId && <Badge variant="outline" className="h-9 px-3 text-sm">{activeCrawlRun ? materialBatchShortTitle(activeCrawlRun) : "本次采集"}</Badge>}
          {runId && <Button size="sm" variant="ghost" onClick={() => selectRun("")}>全部批次</Button>}
        </div>
        {selectedCandidates.length > 0 && <MaterialBatchSelectionBar candidates={selectedCandidates} onClear={clearSelection} onConfirm={() => setOrderDraftOpen(true)} />}
        {orderDraftOpen && <Dialog open onOpenChange={(open) => !open && setOrderDraftOpen(false)}><DialogContent className="!h-[100dvh] !w-[100vw] !max-w-none !gap-0 !overflow-y-auto !rounded-none !p-0"><DialogHeader className="sr-only"><DialogTitle>批量确认文案</DialogTitle><DialogDescription>逐图确认文案后，统一提交出图。</DialogDescription></DialogHeader><CreativeOrderDraft candidates={selectedCandidates} analyses={analyses.data?.analyses ?? []} deselecting={false} onOpenCopyLibrary={onOpenCopyLibrary} onRetrySourceAnalysis={(candidateId) => retryAnalysis.mutate(candidateId)} retryingSourceAnalysis={retryAnalysis.isPending} onDeselect={(candidateId) => setSelectedCandidatesById((current) => {
          const { [candidateId]: _removed, ...remaining } = current;
          return remaining;
        })} onClose={() => setOrderDraftOpen(false)} onDone={(orderId) => {
          clearSelection();
          setOrderDraftOpen(false);
          queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
          onOrderCreated?.(orderId);
        }} /></DialogContent></Dialog>}
        {materials.isLoading ? <div className="py-16 text-center text-sm text-muted-foreground">正在加载素材库...</div> : visible.length === 0 ? (
          <div className="flex min-h-64 flex-col items-center justify-center border border-dashed text-sm text-muted-foreground"><ImageIcon className="mb-3 h-6 w-6" /><p>没有符合条件的素材</p><div className="mt-4 flex gap-2"><Button size="sm" variant="outline" onClick={() => { setQuery(""); setCompetitor(""); setArea(""); setLanguage(""); setMedia(""); setAssetType(""); setSort("recent"); setOffset(0); onFilterChange?.(runId ? "all" : "available"); }}>清除筛选</Button><Button size="sm" variant="outline" onClick={() => setImportOpen(true)}>导入素材</Button></div></div>
        ) : (
          <div className="grid grid-cols-1 gap-px overflow-hidden border bg-border sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            {visible.map((candidate) => {
              const decision = latestDecisions.get(candidate.id);
              const selected = Boolean(selectedCandidatesById[candidate.id]);
              const analysisState = analysisStateByCandidateId.get(candidate.id) ?? materialAnalysisState(candidate, analyses.data?.analyses ?? []);
              const productionState = productionStateByCandidateId.get(candidate.id) ?? creativeMaterialProductionState(
                candidate,
                analysisState,
                decision?.decision,
                generatedCandidateIds.has(candidate.id),
                creativeMaterialAnalysisReadiness(completedAnalyses.get(candidate.id), analysisState, marketPack, copyLibrary),
              );
              return <MaterialTile
                key={candidate.id}
                candidate={candidate}
                analysisState={analysisState}
                productionState={productionState}
                selected={selected}
                decision={decision?.decision ?? ""}
                busy={decideCandidate.isPending || undoDecision.isPending}
                retryingAnalysis={retryAnalysis.isPending}
                analysisSummary={completedAnalyses.get(candidate.id)?.summary ?? ""}
                showProductionState={filter !== "available" || productionState.status !== "available"}
                onToggle={() => {
                  toggleSelection(candidate);
                }}
                onReject={() => setRejecting(candidate)}
                onUndo={() => undoDecision.mutate(candidate)}
                onOpen={() => setPreview(candidate)}
                onRetryAnalysis={() => retryAnalysis.mutate(candidate.id)}
              />;
            })}
          </div>
        )}

        {visible.length > 0 && totalCount > MATERIAL_PAGE_SIZE && <div className="mt-4 flex items-center justify-between border-y py-3 text-sm">
          <span className="text-muted-foreground">显示 {offset + 1}-{Math.min(offset + visible.length, totalCount)} / {totalCount} 条</span>
          <div className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={offset === 0 || materials.isFetching} onClick={() => setOffset(Math.max(0, offset - MATERIAL_PAGE_SIZE))}>上一页</Button><Button size="sm" variant="outline" disabled={nextOffset === null || materials.isFetching} onClick={() => nextOffset !== null && setOffset(nextOffset)}>下一页</Button></div>
        </div>}
      </div>
    </div>
		<MaterialPreview candidate={previewCandidate} analysis={previewAnalysis} canRetryAnalysis={previewCanRetryAnalysis} onClose={() => setPreview(null)} onDirectEdit={() => previewCandidate && setDirectEditCandidate(previewCandidate)} onRetryAnalysis={(candidateId) => retryAnalysis.mutate(candidateId)} retryingAnalysis={retryAnalysis.isPending} />
		<DirectEditDialog candidate={directEditCandidate} onClose={() => setDirectEditCandidate(null)} onCreated={(orderId) => {
			setDirectEditCandidate(null);
			queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
			onOrderCreated?.(orderId);
		}} />
    <CandidateRejectDialog candidate={rejecting} busy={decideCandidate.isPending} onClose={() => setRejecting(null)} onConfirm={(reasonCode, comment, idempotencyKey) => {
      if (!rejecting) return;
      decideCandidate.mutate({ candidate: rejecting, reasonCodes: [reasonCode], comment, idempotencyKey }, { onSuccess: () => setRejecting(null) });
    }} />
    <MaterialImportDialog open={importOpen} onClose={() => setImportOpen(false)} onImported={() => {
      setImportOpen(false);
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.analyses(wsId) });
    }} />
  </div>;
}

export type MaterialCandidateDisplayDetails = {
  duration: string;
  impressions: string;
  media: string;
  market: string;
  languages: string;
  platforms: string;
  tags: string;
  note: string;
};

const MATERIAL_SOURCE_MISSING = "源数据未提供";

export function materialCandidateDisplayDetails(candidate: CreativeMaterialCandidate): MaterialCandidateDisplayDetails {
  const records = candidateSourceRecords(candidate);
  const durationDays = candidate.duration_days ?? firstRecordNumber(records, ["duration_days", "duration"]);
  const impressionEstimate = candidate.impression_estimate ?? firstRecordNumber(records, ["impression_estimate", "impressions", "impression"]);
  const mediaNames = preferredCandidateValues(candidate.media_names, records, ["media_names", "media", "media_sources"]);
  const platformNames = preferredCandidateValues(candidate.platform_names, records, ["platform_names", "platforms", "platform"]);

  return {
    duration: durationDays === null ? MATERIAL_SOURCE_MISSING : `${formatDecimal(durationDays)} 天`,
    impressions: impressionEstimate === null ? MATERIAL_SOURCE_MISSING : formatCompactCount(impressionEstimate),
    media: mediaNames.length > 0 ? mediaNames.join("、") : MATERIAL_SOURCE_MISSING,
    market: displayCandidateValues(preferredCandidateValues(candidate.area_names, records, ["area_names", "areas", "area", "market"])),
    languages: displayCandidateValues(preferredCandidateValues(candidate.language_names, records, ["language_names", "languages", "language", "locale"])),
    platforms: displayCandidateValues(platformNames),
    tags: displayCandidateValues(preferredCandidateValues(candidate.tags, records, ["tags", "tag_names"])),
    note: candidate.note?.trim() || firstRecordString(records, ["note", "remark", "remarks", "comment"]) || MATERIAL_SOURCE_MISSING,
  };
}

export function MaterialTile({ candidate, analysisState, productionState, selected, decision, busy, retryingAnalysis, analysisSummary, showProductionState = true, onToggle, onReject, onUndo, onOpen, onRetryAnalysis }: { candidate: CreativeMaterialCandidate; analysisState: MaterialAnalysisState; productionState: CreativeMaterialProductionState; selected: boolean; decision: string; busy: boolean; retryingAnalysis: boolean; analysisSummary?: string; showProductionState?: boolean; onToggle: () => void; onReject: () => void; onUndo: () => void; onOpen: () => void; onRetryAnalysis: () => void }) {
  const source = candidateSource(candidate);
  const details = materialCandidateDisplayDetails(candidate);
  const isImage = candidate.asset_type === "image";
  const canRetryAnalysis = canRetryMaterialAnalysis(candidate, productionState);
  const selectionActionLabel = creativeMaterialSelectionActionLabel(selected, productionState);
  const analysisActionLabel = "重新分析";
  const statusReason = productionState.status === "failed" || productionState.status === "manual_required" || productionState.status === "unsupported" ? productionState.reason || (analysisState.status === "failed" ? analysisState.error : "") : "";
  const statusBadgeVariant = productionState.status === "failed" || productionState.status === "rejected"
    ? "destructive"
    : productionState.status === "available"
      ? "default"
      : "outline";
  return <div className={selected ? "group min-w-0 overflow-hidden border border-emerald-600 bg-emerald-50/30 text-left ring-1 ring-emerald-600/20 transition-colors hover:bg-emerald-50/50 focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2" : "group min-w-0 overflow-hidden border bg-background text-left transition-colors hover:bg-muted/30 focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2"} data-testid="creative-material-tile" data-candidate-id={candidate.id}><button type="button" onClick={onOpen} className="w-full focus-visible:outline-none">
    <div className="relative aspect-[4/3] bg-muted/40">
      {isImage && source ? <MaterialPreviewImage source={source} alt={candidate.title || candidate.competitor} /> : <div className="flex h-full flex-col items-center justify-center gap-2 px-4 text-center text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />{isImage ? candidate.archive_status === "failed" ? "归档失败" : "正在归档" : "非图片素材"}</div>}
      {candidate.archive_status === "failed" && <Badge variant="destructive" className="absolute left-2 top-2"><AlertTriangle className="h-3 w-3" />归档失败</Badge>}
      {(candidate.archive_status === "pending" || candidate.archive_status === "running") && <Badge variant="secondary" className="absolute left-2 top-2 bg-background/90"><LoaderCircle className="h-3 w-3 animate-spin" />归档中</Badge>}
      {decision === "rejected" && <Badge variant="destructive" className="absolute bottom-2 right-2">已拒绝</Badge>}
      {selected && <Badge className="absolute right-2 top-2 border border-emerald-700 bg-emerald-700 text-white"><Check className="h-3 w-3" />已选</Badge>}
    </div>
    <div className="space-y-1 border-t px-3 py-2.5">
      <p className="truncate text-sm font-medium">{candidate.title || "未命名素材"}</p>
      {showProductionState && <Badge variant={statusBadgeVariant} className="w-fit text-[10px]"><Sparkles className="h-3 w-3" />{productionState.label}</Badge>}
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span className="truncate">{candidate.competitor || sourceLabel(candidate.connector_id)}</span><span className="shrink-0 truncate" title={details.media}>{details.media}</span></div>
      <dl className="grid grid-cols-3 gap-2 border-t pt-2 text-left text-[10px]">
        <MaterialMetric label="投放天数" value={details.duration} />
        <MaterialMetric label="曝光估算" value={details.impressions} />
        <MaterialMetric label="媒体来源" value={details.media} />
      </dl>
      {analysisSummary && <p className="line-clamp-2 border-t pt-2 text-xs text-foreground" title={analysisSummary}>{analysisSummary}</p>}
      <p className="truncate text-[11px] text-muted-foreground" title={`${details.market} · ${details.languages}`}>{details.market} · {details.languages}</p>
      <p className="font-mono text-[10px] text-muted-foreground">素材 ID {candidate.id.slice(0, 8)}</p>
      {statusReason && <p className={`truncate text-[11px] ${productionState.status === "manual_required" ? "text-amber-700" : "text-destructive"}`} title={statusReason}>{statusReason}</p>}
    </div>
  </button><div className="flex items-center justify-between gap-2 border-t px-3 py-2">
    <Button size="sm" variant={selected ? "secondary" : "outline"} disabled={busy || decision === "rejected" || (!selected && !productionState.selectable)} onClick={onToggle}>{selected ? <Check className="h-4 w-4" /> : <Plus className="h-4 w-4" />}{selectionActionLabel}</Button>
    <div className="flex items-center gap-1">
      {canRetryAnalysis && <Button aria-label={`${analysisActionLabel}素材`} title={`${analysisActionLabel}素材`} size="icon-sm" variant="ghost" disabled={busy || retryingAnalysis} onClick={onRetryAnalysis}><RefreshCw className={`h-4 w-4 ${retryingAnalysis ? "animate-spin" : ""}`} /></Button>}
      <Button aria-label={decision === "rejected" ? "撤销拒绝" : "拒绝素材"} title={decision === "rejected" ? "撤销拒绝" : "拒绝素材"} size="icon-sm" variant="ghost" disabled={busy} onClick={decision === "rejected" ? onUndo : onReject}>{decision === "rejected" ? <RotateCcw className="h-4 w-4" /> : <X className="h-4 w-4" />}</Button>
    </div>
  </div></div>;
}

function MaterialPreviewImage({ source, alt }: { source: string; alt: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [source]);
  if (failed) return <div className="flex h-full flex-col items-center justify-center gap-2 px-4 text-center text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />图片暂时无法加载</div>;
  return <img src={source} alt={alt} width={640} height={480} loading="lazy" className="h-full w-full object-contain" onError={() => setFailed(true)} />;
}

export function MaterialBatchSidebar({
  runs,
  activeRunId,
  onSelect,
}: {
  runs: CreativeMaterialCrawlRun[];
  activeRunId: string;
  onSelect: (runId: string) => void;
}) {
  return <aside className="min-w-0 border bg-muted/10 lg:sticky lg:top-4 lg:max-h-[calc(100vh-9rem)] lg:overflow-y-auto" aria-label="采集批次">
    <div className="border-b bg-background px-3 py-2.5">
      <p className="text-sm font-semibold">采集批次</p>
      <p className="mt-0.5 text-xs text-muted-foreground">选择批次后，右侧继续用状态和搜索筛选。</p>
    </div>
    <div className="max-h-80 overflow-y-auto lg:max-h-none">
      <MaterialBatchButton
        active={!activeRunId}
        title="全部素材"
        description="不按采集批次过滤"
        meta=""
        onClick={() => onSelect("")}
      />
      {runs.map((run) => (
        <MaterialBatchButton
          key={run.id}
          active={run.id === activeRunId}
          title={materialBatchShortTitle(run)}
          description={materialBatchDescription(run)}
          meta={materialBatchMeta(run)}
          onClick={() => onSelect(run.id)}
        />
      ))}
      {runs.length === 0 && <div className="px-3 py-5 text-sm text-muted-foreground">还没有采集批次。</div>}
    </div>
  </aside>;
}

function MaterialBatchButton({
  active,
  title,
  description,
  meta,
  onClick,
}: {
  active: boolean;
  title: string;
  description: string;
  meta: string;
  onClick: () => void;
}) {
  return <button
    type="button"
    aria-pressed={active}
    className={active
      ? "block w-full border-b bg-background px-3 py-3 text-left shadow-[inset_2px_0_0_hsl(var(--primary))]"
      : "block w-full border-b px-3 py-3 text-left hover:bg-background/70"}
    onClick={onClick}
  >
    <div className="flex min-w-0 items-center justify-between gap-2">
      <span className="truncate text-sm font-medium">{title}</span>
      {active && <Badge variant="outline" className="shrink-0 text-[10px]">当前</Badge>}
    </div>
    <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{description}</p>
    {meta && <p className="mt-2 text-[11px] text-muted-foreground">{meta}</p>}
  </button>;
}

function compareMaterialCrawlRuns(left: CreativeMaterialCrawlRun, right: CreativeMaterialCrawlRun): number {
  return materialBatchTimestamp(right) - materialBatchTimestamp(left) || right.id.localeCompare(left.id);
}

function materialBatchTimestamp(run: CreativeMaterialCrawlRun): number {
  const parsed = new Date(run.created_at || run.started_at || run.finished_at || "").getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}

function materialBatchShortTitle(run: CreativeMaterialCrawlRun): string {
  const query = run.query_summary?.trim();
  if (query) return query.length > 18 ? `${query.slice(0, 18)}...` : query;
  const time = run.created_at || run.started_at ? formatCreativeDateTime(run.created_at || run.started_at) : "";
  return time ? `${sourceLabel(run.connector_id)} ${time}` : sourceLabel(run.connector_id);
}

function materialBatchDescription(run: CreativeMaterialCrawlRun): string {
  const source = sourceLabel(run.connector_id);
  const time = run.created_at || run.started_at ? formatCreativeDateTime(run.created_at || run.started_at) : "时间未知";
  return `${source} · ${time}`;
}

function materialBatchMeta(run: CreativeMaterialCrawlRun): string {
  const imported = Math.max(run.imported_count ?? 0, 0);
  const total = Math.max(run.total_count ?? 0, 0);
  const selected = Math.max(run.candidate_metrics?.selected ?? 0, 0);
  const base = total > 0 && total !== imported ? `命中 ${total} 张 · 入库 ${imported} 张` : `入库 ${imported} 张`;
  return selected > 0 ? `${base} · 已选 ${selected} 张` : base;
}

export function MaterialBatchSelectionBar({
  candidates,
  onClear,
  onConfirm,
}: {
  candidates: CreativeMaterialCandidate[];
  onClear: () => void;
  onConfirm: () => void;
}) {
  return <section className="sticky bottom-4 z-20 mb-4 border bg-background shadow-lg" aria-label="已选出图素材">
    <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
      <div className="min-w-0"><p className="text-sm font-semibold">已选 {candidates.length} 张素材</p><p className="mt-1 text-xs text-muted-foreground">已在素材卡片中标记，可继续筛选和比较。</p></div>
      <div className="flex items-center gap-2"><Button size="sm" variant="ghost" onClick={onClear}>清空选择</Button><Button size="sm" onClick={onConfirm}><Sparkles className="h-4 w-4" />下一步：确认文案</Button></div>
    </div>
  </section>;
}

export type OrderItemDraft = {
  mode: "pre_adaptation" | "manual";
  visualDirection: CreativeVisualDirection;
  textOverrides: Record<string, string>;
  replacementSources?: Record<string, ReplacementSourceChoice>;
  repaymentPlanOverrides?: Record<string, RepaymentPlanChoice>;
  numericLayoutDrafts?: Record<string, NumericLayoutDraft>;
  manualHeadline: string;
  manualSubheadline: string;
  manualBenefit: string;
  manualSupporting: string;
  manualCta: string;
};

export type RepaymentPlanChoice = {
  planKey: string;
  principal: number;
  tenorMonths: number;
  values: PreparedRepaymentPlanSelection["values"];
};

const EMPTY_REPAYMENT_PLAN_VALUES: RepaymentPlanChoice["values"] = {
  principal: "",
  tenor: "",
  totalInterest: "",
  totalRepayment: "",
  monthlyInstallment: "",
};

export type NumericLayoutDraft = {
  removedScenarioIds?: string[];
  addedScenarios?: Record<string, RepaymentPlanChoice>;
};

export type ReplacementSourceChoice = {
  kind: "library" | "manual" | "recommendation" | "calculation";
  sourceKey?: string;
};

export type SelectedMaterialReadinessStatus = "ready" | "analyzing" | "failed" | "manual_required" | "unavailable";

const EMPTY_VISUAL_DIRECTION: CreativeVisualDirection = {
  schema_version: 1,
  theme: "",
  style_tags: [],
  must_preserve: [],
  avoid: [],
};

type SubmissionRecovery = { issueId: string; orderId: string; submissionKey: string };

const EMPTY_SUBMISSION_RECOVERY: SubmissionRecovery = { issueId: "", orderId: "", submissionKey: "" };
const MATERIAL_ANALYSIS_RUNNING_MESSAGE = "素材仍在分析中，完成后会自动刷新。";
const MATERIAL_ANALYSIS_RUNNING_DETAIL = "无需逐张处理；当前选择会保留。";

export function orderDraftWithPreAdaptation(current: OrderItemDraft | undefined, adaptation: PreparedPreAdaptation | null = null): OrderItemDraft {
  const visualDirection = adaptation ? visualDirectionFromAnalysis(adaptation.analysisResult) : EMPTY_VISUAL_DIRECTION;
  if (current) {
    const normalized = {
      ...current,
      visualDirection: current.visualDirection ?? visualDirection,
      replacementSources: current.replacementSources ?? {},
      repaymentPlanOverrides: current.repaymentPlanOverrides ?? {},
      numericLayoutDrafts: current.numericLayoutDrafts ?? {},
    };
    return current.visualDirection === undefined || current.replacementSources === undefined || current.repaymentPlanOverrides === undefined || current.numericLayoutDrafts === undefined
      ? normalized
      : normalized;
  }
  return { mode: "pre_adaptation", visualDirection, textOverrides: {}, replacementSources: {}, repaymentPlanOverrides: {}, numericLayoutDrafts: {}, manualHeadline: "", manualSubheadline: "", manualBenefit: "", manualSupporting: "", manualCta: "" };
}

export function recoveryForSubmissionKey(recovery: SubmissionRecovery, submissionKey: string): SubmissionRecovery {
  return recovery.submissionKey === submissionKey ? recovery : EMPTY_SUBMISSION_RECOVERY;
}

export function visualDirectionFromAnalysis(result: Record<string, unknown>): CreativeVisualDirection {
  const theme = recordString(result, "theme");
  const visualType = recordString(result, "visual_type");
  const styleTags = uniqueDirectionValues([
    visualType,
    ...recordStringArray(result, "palette_anchors").slice(0, 3),
  ]);
  const mustPreserve = uniqueDirectionValues([
    ...recordStringArray(result, "must_preserve").slice(0, 4),
    ...recordStringArray(result, "visual_anchors").slice(0, 3),
  ]);
  const avoid = uniqueDirectionValues([
    ...recordStringArray(result, "avoid").slice(0, 3),
    ...recordStringArray(result, "forbidden_elements").slice(0, 3),
  ]);
  return { schema_version: 1, theme, style_tags: styleTags, must_preserve: mustPreserve, avoid };
}

function uniqueDirectionValues(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))];
}

export function defaultPreAdaptationResources(resources: CreativeResource[]): { marketPack?: CreativeResource; copyLibrary?: CreativeResource } {
  const marketPacks = resources.filter((resource) => (
    resource.kind === "market_pack"
    && resource.published_version > 0
    && record(resource.published_config).pre_adaptation_default === true
  ));
  const marketPack = marketPacks.length === 1 ? marketPacks[0] : undefined;
  const copyLibraryId = recordString(record(marketPack?.published_config), "copy_library_id");
  const copyLibrary = resources.find((resource) => (
    resource.id === copyLibraryId
    && resource.kind === "copy_library"
    && resource.published_version > 0
  ));
  return { marketPack, copyLibrary };
}

export function approvedRepaymentPlanOptions(copyLibrary: CreativeResource | undefined): RepaymentPlanChoice[] {
  if (!copyLibrary) return [];
  const config = parseCreativeCopyLibraryConfig(copyLibrary.published_config ?? {});
  return config.repayment_plan.entries
    .filter(approvedRepaymentPlanEntry)
    .sort((left, right) => left.principal - right.principal || left.tenor_months - right.tenor_months || left.key.localeCompare(right.key))
    .map(repaymentPlanChoiceFromEntry);
}

function approvedRepaymentPlanEntry(entry: CreativeRepaymentPlanEntry): boolean {
  return entry.status === "approved"
    && entry.key.trim() !== ""
    && entry.source.trim() !== ""
    && Number.isInteger(entry.principal) && entry.principal > 0
    && Number.isInteger(entry.tenor_months) && entry.tenor_months > 0
    && Number.isInteger(entry.monthly_installment) && entry.monthly_installment > 0
    && Number.isInteger(entry.total_interest) && entry.total_interest >= 0
    && Number.isInteger(entry.total_repayment) && entry.total_repayment > 0;
}

function repaymentPlanChoiceFromEntry(entry: CreativeRepaymentPlanEntry): RepaymentPlanChoice {
  return {
    planKey: entry.key,
    principal: entry.principal,
    tenorMonths: entry.tenor_months,
    values: {
      principal: formatIDR(entry.principal),
      tenor: `${entry.tenor_months} Bulan`,
      totalInterest: formatIDR(entry.total_interest),
      totalRepayment: formatIDR(entry.total_repayment),
      monthlyInstallment: formatIDR(entry.monthly_installment),
    },
  };
}

function repaymentPlanChoiceFromSelection(selection: PreparedRepaymentPlanSelection): RepaymentPlanChoice {
  return {
    planKey: selection.planKey,
    principal: selection.principal,
    tenorMonths: selection.tenorMonths,
    values: selection.values,
  };
}

export function manualRepaymentPlanChoice(layoutId: string, values: Partial<RepaymentPlanChoice["values"]>, existingPlanKey = ""): RepaymentPlanChoice {
  const normalizedValues: RepaymentPlanChoice["values"] = {
    principal: values.principal?.trim() ?? "",
    tenor: values.tenor?.trim() ?? "",
    totalInterest: values.totalInterest?.trim() ?? "",
    totalRepayment: values.totalRepayment?.trim() ?? "",
    monthlyInstallment: values.monthlyInstallment?.trim() ?? "",
  };
  return {
    planKey: existingPlanKey.startsWith("manual-") ? existingPlanKey : manualRepaymentPlanKey(layoutId, normalizedValues),
    principal: parsePrincipalAmount(normalizedValues.principal),
    tenorMonths: parseTenorMonths(normalizedValues.tenor),
    values: normalizedValues,
  };
}

function manualRepaymentPlanKey(layoutId: string, values: RepaymentPlanChoice["values"]): string {
  const fingerprint = [
    values.principal,
    values.tenor,
    values.monthlyInstallment,
    values.totalInterest,
    values.totalRepayment,
  ].join("-").toLocaleLowerCase();
  const slug = `${layoutId}-${fingerprint}`
    .replace(/rp\s*/gi, "rp")
    .replace(/[^a-z0-9]+/gi, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 96);
  return `manual-${slug || "repayment-plan"}`;
}

function parsePrincipalAmount(value: string): number {
  const text = value.trim().toLocaleLowerCase();
  if (!text) return 0;
  const numeric = text.match(/\d+(?:[.,]\d+)?/)?.[0]?.replace(",", ".");
  if (!numeric) return 0;
  const multiplier = text.includes("miliar") ? 1_000_000_000 : text.includes("juta") ? 1_000_000 : text.includes("ribu") ? 1_000 : 1;
  if (multiplier > 1) return Math.round(Number(numeric) * multiplier);
  const digits = text.replace(/\D/g, "");
  return digits ? Number(digits) : 0;
}

function parseTenorMonths(value: string): number {
  const text = value.trim().toLocaleLowerCase();
  const numeric = Number(text.match(/\d+/)?.[0] ?? 0);
  if (!Number.isFinite(numeric) || numeric <= 0) return 0;
  if (/\btahun\b/.test(text)) return numeric * 12;
  return numeric;
}

function nextAddedRepaymentPlanScenarioId(layoutId: string, planKey: string, existingIds: ReadonlySet<string>): string {
  const base = `added-${layoutId}-${planKey}`.replace(/[^a-zA-Z0-9_-]+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "") || "added-plan";
  let candidate = base;
  let index = 2;
  while (existingIds.has(candidate)) {
    candidate = `${base}-${index}`;
    index += 1;
  }
  return candidate;
}

function formatIDR(value: number): string {
  return `Rp${Math.round(value).toLocaleString("id-ID")}`;
}

export function creativeMaterialAnalysisReadiness(
  analysis: CreativeSourceAnalysis | undefined,
  analysisState: MaterialAnalysisState,
  marketPack: Pick<CreativeResource, "id"> | undefined,
  copyLibrary: Pick<CreativeResource, "id"> | undefined,
): CreativeMaterialAnalysisReadiness {
  if (!analysisState.ready) {
    return { ready: false, active: analysisState.status !== "failed", failed: analysisState.status === "failed", reason: analysisState.error };
  }
  if (!analysis || !marketPack || !copyLibrary || sourceAnalysisNeedsVisualUpgrade(analysis)) {
    return { ready: false, active: false, failed: true };
  }
  const adaptation = preparedPreAdaptation(analysis, marketPack, copyLibrary);
  if (adaptation?.status === "completed") {
    return { ready: true, active: false, failed: false };
  }
  const adaptationStatus = sourceAnalysisAdaptationStatus(analysis);
  const adaptationEnvelope = record(record(analysis.result).adaptation);
  const adaptationFailureReason = recordString(adaptationEnvelope, "error_message") || recordString(adaptationEnvelope, "summary");
  const unavailable = adaptationStatus === "unavailable" && sourceAnalysisHasNoEditableCopy(analysis);
  const manualRequired = adaptationStatus === "unavailable" && recordString(adaptationEnvelope, "error_code") === "manual_confirmation_required";
  return {
    ready: false,
    active: adaptationStatus === "",
    failed: adaptationStatus !== "" && !manualRequired && !unavailable,
    unavailable,
    manualRequired,
    reason: adaptationStatus !== "" ? adaptationFailureReason : "",
  };
}

function CreativeOrderDraft({ candidates, analyses, deselecting, onDeselect, onClose, onDone, onOpenCopyLibrary, onRetrySourceAnalysis, retryingSourceAnalysis = false }: { candidates: CreativeMaterialCandidate[]; analyses: CreativeSourceAnalysis[]; deselecting: boolean; onDeselect: (candidateId: string) => void; onClose: () => void; onDone: (orderId: string) => void; onOpenCopyLibrary?: () => void; onRetrySourceAnalysis: (candidateId: string) => void; retryingSourceAnalysis?: boolean }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const resources = useQuery(creativeResourcesOptions(wsId));
  const squads = useQuery(squadListOptions(wsId));
  const [drafts, setDrafts] = useState<Record<string, OrderItemDraft>>({});
  const [busy, setBusy] = useState(false);
  const [recoveryMessage, setRecoveryMessage] = useState("");
  const [recovery, setRecovery] = useState<SubmissionRecovery>(EMPTY_SUBMISSION_RECOVERY);
  const [activeCandidateId, setActiveCandidateId] = useState(candidates[0]?.id ?? "");
  const knownCandidateIds = useRef(new Set(candidates.map((candidate) => candidate.id)));
  const { marketPack, copyLibrary } = useMemo(
    () => defaultPreAdaptationResources(resources.data?.resources ?? []),
    [resources.data?.resources],
  );
  const repaymentPlanOptions = useMemo(() => approvedRepaymentPlanOptions(copyLibrary), [copyLibrary]);
  const marketFiles = useQuery(creativeResourceFilesOptions(wsId, marketPack?.id ?? ""));
  const availableSquads = squads.data ?? [];
  const selectedSquad = availableSquads.length === 1 ? availableSquads[0] : undefined;
  const squadMembers = useQuery({
    queryKey: ["workspaces", wsId, "squads", selectedSquad?.id ?? "", "members"],
    queryFn: () => api.listSquadMembers(selectedSquad!.id),
    enabled: !!wsId && !!selectedSquad?.id,
  });
  const completedAnalyses = useMemo(() => latestCompletedAnalyses(analyses), [analyses]);
  useEffect(() => {
    setDrafts((current) => {
      let changed = false;
      const next = { ...current };
      for (const candidate of candidates) {
        const analysis = completedAnalyses.get(candidate.id);
        const adaptation = preparedPreAdaptation(analysis, marketPack, copyLibrary);
        const draft = orderDraftWithPreAdaptation(next[candidate.id], adaptation);
        if (draft !== next[candidate.id]) {
          next[candidate.id] = draft;
          changed = true;
        }
      }
      return changed ? next : current;
    });
  }, [candidates, completedAnalyses, copyLibrary, marketPack]);
  useEffect(() => {
    const nextIds = new Set(candidates.map((candidate) => candidate.id));
    const added = candidates.find((candidate) => !knownCandidateIds.current.has(candidate.id));
    knownCandidateIds.current = nextIds;
    setActiveCandidateId((current) => added?.id ?? (nextIds.has(current) ? current : candidates[0]?.id ?? ""));
  }, [candidates]);
  const incompleteCandidates = candidates.filter((candidate) => !completedAnalyses.has(candidate.id));
  const sourceAnalysisUpgradeCount = candidates.filter((candidate) => sourceAnalysisNeedsVisualUpgrade(completedAnalyses.get(candidate.id))).length;
  const unconfiguredCandidates = candidates.filter((candidate) => {
    const draft = drafts[candidate.id];
    const analysis = completedAnalyses.get(candidate.id);
    const adaptation = preparedPreAdaptation(analysis, marketPack, copyLibrary);
    if (sourceAnalysisNeedsVisualUpgrade(analysis)) return true;
    const pendingRecommendation = Boolean(draft && adaptation?.status === "completed" && adaptation.textReplacements.some((replacement) => hasPendingReplacementConfirmation(replacement, draft.textOverrides, draft.replacementSources ?? {})));
    return !draft
      || adaptation?.status !== "completed"
      || pendingRecommendation;
  });
  const readinessByCandidateId = useMemo(
    () => new Map(candidates.map((candidate) => {
      const analysis = completedAnalyses.get(candidate.id);
      const adaptation = preparedPreAdaptation(analysis, marketPack, copyLibrary);
      const draft = drafts[candidate.id];
      if (draft && adaptation?.status === "completed") {
        const pendingRecommendation = adaptation.textReplacements.some((replacement) => hasPendingReplacementConfirmation(replacement, draft.textOverrides, draft.replacementSources ?? {}));
        return [candidate.id, pendingRecommendation ? "manual_required" as const : "ready" as const];
      }
      if (analysis && marketPack && copyLibrary && !sourceAnalysisNeedsVisualUpgrade(analysis) && !sourceAnalysisAdaptationStatus(analysis)) {
        return [candidate.id, "analyzing" as const];
      }
      if (sourceAnalysisAdaptationErrorCode(analysis) === "manual_confirmation_required") {
        return [candidate.id, "manual_required" as const];
      }
      if (sourceAnalysisAdaptationStatus(analysis) === "unavailable" && sourceAnalysisHasNoEditableCopy(analysis)) {
        return [candidate.id, "unavailable" as const];
      }
      return [candidate.id, "failed" as const];
    })),
    [candidates, completedAnalyses, copyLibrary, drafts, marketPack],
  );
  const analysisRunningCount = [...readinessByCandidateId.values()].filter((status) => status === "analyzing").length;
  const manualRequiredCount = [...readinessByCandidateId.values()].filter((status) => status === "manual_required").length;
  const omittedTextBlockCount = candidates.reduce((count, candidate) => {
    const draft = drafts[candidate.id];
    const adaptation = preparedPreAdaptation(completedAnalyses.get(candidate.id), marketPack, copyLibrary);
    if (!draft || !adaptation || adaptation.status !== "completed") return count;
    const numericBlockIDs = new Set(effectiveNumericLayouts(adaptation, draft.numericLayoutDrafts ?? {}).flatMap((layout) => layout.sourceBlockIds));
    return count + adaptation.textReplacements.filter((replacement) => !numericBlockIDs.has(replacement.blockId) && !replacementText(replacement, draft.textOverrides).trim()).length;
  }, 0);
  const setDraft = (candidateId: string, patch: Partial<OrderItemDraft>) => setDrafts((current) => ({
    ...current,
    [candidateId]: { ...current[candidateId], ...patch } as OrderItemDraft,
  }));
  const setReplacementChoice = (candidateId: string, blockId: string, text: string, source: ReplacementSourceChoice) => setDrafts((current) => {
    const draft = current[candidateId];
    if (!draft) return current;
    return {
      ...current,
      [candidateId]: {
        ...draft,
        textOverrides: { ...draft.textOverrides, [blockId]: text },
        replacementSources: { ...(draft.replacementSources ?? {}), [blockId]: source },
      },
    };
  });
  const setRepaymentPlanChoice = (candidateId: string, scenarioId: string, plan: RepaymentPlanChoice, original: PreparedRepaymentPlanSelection) => setDrafts((current) => {
    const draft = current[candidateId];
    if (!draft) return current;
    const repaymentPlanOverrides = { ...(draft.repaymentPlanOverrides ?? {}) };
    const numericLayoutDrafts = { ...(draft.numericLayoutDrafts ?? {}) };
    let updatedAddedScenario = false;
    for (const [layoutId, layoutDraft] of Object.entries(numericLayoutDrafts)) {
      if (!layoutDraft.addedScenarios?.[scenarioId]) continue;
      numericLayoutDrafts[layoutId] = {
        ...layoutDraft,
        addedScenarios: { ...layoutDraft.addedScenarios, [scenarioId]: plan },
      };
      delete repaymentPlanOverrides[scenarioId];
      updatedAddedScenario = true;
      break;
    }
    if (sameRepaymentPlanChoice(plan, original)) {
      delete repaymentPlanOverrides[scenarioId];
    } else if (!updatedAddedScenario) {
      repaymentPlanOverrides[scenarioId] = plan;
    }
    return {
      ...current,
      [candidateId]: {
        ...draft,
        repaymentPlanOverrides,
        numericLayoutDrafts,
      },
    };
  });
  const addRepaymentPlanRow = (candidateId: string, layoutId: string, plan: RepaymentPlanChoice) => setDrafts((current) => {
    const draft = current[candidateId];
    if (!draft) return current;
    const numericLayoutDrafts = { ...(draft.numericLayoutDrafts ?? {}) };
    const layoutDraft = numericLayoutDrafts[layoutId] ?? {};
    const addedScenarios = { ...(layoutDraft.addedScenarios ?? {}) };
    const scenarioId = nextAddedRepaymentPlanScenarioId(layoutId, plan.planKey, new Set([...Object.keys(addedScenarios), ...(layoutDraft.removedScenarioIds ?? [])]));
    numericLayoutDrafts[layoutId] = {
      ...layoutDraft,
      addedScenarios: { ...addedScenarios, [scenarioId]: plan },
    };
    return { ...current, [candidateId]: { ...draft, numericLayoutDrafts } };
  });
  const removeRepaymentPlanRow = (candidateId: string, layoutId: string, scenarioId: string) => setDrafts((current) => {
    const draft = current[candidateId];
    if (!draft) return current;
    const repaymentPlanOverrides = { ...(draft.repaymentPlanOverrides ?? {}) };
    const numericLayoutDrafts = { ...(draft.numericLayoutDrafts ?? {}) };
    const layoutDraft = numericLayoutDrafts[layoutId] ?? {};
    const addedScenarios = { ...(layoutDraft.addedScenarios ?? {}) };
    const removedScenarioIds = new Set(layoutDraft.removedScenarioIds ?? []);
    if (addedScenarios[scenarioId]) {
      delete addedScenarios[scenarioId];
    } else {
      removedScenarioIds.add(scenarioId);
    }
    delete repaymentPlanOverrides[scenarioId];
    numericLayoutDrafts[layoutId] = {
      ...layoutDraft,
      removedScenarioIds: [...removedScenarioIds],
      addedScenarios,
    };
    return { ...current, [candidateId]: { ...draft, repaymentPlanOverrides, numericLayoutDrafts } };
  });
  const activeCandidate = candidates.find((candidate) => candidate.id === activeCandidateId) ?? candidates[0];
  const activeAnalysis = activeCandidate ? completedAnalyses.get(activeCandidate.id) : undefined;
  const activeAnalysisNeedsVisualUpgrade = sourceAnalysisNeedsVisualUpgrade(activeAnalysis);
  const activePreAdaptation = preparedPreAdaptation(activeAnalysis, marketPack, copyLibrary);
  const activeDraft = activeCandidate ? drafts[activeCandidate.id] : undefined;
  const activeSource = activeCandidate ? candidateSource(activeCandidate) : "";
  const activeReadiness = activeCandidate ? readinessByCandidateId.get(activeCandidate.id) ?? "failed" : "failed";
  const preparePreAdaptation = useMutation({
    mutationFn: async ({ sourceAnalysisId, marketPackId }: { sourceAnalysisId: string; marketPackId: string }) => (
      api.retryCreativePreAdaptation(sourceAnalysisId, marketPackId)
    ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: creativeKeys.analyses(wsId) });
      toast.success("已继续分析，完成后会自动刷新。");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "分析未能继续排队"),
  });
  const autoPreAdaptationAttempts = useRef(new Set<string>());
  useEffect(() => {
    if (!marketPack || !copyLibrary) return;
    for (const candidate of candidates) {
      const analysis = completedAnalyses.get(candidate.id);
      if (!analysis || sourceAnalysisNeedsVisualUpgrade(analysis) || sourceAnalysisAdaptationStatus(analysis)) continue;
      const attemptKey = `${analysis.id}:${marketPack.id}:${copyLibrary.id}`;
      if (autoPreAdaptationAttempts.current.has(attemptKey)) continue;
      autoPreAdaptationAttempts.current.add(attemptKey);
      preparePreAdaptation.mutate({ sourceAnalysisId: analysis.id, marketPackId: marketPack.id });
    }
  }, [candidates, completedAnalyses, copyLibrary, marketPack, preparePreAdaptation]);
  const customCopyValidations = useMemo(() => new Map(candidates.flatMap((candidate) => {
    const draft = drafts[candidate.id];
    const adaptation = preparedPreAdaptation(completedAnalyses.get(candidate.id), marketPack, copyLibrary);
    if (!draft || !adaptation || !copyLibrary) return [];
    const snapshot = preAdaptedCopySnapshot(copyLibrary, adaptation, draft, completedAnalyses.get(candidate.id)?.id ?? "");
    return [[candidate.id, validateCustomCopyFinancialFacts(snapshot, copyLibrary)]] as const;
  })), [candidates, completedAnalyses, copyLibrary, drafts, marketPack]);
  const customCopyIssueCount = [...customCopyValidations.values()].filter((validation) => !validation.allowed).length;
  const activeCustomCopyValidation = activeCandidate ? customCopyValidations.get(activeCandidate.id) : undefined;
  const create = async () => {
    if (!marketPack || !selectedSquad || incompleteCandidates.length > 0 || unconfiguredCandidates.length > 0) return;
    setBusy(true);
    setRecoveryMessage("");
    let submissionKey = "";
    let issueId = "";
    let orderId = "";
    try {
      submissionKey = await creativeSubmissionKey({
        mode: "creative_order",
        candidates: candidates.map((candidate) => ({
          id: candidate.id,
          analysisId: completedAnalyses.get(candidate.id)?.id ?? "",
          draft: drafts[candidate.id],
        })).sort((left, right) => left.id.localeCompare(right.id)),
        marketPackId: marketPack.id,
        squadId: selectedSquad.id,
      });
      const matchingRecovery = recoveryForSubmissionKey(recovery, submissionKey);
      issueId = matchingRecovery.issueId;
      orderId = matchingRecovery.orderId;
      if (!issueId) {
        const issue = await api.createIssue({
          title: `创意订单 · ${candidates.length} 张素材`,
          description: "从创意工厂提交。这里记录关键业务决定、需要处理的问题和最终验收；生成进度与交付结果请在创意订单查看。",
          status: "todo",
          metadata: { workflow: "creative_order", creative_submission_key: submissionKey },
          allow_duplicate: true,
        });
        if (!issue.id) throw new Error("创意订单 Issue 初始化失败，请重试");
        issueId = issue.id;
        setRecovery({ issueId, orderId: "", submissionKey });
      }
      await Promise.all(candidates.map((candidate) => api.updateCreativeMaterialCandidate(issueId, candidate.id, {
        status: "selected",
        note: candidate.note,
      })));
      if (!orderId) {
        const runIds = [...new Set(candidates.map((candidate) => candidate.source_run_id).filter(Boolean))];
        const order = await api.createCreativeOrder({
          issue_id: issueId,
          submission_key: submissionKey,
          status: "queued",
          input_snapshot: {
            market_pack: { id: marketPack.id, version: marketPack.published_version, config: marketPack.published_config ?? {}, files: marketFiles.data?.files ?? [] },
            squad_snapshot: {
              squad_id: selectedSquad.id,
              squad_name: selectedSquad.name,
              leader_agent_id: selectedSquad.leader_id,
              members: creativeSquadMemberSnapshot(squadMembers.data ?? []),
            },
          },
          trigger_evidence_kind: runIds.length === 1 ? "creative_crawl_run" : "manual",
          trigger_evidence_ref_id: runIds.length === 1 ? runIds[0]! : "",
          items: candidates.map((candidate) => {
            const draft = drafts[candidate.id]!;
            const adaptation = preparedPreAdaptation(completedAnalyses.get(candidate.id), marketPack, copyLibrary);
            if (!copyLibrary || !adaptation || adaptation.status !== "completed") throw new Error(`素材 ${candidate.id.slice(0, 8)} 尚未完成逐块文字填充`);
            return creativeOrderItemInput(
              candidate.id,
              completedAnalyses.get(candidate.id)!.id,
              preAdaptedCopySnapshot(copyLibrary, adaptation, draft, completedAnalyses.get(candidate.id)!.id) as unknown as Record<string, unknown>,
              visualDirectionSummary(draft.visualDirection),
            );
          }),
        });
        if (!order.id) throw new Error("创意订单初始化失败，请重试");
        orderId = order.id;
        setRecovery({ issueId, orderId, submissionKey });
      }
      await Promise.all(candidates.map((candidate) => api.createCreativeFeedback(
        candidateDecisionFeedbackInput(candidate, "selected", undefined, undefined, `creative-order:${submissionKey}:${candidate.id}`),
      )));
      await api.setIssueMetadataKey(issueId, "creative_order_id", orderId);
      await api.updateIssue(issueId, { assignee_type: "squad", assignee_id: selectedSquad.id });
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "candidate", "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      toast.success(`已提交 ${candidates.length} 组图片，正在开始出图`);
      setRecovery(EMPTY_SUBMISSION_RECOVERY);
      onDone(orderId);
    } catch (error) {
      if (submissionKey) setRecovery({ issueId, orderId, submissionKey });
      const message = error instanceof Error ? error.message : "订单创建未完成。可以在当前面板继续，不会重复创建已完成的步骤。";
      setRecoveryMessage(message);
      toast.error(message);
    } finally {
      setBusy(false);
    }
  };

  return <section className="min-w-0 border bg-background" aria-labelledby="creative-order-draft-title">
    <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
      <div className="min-w-0"><h3 id="creative-order-draft-title" className="text-sm font-semibold">批量确认文案</h3><p className="mt-1 text-xs text-muted-foreground">确认画面文案后，会一次提交这批素材出图。</p></div>
      <div className="flex min-w-0 flex-wrap items-center justify-end gap-2"><Badge variant="outline" className="max-w-full truncate">{marketPack ? marketPack.name : "市场规则未配置"}</Badge>{selectedSquad && <Badge variant="outline" className="max-w-full truncate">{selectedSquad.name}</Badge>}<Badge variant="outline">可提交 {candidates.length - unconfiguredCandidates.length}/{candidates.length}</Badge><Badge>{candidates.length} 张素材</Badge><Button size="sm" variant="outline" onClick={onClose}><ArrowLeft className="h-4 w-4" />返回素材库</Button></div>
    </div>
    <SelectedMaterialStrip candidates={candidates} activeCandidateId={activeCandidate?.id ?? ""} readinessByCandidateId={readinessByCandidateId} deselecting={deselecting} onSelect={setActiveCandidateId} onDeselect={onDeselect} />
    {activeCandidate && <article key={activeCandidate.id} className="min-w-0 space-y-4 p-4" data-testid="creative-order-active-editor" data-candidate-id={activeCandidate.id}>
      <div className="min-w-0 border-b border-l-2 border-emerald-600 pb-4 pl-3"><div className="flex flex-wrap items-center gap-2"><Sparkles className="h-4 w-4 text-emerald-700" /><p className="break-words text-sm font-medium">{activeAnalysis?.summary || "素材分析中"}</p><span className="font-mono text-[10px] text-muted-foreground">素材 ID {activeCandidate.id.slice(0, 8)}</span></div>{activeAnalysis && <AnalysisHighlights analysis={activeAnalysis} adaptation={activePreAdaptation} />}{activePreAdaptation ? <PreAdaptationSummary adaptation={activePreAdaptation} /> : activeReadiness === "analyzing" ? <p className="mt-2 text-xs text-amber-700">{MATERIAL_ANALYSIS_RUNNING_MESSAGE}</p> : <p className="mt-2 text-xs text-destructive">素材分析未完成，暂时不能提交出图。</p>}</div>
      <div className="min-w-0 space-y-4">
         {activeAnalysisNeedsVisualUpgrade ? <div role="alert" className="space-y-3 border border-amber-300 bg-amber-50/50 px-3 py-3 text-xs text-amber-900"><p className="font-medium">这张素材需要重新分析后才能提交。</p><p>重新分析会刷新画面文字区域、业务信息和出图配置。</p><Button size="sm" disabled={retryingSourceAnalysis} onClick={() => onRetrySourceAnalysis(activeCandidate.id)}><RefreshCw className={`h-4 w-4 ${retryingSourceAnalysis ? "animate-spin" : ""}`} />{retryingSourceAnalysis ? "正在重新分析" : "重新分析"}</Button></div> : activePreAdaptation?.status === "completed" && activeDraft ? <TextReplacementPlan sourceImage={activeSource} sourceImageAlt={activeCandidate.title || activeCandidate.competitor} adaptation={activePreAdaptation} copyLibrary={copyLibrary} overrides={activeDraft.textOverrides} replacementSources={activeDraft.replacementSources ?? {}} repaymentPlanOverrides={activeDraft.repaymentPlanOverrides ?? {}} numericLayoutDrafts={activeDraft.numericLayoutDrafts ?? {}} repaymentPlanOptions={repaymentPlanOptions} visualDirection={activeDraft.visualDirection} onVisualDirectionChange={(value) => setDraft(activeCandidate.id, { visualDirection: value })} onOpenCopyLibrary={onOpenCopyLibrary} onRetrySourceAnalysis={() => onRetrySourceAnalysis(activeCandidate.id)} retryingSourceAnalysis={retryingSourceAnalysis} onChooseReplacement={(blockId, value, source) => setReplacementChoice(activeCandidate.id, blockId, value, source)} onChooseRepaymentPlan={(scenarioId, plan, original) => setRepaymentPlanChoice(activeCandidate.id, scenarioId, plan, original)} onAddRepaymentPlan={(layoutId, plan) => addRepaymentPlanRow(activeCandidate.id, layoutId, plan)} onRemoveRepaymentPlan={(layoutId, scenarioId) => removeRepaymentPlanRow(activeCandidate.id, layoutId, scenarioId)} /> : activeReadiness === "unavailable" ? <div role="status" className="space-y-2 border border-slate-300 bg-slate-50 px-3 py-3 text-xs text-slate-700"><p className="font-medium">这张素材没有可配置的原图文案。</p><p>平台已保留分析结果，不会继续重试，也不会把它加入出图配置；可选择其他有可编辑文案的素材。</p></div> : activeReadiness === "analyzing" ? <div role="status" className="flex items-start gap-2 border border-amber-300 bg-amber-50/50 px-3 py-3 text-xs text-amber-900"><LoaderCircle className="mt-0.5 h-4 w-4 shrink-0 animate-spin" /><div className="min-w-0 flex-1"><p className="font-medium">{MATERIAL_ANALYSIS_RUNNING_MESSAGE}</p><p className="mt-1 text-amber-800">{MATERIAL_ANALYSIS_RUNNING_DETAIL}</p></div><Button size="sm" variant="outline" disabled={retryingSourceAnalysis} onClick={() => onRetrySourceAnalysis(activeCandidate.id)}><RefreshCw className={`h-4 w-4 ${retryingSourceAnalysis ? "animate-spin" : ""}`} />{retryingSourceAnalysis ? "正在重新分析" : "重新分析"}</Button></div> : activeReadiness === "manual_required" ? <div role="alert" className="space-y-2 border border-amber-300 bg-amber-50/50 px-3 py-3 text-xs text-amber-900"><p className="font-medium">预适配已重试一次仍未通过，需人工确认文案与数值映射。</p><p className="text-amber-800">这张素材不会进入可用素材，也不会继续自动重试。</p></div> : <div role="alert" className="space-y-3 border border-amber-300 bg-amber-50/50 px-3 py-3 text-xs text-amber-800"><p>{!marketPack ? "当前工作区尚未发布唯一市场配置，素材分析暂不可用。" : !copyLibrary ? "当前市场配置没有绑定已发布文案库，素材分析暂不可用。" : "本图尚未按当前市场配置完成分析，不会替换为泛文案。"}</p>{activeAnalysis && <Button size="sm" variant="outline" disabled={retryingSourceAnalysis} onClick={() => onRetrySourceAnalysis(activeCandidate.id)}><RefreshCw className={`h-4 w-4 ${retryingSourceAnalysis ? "animate-spin" : ""}`} />{retryingSourceAnalysis ? "正在重新分析" : "重新分析"}</Button>}</div>}
        {activeCustomCopyValidation && !activeCustomCopyValidation.allowed && <p className="border border-amber-300 bg-amber-50/50 px-3 py-2 text-xs text-amber-900">{activeCustomCopyValidation.message} 这是本次人工改写的提示，不会阻止提交；请确认该数字已获业务确认。</p>}
      </div>
    </article>}
    <div className="flex flex-wrap items-center justify-between gap-3 border-t bg-muted/10 px-4 py-3"><div className="text-xs text-muted-foreground">{recoveryMessage ? <span className="text-destructive">{recoveryMessage}</span> : !marketPack ? "当前工作区尚未发布唯一市场配置" : !selectedSquad ? "当前工作区尚未配置执行服务" : incompleteCandidates.length > 0 ? `${incompleteCandidates.length} 张素材仍在分析中` : sourceAnalysisUpgradeCount > 0 ? `${sourceAnalysisUpgradeCount} 张素材需要重新分析` : analysisRunningCount > 0 ? `${analysisRunningCount} 张素材仍在分析中，完成后会自动刷新` : manualRequiredCount > 0 ? `${manualRequiredCount} 张素材需人工确认文案与数值映射` : unconfiguredCandidates.length > 0 ? `${unconfiguredCandidates.length} 张素材当前分析不可用；请返回素材库重新分析或检查市场配置` : customCopyIssueCount > 0 ? `${customCopyIssueCount} 张素材含人工确认的金融数值，请确认来源后提交` : omittedTextBlockCount > 0 ? `${omittedTextBlockCount} 个文字区块未填，生成时会移除原竞品文字；可在上方补充。` : "全部素材已配置，可以提交出图。"}</div><Button disabled={busy || !marketPack || !selectedSquad || incompleteCandidates.length > 0 || unconfiguredCandidates.length > 0} onClick={() => void create()}>{busy ? "正在提交" : recovery.issueId ? "继续提交" : `提交 ${candidates.length} 组出图`}</Button></div>
  </section>;
}

export function SelectedMaterialStrip({
  candidates,
  activeCandidateId,
  readinessByCandidateId,
  deselecting,
  onSelect,
  onDeselect,
}: {
  candidates: CreativeMaterialCandidate[];
  activeCandidateId: string;
  readinessByCandidateId: ReadonlyMap<string, SelectedMaterialReadinessStatus>;
  deselecting: boolean;
  onSelect: (candidateId: string) => void;
  onDeselect: (candidateId: string) => void;
}) {
  return <nav className="flex gap-2 overflow-x-auto border-b bg-muted/10 p-3" aria-label="已选素材">{candidates.map((candidate, index) => {
    const source = candidateSource(candidate);
    const active = candidate.id === activeCandidateId;
    return <div key={candidate.id} className={`relative min-w-52 border bg-background ${active ? "border-emerald-600 ring-1 ring-emerald-600/20" : ""}`}>
      <button type="button" aria-pressed={active} onClick={() => onSelect(candidate.id)} className="grid w-full grid-cols-[56px_minmax(0,1fr)] gap-2 p-2 pr-9 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2">
        <span className="aspect-[4/3] overflow-hidden bg-muted">{source ? <img src={source} alt="" width={160} height={120} loading="lazy" className="h-full w-full object-contain" /> : <span className="flex h-full items-center justify-center"><ImageIcon className="h-4 w-4 text-muted-foreground" /></span>}</span>
        <span className="min-w-0"><span className="block truncate text-xs font-medium">{index + 1}. {candidate.title || candidate.competitor}</span><span className="mt-1 block text-[11px] text-muted-foreground">{selectedMaterialReadinessLabel(readinessByCandidateId.get(candidate.id) ?? "failed")}</span><span className="mt-0.5 block font-mono text-[10px] text-muted-foreground">{candidate.id.slice(0, 8)}</span></span>
      </button>
      <Button aria-label="取消选择素材" title="取消选择素材" size="icon-sm" variant="outline" className="absolute right-1.5 top-1.5 bg-background hover:border-destructive hover:text-destructive" disabled={deselecting} onClick={() => onDeselect(candidate.id)}><X className="h-4 w-4" /></Button>
    </div>;
  })}</nav>;
}

export function selectedMaterialReadinessLabel(status: SelectedMaterialReadinessStatus): string {
  if (status === "ready") return "可用";
  if (status === "analyzing") return "分析中";
  if (status === "manual_required") return "待人工确认";
  if (status === "unavailable") return "无可配置文案";
  return "处理失败";
}

const CANDIDATE_REJECTION_REASONS = [
  { value: "irrelevant", label: "与本次方向无关" },
  { value: "low_quality", label: "原图质量不足" },
  { value: "composition_unsuitable", label: "构图不适合改造" },
  { value: "copy_unsuitable", label: "文案机制不适合" },
  { value: "competitor_hard_to_replace", label: "竞品元素难以替换" },
  { value: "app_ui_unsuitable", label: "App 界面不适合" },
  { value: "duplicate", label: "重复素材" },
  { value: "other", label: "其他" },
];

export function canCreateDirectEdit(candidate: Pick<CreativeMaterialCandidate, "source_attachment_id"> | null, instruction: string, squadId: string): boolean {
  return Boolean(candidate?.source_attachment_id && instruction.trim() && squadId);
}

function DirectEditDialog({ candidate, onClose, onCreated }: { candidate: CreativeMaterialCandidate | null; onClose: () => void; onCreated: (orderId: string) => void }) {
  const wsId = useWorkspaceId();
  const squads = useQuery(squadListOptions(wsId));
  const [instruction, setInstruction] = useState("");
  const [targetSize, setTargetSize] = useState<"1080x1080" | "1200x628" | "800x1000">("1080x1080");
  const [deliveryMode, setDeliveryMode] = useState<"preview" | "publish">("preview");
  const [recovery, setRecovery] = useState<SubmissionRecovery>(EMPTY_SUBMISSION_RECOVERY);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const availableSquads = squads.data ?? [];
  const selectedSquad = availableSquads.length === 1 ? availableSquads[0] : undefined;

  useEffect(() => {
    setInstruction("");
    setTargetSize("1080x1080");
    setDeliveryMode("preview");
    setRecovery(EMPTY_SUBMISSION_RECOVERY);
    setError("");
  }, [candidate?.id]);

  const submit = async () => {
    if (!candidate || !candidate.source_attachment_id || !selectedSquad || !canCreateDirectEdit(candidate, instruction, selectedSquad.id)) return;
    setBusy(true);
    setError("");
    let submissionKey = "";
    let issueId = "";
    let orderId = "";
    try {
      submissionKey = await creativeSubmissionKey({
        mode: "creative_direct_edit",
        candidateId: candidate.id,
        instruction: instruction.trim(),
        targetSize,
        deliveryMode,
        squadId: selectedSquad.id,
      });
      const matchingRecovery = recoveryForSubmissionKey(recovery, submissionKey);
      issueId = matchingRecovery.issueId;
      orderId = matchingRecovery.orderId;
      if (!issueId) {
        const issue = await api.createIssue({
          title: `直接改图 · ${candidate.title || candidate.competitor || candidate.id.slice(0, 8)}`,
          description: `用户修改要求：${instruction.trim()}`,
          status: "todo",
          attachment_ids: [candidate.source_attachment_id],
          metadata: { workflow: "creative_direct_edit", delivery_mode: deliveryMode, creative_submission_key: submissionKey },
        });
        if (!issue.id) throw new Error("直接改图 Issue 初始化失败，请重试");
        issueId = issue.id;
        setRecovery({ issueId, orderId: "", submissionKey });
      }
      const directEdit = await api.createCreativeDirectEdit({
        issue_id: issueId,
        submission_key: submissionKey,
        candidate_id: candidate.id,
        user_request: instruction.trim(),
        target_size: targetSize,
        delivery_mode: deliveryMode,
        squad_id: selectedSquad.id,
      });
      orderId = directEdit.order.id;
      setRecovery({ issueId, orderId, submissionKey });
      if (!orderId) throw new Error("直接改图订单初始化失败，请重试");
      await api.setIssueMetadataKey(issueId, "creative_order_id", orderId);
      await api.updateIssue(issueId, { assignee_type: "squad", assignee_id: selectedSquad.id });
      toast.success(deliveryMode === "publish" ? "直接改图已提交，将自动合成品牌组件并进入质检" : "直接改图预览已提交");
      onCreated(orderId);
    } catch (submitError) {
      if (submissionKey) setRecovery({ issueId, orderId, submissionKey });
      setError(submitError instanceof Error ? submitError.message : "提交未完成。可继续提交，不会重复创建已完成的步骤。");
    } finally {
      setBusy(false);
    }
  };

  return <Dialog open={candidate !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>直接改图</DialogTitle><DialogDescription>原图会固定为本次修改的来源。预览只生成底图；正式投放会自动合成品牌组件并进入视觉质检。</DialogDescription></DialogHeader>
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="目标尺寸"><NativeSelect disabled={Boolean(recovery.issueId)} value={targetSize} onChange={(event) => setTargetSize(event.target.value as typeof targetSize)}><NativeSelectOption value="1080x1080">1080 x 1080</NativeSelectOption><NativeSelectOption value="1200x628">1200 x 628</NativeSelectOption><NativeSelectOption value="800x1000">800 x 1000</NativeSelectOption></NativeSelect></Field>
        <Field label="交付方式"><NativeSelect disabled={Boolean(recovery.issueId)} value={deliveryMode} onChange={(event) => setDeliveryMode(event.target.value as typeof deliveryMode)}><NativeSelectOption value="preview">预览</NativeSelectOption><NativeSelectOption value="publish">作为正式投放素材</NativeSelectOption></NativeSelect></Field>
      </div>
      <Field label="修改要求" wide><Textarea disabled={Boolean(recovery.issueId)} rows={5} value={instruction} onChange={(event) => setInstruction(event.target.value)} placeholder="例如：保留人物和绿色信息卡片，将主标题改得更醒目，移除右上角竞品标识。" /></Field>
      {!candidate?.source_attachment_id && <p className="border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">该素材尚无稳定源文件，暂时不能直接改图。</p>}
      {recovery.issueId && <p className="border bg-muted/20 p-3 text-xs text-muted-foreground">本次提交参数已冻结。继续提交会恢复同一任务；关闭窗口后可重新配置。</p>}
      {error && <p role="alert" className="border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">{error}</p>}
    </div>
    <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={busy || !canCreateDirectEdit(candidate, instruction, selectedSquad?.id ?? "")} onClick={() => void submit()}>{busy ? "正在提交" : recovery.issueId ? "继续提交" : "提交改图"}</Button></DialogFooter>
  </DialogContent></Dialog>;
}

function CandidateRejectDialog({ candidate, busy, onClose, onConfirm }: { candidate: CreativeMaterialCandidate | null; busy: boolean; onClose: () => void; onConfirm: (reasonCode: string, comment: string, idempotencyKey: string) => void }) {
  const [reasonCode, setReasonCode] = useState("irrelevant");
  const [comment, setComment] = useState("");
  const [actionId, setActionId] = useState("");
  useEffect(() => { setReasonCode("irrelevant"); setComment(""); setActionId(candidate ? crypto.randomUUID() : ""); }, [candidate?.id]);
  return <Dialog open={candidate !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-lg"><DialogHeader><DialogTitle>不采用这张素材</DialogTitle><DialogDescription>{candidate?.title || candidate?.competitor || "选择一个原因，帮助后续优化采集与推荐。"}</DialogDescription></DialogHeader><div className="space-y-4"><Field label="原因" wide><NativeSelect value={reasonCode} onChange={(event) => setReasonCode(event.target.value)}>{CANDIDATE_REJECTION_REASONS.map((reason) => <NativeSelectOption key={reason.value} value={reason.value}>{reason.label}</NativeSelectOption>)}</NativeSelect></Field><Field label="补充说明" wide><Textarea rows={3} value={comment} onChange={(event) => setComment(event.target.value)} placeholder="可选" /></Field></div><DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button variant="destructive" disabled={busy || !actionId} onClick={() => candidate && onConfirm(reasonCode, comment.trim(), candidateFeedbackIdempotencyKey(candidate.id, "rejected", actionId))}>{busy ? "正在记录" : "确认不采用"}</Button></DialogFooter></DialogContent></Dialog>;
}

function AnalysisFacts({ analysis, adaptedCopy }: { analysis: CreativeSourceAnalysis; adaptedCopy?: CreativeCopySnapshot }) {
  const result = analysis.result ?? {};
  const theme = recordString(result, "theme");
  const themeElements = recordStringArray(result, "theme_elements");
  const benefit = recordString(result, "primary_benefit");
  const secondaryBenefits = recordStringArray(result, "secondary_benefits");
  const value = recordString(result, "benefit_value");
  const semantics = recordString(result, "source_semantics");
  const mechanism = recordString(result, "information_mechanism");
  const visualType = recordString(result, "visual_type");
  const visualAnchors = recordStringArray(result, "visual_anchors");
  const paletteAnchors = recordStringArray(result, "palette_anchors");
  const mustPreserve = recordStringArray(result, "must_preserve");
  const allowedVariations = recordStringArray(result, "allowed_variations");
  const selectedRepaymentPlans = adaptedCopy?.pre_adaptation?.repayment_plan_selections ?? [];
  return <dl className="mt-3 grid gap-x-4 gap-y-2 text-xs sm:grid-cols-2">
    <AnalysisFact label="主题" value={theme || "未识别"} />
    <AnalysisFact label="主利益点" value={benefit || "未识别"} />
    {themeElements.length > 0 && <AnalysisFact label="主题元素" value={themeElements.join("、")} />}
    {secondaryBenefits.length > 0 && <AnalysisFact label="次要利益点" value={secondaryBenefits.join("、")} />}
    {value && <AnalysisFact label="竞品观察" value={`${value}，只作结构参考`} wide />}
    {selectedRepaymentPlans.length > 0 && <AnalysisFact label="已选我方还款计划" value={selectedRepaymentPlans.slice(0, 4).map((plan) => `${plan.values.principal} / ${plan.values.tenor} / ${plan.values.monthly_installment}`).join("；")} wide />}
    {semantics && <AnalysisFact label="业务语义" value={semantics} wide />}
    {mechanism && <AnalysisFact label="信息机制" value={mechanism} wide />}
    {visualType && <AnalysisFact label="视觉类型" value={visualType} />}
    {visualAnchors.length > 0 && <AnalysisFact label="视觉锚点" value={visualAnchors.join("、")} wide />}
    {paletteAnchors.length > 0 && <AnalysisFact label="色彩锚点" value={paletteAnchors.join("、")} wide />}
    {mustPreserve.length > 0 && <AnalysisFact label="必须保留" value={mustPreserve.join("、")} wide />}
    {allowedVariations.length > 0 && <AnalysisFact label="可调整范围" value={allowedVariations.join("、")} wide />}
  </dl>;
}

function AnalysisHighlights({ analysis, adaptation }: { analysis: CreativeSourceAnalysis; adaptation: PreparedPreAdaptation | null }) {
  const result = analysis.result ?? {};
  const highlights = adaptation?.analysisHighlights.length
    ? adaptation.analysisHighlights
    : [
      recordString(result, "information_mechanism"),
      recordString(result, "primary_benefit"),
      ...recordStringArray(result, "must_preserve").slice(0, 2),
      ...recordStringArray(result, "allowed_variations").slice(0, 1),
    ].filter(Boolean).slice(0, 5);
  return highlights.length > 0 ? <ul className="mt-3 grid gap-1.5 text-xs text-muted-foreground">{highlights.map((highlight, index) => <li key={`${index}:${highlight}`} className="border-l-2 border-muted-foreground/30 pl-2">{highlight}</li>)}</ul> : null;
}

function AnalysisFact({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return <div className={wide ? "sm:col-span-2" : undefined}><dt className="text-muted-foreground">{label}</dt><dd className="mt-0.5 break-words text-foreground">{value}</dd></div>;
}

export type PreparedTextReplacement = {
  blockId: string;
  visualRegionId?: string;
  location: string;
  role: string;
  semanticKind?: string;
  sourceText: string;
  visualBounds?: NormalizedVisualBounds;
  replacementText: string;
  sourceKeys: string[];
  status: "ready" | "recommended" | "calculated" | "missing";
  note: string;
  recommendationBasis?: string[];
  calculation?: { formula: string; inputs: string[]; result: string };
};

export type NormalizedVisualBounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type PreparedRepaymentPlanSelection = {
  id: string;
  planKey: string;
  principal: number;
  tenorMonths: number;
  values: {
    principal: string;
    tenor: string;
    totalInterest: string;
    totalRepayment: string;
    monthlyInstallment: string;
  };
};

export type PreparedNumericLayout = {
  id: string;
  visualRegionId?: string;
  sourceBlockIds: string[];
  location: string;
  visualBounds?: NormalizedVisualBounds;
  layoutKind: "table" | "card_grid" | "comparison" | "single_card" | "single_value" | "option_buttons" | "table_row";
  scenarioIds: string[];
  targetColumns: Array<"principal" | "tenor" | "monthly_installment" | "total_interest" | "total_repayment">;
  renderInstruction: string;
};

export type PreparedVisualRegion = {
  id: string;
  location: string;
  kind: "copy" | "numeric";
  sourceBlockIds: string[];
  visualBounds?: NormalizedVisualBounds;
};

export type PreparedVisualReviewRegion = PreparedVisualRegion & {
  textReplacements: PreparedTextReplacement[];
  numericLayouts: PreparedNumericLayout[];
};

export type PreparedPreAdaptation = {
  status: "completed" | "unavailable";
  summary: string;
  errorCode?: string;
  errorMessage?: string;
  decision: string;
  preferredFragmentKeys: string[];
  reasons: string[];
  gaps: string[];
  analysisHighlights: string[];
  analysisResult: Record<string, unknown>;
  visualRegions?: PreparedVisualRegion[];
  textReplacements: PreparedTextReplacement[];
  repaymentPlanSelections: PreparedRepaymentPlanSelection[];
  numericLayouts: PreparedNumericLayout[];
};

export function preparedPreAdaptation(analysis: CreativeSourceAnalysis | undefined, marketPack: { id: string } | undefined, copyLibrary: { id: string } | undefined): PreparedPreAdaptation | null {
  if (!analysis || !marketPack || !copyLibrary) return null;
  const adaptation = record(analysis.result).adaptation;
  const envelope = record(adaptation);
  const result = record(envelope.result);
  const status = recordString(envelope, "status");
  if ((status !== "completed" && status !== "unavailable")
    || recordString(result, "market_pack_id") !== marketPack.id
    || recordString(result, "copy_library_id") !== copyLibrary.id) return null;
  const sourceResult = record(analysis.result);
  const visualBoundsByBlockId = sourceTextBlockVisualBounds(sourceResult);
  const semanticKindsByBlockId = sourceTextBlockSemanticKinds(sourceResult);
  const visualRegions = sourceVisualRegions(sourceResult);
  const visualRegionByID = new Map(visualRegions.map((region) => [region.id, region]));
  const textReplacements = Array.isArray(result.text_replacements)
    ? result.text_replacements.map((item) => preparedTextReplacement(item, visualBoundsByBlockId, semanticKindsByBlockId)).filter((item): item is PreparedTextReplacement => item !== null)
    : [];
  const repaymentPlanSelections = Array.isArray(result.repayment_plan_selections)
    ? result.repayment_plan_selections.map(preparedRepaymentPlanSelection).filter((item): item is PreparedRepaymentPlanSelection => item !== null)
    : [];
  const numericLayouts = Array.isArray(result.numeric_layouts)
    ? result.numeric_layouts.map((item) => preparedNumericLayout(item, visualBoundsByBlockId, visualRegionByID)).filter((item): item is PreparedNumericLayout => item !== null)
    : [];
  // A legacy completed adaptation only carried a generic recommendation. It is
  // not safe to treat that as a completed image-specific text replacement plan.
  if (status === "completed" && textReplacements.length === 0 && numericLayouts.length === 0) return null;
  return {
    status,
    summary: recordString(envelope, "summary"),
    errorCode: recordString(envelope, "error_code"),
    errorMessage: recordString(envelope, "error_message"),
    decision: recordString(result, "decision"),
    preferredFragmentKeys: recordStringArray(result, "preferred_fragment_keys"),
    reasons: recordStringArray(result, "reasons"),
    gaps: recordStringArray(result, "gaps"),
    analysisHighlights: recordStringArray(result, "analysis_highlights").slice(0, 5),
    analysisResult: sourceResult,
    visualRegions,
    textReplacements,
    repaymentPlanSelections,
    numericLayouts,
  };
}

export function sourceAnalysisAdaptationStatus(analysis: CreativeSourceAnalysis | undefined): string {
  if (!analysis) return "";
  return recordString(record(record(analysis.result).adaptation), "status");
}

export function sourceAnalysisAdaptationErrorCode(analysis: CreativeSourceAnalysis | undefined): string {
  if (!analysis) return "";
  return recordString(record(record(analysis.result).adaptation), "error_code");
}

function preparedTextReplacement(value: unknown, visualBoundsByBlockId: ReadonlyMap<string, NormalizedVisualBounds>, semanticKindsByBlockId: ReadonlyMap<string, string>): PreparedTextReplacement | null {
  const item = record(value);
  const blockId = recordString(item, "block_id");
  const location = recordString(item, "location");
  if (!blockId || !location) return null;
  const status = recordString(item, "status");
  const calculation = record(item.calculation);
  const calculationFormula = recordString(calculation, "formula");
  const calculationResult = recordString(calculation, "result");
  const calculationInputs = recordStringArray(calculation, "inputs");
  return {
    blockId,
    visualRegionId: recordString(item, "visual_region_id"),
    location,
    role: recordString(item, "role") || "supporting",
    semanticKind: semanticKindsByBlockId.get(blockId) ?? "",
    sourceText: recordString(item, "source_text"),
    visualBounds: visualBoundsByBlockId.get(blockId),
    replacementText: recordString(item, "replacement_text"),
    sourceKeys: recordStringArray(item, "source_keys"),
    status: status === "ready" || status === "recommended" || status === "calculated" ? status : "missing",
    note: recordString(item, "note"),
    recommendationBasis: recordStringArray(item, "recommendation_basis"),
    ...(calculationFormula && calculationResult && calculationInputs.length > 0 ? { calculation: { formula: calculationFormula, inputs: calculationInputs, result: calculationResult } } : {}),
  };
}

export function sourceVisualRegions(result: Record<string, unknown>): PreparedVisualRegion[] {
  const regions = Array.isArray(result.visual_regions) ? result.visual_regions : [];
  const seen = new Set<string>();
  return regions.flatMap((value) => {
    const region = record(value);
    const id = recordString(region, "id");
    const location = recordString(region, "location");
    const kind = recordString(region, "kind");
    const sourceBlockIds = recordStringArray(region, "source_block_ids");
    if (!id || !location || (kind !== "copy" && kind !== "numeric") || sourceBlockIds.length === 0 || seen.has(id)) return [];
    seen.add(id);
    const visualBounds = normalizedVisualBounds(region.visual_bounds);
    return [{ id, location, kind, sourceBlockIds, ...(visualBounds ? { visualBounds } : {}) }];
  });
}

const repaymentPlanColumns = ["principal", "tenor", "monthly_installment", "total_interest", "total_repayment"] as const;

function numericLayoutColumn(semanticKind: string): PreparedNumericLayout["targetColumns"][number] | undefined {
  return repaymentPlanColumns.includes(semanticKind as typeof repaymentPlanColumns[number])
    ? semanticKind as PreparedNumericLayout["targetColumns"][number]
    : undefined;
}

export function pendingNumericLayouts(adaptation: PreparedPreAdaptation): PreparedNumericLayout[] {
  const visualRegions = adaptation.visualRegions ?? [];
  if (visualRegions.length === 0) return [];
  const coveredBlockIDs = new Set(adaptation.numericLayouts.flatMap((layout) => layout.sourceBlockIds));
  const replacementsByBlockID = new Map(adaptation.textReplacements.map((replacement) => [replacement.blockId, replacement]));
  return visualRegions.flatMap((region) => {
    if (region.kind !== "numeric") return [];
    const sourceBlockIDs = region.sourceBlockIds.filter((blockID) => {
      if (coveredBlockIDs.has(blockID)) return false;
      const replacement = replacementsByBlockID.get(blockID);
      return replacement?.status === "missing" && numericLayoutColumn(replacement.semanticKind ?? "") !== undefined;
    });
    if (sourceBlockIDs.length === 0) return [];
    const targetColumns = repaymentPlanColumns.filter((column) => sourceBlockIDs.some((blockID) => (
      numericLayoutColumn(replacementsByBlockID.get(blockID)?.semanticKind ?? "") === column
    )));
    if (targetColumns.length === 0) return [];
    return [{
      id: `pending-numeric:${region.id}`,
      visualRegionId: region.id,
      sourceBlockIds: sourceBlockIDs,
      location: region.location,
      visualBounds: region.visualBounds,
      layoutKind: sourceBlockIDs.length === 1 ? "single_value" : "table",
      scenarioIds: [],
      targetColumns: [...targetColumns],
      renderInstruction: "未匹配到冻结还款方案；请选择已审核方案，或留空移除该数值区域。",
    }];
  });
}

function isPendingNumericLayout(layout: PreparedNumericLayout): boolean {
  return layout.id.startsWith("pending-numeric:");
}

export function sourceAnalysisNeedsVisualUpgrade(analysis: CreativeSourceAnalysis | undefined): boolean {
	if (!analysis || analysis.status !== "completed") return false;
	const result = record(analysis.result);
	const textBlocks = Array.isArray(result.text_blocks) ? result.text_blocks : [];
	return textBlocks.length > 0 && sourceVisualRegions(result).length === 0;
}

export function sourceAnalysisHasNoEditableCopy(analysis: CreativeSourceAnalysis | undefined): boolean {
  if (!analysis || analysis.status !== "completed") return false;
  const result = record(analysis.result);
  return !Array.isArray(result.text_blocks) || result.text_blocks.length === 0;
}

export function sourceTextBlockVisualBounds(result: Record<string, unknown>): Map<string, NormalizedVisualBounds> {
  const bounds = new Map<string, NormalizedVisualBounds>();
  const textBlocks = Array.isArray(result.text_blocks) ? result.text_blocks : [];
  for (const textBlock of textBlocks) {
    const block = record(textBlock);
    const blockId = recordString(block, "id");
    const visualBounds = normalizedVisualBounds(block.visual_bounds);
    if (blockId && visualBounds) bounds.set(blockId, visualBounds);
  }
  return bounds;
}

export function sourceTextBlockSemanticKinds(result: Record<string, unknown>): Map<string, string> {
  const kinds = new Map<string, string>();
  const textBlocks = Array.isArray(result.text_blocks) ? result.text_blocks : [];
  for (const textBlock of textBlocks) {
    const block = record(textBlock);
    const blockId = recordString(block, "id");
    const semanticKind = recordString(block, "semantic_kind");
    if (blockId) kinds.set(blockId, semanticKind);
  }
  return kinds;
}

export function normalizedVisualBounds(value: unknown): NormalizedVisualBounds | null {
  const bounds = record(value);
  const x = typeof bounds.x === "number" ? bounds.x : NaN;
  const y = typeof bounds.y === "number" ? bounds.y : NaN;
  const width = typeof bounds.width === "number" ? bounds.width : NaN;
  const height = typeof bounds.height === "number" ? bounds.height : NaN;
  if (!Number.isFinite(x) || !Number.isFinite(y) || !Number.isFinite(width) || !Number.isFinite(height)
    || x < 0 || y < 0 || width <= 0 || height <= 0 || x + width > 1000 || y + height > 1000) return null;
  return { x, y, width, height };
}

function preparedRepaymentPlanSelection(value: unknown): PreparedRepaymentPlanSelection | null {
  const item = record(value);
  const values = record(item.values);
  const id = recordString(item, "id");
  const planKey = recordString(item, "plan_key");
  const principal = typeof item.principal === "number" ? item.principal : NaN;
  const tenorMonths = typeof item.tenor_months === "number" ? item.tenor_months : NaN;
  if (!id || !planKey || !Number.isFinite(principal) || !Number.isFinite(tenorMonths)) return null;
  const principalText = recordString(values, "principal");
  const tenor = recordString(values, "tenor");
  const totalInterest = recordString(values, "total_interest");
  const totalRepayment = recordString(values, "total_repayment");
  const monthlyInstallment = recordString(values, "monthly_installment");
  if (!principalText || !tenor || !totalInterest || !totalRepayment || !monthlyInstallment) return null;
  return { id, planKey, principal, tenorMonths, values: { principal: principalText, tenor, totalInterest, totalRepayment, monthlyInstallment } };
}

function preparedNumericLayout(value: unknown, visualBoundsByBlockId: ReadonlyMap<string, NormalizedVisualBounds>, visualRegionByID: ReadonlyMap<string, PreparedVisualRegion>): PreparedNumericLayout | null {
  const item = record(value);
  const allowedColumns = ["principal", "tenor", "monthly_installment", "total_interest", "total_repayment"] as const;
  const targetColumns = recordStringArray(item, "target_columns").filter((column): column is typeof allowedColumns[number] => allowedColumns.includes(column as typeof allowedColumns[number]));
  const id = recordString(item, "id");
  const visualRegionId = recordString(item, "visual_region_id");
  const location = recordString(item, "location");
  const renderInstruction = recordString(item, "render_instruction");
  const sourceBlockIds = recordStringArray(item, "source_block_ids");
  const scenarioIds = recordStringArray(item, "scenario_ids");
  const layoutKind = preparedNumericLayoutKind(recordString(item, "layout_kind"), sourceBlockIds, scenarioIds, targetColumns);
  if (!id || !location || !renderInstruction || sourceBlockIds.length === 0 || scenarioIds.length === 0 || targetColumns.length === 0) return null;
  const visualBounds = visualRegionByID.get(visualRegionId)?.visualBounds ?? unionVisualBounds(sourceBlockIds.map((blockId) => visualBoundsByBlockId.get(blockId)));
  return { id, visualRegionId, location, visualBounds, layoutKind, sourceBlockIds, scenarioIds, targetColumns, renderInstruction };
}

function preparedNumericLayoutKind(raw: string, sourceBlockIds: string[], scenarioIds: string[], targetColumns: PreparedNumericLayout["targetColumns"]): PreparedNumericLayout["layoutKind"] {
  const kind = raw.trim().toLowerCase().replace(/[-\s]+/g, "_").replace(/_+/g, "_").replace(/^_|_$/g, "");
  const canonical = ["table", "card_grid", "comparison", "single_card", "single_value", "option_buttons", "table_row"] as const;
  if (canonical.includes(kind as typeof canonical[number])) return kind as PreparedNumericLayout["layoutKind"];
  if (targetColumns.length === 1 && targetColumns[0] === "tenor" && (scenarioIds.length > 1 || sourceBlockIds.length > 1)) return "option_buttons";
  if (["tenor", "term", "duration", "tenor_button", "tenor_buttons", "period_option", "period_options"].includes(kind)) return "option_buttons";
  if (["repayment_table", "repayment_summary", "loan_summary", "installment_table", "summary_table"].includes(kind)) return "table";
  if (["principal", "amount", "amount_box", "loan_amount", "principal_amount", "credit_limit", "limit"].includes(kind)) return "single_value";
  if (targetColumns.length === 1 && scenarioIds.length <= 1) return "single_value";
  if (targetColumns.length === 1) return "card_grid";
  if (scenarioIds.length === 1 && targetColumns.length <= 2 && sourceBlockIds.length <= 2) return "table_row";
  return "table";
}

export function unionVisualBounds(bounds: Array<NormalizedVisualBounds | undefined>): NormalizedVisualBounds | undefined {
  const valid = bounds.filter((item): item is NormalizedVisualBounds => Boolean(item));
  if (valid.length === 0) return undefined;
  const left = Math.min(...valid.map((item) => item.x));
  const top = Math.min(...valid.map((item) => item.y));
  const right = Math.max(...valid.map((item) => item.x + item.width));
  const bottom = Math.max(...valid.map((item) => item.y + item.height));
  return { x: left, y: top, width: right - left, height: bottom - top };
}

export function visualReviewRegions(adaptation: PreparedPreAdaptation, numericLayouts: PreparedNumericLayout[] = adaptation.numericLayouts): PreparedVisualReviewRegion[] {
  const explicitRegions = adaptation.visualRegions ?? [];
  const claimedBlocks = new Set(numericLayouts.flatMap((layout) => layout.sourceBlockIds));
  const claimedLayouts = new Set<string>();
  const regions = explicitRegions.map((region) => {
    const textReplacements = adaptation.textReplacements.filter((replacement) => replacement.visualRegionId === region.id && !claimedBlocks.has(replacement.blockId));
    const regionNumericLayouts = numericLayouts.filter((layout) => layout.visualRegionId === region.id);
    textReplacements.forEach((replacement) => claimedBlocks.add(replacement.blockId));
    regionNumericLayouts.forEach((layout) => claimedLayouts.add(layout.id));
    return { ...region, textReplacements, numericLayouts: regionNumericLayouts };
  });
  const legacyTextGroups = groupTextReplacementsByLocation(adaptation.textReplacements.filter((replacement) => !claimedBlocks.has(replacement.blockId)));
  for (const { location, replacements } of legacyTextGroups) {
    regions.push({ id: `copy:${location}`, location, kind: "copy", sourceBlockIds: replacements.map((replacement) => replacement.blockId), visualBounds: unionVisualBounds(replacements.map((replacement) => replacement.visualBounds)), textReplacements: replacements, numericLayouts: [] });
  }
  for (const layout of numericLayouts.filter((item) => !claimedLayouts.has(item.id))) {
    regions.push({ id: `numeric:${layout.id}`, location: layout.location, kind: "numeric", sourceBlockIds: layout.sourceBlockIds, visualBounds: layout.visualBounds, textReplacements: [], numericLayouts: [layout] });
  }
  return regions.filter((region) => region.textReplacements.length > 0 || region.numericLayouts.length > 0);
}

export function hasCompleteTextReplacements(replacements: PreparedTextReplacement[], overrides: Record<string, string> = {}): boolean {
  return replacements.every((replacement) => (
    replacementText(replacement, overrides).trim().length > 0
    && (replacement.status === "ready" || Boolean(overrides[replacement.blockId]?.trim()))
  ));
}

function replacementText(replacement: PreparedTextReplacement, overrides: Record<string, string>): string {
  return overrides[replacement.blockId] ?? replacement.replacementText;
}

export function hasPendingReplacementConfirmation(replacement: PreparedTextReplacement, overrides: Record<string, string> = {}, sources: Record<string, ReplacementSourceChoice> = {}): boolean {
  if (replacement.status !== "recommended" && replacement.status !== "calculated") return false;
  if (!replacementText(replacement, overrides).trim()) return false;
  const source = sources[replacement.blockId];
  return source === undefined;
}

function replacementSource(replacement: PreparedTextReplacement, sources: Record<string, ReplacementSourceChoice>): ReplacementSourceChoice {
  return sources[replacement.blockId]
    ?? { kind: replacement.status === "calculated" ? "calculation" : replacement.status === "recommended" ? "recommendation" : "library" };
}

function replacementRecommendationSource(replacement: PreparedTextReplacement): ReplacementSourceChoice {
  return { kind: replacement.status === "calculated" ? "calculation" : "recommendation" };
}

function compactInstruction(value: string): string {
  return value.split(/\n+/).map((line) => line.trim()).filter(Boolean).join("；");
}

function sameRepaymentPlanChoice(choice: RepaymentPlanChoice, selection: PreparedRepaymentPlanSelection): boolean {
  return choice.planKey === selection.planKey
    && choice.principal === selection.principal
    && choice.tenorMonths === selection.tenorMonths
    && choice.values.principal === selection.values.principal
    && choice.values.tenor === selection.values.tenor
    && choice.values.totalInterest === selection.values.totalInterest
    && choice.values.totalRepayment === selection.values.totalRepayment
    && choice.values.monthlyInstallment === selection.values.monthlyInstallment;
}

export function effectiveRepaymentPlanSelections(
  adaptation: PreparedPreAdaptation,
  repaymentPlanOverrides: Record<string, RepaymentPlanChoice> = {},
  numericLayoutDrafts: Record<string, NumericLayoutDraft> = {},
): PreparedRepaymentPlanSelection[] {
  const layouts = effectiveNumericLayouts(adaptation, numericLayoutDrafts);
  const usedScenarioIds = new Set(layouts.flatMap((layout) => layout.scenarioIds));
  const keepAll = layouts.length === 0;
  const selections = adaptation.repaymentPlanSelections
    .filter((selection) => keepAll || usedScenarioIds.has(selection.id))
    .map((selection) => repaymentPlanSelectionWithChoice(selection, repaymentPlanOverrides[selection.id]));
  for (const draft of Object.values(numericLayoutDrafts)) {
    for (const [scenarioId, choice] of Object.entries(draft.addedScenarios ?? {})) {
      if (!keepAll && !usedScenarioIds.has(scenarioId)) continue;
      selections.push(repaymentPlanSelectionWithChoice(repaymentPlanSelectionFromChoice(scenarioId, choice), repaymentPlanOverrides[scenarioId]));
    }
  }
  const seen = new Set<string>();
  return selections.filter((selection) => {
    if (seen.has(selection.id)) return false;
    seen.add(selection.id);
    return true;
  });
}

export function effectiveNumericLayouts(
  adaptation: PreparedPreAdaptation,
  numericLayoutDrafts: Record<string, NumericLayoutDraft> = {},
): PreparedNumericLayout[] {
  return [...adaptation.numericLayouts, ...pendingNumericLayouts(adaptation)].map((layout) => {
    const draft = numericLayoutDrafts[layout.id] ?? {};
    const removed = new Set((draft.removedScenarioIds ?? []).map((scenarioId) => scenarioId.trim()).filter(Boolean));
    const addedScenarioIds = Object.keys(draft.addedScenarios ?? {}).filter((scenarioId) => !removed.has(scenarioId));
    const scenarioIds = Array.from(new Set([
      ...layout.scenarioIds.filter((scenarioId) => !removed.has(scenarioId)),
      ...addedScenarioIds,
    ]));
    return { ...layout, scenarioIds };
  });
}

function repaymentPlanSelectionWithChoice(selection: PreparedRepaymentPlanSelection, choice: RepaymentPlanChoice | undefined): PreparedRepaymentPlanSelection {
  return choice
    ? { ...selection, planKey: choice.planKey, principal: choice.principal, tenorMonths: choice.tenorMonths, values: choice.values }
    : selection;
}

function repaymentPlanSelectionFromChoice(id: string, choice: RepaymentPlanChoice): PreparedRepaymentPlanSelection {
  return {
    id,
    planKey: choice.planKey,
    principal: choice.principal,
    tenorMonths: choice.tenorMonths,
    values: choice.values,
  };
}

function scenarioByID(scenarios: PreparedRepaymentPlanSelection[]): Map<string, PreparedRepaymentPlanSelection> {
  return new Map(scenarios.map((scenario) => [scenario.id, scenario]));
}

function numericLayoutByID(layouts: PreparedNumericLayout[]): Map<string, PreparedNumericLayout> {
  return new Map(layouts.map((layout) => [layout.id, layout]));
}

function numericColumnLabel(column: PreparedNumericLayout["targetColumns"][number]): string {
  switch (column) {
    case "principal": return "借款金额";
    case "tenor": return "期限";
    case "monthly_installment": return "每月还款";
    case "total_interest": return "总利息";
    case "total_repayment": return "总还款";
  }
}

function numericColumnValue(scenario: PreparedRepaymentPlanSelection, column: PreparedNumericLayout["targetColumns"][number]): string {
  switch (column) {
    case "principal": return scenario.values.principal;
    case "tenor": return scenario.values.tenor;
    case "monthly_installment": return scenario.values.monthlyInstallment;
    case "total_interest": return scenario.values.totalInterest;
    case "total_repayment": return scenario.values.totalRepayment;
  }
}

function generatedNumericLayoutInstruction(layout: PreparedNumericLayout, scenarios: ReadonlyMap<string, PreparedRepaymentPlanSelection>): string {
  const rows = layout.scenarioIds
    .map((scenarioID) => scenarios.get(scenarioID))
    .filter((scenario): scenario is PreparedRepaymentPlanSelection => Boolean(scenario))
    .map((scenario) => layout.targetColumns.map((column) => `${numericColumnLabel(column)} ${numericColumnValue(scenario, column)}`).join("，"));
  return rows.length > 0
    ? `按当前已审核还款方案保持原版式展示：${rows.join("；")}。保持原图对应区域的对齐、字号层级和细分隔线。`
    : "";
}

function numericLayoutInstruction(
  layout: PreparedNumericLayout,
  scenarios: ReadonlyMap<string, PreparedRepaymentPlanSelection>,
  repaymentPlanOverrides: Record<string, RepaymentPlanChoice> = {},
  originalLayout?: PreparedNumericLayout,
): string {
  const needsGeneratedInstruction = layout.scenarioIds.some((scenarioID) => repaymentPlanOverrides[scenarioID])
    || (originalLayout === undefined && layout.scenarioIds.length > 0)
    || (originalLayout !== undefined && !sameStringArray(layout.scenarioIds, originalLayout.scenarioIds));
  if (!needsGeneratedInstruction) return compactInstruction(layout.renderInstruction);
  return generatedNumericLayoutInstruction(layout, scenarios) || compactInstruction(layout.renderInstruction);
}

function sameStringArray(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

export function frozenCopySnapshot(
  copy: CreativeCopySnapshot,
  adaptation: PreparedPreAdaptation,
  draft: OrderItemDraft,
  sourceAnalysisID: string,
): CreativeCopySnapshot {
  const repaymentPlanOverrides = draft.repaymentPlanOverrides ?? {};
  const numericLayoutDrafts = draft.numericLayoutDrafts ?? {};
  const repaymentPlanSelections = effectiveRepaymentPlanSelections(adaptation, repaymentPlanOverrides, numericLayoutDrafts);
  const repaymentPlanSelectionsByID = scenarioByID(repaymentPlanSelections);
  const numericLayouts = effectiveNumericLayouts(adaptation, numericLayoutDrafts).filter((layout) => layout.scenarioIds.length > 0);
  const numericLayoutBlockIDs = new Set(numericLayouts.flatMap((layout) => layout.sourceBlockIds));
  const originalLayoutsByID = numericLayoutByID(adaptation.numericLayouts);
  return {
    ...copy,
    visual_direction: draft.visualDirection,
    pre_adaptation: {
      schema_version: 1,
      source_analysis_id: sourceAnalysisID,
      summary: adaptation.summary,
      analysis_highlights: adaptation.analysisHighlights,
      text_replacements: adaptation.textReplacements.filter((replacement) => !numericLayoutBlockIDs.has(replacement.blockId)).map((replacement) => {
        const replacementTextValue = replacementText(replacement, draft.textOverrides);
        const source = replacementSource(replacement, draft.replacementSources ?? {});
        const ready = replacementTextValue.trim().length > 0;
        return {
        block_id: replacement.blockId,
        location: replacement.location,
        role: replacement.role,
        semantic_kind: replacement.semanticKind ?? "",
        source_text: replacement.sourceText,
        replacement_text: replacementTextValue,
        source_kind: source.kind,
        source_keys: source.kind === "library" ? (source.sourceKey ? [source.sourceKey] : replacement.sourceKeys) : [],
        status: ready ? "ready" : "missing",
        note: replacement.note,
        recommendation_basis: replacement.recommendationBasis ?? [],
        calculation: replacement.calculation,
        };
      }),
      repayment_plan_selections: repaymentPlanSelections.map((scenario) => ({
        id: scenario.id,
        plan_key: scenario.planKey,
        principal: scenario.principal,
        tenor_months: scenario.tenorMonths,
        values: {
          principal: scenario.values.principal,
          tenor: scenario.values.tenor,
          total_interest: scenario.values.totalInterest,
          total_repayment: scenario.values.totalRepayment,
          monthly_installment: scenario.values.monthlyInstallment,
        },
      })),
      numeric_layouts: numericLayouts.map((layout) => ({
        id: layout.id,
        source_block_ids: layout.sourceBlockIds,
        location: layout.location,
        layout_kind: layout.layoutKind,
        scenario_ids: layout.scenarioIds,
        target_columns: layout.targetColumns,
        render_instruction: numericLayoutInstruction(layout, repaymentPlanSelectionsByID, repaymentPlanOverrides, originalLayoutsByID.get(layout.id)),
      })),
    },
  };
}

export function preAdaptedCopySnapshot(
  library: { id: string; published_version: number },
  adaptation: PreparedPreAdaptation,
  draft: OrderItemDraft,
  sourceAnalysisID: string,
): CreativeCopySnapshot {
  const textForRole = (role: string) => adaptation.textReplacements
    .filter((replacement) => replacement.role === role)
    .map((replacement) => replacementText(replacement, draft.textOverrides))
    .filter(Boolean)
    .join("\n");
  const hasPlan = effectiveRepaymentPlanSelections(adaptation, draft.repaymentPlanOverrides ?? {}, draft.numericLayoutDrafts ?? {}).length > 0;
  const snapshot: CreativeCopySnapshot = {
    schema_version: 3,
    id: "model-pre-adaptation",
    library_id: library.id,
    library_version: library.published_version,
    composition_id: "model-pre-adaptation",
    composition_key: "model-pre-adaptation",
    creative_type: hasPlan ? "repayment_plan" : "num",
    headline: textForRole("headline"),
    subheadline: textForRole("subheadline"),
    benefit: textForRole("benefit"),
    supporting: textForRole("supporting"),
    cta: textForRole("cta"),
    legal_text: textForRole("legal"),
    fragments: [],
    repayment_plan_entries: [],
    recommendation: { score: 0, reasons: ["模型基于原图逐块填充"], matched_signals: [] },
    status: "model_pre_adapted",
    visual_direction: draft.visualDirection,
  };
  return frozenCopySnapshot(snapshot, adaptation, draft, sourceAnalysisID);
}

function PreAdaptationSummary({ adaptation }: { adaptation: PreparedPreAdaptation }) {
  if (adaptation.status === "unavailable" && adaptation.errorCode === "manual_confirmation_required") return <p className="mt-2 text-xs text-amber-700">待人工确认：{adaptation.errorMessage || adaptation.summary}</p>;
  if (adaptation.status === "unavailable") return <p className="mt-2 text-xs text-amber-700">文案库缺少同机制已审核文案：{adaptation.gaps.join("、") || adaptation.summary}</p>;
  const pendingRecommendations = adaptation.textReplacements.filter((replacement) => replacement.status === "recommended" || replacement.status === "calculated").length;
  if (pendingRecommendations > 0) return <p className="mt-2 text-xs text-amber-800">已完成逐块适配，其中 {pendingRecommendations} 个系统候选待确认后才能提交。</p>;
  const pendingPlans = pendingNumericLayouts(adaptation).length;
  if (pendingPlans > 0) return <p className="mt-2 text-xs text-amber-800">已识别还款计划区域，其中 {pendingPlans} 个区域未匹配到冻结方案，可在确认页选择已审核方案或留空移除。</p>;
  return <p className="mt-2 text-xs text-emerald-800">已完成逐块文字填充：{adaptation.summary || adaptation.reasons[0] || "按原图机制匹配"}</p>;
}

export function groupTextReplacementsByLocation(replacements: PreparedTextReplacement[]): { location: string; replacements: PreparedTextReplacement[] }[] {
  const groups = new Map<string, PreparedTextReplacement[]>();
  for (const replacement of replacements) {
    const location = replacement.location.trim();
    const group = groups.get(location);
    if (group) group.push(replacement);
    else groups.set(location, [replacement]);
  }
  return [...groups.entries()].map(([location, groupedReplacements]) => ({ location, replacements: groupedReplacements }));
}

function VisualDirectionEditor({ value, onChange }: { value: CreativeVisualDirection; onChange: (value: CreativeVisualDirection) => void }) {
  const updateList = (key: "style_tags" | "must_preserve" | "avoid", raw: string) => {
    onChange({ ...value, [key]: raw.split(/[\n,，、]/).map((item) => item.trim()).filter(Boolean) });
  };
  return <section className="border-t px-3 py-3" aria-labelledby="visual-direction-title">
    <div className="mb-3"><h4 id="visual-direction-title" className="text-sm font-semibold">主题与风格约束</h4><p className="mt-0.5 text-[11px] text-muted-foreground">只描述希望保留的视觉方向；最终提示词由出图智能体根据原图继承和目标尺寸自行组织。</p></div>
    <div className="grid gap-3 lg:grid-cols-2">
      <div className="min-w-0 space-y-1.5 lg:col-span-2"><Label htmlFor="creative-visual-theme" className="text-xs text-muted-foreground">主题</Label><Input id="creative-visual-theme" value={value.theme} onChange={(event) => onChange({ ...value, theme: event.target.value })} placeholder="例如：灵活融资、轻量可信、适合移动端阅读" /></div>
      <div className="min-w-0 space-y-1.5"><Label htmlFor="creative-visual-style-tags" className="text-xs text-muted-foreground">风格标签</Label><Input id="creative-visual-style-tags" value={value.style_tags.join("、")} onChange={(event) => updateList("style_tags", event.target.value)} placeholder="例如：红黑高对比、信息型、现代" /></div>
      <div className="min-w-0 space-y-1.5"><Label htmlFor="creative-visual-preserve" className="text-xs text-muted-foreground">必须保留</Label><Input id="creative-visual-preserve" value={value.must_preserve.join("、")} onChange={(event) => updateList("must_preserve", event.target.value)} placeholder="例如：原图阅读顺序、主体层次、关键图形" /></div>
      <div className="min-w-0 space-y-1.5 lg:col-span-2"><Label htmlFor="creative-visual-avoid" className="text-xs text-muted-foreground">避免</Label><Input id="creative-visual-avoid" value={value.avoid.join("、")} onChange={(event) => updateList("avoid", event.target.value)} placeholder="例如：拥挤底部、遮挡图标、竞品品牌识别" /></div>
    </div>
  </section>;
}

function TextReplacementPlan({ sourceImage, sourceImageAlt, adaptation, copyLibrary, overrides, replacementSources, repaymentPlanOverrides, numericLayoutDrafts, repaymentPlanOptions, visualDirection, onVisualDirectionChange, onChooseReplacement, onChooseRepaymentPlan, onAddRepaymentPlan, onRemoveRepaymentPlan, onOpenCopyLibrary, onRetrySourceAnalysis, retryingSourceAnalysis = false }: { sourceImage: string; sourceImageAlt: string; adaptation: PreparedPreAdaptation; copyLibrary?: CreativeResource; overrides: Record<string, string>; replacementSources: Record<string, ReplacementSourceChoice>; repaymentPlanOverrides: Record<string, RepaymentPlanChoice>; numericLayoutDrafts: Record<string, NumericLayoutDraft>; repaymentPlanOptions: RepaymentPlanChoice[]; visualDirection: CreativeVisualDirection; onVisualDirectionChange: (value: CreativeVisualDirection) => void; onChooseReplacement: (blockId: string, value: string, source: ReplacementSourceChoice) => void; onChooseRepaymentPlan: (scenarioId: string, plan: RepaymentPlanChoice, original: PreparedRepaymentPlanSelection) => void; onAddRepaymentPlan: (layoutId: string, plan: RepaymentPlanChoice) => void; onRemoveRepaymentPlan: (layoutId: string, scenarioId: string) => void; onOpenCopyLibrary?: () => void; onRetrySourceAnalysis?: () => void; retryingSourceAnalysis?: boolean }) {
  const effectiveLayouts = effectiveNumericLayouts(adaptation, numericLayoutDrafts);
  const effectiveScenarios = effectiveRepaymentPlanSelections(adaptation, repaymentPlanOverrides, numericLayoutDrafts);
  const pendingNumericPlanLayouts = effectiveLayouts.filter((layout) => isPendingNumericLayout(layout) && layout.scenarioIds.length === 0);
  const pendingRecommendations = adaptation.textReplacements.filter((replacement) => hasPendingReplacementConfirmation(replacement, overrides, replacementSources));
  const reviewRegions = useMemo(() => visualReviewRegions(adaptation, effectiveLayouts), [adaptation, effectiveLayouts]);
  const missingReplacements = reviewRegions.flatMap((region) => region.textReplacements).filter((replacement) => !replacementText(replacement, overrides).trim());
  const positionedRegions = useMemo(() => reviewRegions.filter((region): region is PreparedVisualReviewRegion & { visualBounds: NormalizedVisualBounds } => Boolean(region.visualBounds)), [reviewRegions]);
  const [activeRegionId, setActiveRegionId] = useState(positionedRegions[0]?.id ?? reviewRegions[0]?.id ?? "");
  const [selectorBlockId, setSelectorBlockId] = useState("");
  const selectorReplacement = adaptation.textReplacements.find((replacement) => replacement.blockId === selectorBlockId) ?? null;
  useEffect(() => {
    if (reviewRegions.some((region) => region.id === activeRegionId)) return;
    setActiveRegionId(positionedRegions[0]?.id ?? reviewRegions[0]?.id ?? "");
  }, [activeRegionId, positionedRegions, reviewRegions]);
  const selectRegion = (regionId: string, shouldScroll = false) => {
    setActiveRegionId(regionId);
    if (shouldScroll) document.getElementById(`visual-review-region-${regionId}`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  };
  const visualIndexByRegionId = new Map(positionedRegions.map((region, index) => [region.id, index + 1]));
  return <section aria-labelledby="text-replacement-title" className="min-w-0 border">
    <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2"><div><h4 id="text-replacement-title" className="text-sm font-semibold">画面内容核对</h4><p className="mt-0.5 text-[11px] text-muted-foreground">对着原图逐区确认文案和数值；系统生成的候选会先标记待确认。</p></div><div className="flex flex-wrap items-center gap-2">{pendingRecommendations.length > 0 && <Badge variant="outline" className="border-amber-300 bg-amber-50 text-amber-900">{pendingRecommendations.length} 个推荐待确认</Badge>}{pendingNumericPlanLayouts.length > 0 && <Badge variant="outline" className="border-amber-300 bg-amber-50 text-amber-900">{pendingNumericPlanLayouts.length} 个数值区域待选择</Badge>}<Badge variant={missingReplacements.length === 0 && pendingRecommendations.length === 0 && pendingNumericPlanLayouts.length === 0 ? "default" : "outline"}>{missingReplacements.length > 0 ? `${missingReplacements.length} 个文字可留空移除` : pendingRecommendations.length > 0 ? "推荐确认后可提交" : pendingNumericPlanLayouts.length > 0 ? "请选择还款方案或留空移除" : `${reviewRegions.length} 个区域已处理`}</Badge></div></div>
    <div className="grid min-w-0 xl:grid-cols-[minmax(360px,0.9fr)_minmax(0,1.1fr)] xl:items-start"><SourceTextVisualReview sourceImage={sourceImage} sourceImageAlt={sourceImageAlt} regions={positionedRegions} activeRegionId={activeRegionId} onSelect={(regionId) => selectRegion(regionId, true)} onRetrySourceAnalysis={onRetrySourceAnalysis} retryingSourceAnalysis={retryingSourceAnalysis} />
      <div className="min-w-0 border-t xl:border-l xl:border-t-0"><div className="space-y-3 p-3">{reviewRegions.map((region) => <VisualReviewRegionCard key={region.id} region={region} visualIndex={visualIndexByRegionId.get(region.id)} active={activeRegionId === region.id} overrides={overrides} replacementSources={replacementSources} scenarios={effectiveScenarios} originalScenarios={adaptation.repaymentPlanSelections} originalNumericLayouts={adaptation.numericLayouts} repaymentPlanOverrides={repaymentPlanOverrides} repaymentPlanOptions={repaymentPlanOptions} onSelect={() => selectRegion(region.id)} onOpenSelector={setSelectorBlockId} onChooseReplacement={onChooseReplacement} onChooseRepaymentPlan={onChooseRepaymentPlan} onAddRepaymentPlan={onAddRepaymentPlan} onRemoveRepaymentPlan={onRemoveRepaymentPlan} />)}</div></div>
    </div>
    <VisualDirectionEditor value={visualDirection} onChange={onVisualDirectionChange} />
    <ReplacementSourceDialog open={selectorReplacement !== null} replacement={selectorReplacement} currentValue={selectorReplacement ? replacementText(selectorReplacement, overrides) : ""} copyLibrary={copyLibrary} onOpenChange={(open) => !open && setSelectorBlockId("")} onChoose={(value, source) => { if (selectorReplacement) onChooseReplacement(selectorReplacement.blockId, value, source); setSelectorBlockId(""); }} onOpenCopyLibrary={onOpenCopyLibrary} />
  </section>;
}

function VisualReviewRegionCard({ region, visualIndex, active, overrides, replacementSources, scenarios, originalScenarios, originalNumericLayouts, repaymentPlanOverrides, repaymentPlanOptions, onSelect, onOpenSelector, onChooseReplacement, onChooseRepaymentPlan, onAddRepaymentPlan, onRemoveRepaymentPlan }: { region: PreparedVisualReviewRegion; visualIndex?: number; active: boolean; overrides: Record<string, string>; replacementSources: Record<string, ReplacementSourceChoice>; scenarios: PreparedRepaymentPlanSelection[]; originalScenarios: PreparedRepaymentPlanSelection[]; originalNumericLayouts: PreparedNumericLayout[]; repaymentPlanOverrides: Record<string, RepaymentPlanChoice>; repaymentPlanOptions: RepaymentPlanChoice[]; onSelect: () => void; onOpenSelector: (blockId: string) => void; onChooseReplacement: (blockId: string, value: string, source: ReplacementSourceChoice) => void; onChooseRepaymentPlan: (scenarioId: string, plan: RepaymentPlanChoice, original: PreparedRepaymentPlanSelection) => void; onAddRepaymentPlan: (layoutId: string, plan: RepaymentPlanChoice) => void; onRemoveRepaymentPlan: (layoutId: string, scenarioId: string) => void }) {
  return <section id={`visual-review-region-${region.id}`} className={`min-w-0 border transition-colors ${active ? "border-foreground bg-muted/20" : ""}`} onClick={onSelect}>
    <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/20 px-3 py-2"><div className="flex min-w-0 items-center gap-2"><span className="break-words text-xs font-medium">{region.location}</span>{visualIndex && <span className="inline-flex size-5 items-center justify-center rounded-full bg-foreground font-mono text-[10px] font-semibold text-background">{visualIndex}</span>}</div><Badge variant="outline">{region.kind === "numeric" ? "数值组件" : "文案区域"}</Badge></div>
    {region.textReplacements.map((replacement) => {
      const source = replacementSource(replacement, replacementSources);
      const value = replacementText(replacement, overrides);
      const pendingConfirmation = hasPendingReplacementConfirmation(replacement, overrides, replacementSources);
      const unavailable = replacement.status === "missing" && replacementSources[replacement.blockId] === undefined && !value.trim();
      const sourceLabel = unavailable ? "暂无可用内容" : pendingConfirmation ? "待确认推荐" : source.kind === "library" ? "已审核文案库" : source.kind === "calculation" ? "公式计算" : source.kind === "recommendation" ? "已采用系统推荐" : "本次人工改写";
      const sourceTone = unavailable || pendingConfirmation ? "border-amber-300 bg-amber-50 text-amber-900" : source.kind === "calculation" ? "border-cyan-300 bg-cyan-50 text-cyan-900" : source.kind === "manual" ? "border-slate-300 bg-slate-50 text-slate-800" : "border-emerald-300 bg-emerald-50 text-emerald-900";
      const detail = replacement.status === "calculated" && replacement.calculation ? `${replacement.calculation.formula}；${replacement.calculation.inputs.join("；")}` : (replacement.recommendationBasis ?? []).join("；") || replacement.note || (replacement.status === "missing" ? (region.kind === "numeric" ? "未匹配到冻结方案，可选择已审核方案或留空移除。" : "没有可自动采用的内容。") : "来自当前冻结文案库。");
      return <div key={replacement.blockId} className="grid min-w-0 gap-4 border-b p-3 last:border-b-0 xl:grid-cols-[minmax(180px,0.8fr)_minmax(0,1.2fr)] xl:items-start"><div className="min-w-0 xl:border-r xl:pr-4"><p className="text-[11px] font-medium text-muted-foreground">原图文字</p><p className="mt-1 whitespace-pre-wrap break-words text-sm leading-6 text-foreground">{replacement.sourceText || "未识别清晰文字"}</p>{replacement.semanticKind && <p className="mt-2 font-mono text-[10px] text-muted-foreground">{replacement.semanticKind}</p>}</div><div className="min-w-0"><div className="flex flex-wrap items-center justify-between gap-2"><p className="text-[11px] font-medium text-muted-foreground">最终替换文字</p><Badge variant="outline" className={sourceTone}>{sourceLabel}</Badge></div><Textarea rows={2} value={value} onFocus={onSelect} onChange={(event) => onChooseReplacement(replacement.blockId, event.target.value, { kind: "manual" })} placeholder="留空则移除原文" className="mt-1 min-h-12 resize-y text-sm leading-6" /><div className="mt-2 flex flex-wrap items-center gap-2">{pendingConfirmation && <Button size="sm" onClick={(event) => { event.stopPropagation(); onChooseReplacement(replacement.blockId, replacement.replacementText, replacementRecommendationSource(replacement)); }}><Check className="h-4 w-4" />{replacement.status === "calculated" ? "确认计算" : "确认推荐"}</Button>}<Button size="sm" variant="outline" onClick={(event) => { event.stopPropagation(); onOpenSelector(replacement.blockId); }}><BookOpenText className="h-4 w-4" />选择来源</Button></div><p className={`mt-2 break-words border-l-2 pl-2 text-[11px] leading-4 ${replacement.status === "missing" ? "border-amber-300 text-amber-800" : "border-muted-foreground/30 text-muted-foreground"}`}>{detail}</p></div></div>;
    })}
    {region.numericLayouts.length > 0 && <div className="p-3"><p className="mb-2 text-[11px] font-medium text-muted-foreground">我方数值版式</p><NumericLayoutCards layouts={region.numericLayouts} scenarios={scenarios} originalScenarios={originalScenarios} originalNumericLayouts={originalNumericLayouts} repaymentPlanOverrides={repaymentPlanOverrides} repaymentPlanOptions={repaymentPlanOptions} onChooseRepaymentPlan={onChooseRepaymentPlan} onAddRepaymentPlan={onAddRepaymentPlan} onRemoveRepaymentPlan={onRemoveRepaymentPlan} /></div>}
  </section>;
}

function ReplacementSourceDialog({ open, replacement, currentValue, copyLibrary, onOpenChange, onChoose, onOpenCopyLibrary }: { open: boolean; replacement: PreparedTextReplacement | null; currentValue: string; copyLibrary?: CreativeResource; onOpenChange: (open: boolean) => void; onChoose: (value: string, source: ReplacementSourceChoice) => void; onOpenCopyLibrary?: () => void }) {
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<"library" | "manual">("library");
  const [manualValue, setManualValue] = useState("");
  useEffect(() => { setQuery(""); setMode("library"); setManualValue(currentValue); }, [replacement?.blockId, currentValue]);
  const fragments = useMemo(() => copyLibrary ? parseCreativeCopyLibraryConfig(copyLibrary.published_config ?? {}).fragments.filter((fragment) => fragment.status === "approved") : [], [copyLibrary]);
  const compatible = useMemo(() => fragments.filter((fragment) => replacement ? fragmentSupportsReplacement(fragment, replacement) : false), [fragments, replacement]);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const visible = compatible.filter((fragment) => !normalizedQuery || [fragment.text, fragment.name, fragment.key].join(" ").toLocaleLowerCase().includes(normalizedQuery));
  const recommendation = replacement?.status === "recommended" || replacement?.status === "calculated" ? replacement : null;
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-h-[90vh] overflow-y-auto p-0 sm:max-w-[min(94vw,820px)]"><DialogHeader className="border-b px-5 py-4 pr-12"><DialogTitle className="text-base">选择替换内容</DialogTitle><DialogDescription>{replacement ? `${replacement.location} · 原图：${replacement.sourceText || "未识别文字"}` : ""}</DialogDescription></DialogHeader><div className="space-y-4 px-5 py-4">{recommendation && <div className={`border p-3 ${replacement?.status === "calculated" ? "border-cyan-300 bg-cyan-50/70" : "border-amber-300 bg-amber-50/70"}`}><div className="flex flex-wrap items-center justify-between gap-2"><div><p className="text-sm font-medium">{replacement?.status === "calculated" ? "公式计算建议" : "系统推荐"}</p><p className="mt-1 text-xs text-muted-foreground">{replacement?.status === "calculated" ? replacement?.calculation?.formula : (recommendation.recommendationBasis ?? []).join("；")}</p></div><Button size="sm" onClick={() => onChoose(recommendation.replacementText, { kind: replacement?.status === "calculated" ? "calculation" : "recommendation" })}><Check className="h-4 w-4" />采用本次建议</Button></div><p className="mt-3 break-words text-sm font-medium">{recommendation.replacementText}</p>{replacement?.calculation && <p className="mt-1 text-xs text-muted-foreground">{replacement.calculation.inputs.join("；")}</p>}</div>}<div className="flex w-fit border p-0.5"><Button size="sm" variant={mode === "library" ? "secondary" : "ghost"} onClick={() => setMode("library")}>已审核文案</Button><Button size="sm" variant={mode === "manual" ? "secondary" : "ghost"} onClick={() => setMode("manual")}>本次改写</Button></div>{mode === "library" ? <div className="space-y-3"><div className="flex flex-wrap items-end justify-between gap-2"><div><p className="text-sm font-medium">匹配当前语义的已审核文案</p><p className="mt-1 text-xs text-muted-foreground">只展示与当前文字区域兼容的文案，避免将利率、金额和期限混用。</p></div><Badge variant="outline">{compatible.length} 条可用</Badge></div><Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索文案库" aria-label="搜索可用文案" />{visible.length > 0 ? <div className="divide-y border">{visible.map((fragment) => <button key={fragment.id} type="button" className="grid w-full grid-cols-[minmax(0,1fr)_auto] gap-3 p-3 text-left hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => onChoose(fragment.text, { kind: "library", sourceKey: fragment.key })}><div className="min-w-0"><p className="break-words text-sm font-medium">{fragment.text}</p><p className="mt-1 text-xs text-muted-foreground">{fragment.name || fragment.key}{fragment.semantic_group ? ` · ${fragment.semantic_group}` : ""}</p></div><Check className="mt-0.5 h-4 w-4 text-muted-foreground" /></button>)}</div> : <div className="border border-dashed px-4 py-7 text-center"><p className="text-sm font-medium">没有可用的已审核文案</p><p className="mt-1 text-xs text-muted-foreground">可采用系统建议，或仅为本次订单人工改写。</p>{onOpenCopyLibrary && <Button size="sm" variant="outline" className="mt-3" onClick={onOpenCopyLibrary}>前往文案库</Button>}</div>}</div> : <div className="space-y-3"><div><p className="text-sm font-medium">仅用于本次订单</p><p className="mt-1 text-xs text-muted-foreground">这不会自动写入文案库；需要复用时再到文案库创建草稿并发布。</p></div><Textarea rows={4} value={manualValue} onChange={(event) => setManualValue(event.target.value)} placeholder="输入本次要使用的文字；留空则移除原文" /><div className="flex justify-end"><Button disabled={!manualValue.trim()} onClick={() => onChoose(manualValue, { kind: "manual" })}>采用本次改写</Button></div></div>}</div><DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button></DialogFooter></DialogContent></Dialog>;
}

function fragmentSupportsReplacement(fragment: ReturnType<typeof parseCreativeCopyLibraryConfig>["fragments"][number], replacement: PreparedTextReplacement): boolean {
  const semanticKind = replacement.semanticKind ?? "";
  if (!semanticKind || semanticKind === "copy" || semanticKind === "daily_interest_label") return fragment.role === replacement.role || fragment.role === "supporting";
  if (semanticKind === "principal") return fragment.semantic_group === "principal" || fragment.semantic_group === "limit";
  return fragment.semantic_group === semanticKind;
}

function SourceTextVisualReview({ sourceImage, sourceImageAlt, regions, activeRegionId, onSelect, onRetrySourceAnalysis, retryingSourceAnalysis = false }: { sourceImage: string; sourceImageAlt: string; regions: Array<PreparedVisualReviewRegion & { visualBounds: NormalizedVisualBounds }>; activeRegionId: string; onSelect: (regionId: string) => void; onRetrySourceAnalysis?: () => void; retryingSourceAnalysis?: boolean }) {
  return <aside aria-label="原图内容定位" className="min-w-0 bg-muted/10 xl:sticky xl:top-3">
    <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
      <div>
        <p className="text-sm font-medium">原图内容定位</p>
        <p className="mt-0.5 text-[11px] text-muted-foreground">选中区域使用暖金描边；编号置于区域外侧。</p>
      </div>
      <Badge variant="outline">{regions.length} 个区域</Badge>
    </div>
    {sourceImage && regions.length > 0 ? <div className="p-3">
      <div className="relative mx-auto w-full max-w-[720px] p-7">
        <div className="relative">
          <img src={sourceImage} alt={sourceImageAlt} width={1200} height={900} className="block h-auto w-full" />
          <div className="absolute inset-0">
            {regions.map((region, index) => {
              const active = region.id === activeRegionId;
              const bounds = region.visualBounds;
              return <button
                key={region.id}
                type="button"
                aria-label={`定位内容区域 ${index + 1}：${region.location}`}
                aria-pressed={active}
                title={region.location}
                onClick={() => onSelect(region.id)}
                style={{ left: `${bounds.x / 10}%`, top: `${bounds.y / 10}%`, width: `${bounds.width / 10}%`, height: `${bounds.height / 10}%` }}
                className={`absolute border-2 text-left outline-none transition-[background-color,border-color,box-shadow,transform] focus-visible:ring-4 focus-visible:ring-amber-300/70 focus-visible:ring-offset-2 ${active ? "z-20 scale-[1.01] border-[#d6a84f] bg-[#d6a84f]/20 shadow-[0_0_0_2px_#ffffff,0_0_0_5px_#111827,0_0_0_8px_#d6a84f]" : "border-white bg-transparent shadow-[0_0_0_1px_#111827,0_0_0_3px_#ffffff] hover:border-[#e2c47a] hover:bg-[#e2c47a]/15 hover:shadow-[0_0_0_2px_#111827,0_0_0_5px_#e2c47a]"}`}
              >
                <span className={`pointer-events-none absolute -left-1 -top-1 grid -translate-x-full -translate-y-full place-items-center rounded-full border-2 font-mono font-semibold ${active ? "size-7 border-[#d6a84f] bg-[#111827] text-xs text-[#f7d98a] shadow-[0_0_0_2px_#ffffff,0_0_0_5px_#d6a84f]" : "size-5 border-[#111827] bg-white text-[10px] text-[#111827] shadow-[0_0_0_2px_#ffffff,0_0_0_4px_#111827]"}`}>
                  {index + 1}
                </span>
              </button>;
            })}
          </div>
        </div>
      </div>
    </div> : <div className="flex min-h-48 flex-col items-center justify-center gap-2 px-6 py-8 text-center">
      <ImageIcon className="h-5 w-5 text-muted-foreground" />
      <p className="text-sm font-medium">本次分析没有可用的画面区域坐标</p>
      <p className="max-w-sm text-xs leading-5 text-muted-foreground">仍可在右侧逐项编辑；重新分析该素材后，系统会记录区域归属和位置。</p>
      {onRetrySourceAnalysis && <Button size="sm" variant="outline" disabled={retryingSourceAnalysis} onClick={onRetrySourceAnalysis}><RefreshCw className={`h-4 w-4 ${retryingSourceAnalysis ? "animate-spin" : ""}`} />{retryingSourceAnalysis ? "正在重新分析" : "重新分析"}</Button>}
    </div>}
  </aside>;
}

function NumericLayoutCards({
  layouts,
  scenarios,
  originalScenarios,
  originalNumericLayouts,
  repaymentPlanOverrides,
  repaymentPlanOptions,
  onChooseRepaymentPlan,
  onAddRepaymentPlan,
  onRemoveRepaymentPlan,
}: {
  layouts: PreparedNumericLayout[];
  scenarios: PreparedRepaymentPlanSelection[];
  originalScenarios: PreparedRepaymentPlanSelection[];
  originalNumericLayouts: PreparedNumericLayout[];
  repaymentPlanOverrides: Record<string, RepaymentPlanChoice>;
  repaymentPlanOptions: RepaymentPlanChoice[];
  onChooseRepaymentPlan: (scenarioId: string, plan: RepaymentPlanChoice, original: PreparedRepaymentPlanSelection) => void;
  onAddRepaymentPlan: (layoutId: string, plan: RepaymentPlanChoice) => void;
  onRemoveRepaymentPlan: (layoutId: string, scenarioId: string) => void;
}) {
  const [manualEditor, setManualEditor] = useState<{
    layout: PreparedNumericLayout;
    scenarioId?: string;
    originalScenario?: PreparedRepaymentPlanSelection;
    initialChoice?: RepaymentPlanChoice;
  } | null>(null);
  const scenariosByID = scenarioByID(scenarios);
  const originalScenariosByID = scenarioByID(originalScenarios);
  const originalLayoutsByID = numericLayoutByID(originalNumericLayouts);
  const planByKey = new Map(repaymentPlanOptions.map((plan) => [plan.planKey, plan]));
  return <>
  <div className="grid gap-3">{layouts.map((layout) => {
    const instruction = numericLayoutInstruction(layout, scenariosByID, repaymentPlanOverrides, originalLayoutsByID.get(layout.id));
    const nextPlan = nextRepaymentPlanOptionForLayout(layout, scenariosByID, repaymentPlanOptions);
    const pendingSelection = isPendingNumericLayout(layout) && layout.scenarioIds.length === 0;
    return <div key={layout.id} className="min-w-0 border bg-muted/20">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
        <span className="break-words text-xs font-medium">{layout.location}</span>
        <Badge variant="outline">{numericLayoutKindLabel(layout.layoutKind)}</Badge>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[720px] text-xs">
          <thead className="border-b text-left text-muted-foreground">
            <tr>
              <th className="w-[300px] px-2 py-2 font-medium">已审核方案</th>
              {layout.targetColumns.map((column) => <th key={column} className="break-words px-2 py-2 font-medium">{numericColumnLabel(column)}</th>)}
              <th className="w-24 px-2 py-2 font-medium"><span className="sr-only">操作</span></th>
            </tr>
          </thead>
          <tbody>
            {layout.scenarioIds.map((scenarioID) => {
              const scenario = scenariosByID.get(scenarioID);
              if (!scenario) return null;
              const originalScenario = originalScenariosByID.get(scenarioID) ?? scenario;
              const current = repaymentPlanOverrides[scenarioID] ?? repaymentPlanChoiceFromSelection(scenario);
              return <tr key={scenarioID} className="border-b last:border-b-0">
                <td className="px-2 py-2 align-top">
                  {repaymentPlanOptions.length > 0 ? <NativeSelect
                    size="sm"
                    className="w-full"
                    aria-label={`选择${layout.location}还款方案`}
                    value={current.planKey}
                    onChange={(event) => {
                      const plan = planByKey.get(event.target.value);
                      if (plan) onChooseRepaymentPlan(scenarioID, plan, originalScenario);
                    }}
                  >
                    {!planByKey.has(current.planKey) && <NativeSelectOption value={current.planKey}>{repaymentPlanOptionLabel(current)}</NativeSelectOption>}
                    {repaymentPlanOptions.map((plan) => <NativeSelectOption key={plan.planKey} value={plan.planKey}>{repaymentPlanOptionLabel(plan)}</NativeSelectOption>)}
                  </NativeSelect> : <span className="text-muted-foreground">{repaymentPlanOptionLabel(current)}</span>}
                </td>
                {layout.targetColumns.map((column) => <td key={column} className="break-words px-2 py-2 align-top text-foreground">{numericColumnValue(scenario, column)}</td>)}
                <td className="px-2 py-2 align-top">
                  <div className="flex items-center justify-end gap-1">
                    <Button type="button" size="icon" variant="ghost" aria-label={`手填${layout.location}中的数值`} title="手填数值" onClick={() => setManualEditor({ layout, scenarioId: scenarioID, originalScenario, initialChoice: current })}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button type="button" size="icon" variant="ghost" aria-label={`删除${layout.location}中的一行`} onClick={() => onRemoveRepaymentPlan(layout.id, scenarioID)}>
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                </td>
              </tr>;
            })}
            {layout.scenarioIds.length === 0 && <tr><td colSpan={layout.targetColumns.length + 2} className="px-2 py-5 text-center text-muted-foreground">这个数值区域已清空，可继续加一行。</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2">
        <p className="min-w-0 flex-1 whitespace-pre-wrap break-words text-xs leading-5 text-muted-foreground">{pendingSelection ? "未匹配到冻结还款方案；请选择已审核方案，或留空移除该数值区域。" : instruction || "已按当前选择更新数值区域。"}</p>
        <div className="flex flex-wrap items-center justify-end gap-2">
          {pendingSelection && repaymentPlanOptions.length > 0 ? <NativeSelect
            size="sm"
            className="min-w-64"
            aria-label={`选择${layout.location}还款方案`}
            value=""
            onChange={(event) => {
              const plan = planByKey.get(event.target.value);
              if (plan) onAddRepaymentPlan(layout.id, plan);
            }}
          >
            <NativeSelectOption value="">选择已审核还款方案</NativeSelectOption>
            {repaymentPlanOptions.map((plan) => <NativeSelectOption key={plan.planKey} value={plan.planKey}>{repaymentPlanOptionLabel(plan)}</NativeSelectOption>)}
          </NativeSelect> : nextPlan && <Button type="button" size="sm" variant="outline" onClick={() => onAddRepaymentPlan(layout.id, nextPlan)}>
            <Plus className="h-4 w-4" />
            加一行
          </Button>}
          <Button type="button" size="sm" variant="outline" onClick={() => setManualEditor({ layout })}>
            <Pencil className="h-4 w-4" />
            手填数值
          </Button>
        </div>
      </div>
    </div>;
  })}</div>
  <ManualRepaymentPlanDialog
    editor={manualEditor}
    onClose={() => setManualEditor(null)}
    onSave={(choice) => {
      if (!manualEditor) return;
      if (manualEditor.scenarioId && manualEditor.originalScenario) {
        onChooseRepaymentPlan(manualEditor.scenarioId, choice, manualEditor.originalScenario);
      } else {
        onAddRepaymentPlan(manualEditor.layout.id, choice);
      }
      setManualEditor(null);
    }}
  />
  </>;
}

function ManualRepaymentPlanDialog({
  editor,
  onClose,
  onSave,
}: {
  editor: {
    layout: PreparedNumericLayout;
    initialChoice?: RepaymentPlanChoice;
  } | null;
  onClose: () => void;
  onSave: (choice: RepaymentPlanChoice) => void;
}) {
  const [values, setValues] = useState<RepaymentPlanChoice["values"]>(EMPTY_REPAYMENT_PLAN_VALUES);
  useEffect(() => {
    setValues(editor ? manualValuesForLayout(editor.layout, editor.initialChoice) : EMPTY_REPAYMENT_PLAN_VALUES);
  }, [editor]);
  const columns = editor?.layout.targetColumns ?? [];
  const canSave = Boolean(editor) && columns.every((column) => numericColumnChoiceValue(values, column).trim());
  const update = (column: PreparedNumericLayout["targetColumns"][number], value: string) => {
    setValues((current) => ({ ...current, [repaymentPlanValueKey(column)]: value }));
  };
  return <Dialog open={editor !== null} onOpenChange={(open) => !open && onClose()}>
    <DialogContent className="max-w-lg">
      <DialogHeader>
        <DialogTitle>手填数值</DialogTitle>
        <DialogDescription>{editor ? `${editor.layout.location} · 仅用于本次订单，不写入文案库。` : ""}</DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        {columns.map((column) => <div key={column} className="space-y-1.5">
          <Label htmlFor={`manual-repayment-${column}`} className="text-xs text-muted-foreground">{numericColumnLabel(column)}</Label>
          <Input
            id={`manual-repayment-${column}`}
            value={numericColumnChoiceValue(values, column)}
            onChange={(event) => update(column, event.target.value)}
            placeholder={manualRepaymentPlaceholder(column)}
          />
        </div>)}
        <p className="text-xs leading-5 text-muted-foreground">生成时会按这里的展示值覆盖当前数值区域；请只填写已确认的业务口径。</p>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>取消</Button>
        <Button disabled={!canSave || !editor} onClick={() => editor && onSave(manualRepaymentPlanChoice(editor.layout.id, values, editor.initialChoice?.planKey))}>采用手填数值</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

function manualValuesForLayout(layout: PreparedNumericLayout, choice: RepaymentPlanChoice | undefined): RepaymentPlanChoice["values"] {
  const values = { ...EMPTY_REPAYMENT_PLAN_VALUES };
  for (const column of layout.targetColumns) {
    values[repaymentPlanValueKey(column)] = choice ? numericColumnChoiceValue(choice.values, column) : "";
  }
  return values;
}

function repaymentPlanValueKey(column: PreparedNumericLayout["targetColumns"][number]): keyof RepaymentPlanChoice["values"] {
  switch (column) {
    case "principal": return "principal";
    case "tenor": return "tenor";
    case "monthly_installment": return "monthlyInstallment";
    case "total_interest": return "totalInterest";
    case "total_repayment": return "totalRepayment";
  }
}

function numericColumnChoiceValue(values: RepaymentPlanChoice["values"], column: PreparedNumericLayout["targetColumns"][number]): string {
  return values[repaymentPlanValueKey(column)];
}

function manualRepaymentPlaceholder(column: PreparedNumericLayout["targetColumns"][number]): string {
  switch (column) {
    case "principal": return "例如：Rp100 Juta";
    case "tenor": return "例如：6 Bulan";
    case "monthly_installment": return "例如：Rp1.405.333";
    case "total_interest": return "例如：Rp431.998";
    case "total_repayment": return "例如：Rp8.431.998";
  }
}

function nextRepaymentPlanOptionForLayout(layout: PreparedNumericLayout, scenarios: ReadonlyMap<string, PreparedRepaymentPlanSelection>, options: RepaymentPlanChoice[]): RepaymentPlanChoice | undefined {
  const usedPlanKeys = new Set(layout.scenarioIds.map((scenarioId) => scenarios.get(scenarioId)?.planKey).filter((planKey): planKey is string => Boolean(planKey)));
  return options.find((option) => !usedPlanKeys.has(option.planKey)) ?? options[0];
}

function repaymentPlanOptionLabel(plan: RepaymentPlanChoice): string {
  return [
    plan.values.principal,
    plan.values.tenor,
    plan.values.monthlyInstallment ? `月还 ${plan.values.monthlyInstallment}` : "",
    plan.values.totalRepayment ? `总还款 ${plan.values.totalRepayment}` : "",
  ].filter(Boolean).join(" / ") || plan.planKey;
}

function numericLayoutKindLabel(kind: PreparedNumericLayout["layoutKind"]): string {
  switch (kind) {
    case "table": return "表格";
    case "card_grid": return "卡片网格";
    case "comparison": return "对照区";
    case "single_card": return "单卡";
    case "single_value": return "单值";
    case "option_buttons": return "选项按钮";
    case "table_row": return "表格行";
  }
}

export function manualCopySnapshot(
  draft: OrderItemDraft,
  creativeType: CreativeCopySnapshot["creative_type"] = "num",
  library?: { id: string; published_version: number },
): CreativeCopySnapshot {
  return {
    schema_version: 3,
    id: "",
    library_id: library?.id ?? "",
    library_version: library?.published_version ?? 0,
    creative_type: creativeType,
    headline: draft.manualHeadline.trim(),
    subheadline: draft.manualSubheadline.trim(),
    benefit: draft.manualBenefit.trim(),
    supporting: draft.manualSupporting.trim(),
    cta: draft.manualCta.trim(),
    legal_text: "",
    fragments: [],
    repayment_plan_entries: [],
    recommendation: { score: 0, reasons: ["用户手动调整"], matched_signals: [] },
    status: "user_custom",
    visual_direction: draft.visualDirection,
  };
}

export function visualDirectionSummary(direction: CreativeVisualDirection): string {
  const parts = [
    direction.theme.trim() ? `主题：${direction.theme.trim()}` : "",
    direction.style_tags.length > 0 ? `风格：${direction.style_tags.join("、")}` : "",
    direction.must_preserve.length > 0 ? `保留：${direction.must_preserve.join("、")}` : "",
    direction.avoid.length > 0 ? `避免：${direction.avoid.join("、")}` : "",
  ].filter(Boolean);
  return parts.join("\n");
}

// This is an identity key, not a security primitive. Keeping it derived from
// the immutable submit payload lets the server recover the same submission
// after a refresh without relying on a truncated client-side list.
export async function creativeSubmissionKey(payload: unknown): Promise<string> {
  const value = JSON.stringify(payload);
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value));
  return `creative:${Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

export function creativeFeedbackIdempotencyKey(
  submissionKey: string,
  subjectType: string,
  subjectId: string,
  eventType: string,
  decision: string,
  contextKey = "",
): string {
  const identity = [subjectType, subjectId, contextKey, eventType, decision || "none"]
    .map((part) => encodeURIComponent(part))
    .join(":");
  return `${submissionKey}:feedback:${identity}`;
}

export function candidateFeedbackIdempotencyKey(candidateId: string, decision: string, actionId: string): string {
  return `creative:feedback:candidate:${encodeURIComponent(candidateId)}:${encodeURIComponent(decision)}:${encodeURIComponent(actionId)}`;
}

export function recommendedCopyViewedFeedbackInput({
  candidateId,
  sourceAnalysisId,
  libraryId,
  libraryVersion,
  compositionId,
  rank,
  recommendationReasons,
  copySnapshot,
}: {
  candidateId: string;
  sourceAnalysisId: string;
  libraryId: string;
  libraryVersion: number;
  compositionId: string;
  rank: number;
  recommendationReasons: string[];
  copySnapshot: CreativeCopySnapshot;
}): CreateCreativeFeedbackRequest {
  const identity = [candidateId, sourceAnalysisId || "none", libraryId, String(libraryVersion), compositionId]
    .map((part) => encodeURIComponent(part))
    .join(":");
  return {
    idempotency_key: `creative:feedback:recommended-copy-viewed:${identity}`,
    issue_id: "",
    subject_type: "recommended_copy",
    subject_id: compositionId,
    event_type: "viewed",
    decision: "",
    context_snapshot: {
      candidate_id: candidateId,
      source_analysis_id: sourceAnalysisId,
      copy_library_id: libraryId,
      copy_library_version: libraryVersion,
      composition_id: compositionId,
      rank,
      recommendation_reasons: recommendationReasons,
      copy_snapshot: copySnapshot,
    },
  };
}

export function recommendedCopyDecisionFeedbackInput({
  submissionKey,
  issueId,
  orderId,
  candidateId,
  selectedCopyId,
  selectedRank,
  libraryId,
  libraryVersion,
}: {
  submissionKey: string;
  issueId: string;
  orderId: string;
  candidateId: string;
  selectedCopyId: string;
  selectedRank: number;
  libraryId: string;
  libraryVersion: number;
}): CreateCreativeFeedbackRequest {
  return {
    idempotency_key: creativeFeedbackIdempotencyKey(submissionKey, "recommended_copy", selectedCopyId, "decision", "accepted", candidateId),
    issue_id: issueId,
    subject_type: "recommended_copy",
    subject_id: selectedCopyId,
    event_type: "decision",
    decision: "accepted",
    context_snapshot: {
      candidate_id: candidateId,
      order_id: orderId,
      selected_rank: selectedRank,
      copy_library_id: libraryId,
      copy_library_version: libraryVersion,
    },
  };
}

export function creativeOrderItemInput(
  candidateId: string,
  sourceAnalysisId: string,
  frozenCopy: Record<string, unknown> | CreativeCopySnapshot,
  direction: string,
): CreateCreativeOrderRequest["items"][number] {
  return {
    candidate_id: candidateId,
    source_analysis_id: sourceAnalysisId,
    copy_snapshot: frozenCopy as unknown as Record<string, unknown>,
    direction: direction.trim(),
  };
}

export function creativeSquadMemberSnapshot(
  members: Pick<SquadMember, "member_type" | "member_id" | "role">[],
): { role: string; agent_id: string }[] {
  return members
    .filter((member) => member.member_type === "agent")
    .map((member) => ({ role: member.role, agent_id: member.member_id }));
}

function recordString(record: Record<string, unknown> | undefined, key: string): string {
  const value = record?.[key];
  return typeof value === "string" ? value : "";
}

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function recordStringArray(record: Record<string, unknown> | undefined, key: string): string[] {
  const value = record?.[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function MaterialPreview({ candidate, analysis, canRetryAnalysis, onClose, onDirectEdit, onRetryAnalysis, retryingAnalysis }: { candidate: CreativeMaterialCandidate | null; analysis?: CreativeSourceAnalysis; canRetryAnalysis: boolean; onClose: () => void; onDirectEdit: () => void; onRetryAnalysis: (candidateId: string) => void; retryingAnalysis: boolean }) {
  const source = candidate ? candidateSource(candidate) : "";
  const original = candidate ? resolvePublicFileUrl(candidate.original_url) : "";
  const details = candidate ? materialCandidateDisplayDetails(candidate) : null;
  const previewPaneRef = useRef<HTMLDivElement>(null);
  const detailPaneRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    previewPaneRef.current?.scrollTo({ top: 0 });
    detailPaneRef.current?.scrollTo({ top: 0 });
  }, [candidate?.id]);
  return <Dialog open={candidate !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-h-[94vh] overflow-hidden p-0 sm:max-w-[min(94vw,1100px)]">
    <DialogHeader className="border-b px-4 py-3 pr-12"><DialogTitle className="truncate text-sm">{candidate?.title || "素材详情"}</DialogTitle><DialogDescription>{candidate?.competitor || sourceLabel(candidate?.connector_id ?? "")}</DialogDescription></DialogHeader>
    <div className="grid h-[min(82vh,760px)] min-h-0 overflow-hidden md:grid-cols-[minmax(0,1fr)_340px]">
      <div ref={previewPaneRef} className="flex min-h-0 items-start justify-center overflow-auto bg-transparent p-3">{source ? <img src={source} alt={candidate?.title || "素材"} width={1600} height={1200} className="my-auto max-h-full w-full object-contain" /> : <ImageIcon className="my-auto h-8 w-8 text-muted-foreground" />}</div>
      <div ref={detailPaneRef} className="min-h-0 space-y-5 overflow-y-auto border-t p-4 text-sm md:border-l md:border-t-0">
        <section aria-labelledby="material-analysis-title" className="border-b pb-5">
          <div className="flex flex-wrap items-center gap-2"><Sparkles className="h-4 w-4" /><h3 id="material-analysis-title" className="font-medium">参考分析</h3>{analysis && <Badge variant="outline">已分析 v{analysis.analysis_version}</Badge>}</div>
          {analysis ? <><p className="mt-3 break-words text-sm">{analysis.summary || "分析已完成"}</p><AnalysisFacts analysis={analysis} /></> : <p className="mt-2 text-xs text-muted-foreground">该素材还没有完成参考分析。</p>}
		  {canRetryAnalysis && <Button className="mt-3" size="sm" variant="outline" disabled={retryingAnalysis} onClick={() => candidate && onRetryAnalysis(candidate.id)}><RefreshCw className={retryingAnalysis ? "h-4 w-4 animate-spin" : "h-4 w-4"} />{retryingAnalysis ? "正在重新分析" : "重新分析"}</Button>}
        </section>
        <div className="grid grid-cols-2 gap-4">
          <MetadataRow label="投放天数" value={details?.duration ?? MATERIAL_SOURCE_MISSING} />
          <MetadataRow label="曝光估算" value={details?.impressions ?? MATERIAL_SOURCE_MISSING} />
        </div>
        <MetadataRow label="媒体来源" value={details?.media ?? MATERIAL_SOURCE_MISSING} />
        <MetadataRow label="采集来源" value={sourceLabel(candidate?.connector_id ?? "")} />
        <MetadataRow label="市场" value={details?.market ?? MATERIAL_SOURCE_MISSING} />
        <MetadataRow label="语言" value={details?.languages ?? MATERIAL_SOURCE_MISSING} />
        <MetadataRow label="设备" value={details?.platforms ?? MATERIAL_SOURCE_MISSING} />
        {details?.tags && details.tags !== MATERIAL_SOURCE_MISSING && <MetadataRow label="标签" value={details.tags} />}
        {details?.note && details.note !== MATERIAL_SOURCE_MISSING && <MetadataRow label="备注" value={details.note} />}
        {candidate?.archive_status === "failed" && <div className="border border-destructive/30 bg-destructive/5 p-3"><p className="flex items-center gap-2 text-sm font-medium text-destructive"><AlertTriangle className="h-4 w-4" />稳定归档失败</p><p className="mt-2 break-words text-xs text-muted-foreground">{archiveErrorLabel(candidate.archive_error)}</p><p className="mt-2 text-xs text-muted-foreground">请在工作台的采集计划中处理归档修复。</p></div>}
        <div className="flex flex-wrap gap-2 border-t pt-4">
          {candidate?.source_attachment_id && <Button size="sm" variant="outline" onClick={onDirectEdit}><Sparkles className="h-4 w-4" />直接改图</Button>}
          {source && <a href={source} download className={buttonVariants({ size: "sm", variant: "outline" })}><Download className="h-4 w-4" />下载归档文件</a>}
          {original && <a href={original} target="_blank" rel="noreferrer" className={buttonVariants({ size: "sm", variant: "outline" })}><ExternalLink className="h-4 w-4" />查看来源</a>}
        </div>
      </div>
    </div>
  </DialogContent></Dialog>;
}

export function materialImportNotice(result: CreativeMaterialImportResult): { level: "success" | "warning"; message: string } {
  switch (result.analysis.action) {
    case "queued":
      return { level: "success", message: "素材已导入，参考分析已排队" };
    case "already_queued":
      return { level: "success", message: result.analysis.status === "running" ? "素材已导入，参考分析正在进行" : "素材已导入，参考分析已在队列中" };
    case "already_completed":
      return { level: "success", message: "素材已导入，参考分析已完成" };
    case "unavailable":
      return { level: "warning", message: "素材已导入，但尚未配置参考分析智能体" };
    case "enqueue_failed":
      return { level: "warning", message: "素材已导入，但参考分析排队失败" };
    default:
      return { level: "success", message: "素材已导入工作区素材库" };
  }
}

function MaterialImportDialog({ open, onClose, onImported }: { open: boolean; onClose: () => void; onImported: () => void }) {
  const { upload, uploading } = useFileUpload(api);
  const fileRef = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState<ImportDraft>(EMPTY_IMPORT);
  const [file, setFile] = useState<File | null>(null);
  const save = useMutation({
    mutationFn: async () => {
      let attachmentId: string | undefined;
      if (draft.mode === "file") {
        if (!file) throw new Error("请选择要导入的图片或视频");
        const uploaded = await upload(file);
        if (!uploaded) throw new Error("文件上传失败");
        attachmentId = uploaded.id;
      } else if (!draft.sourceUrl.trim()) {
        throw new Error("请输入素材 URL");
      }
      return api.importCreativeMaterialLibrary({
        attachment_id: attachmentId,
        source_url: draft.mode === "url" ? draft.sourceUrl.trim() : undefined,
        title: draft.title.trim() || file?.name,
        competitor: draft.competitor.trim(),
        asset_type: file?.type.startsWith("video/") ? "video" : "image",
        area_names: splitTags(draft.area),
        language_names: splitTags(draft.language),
        tags: splitTags(draft.tags),
        note: draft.note.trim(),
      });
    },
    onSuccess: (result) => {
      setDraft(EMPTY_IMPORT);
      setFile(null);
      onImported();
      const notice = materialImportNotice(result);
      if (notice.level === "warning") toast.warning(notice.message);
      else toast.success(notice.message);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法导入素材"),
  });
  const set = (field: keyof ImportDraft, value: string) => setDraft((current) => ({ ...current, [field]: value }));
  return <Dialog open={open} onOpenChange={(next) => !next && onClose()}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>导入素材</DialogTitle><DialogDescription>导入后会自动进入参考分析；分析完成后即可选择并开始生成。</DialogDescription></DialogHeader>
    <div className="flex w-fit border p-0.5"><Button size="sm" variant={draft.mode === "file" ? "secondary" : "ghost"} onClick={() => set("mode", "file")}>本地文件</Button><Button size="sm" variant={draft.mode === "url" ? "secondary" : "ghost"} onClick={() => set("mode", "url")}>素材 URL</Button></div>
    <div className="grid gap-4 sm:grid-cols-2">
      {draft.mode === "file" ? <div className="sm:col-span-2"><input ref={fileRef} type="file" accept="image/*,video/*" className="hidden" onChange={(event) => setFile(event.target.files?.[0] ?? null)} /><Button variant="outline" onClick={() => fileRef.current?.click()}><Upload className="h-4 w-4" />{file?.name || "选择图片或视频"}</Button></div> : <Field label="素材 URL" wide><Input value={draft.sourceUrl} onChange={(event) => set("sourceUrl", event.target.value)} placeholder="https://..." /></Field>}
      <Field label="标题"><Input value={draft.title} onChange={(event) => set("title", event.target.value)} placeholder="素材名称" /></Field>
      <Field label="竞品 / 来源"><Input value={draft.competitor} onChange={(event) => set("competitor", event.target.value)} placeholder="例如 Easycash" /></Field>
      <Field label="市场"><Input value={draft.area} onChange={(event) => set("area", event.target.value)} placeholder="例如 Indonesia，Malaysia" /></Field>
      <Field label="语言"><Input value={draft.language} onChange={(event) => set("language", event.target.value)} placeholder="例如 Indonesian，English" /></Field>
      <Field label="标签" wide><Input value={draft.tags} onChange={(event) => set("tags", event.target.value)} placeholder="跑量, 首页, 利率" /></Field>
      <Field label="备注" wide><Textarea rows={4} value={draft.note} onChange={(event) => set("note", event.target.value)} placeholder="记录用途、选材原因或后续注意事项" /></Field>
    </div>
    <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={uploading || save.isPending || (draft.mode === "file" ? !file : !draft.sourceUrl.trim())} onClick={() => save.mutate()}>导入</Button></DialogFooter>
  </DialogContent></Dialog>;
}

export function Field({ label, children, wide = false }: { label: string; children: React.ReactElement<{ id?: string }>; wide?: boolean }) {
  const generatedId = useId();
  const controlId = children.props.id || generatedId;
  return <div className={wide ? "space-y-1.5 sm:col-span-2" : "space-y-1.5"}><Label htmlFor={controlId} className="text-xs text-muted-foreground">{label}</Label>{cloneElement(children, { id: controlId })}</div>;
}

function MetadataRow({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 break-words">{value}</p></div>;
}

function MaterialMetric({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0"><dt className="text-muted-foreground">{label}</dt><dd className="truncate font-medium text-foreground" title={value}>{value}</dd></div>;
}

function candidateSource(candidate: CreativeMaterialCandidate): string {
  return resolvePublicFileUrl(candidate.archived_url || candidate.poster_url || candidate.preview_url) ?? "";
}

function sourceLabel(connector: string): string {
  if (connector === "manual_upload") return "人工上传";
  if (connector === "manual_url") return "URL 导入";
  if (connector.toLocaleLowerCase() === "appgrowing") return "AppGrowing";
  return connector || "未知来源";
}

function materialFacetValues(
  candidates: CreativeMaterialCandidate[],
  read: (candidate: CreativeMaterialCandidate) => string | readonly string[] | undefined,
): string[] {
  return [...new Set(candidates.flatMap((candidate) => {
    const value = read(candidate);
    return Array.isArray(value) ? value : [value ?? ""];
  }).map((value) => value.trim()).filter(Boolean))].sort((left, right) => left.localeCompare(right));
}

function candidateSourceRecords(candidate: CreativeMaterialCandidate): Record<string, unknown>[] {
  const candidateRecord = candidate as unknown as Record<string, unknown>;
  return [candidateRecord, toRecord(candidateRecord.metadata), toRecord(candidateRecord.raw)].filter((record): record is Record<string, unknown> => record !== null);
}

function toRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value === "string") {
    try {
      const parsed: unknown = JSON.parse(value);
      return toRecord(parsed);
    } catch {
      return null;
    }
  }
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function preferredCandidateValues(current: string[] | undefined, records: Record<string, unknown>[], keys: string[]): string[] {
  const direct = uniqueNonEmptyStrings(current ?? []);
  if (direct.length > 0) return direct;
  for (const record of records) {
    for (const key of keys) {
      const values = valueStrings(record[key]);
      if (values.length > 0) return values;
    }
  }
  return [];
}

function firstRecordString(records: Record<string, unknown>[], keys: string[]): string {
  for (const record of records) {
    for (const key of keys) {
      const value = record[key];
      if (typeof value === "string" && value.trim()) return value.trim();
    }
  }
  return "";
}

function firstRecordNumber(records: Record<string, unknown>[], keys: string[]): number | null {
  for (const record of records) {
    for (const key of keys) {
      const value = record[key];
      if (typeof value === "number" && Number.isFinite(value)) return value;
      if (typeof value === "string" && value.trim()) {
        const parsed = Number(value);
        if (Number.isFinite(parsed)) return parsed;
      }
    }
  }
  return null;
}

function valueStrings(value: unknown): string[] {
  if (Array.isArray(value)) return uniqueNonEmptyStrings(value.filter((item): item is string => typeof item === "string"));
  if (typeof value === "string") return uniqueNonEmptyStrings(value.split(/[,，]/));
  return [];
}

function uniqueNonEmptyStrings(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))];
}

function displayCandidateValues(values: string[]): string {
  return values.length > 0 ? values.join("、") : MATERIAL_SOURCE_MISSING;
}

function formatDecimal(value: number): string {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format(value);
}

function formatCompactCount(value: number): string {
  if (Math.abs(value) >= 10_000) return `${formatDecimal(value / 10_000)} 万`;
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function archiveErrorLabel(error: string): string {
  const normalized = error.toLocaleLowerCase();
  if (normalized.includes("lookup") || normalized.includes("no such host")) return "素材源站暂时无法解析。可直接重试；若源链接已过期，请重新执行采集以刷新链接。";
  if (normalized.includes("http 401") || normalized.includes("http 403")) return "素材源链接授权已过期，请重新执行采集以刷新链接。";
  if (normalized.includes("http 404")) return "素材源文件已失效，请重新采集或人工导入。";
  return error || "归档没有完成，可先重试；仍失败时重新采集以刷新素材源链接。";
}

function splitTags(value: string): string[] {
  return value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean);
}
