"use client";

import { cloneElement, useEffect, useId, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, Download, ExternalLink, ImageIcon, LoaderCircle, Plus, RefreshCw, RotateCcw, Search, Sparkles, Upload, X } from "lucide-react";
import { api } from "@multica/core/api";
import {
  creativeFeedbackOptions,
  creativeKeys,
  creativeMaterialLibraryOptions,
  recommendCreativeCopy,
  creativeResourceFilesOptions,
  creativeResourcesOptions,
  creativeSourceAnalysesOptions,
  validateCustomCopyFinancialFacts,
} from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { squadListOptions } from "@multica/core/workspace/queries";
import type { CreateCreativeFeedbackRequest, CreateCreativeOrderRequest, CreativeCopySnapshot, CreativeMaterialCandidate, CreativeMaterialImportResult, CreativeSourceAnalysis, SquadMember } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button, buttonVariants } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import type { CreativeCopyRecommendationBrief as CopyRecommendationBrief } from "@multica/core/creative";

type ImportDraft = {
  mode: "file" | "url";
  sourceUrl: string;
  title: string;
  competitor: string;
  tags: string;
  note: string;
};

const EMPTY_IMPORT: ImportDraft = {
  mode: "file",
  sourceUrl: "",
  title: "",
  competitor: "",
  tags: "",
  note: "",
};

export function CreativeMaterialLibrary({ onOrderCreated }: { onOrderCreated?: (orderId: string) => void } = {}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const materials = useQuery(creativeMaterialLibraryOptions(wsId));
  const [query, setQuery] = useState("");
  const [competitor, setCompetitor] = useState("");
  const [preview, setPreview] = useState<CreativeMaterialCandidate | null>(null);
  const [directEditCandidate, setDirectEditCandidate] = useState<CreativeMaterialCandidate | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [dismissedSelectionEventIds, setDismissedSelectionEventIds] = useState<Set<string>>(() => new Set());
  const [rejecting, setRejecting] = useState<CreativeMaterialCandidate | null>(null);
  const orderDraftRef = useRef<HTMLDivElement>(null);
  const pendingOrderDraftCandidateId = useRef("");
  const analyses = useQuery(creativeSourceAnalysesOptions(wsId));
  const feedback = useQuery(creativeFeedbackOptions(wsId, "candidate"));
  const candidates = useMemo(() => materials.data?.candidates ?? [], [materials.data?.candidates]);
  const completedAnalyses = useMemo(() => latestCompletedAnalyses(analyses.data?.analyses ?? []), [analyses.data?.analyses]);
  const latestDecisions = useMemo(() => latestCandidateFeedback(feedback.data?.events ?? []), [feedback.data?.events]);
  const selectedCandidateIds = useMemo(
    () => new Set([...latestDecisions.entries()].filter(([, decision]) => decision.decision === "selected").map(([candidateId]) => candidateId)),
    [latestDecisions],
  );
  const draftCandidateIds = useMemo(
    () => new Set([...latestDecisions.entries()].filter(([, decision]) => decision.decision === "selected" && !dismissedSelectionEventIds.has(decision.id)).map(([candidateId]) => candidateId)),
    [dismissedSelectionEventIds, latestDecisions],
  );
  useEffect(() => {
    const activeSelectionEvents = new Set([...latestDecisions.values()].filter((decision) => decision.decision === "selected").map((decision) => decision.id));
    setDismissedSelectionEventIds((current) => {
      const next = new Set([...current].filter((id) => activeSelectionEvents.has(id)));
      return next.size === current.size && [...next].every((id) => current.has(id)) ? current : next;
    });
  }, [latestDecisions]);
  useEffect(() => {
    const pendingCandidateId = pendingOrderDraftCandidateId.current;
    if (!pendingCandidateId || !draftCandidateIds.has(pendingCandidateId)) return;
    pendingOrderDraftCandidateId.current = "";
    orderDraftRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [draftCandidateIds]);
  const failed = candidates.filter((item) => !item.archived_url && item.archive_status === "failed");
  const retryArchives = useMutation({
    mutationFn: (candidateIds: string[]) => api.retryCreativeMaterialArchives(candidateIds),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      toast.success(`已重新安排 ${result.scheduled_count} 条素材归档`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法重新归档素材"),
  });
  const retryAnalysis = useMutation({
    mutationFn: (candidateId: string) => api.retryCreativeMaterialReferenceAnalysis(candidateId),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.analyses(wsId) });
      const notice = materialImportNotice(result);
      notice.level === "warning" ? toast.warning(notice.message.replace("素材已导入，", "")) : toast.success(notice.message.replace("素材已导入，", ""));
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法重新排队参考分析"),
  });
  const decideCandidate = useMutation({
    mutationFn: ({ candidate, decision, reasonCodes, comment, idempotencyKey }: { candidate: CreativeMaterialCandidate; decision: "selected" | "rejected"; reasonCodes?: string[]; comment?: string; idempotencyKey?: string }) => api.createCreativeFeedback(
      candidateDecisionFeedbackInput(candidate, decision, reasonCodes, comment, idempotencyKey),
    ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "candidate", "") });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法记录素材决定"),
  });
  const undoDecision = useMutation({
    mutationFn: (eventId: string) => api.undoCreativeFeedback(eventId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "candidate", "") }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法撤销素材决定"),
  });
  const competitors = useMemo(
    () => Array.from(new Set(candidates.map((item) => item.competitor).filter(Boolean))).sort(),
    [candidates],
  );
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return candidates.filter((item) => {
      if (competitor && item.competitor !== competitor) return false;
      if (!needle) return true;
      return [item.title, item.competitor, item.connector_id, ...item.tags]
        .join(" ")
        .toLocaleLowerCase()
        .includes(needle);
    });
  }, [candidates, competitor, query]);
  const previewCandidate = candidates.find((item) => item.id === preview?.id) ?? preview;
  const previewAnalysis = previewCandidate ? completedAnalyses.get(previewCandidate.id) : undefined;

  return <div className="mx-auto max-w-[1440px]">
    <div className="mb-4 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h2 className="text-base font-semibold">工作区素材库</h2>
        <p className="mt-1 text-sm text-muted-foreground">AppGrowing 采集和人工导入的素材统一归档，候选池从这里引用稳定文件。</p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="outline">{visible.length} / {candidates.length} 条</Badge>
        {selectedCandidateIds.size > 0 && <Badge>{selectedCandidateIds.size} 已选</Badge>}
        {failed.length > 0 && <Button size="sm" variant="outline" disabled={retryArchives.isPending} onClick={() => retryArchives.mutate(failed.map((item) => item.id))}><RefreshCw className={retryArchives.isPending ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重试归档 ({failed.length})</Button>}
        <Button size="sm" onClick={() => setImportOpen(true)}><Plus className="h-4 w-4" />导入素材</Button>
      </div>
    </div>
    <div className="mb-4 flex flex-wrap gap-2 border-y bg-muted/10 py-3">
      <div className="relative min-w-56 flex-1 md:max-w-md"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、竞品或标签" /></div>
      <select className="h-9 min-w-40 border bg-background px-3 text-sm" value={competitor} onChange={(event) => setCompetitor(event.target.value)} aria-label="按竞品筛选">
        <option value="">全部竞品</option>
        {competitors.map((name) => <option key={name} value={name}>{name}</option>)}
      </select>
    </div>
    {draftCandidateIds.size > 0 && <div ref={orderDraftRef} className="scroll-mt-4"><CreativeOrderDraft candidates={candidates.filter((candidate) => draftCandidateIds.has(candidate.id))} analyses={analyses.data?.analyses ?? []} onDone={(orderId) => {
      setDismissedSelectionEventIds(new Set([...latestDecisions.values()].filter((decision) => decision.decision === "selected").map((decision) => decision.id)));
      onOrderCreated?.(orderId);
    }} /></div>}
    {materials.isLoading ? <div className="py-16 text-center text-sm text-muted-foreground">正在加载素材库...</div> : visible.length === 0 ? (
      <div className="flex min-h-64 flex-col items-center justify-center border border-dashed text-sm text-muted-foreground"><ImageIcon className="mb-3 h-6 w-6" /><p>没有符合条件的素材</p><Button className="mt-4" size="sm" variant="outline" onClick={() => setImportOpen(true)}>导入第一条素材</Button></div>
    ) : (
      <div className="grid grid-cols-2 gap-px overflow-hidden border bg-border sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
        {visible.map((candidate) => {
          const decision = latestDecisions.get(candidate.id);
          const selected = selectedCandidateIds.has(candidate.id);
          const analysisState = materialAnalysisState(candidate, analyses.data?.analyses ?? []);
          return <MaterialTile
            key={candidate.id}
            candidate={candidate}
            analysisState={analysisState}
            selected={selected}
            decision={decision?.decision ?? ""}
            busy={decideCandidate.isPending || undoDecision.isPending}
            onToggle={() => {
              if (selected) {
                const selectedEvent = latestDecisions.get(candidate.id);
                if (selectedEvent?.decision === "selected") undoDecision.mutate(selectedEvent.id);
                return;
              }
              pendingOrderDraftCandidateId.current = candidate.id;
              decideCandidate.mutate(
                { candidate, decision: "selected" },
                { onError: () => { pendingOrderDraftCandidateId.current = ""; } },
              );
            }}
            onReject={() => setRejecting(candidate)}
            onUndo={() => decision && undoDecision.mutate(decision.id)}
            onOpen={() => setPreview(candidate)}
			onDirectEdit={() => setDirectEditCandidate(candidate)}
			onRetryAnalysis={() => retryAnalysis.mutate(candidate.id)}
          />;
        })}
      </div>
    )}
		<MaterialPreview candidate={previewCandidate} analysis={previewAnalysis} onClose={() => setPreview(null)} onRetry={(candidateId) => retryArchives.mutate([candidateId])} retrying={retryArchives.isPending} onRetryAnalysis={(candidateId) => retryAnalysis.mutate(candidateId)} retryingAnalysis={retryAnalysis.isPending} />
		<DirectEditDialog candidate={directEditCandidate} onClose={() => setDirectEditCandidate(null)} onCreated={(orderId) => {
			setDirectEditCandidate(null);
			queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
			onOrderCreated?.(orderId);
		}} />
    <CandidateRejectDialog candidate={rejecting} busy={decideCandidate.isPending} onClose={() => setRejecting(null)} onConfirm={(reasonCode, comment, idempotencyKey) => {
      if (!rejecting) return;
      decideCandidate.mutate({ candidate: rejecting, decision: "rejected", reasonCodes: [reasonCode], comment, idempotencyKey }, { onSuccess: () => setRejecting(null) });
    }} />
    <MaterialImportDialog open={importOpen} onClose={() => setImportOpen(false)} onImported={() => {
      setImportOpen(false);
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.analyses(wsId) });
    }} />
  </div>;
}

export type MaterialAnalysisState = { ready: boolean; status: string; error: string; version: number };

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
    media: mediaNames.length > 0
      ? mediaNames.join("、")
      : platformNames.length > 0
        ? `${platformNames.join("、")}（投放平台）`
        : `${sourceLabel(candidate.connector_id)}（采集来源）`,
    market: displayCandidateValues(preferredCandidateValues(candidate.area_names, records, ["area_names", "areas", "area", "market"])),
    languages: displayCandidateValues(preferredCandidateValues(candidate.language_names, records, ["language_names", "languages", "language", "locale"])),
    platforms: displayCandidateValues(platformNames),
    tags: displayCandidateValues(preferredCandidateValues(candidate.tags, records, ["tags", "tag_names"])),
    note: candidate.note?.trim() || firstRecordString(records, ["note", "remark", "remarks", "comment"]) || MATERIAL_SOURCE_MISSING,
  };
}

export function MaterialTile({ candidate, analysisState, selected, decision, busy, onToggle, onReject, onUndo, onOpen, onDirectEdit, onRetryAnalysis }: { candidate: CreativeMaterialCandidate; analysisState: MaterialAnalysisState; selected: boolean; decision: string; busy: boolean; onToggle: () => void; onReject: () => void; onUndo: () => void; onOpen: () => void; onDirectEdit: () => void; onRetryAnalysis: () => void }) {
  const source = candidateSource(candidate);
  const details = materialCandidateDisplayDetails(candidate);
  return <div className="relative min-w-0 bg-background text-left transition-colors hover:bg-muted/30" data-testid="creative-material-tile" data-candidate-id={candidate.id}><button type="button" onClick={onOpen} className="w-full">
    <div className="relative aspect-[4/3] bg-muted/40">
      {source ? <img src={source} alt={candidate.title || candidate.competitor} width={640} height={480} loading="lazy" className="h-full w-full object-contain" /> : <div className="flex h-full flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />{candidate.archive_status === "failed" ? "归档失败" : "正在归档"}</div>}
      {candidate.archive_status === "failed" && <Badge variant="destructive" className="absolute left-2 top-2"><AlertTriangle className="h-3 w-3" />归档失败</Badge>}
      {(candidate.archive_status === "pending" || candidate.archive_status === "running") && <Badge variant="secondary" className="absolute left-2 top-2 bg-background/90"><LoaderCircle className="h-3 w-3 animate-spin" />归档中</Badge>}
      {analysisState.ready && <Badge variant="outline" className="absolute bottom-2 left-2 bg-background/90"><Sparkles className="h-3 w-3" />已分析{analysisState.version > 0 ? ` v${analysisState.version}` : ""}</Badge>}
      {analysisState.status && analysisState.status !== "completed" && <Badge variant="outline" className="absolute bottom-2 left-2 bg-background/90">{analysisState.status === "failed" ? "分析失败" : analysisState.status === "running" ? "分析中" : "等待分析"}</Badge>}
      {decision === "rejected" && <Badge variant="destructive" className="absolute bottom-2 right-2">已拒绝</Badge>}
    </div>
    <div className="space-y-1 border-t px-3 py-2.5">
      <p className="truncate text-sm font-medium">{candidate.title || "未命名素材"}</p>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span className="truncate">{candidate.competitor || sourceLabel(candidate.connector_id)}</span><span className="shrink-0">{candidate.area_names[0] ?? ""}</span></div>
      <dl className="grid grid-cols-3 gap-2 border-t pt-2 text-left text-[10px]">
        <MaterialMetric label="投放天数" value={details.duration} />
        <MaterialMetric label="曝光估算" value={details.impressions} />
        <MaterialMetric label="媒体来源" value={details.media} />
      </dl>
      <p className="font-mono text-[10px] text-muted-foreground">素材 ID {candidate.id.slice(0, 8)}</p>
      {analysisState.status === "failed" && <p className="truncate text-[11px] text-destructive" title={analysisState.error}>{analysisState.error || "请重新发起分析"}</p>}
    </div>
  </button><div className="absolute right-2 top-2 flex gap-1">{analysisState.status === "failed" && <Button aria-label="重新分析素材" title="重新分析素材" size="icon-sm" variant="outline" disabled={busy} onClick={onRetryAnalysis}><RefreshCw className="h-4 w-4" /></Button>}<Button aria-label="直接改图" title="直接改图" size="icon-sm" variant="outline" onClick={onDirectEdit}><Sparkles className="h-4 w-4" /></Button><Button aria-label={decision === "rejected" ? "撤销拒绝" : "拒绝素材"} title={decision === "rejected" ? "撤销拒绝" : "拒绝素材"} size="icon-sm" variant="outline" disabled={busy} onClick={decision === "rejected" ? onUndo : onReject}>{decision === "rejected" ? <RotateCcw className="h-4 w-4" /> : <X className="h-4 w-4" />}</Button><Button aria-label={selected ? "取消选择素材" : "选择素材"} title={selected ? "取消选择素材" : analysisState.ready ? "选择素材" : "等待参考分析完成后可选择素材"} size="icon-sm" variant={selected ? "default" : "outline"} disabled={busy || decision === "rejected" || (!selected && !analysisState.ready)} onClick={onToggle}>{selected ? <Check className="h-4 w-4" /> : <Plus className="h-4 w-4" />}</Button></div></div>;
}

export type OrderItemDraft = {
  copyEntryId: string;
  direction: string;
  replacementReason: string;
  customHeadline: string;
  customBenefit: string;
  customCta: string;
};

type SubmissionRecovery = { issueId: string; orderId: string; submissionKey: string };

const EMPTY_SUBMISSION_RECOVERY: SubmissionRecovery = { issueId: "", orderId: "", submissionKey: "" };

export function orderDraftWithRecommendation(current: OrderItemDraft | undefined, recommendedCopyEntryId: string): OrderItemDraft {
  if (!current) {
    return {
      copyEntryId: recommendedCopyEntryId,
      direction: "",
      replacementReason: "benefit_mismatch",
      customHeadline: "",
      customBenefit: "",
      customCta: "",
    };
  }
  if (!current.copyEntryId && recommendedCopyEntryId) return { ...current, copyEntryId: recommendedCopyEntryId };
  return current;
}

export function recoveryForSubmissionKey(recovery: SubmissionRecovery, submissionKey: string): SubmissionRecovery {
  return recovery.submissionKey === submissionKey ? recovery : EMPTY_SUBMISSION_RECOVERY;
}

function CreativeOrderDraft({ candidates, analyses, onDone }: { candidates: CreativeMaterialCandidate[]; analyses: CreativeSourceAnalysis[]; onDone: (orderId: string) => void }) {
  const wsId = useWorkspaceId();
  const resources = useQuery(creativeResourcesOptions(wsId));
  const squads = useQuery(squadListOptions(wsId));
  const [marketPackId, setMarketPackId] = useState("");
  const [squadId, setSquadId] = useState("");
  const [drafts, setDrafts] = useState<Record<string, OrderItemDraft>>({});
  const [busy, setBusy] = useState(false);
  const [recoveryMessage, setRecoveryMessage] = useState("");
  const [recovery, setRecovery] = useState<SubmissionRecovery>(EMPTY_SUBMISSION_RECOVERY);
  const [activeCandidateId, setActiveCandidateId] = useState(candidates[0]?.id ?? "");
  const knownCandidateIds = useRef(new Set(candidates.map((candidate) => candidate.id)));
  const viewedCopyIds = useRef(new Set<string>());
  const marketPacks = (resources.data?.resources ?? []).filter((resource) => resource.kind === "market_pack" && resource.published_version > 0);
  const marketPack = marketPacks.find((resource) => resource.id === marketPackId) ?? marketPacks[0];
  const marketPackConfig = marketPack?.published_config ?? {};
  const copyLibraryId = recordString(marketPackConfig, "copy_library_id");
  const copyLibrary = (resources.data?.resources ?? []).find((resource) => resource.id === copyLibraryId && resource.kind === "copy_library" && resource.published_version > 0);
  const marketFiles = useQuery(creativeResourceFilesOptions(wsId, marketPack?.id ?? ""));
  const selectedSquad = (squads.data ?? []).find((squad) => squad.id === squadId) ?? (squads.data ?? [])[0];
  const squadMembers = useQuery({
    queryKey: ["workspaces", wsId, "squads", selectedSquad?.id ?? "", "members"],
    queryFn: () => api.listSquadMembers(selectedSquad!.id),
    enabled: !!wsId && !!selectedSquad?.id,
  });
  const completedAnalyses = useMemo(() => latestCompletedAnalyses(analyses), [analyses]);
  const recommendations = useMemo(() => new Map(candidates.map((candidate) => {
    const analysis = completedAnalyses.get(candidate.id);
    return [candidate.id, copyLibrary ? recommendCreativeCopy(candidate, copyLibrary, analysisBrief(analysis)) : []];
  })), [candidates, completedAnalyses, copyLibrary]);

  useEffect(() => {
    if (!marketPackId && marketPacks[0]) setMarketPackId(marketPacks[0].id);
  }, [marketPackId, marketPacks]);
  useEffect(() => {
    if (squadId || !squads.data?.length) return;
    const preferred = squads.data.find((squad) => squad.name.includes("素材小队")) ?? squads.data[0];
    if (preferred) setSquadId(preferred.id);
  }, [squadId, squads.data]);
  useEffect(() => {
    setDrafts((current) => {
      let changed = false;
      const next = { ...current };
      for (const candidate of candidates) {
        const draft = orderDraftWithRecommendation(next[candidate.id], recommendations.get(candidate.id)?.[0]?.recipe.id ?? "");
        if (draft !== next[candidate.id]) {
          next[candidate.id] = draft;
          changed = true;
        }
      }
      return changed ? next : current;
    });
  }, [candidates, recommendations]);
  useEffect(() => {
    const nextIds = new Set(candidates.map((candidate) => candidate.id));
    const added = candidates.find((candidate) => !knownCandidateIds.current.has(candidate.id));
    knownCandidateIds.current = nextIds;
    setActiveCandidateId((current) => added?.id ?? (nextIds.has(current) ? current : candidates[0]?.id ?? ""));
  }, [candidates]);
  useEffect(() => {
    for (const candidate of candidates) {
      const analysis = completedAnalyses.get(candidate.id);
      for (const [rank, recommendation] of (recommendations.get(candidate.id) ?? []).slice(0, 3).entries()) {
        const key = `${candidate.id}:${recommendation.recipe.id}`;
        if (viewedCopyIds.current.has(key)) continue;
        viewedCopyIds.current.add(key);
        void api.createCreativeFeedback({
          issue_id: "",
          subject_type: "recommended_copy",
          subject_id: recommendation.recipe.id,
          event_type: "viewed",
          decision: "",
          context_snapshot: { candidate_id: candidate.id, source_analysis_id: analysis?.id ?? "", rank: rank + 1, recommendation_reasons: recommendation.reasons },
        }).catch(() => viewedCopyIds.current.delete(key));
      }
    }
  }, [candidates, completedAnalyses, recommendations]);

  const incompleteCandidates = candidates.filter((candidate) => !completedAnalyses.has(candidate.id));
  const unconfiguredCandidates = candidates.filter((candidate) => {
    const draft = drafts[candidate.id];
    return !draft || (draft.copyEntryId === "__custom__" ? !draft.customHeadline.trim() : !draft.copyEntryId);
  });

  const setDraft = (candidateId: string, patch: Partial<OrderItemDraft>) => setDrafts((current) => ({
    ...current,
    [candidateId]: { ...current[candidateId], ...patch } as OrderItemDraft,
  }));
  const activeCandidate = candidates.find((candidate) => candidate.id === activeCandidateId) ?? candidates[0];
  const activeAnalysis = activeCandidate ? completedAnalyses.get(activeCandidate.id) : undefined;
  const activeRanked = activeCandidate ? recommendations.get(activeCandidate.id) ?? [] : [];
  const activeDraft = activeCandidate ? drafts[activeCandidate.id] : undefined;
  const activeSelectedCopy = activeRanked.find((item) => item.recipe.id === activeDraft?.copyEntryId);
  const activeRecommended = activeRanked[0];
  const activeSource = activeCandidate ? candidateSource(activeCandidate) : "";
  const customCopyValidations = useMemo(() => new Map(candidates.flatMap((candidate) => {
    const draft = drafts[candidate.id];
    if (!draft || draft.copyEntryId !== "__custom__") return [];
    const recommendation = recommendations.get(candidate.id)?.[0];
    const snapshot = customCopySnapshot(draft, recommendation?.snapshot.creative_type, copyLibrary);
    return [[candidate.id, validateCustomCopyFinancialFacts(snapshot, copyLibrary)]] as const;
  })), [candidates, copyLibrary, drafts, recommendations]);
  const customCopyIssueCount = [...customCopyValidations.values()].filter((validation) => !validation.allowed).length;
  const activeCustomCopyValidation = activeCandidate ? customCopyValidations.get(activeCandidate.id) : undefined;

  const create = async () => {
    if (!marketPack || !selectedSquad || incompleteCandidates.length > 0 || unconfiguredCandidates.length > 0 || customCopyIssueCount > 0) return;
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
            const recommendation = recommendations.get(candidate.id)?.find((item) => item.recipe.id === draft.copyEntryId);
            return creativeOrderItemInput(
              candidate.id,
              completedAnalyses.get(candidate.id)!.id,
              recommendation
                ? { ...recommendation.snapshot }
                : customCopySnapshot(draft, recommendations.get(candidate.id)?.[0]?.snapshot.creative_type, copyLibrary),
              draft.direction,
            );
          }),
        });
        if (!order.id) throw new Error("创意订单初始化失败，请重试");
        orderId = order.id;
        setRecovery({ issueId, orderId, submissionKey });
      }
      await api.setIssueMetadataKey(issueId, "creative_order_id", orderId);
      await Promise.all(candidates.map(async (candidate) => {
        const draft = drafts[candidate.id]!;
        const recommended = recommendations.get(candidate.id)?.[0]?.recipe;
        if (!recommended) return;
        await api.createCreativeFeedback(recommendedCopyDecisionFeedbackInput({
          submissionKey,
          issueId,
          orderId,
          candidateId: candidate.id,
          recommendedCopyId: recommended.id,
          selectedCopyId: draft.copyEntryId,
          replacementReason: draft.replacementReason,
        }));
      }));
      await api.updateIssue(issueId, { assignee_type: "squad", assignee_id: selectedSquad.id });
      toast.success("创意订单已提交，开始生成");
      setRecovery(EMPTY_SUBMISSION_RECOVERY);
      onDone(orderId);
    } catch (error) {
      if (submissionKey) setRecovery({ issueId, orderId, submissionKey });
      setRecoveryMessage(error instanceof Error ? error.message : "订单创建未完成。可以在当前面板继续，不会重复创建已完成的步骤。");
    } finally {
      setBusy(false);
    }
  };

  return <section className="mt-4 border bg-background" aria-labelledby="creative-order-draft-title">
    <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3"><div><h3 id="creative-order-draft-title" className="text-sm font-semibold">配置创意订单</h3><p className="mt-1 text-xs text-muted-foreground">逐图确认参考分析、完整文案和补充方向</p></div><div className="flex items-center gap-2"><Badge variant="outline">已配置 {candidates.length - unconfiguredCandidates.length}/{candidates.length}</Badge><Badge>{candidates.length} 张素材</Badge></div></div>
    <div className="grid gap-3 border-b p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
      <Field label="市场规则"><NativeSelect value={marketPack?.id ?? ""} onChange={(event) => setMarketPackId(event.target.value)}><NativeSelectOption value="">选择已发布市场规则</NativeSelectOption>{marketPacks.map((resource) => <NativeSelectOption key={resource.id} value={resource.id}>{resource.name} · v{resource.published_version}</NativeSelectOption>)}</NativeSelect></Field>
      <details className="min-w-52 border px-3 py-2"><summary className="cursor-pointer list-none text-xs font-medium marker:content-none">高级设置 · {selectedSquad?.name || "未配置"}</summary><div className="mt-3"><Field label="生成服务"><NativeSelect value={selectedSquad?.id ?? ""} onChange={(event) => setSquadId(event.target.value)}><NativeSelectOption value="">选择可用服务</NativeSelectOption>{(squads.data ?? []).map((squad) => <NativeSelectOption key={squad.id} value={squad.id}>{squad.name}</NativeSelectOption>)}</NativeSelect></Field></div></details>
    </div>
    <nav className="flex gap-2 overflow-x-auto border-b bg-muted/10 p-3" aria-label="已选素材">{candidates.map((candidate, index) => {
      const candidateDraft = drafts[candidate.id];
      const configured = candidateDraft?.copyEntryId === "__custom__" ? Boolean(candidateDraft.customHeadline.trim()) : Boolean(candidateDraft?.copyEntryId);
      const analyzed = completedAnalyses.has(candidate.id);
      const source = candidateSource(candidate);
      return <button key={candidate.id} type="button" aria-pressed={candidate.id === activeCandidate?.id} onClick={() => setActiveCandidateId(candidate.id)} className={`grid min-w-52 grid-cols-[56px_minmax(0,1fr)] gap-2 border bg-background p-2 text-left ${candidate.id === activeCandidate?.id ? "border-emerald-600 ring-1 ring-emerald-600/20" : ""}`}>
        <span className="aspect-[4/3] overflow-hidden bg-muted">{source ? <img src={source} alt="" width={160} height={120} loading="lazy" className="h-full w-full object-contain" /> : <span className="flex h-full items-center justify-center"><ImageIcon className="h-4 w-4 text-muted-foreground" /></span>}</span>
        <span className="min-w-0"><span className="block truncate text-xs font-medium">{index + 1}. {candidate.title || candidate.competitor}</span><span className="mt-1 block text-[11px] text-muted-foreground">{analyzed ? "已分析" : "等待分析"} · {configured ? "文案已定" : "待定文案"}</span><span className="mt-0.5 block font-mono text-[10px] text-muted-foreground">{candidate.id.slice(0, 8)}</span></span>
      </button>;
    })}</nav>
    {activeCandidate && <article key={activeCandidate.id} className="grid gap-5 p-4 lg:grid-cols-[220px_minmax(0,1fr)]" data-testid="creative-order-active-editor" data-candidate-id={activeCandidate.id}>
      <div><div className="aspect-[4/3] overflow-hidden bg-muted">{activeSource ? <img src={activeSource} alt={activeCandidate.title || activeCandidate.competitor} width={880} height={660} loading="lazy" className="h-full w-full object-contain" /> : <div className="flex h-full items-center justify-center"><ImageIcon className="h-6 w-6 text-muted-foreground" /></div>}</div><p className="mt-2 break-words text-xs font-medium">{activeCandidate.title || activeCandidate.competitor}</p><p className="mt-1 font-mono text-[10px] text-muted-foreground">素材 ID {activeCandidate.id.slice(0, 8)}</p></div>
      <div className="min-w-0 space-y-4">
        <div className="border-l-2 border-emerald-600 pl-3"><div className="flex flex-wrap items-center gap-2"><Sparkles className="h-4 w-4 text-emerald-700" /><p className="text-sm font-medium">{activeAnalysis?.summary || "等待参考分析"}</p>{activeAnalysis && <Badge variant="outline">v{activeAnalysis.analysis_version}</Badge>}</div>{activeAnalysis && <AnalysisFacts analysis={activeAnalysis} />}{!activeAnalysis && <p className="mt-2 text-xs text-destructive">分析完成后才能提交该素材。</p>}</div>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="选用文案"><NativeSelect value={activeDraft?.copyEntryId ?? ""} onChange={(event) => setDraft(activeCandidate.id, { copyEntryId: event.target.value })}><NativeSelectOption value="">选择已发布组合方案</NativeSelectOption>{activeRanked.slice(0, 20).map((item, index) => <NativeSelectOption key={item.recipe.id} value={item.recipe.id}>{index === 0 ? "推荐 · " : ""}{item.recipe.name} · {item.snapshot.headline}</NativeSelectOption>)}<NativeSelectOption value="__custom__">自定义文案</NativeSelectOption></NativeSelect></Field>
          {activeRecommended && activeDraft?.copyEntryId !== activeRecommended.recipe.id && <Field label="替换推荐原因"><NativeSelect value={activeDraft?.replacementReason ?? "benefit_mismatch"} onChange={(event) => setDraft(activeCandidate.id, { replacementReason: event.target.value })}>{COPY_REPLACEMENT_REASONS.map((reason) => <NativeSelectOption key={reason.value} value={reason.value}>{reason.label}</NativeSelectOption>)}</NativeSelect></Field>}
        </div>
        {activeDraft?.copyEntryId === "__custom__" ? <div className="grid gap-3 border bg-muted/10 p-3 sm:grid-cols-2"><Field label="自定义主标题" wide><Input value={activeDraft.customHeadline} onChange={(event) => setDraft(activeCandidate.id, { customHeadline: event.target.value })} /></Field><Field label="自定义卖点" wide><Textarea rows={3} value={activeDraft.customBenefit} onChange={(event) => setDraft(activeCandidate.id, { customBenefit: event.target.value })} /></Field><Field label="自定义 CTA"><Input value={activeDraft.customCta} onChange={(event) => setDraft(activeCandidate.id, { customCta: event.target.value })} /></Field><p className="self-end text-xs text-muted-foreground">创意语言可自由编辑；货币、利率、期限等金融事实必须与已审核产品事实一致。</p>{activeCustomCopyValidation && !activeCustomCopyValidation.allowed && <p role="alert" className="sm:col-span-2 border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{activeCustomCopyValidation.message}</p>}</div> : activeSelectedCopy ? <CopyPreview recommendation={activeSelectedCopy} isPrimaryRecommendation={activeSelectedCopy.recipe.id === activeRecommended?.recipe.id} /> : <p className="text-xs text-destructive">请选择完整文案。</p>}
        <Field label="补充创意想法" wide><Textarea rows={3} value={activeDraft?.direction ?? ""} onChange={(event) => setDraft(activeCandidate.id, { direction: event.target.value })} placeholder="例如：保留绿色信息卡片，人物更生活化，CTA 更醒目。不要在这里补写未经审核的金融事实。" /></Field>
      </div>
    </article>}
    <div className="flex flex-wrap items-center justify-between gap-3 border-t bg-muted/10 px-4 py-3"><div className="text-xs text-muted-foreground">{recoveryMessage ? <span className="text-destructive">{recoveryMessage}</span> : incompleteCandidates.length > 0 ? `${incompleteCandidates.length} 张素材仍在等待分析` : unconfiguredCandidates.length > 0 ? `${unconfiguredCandidates.length} 张素材尚未选择文案` : customCopyIssueCount > 0 ? `${customCopyIssueCount} 张素材的自定义文案含未审核金融事实` : !selectedSquad ? "暂无可用生成服务" : "全部素材已配置，可以开始生成。"}</div><Button disabled={busy || !marketPack || !selectedSquad || incompleteCandidates.length > 0 || unconfiguredCandidates.length > 0 || customCopyIssueCount > 0} onClick={() => void create()}>{busy ? "正在提交" : recovery.issueId ? "继续提交" : "开始生成"}</Button></div>
  </section>;
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

const COPY_REPLACEMENT_REASONS = [
  { value: "benefit_mismatch", label: "利益点不匹配" },
  { value: "facts_inapplicable", label: "事实不适用" },
  { value: "unnatural", label: "表达不自然" },
  { value: "tone_mismatch", label: "语气不合适" },
  { value: "too_long", label: "文案过长" },
  { value: "compliance_risk", label: "存在合规风险" },
  { value: "translation", label: "翻译问题" },
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
  const [squadId, setSquadId] = useState("");
  const [recovery, setRecovery] = useState<SubmissionRecovery>(EMPTY_SUBMISSION_RECOVERY);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const selectedSquad = (squads.data ?? []).find((squad) => squad.id === squadId) ?? (squads.data ?? []).find((squad) => squad.name.includes("素材小队")) ?? (squads.data ?? [])[0];

  useEffect(() => {
    setInstruction("");
    setTargetSize("1080x1080");
    setDeliveryMode("preview");
    setSquadId("");
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
      toast.success(deliveryMode === "publish" ? "直接改图已提交，将进入 Prime 和 QC" : "直接改图预览已提交");
      onCreated(orderId);
    } catch (submitError) {
      if (submissionKey) setRecovery({ issueId, orderId, submissionKey });
      setError(submitError instanceof Error ? submitError.message : "提交未完成。可继续提交，不会重复创建已完成的步骤。");
    } finally {
      setBusy(false);
    }
  };

  return <Dialog open={candidate !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>直接改图</DialogTitle><DialogDescription>原图会固定为本次修改的来源。预览只生成底图；正式投放会继续进入 Prime 和双路 QC。</DialogDescription></DialogHeader>
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="目标尺寸"><NativeSelect disabled={Boolean(recovery.issueId)} value={targetSize} onChange={(event) => setTargetSize(event.target.value as typeof targetSize)}><NativeSelectOption value="1080x1080">1080 x 1080</NativeSelectOption><NativeSelectOption value="1200x628">1200 x 628</NativeSelectOption><NativeSelectOption value="800x1000">800 x 1000</NativeSelectOption></NativeSelect></Field>
        <Field label="交付方式"><NativeSelect disabled={Boolean(recovery.issueId)} value={deliveryMode} onChange={(event) => setDeliveryMode(event.target.value as typeof deliveryMode)}><NativeSelectOption value="preview">预览</NativeSelectOption><NativeSelectOption value="publish">作为正式投放素材</NativeSelectOption></NativeSelect></Field>
      </div>
      <details className="border px-3 py-2"><summary className="cursor-pointer list-none text-xs font-medium marker:content-none">执行设置 · {selectedSquad?.name || "未配置"}</summary><div className="mt-3"><Field label="执行小队"><NativeSelect disabled={Boolean(recovery.issueId)} value={selectedSquad?.id ?? ""} onChange={(event) => setSquadId(event.target.value)}><NativeSelectOption value="">选择小队</NativeSelectOption>{(squads.data ?? []).map((squad) => <NativeSelectOption key={squad.id} value={squad.id}>{squad.name}</NativeSelectOption>)}</NativeSelect></Field></div></details>
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

function AnalysisFacts({ analysis }: { analysis: CreativeSourceAnalysis }) {
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
  return <dl className="mt-3 grid gap-x-4 gap-y-2 text-xs sm:grid-cols-2">
    <AnalysisFact label="主题" value={theme || "未识别"} />
    <AnalysisFact label="主利益点" value={benefit || "未识别"} />
    {themeElements.length > 0 && <AnalysisFact label="主题元素" value={themeElements.join("、")} />}
    {secondaryBenefits.length > 0 && <AnalysisFact label="次要利益点" value={secondaryBenefits.join("、")} />}
    {value && <AnalysisFact label="竞品观察" value={`${value}，只作结构参考`} wide />}
    {semantics && <AnalysisFact label="业务语义" value={semantics} wide />}
    {mechanism && <AnalysisFact label="信息机制" value={mechanism} wide />}
    {visualType && <AnalysisFact label="视觉类型" value={visualType} />}
    {visualAnchors.length > 0 && <AnalysisFact label="视觉锚点" value={visualAnchors.join("、")} wide />}
    {paletteAnchors.length > 0 && <AnalysisFact label="色彩锚点" value={paletteAnchors.join("、")} wide />}
    {mustPreserve.length > 0 && <AnalysisFact label="必须保留" value={mustPreserve.join("、")} wide />}
    {allowedVariations.length > 0 && <AnalysisFact label="可调整范围" value={allowedVariations.join("、")} wide />}
  </dl>;
}

function AnalysisFact({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return <div className={wide ? "sm:col-span-2" : undefined}><dt className="text-muted-foreground">{label}</dt><dd className="mt-0.5 break-words text-foreground">{value}</dd></div>;
}

function CopyPreview({ recommendation, isPrimaryRecommendation }: { recommendation: ReturnType<typeof recommendCreativeCopy>[number]; isPrimaryRecommendation: boolean }) {
  const copy = recommendation.snapshot;
  return <div className="border bg-muted/10 p-3 text-xs"><div className="flex flex-wrap items-center gap-2"><Badge variant="outline">{isPrimaryRecommendation ? "推荐" : "已选"}</Badge><span className="font-medium">{recommendation.recipe.name}</span><span className="font-mono text-muted-foreground">文案库 v{copy.library_version}</span>{recommendation.reasons.map((reason) => <Badge key={reason} variant="secondary">{reason}</Badge>)}</div><dl className="mt-3 grid gap-2 sm:grid-cols-[72px_minmax(0,1fr)]"><dt className="text-muted-foreground">主标题</dt><dd className="whitespace-pre-wrap break-words font-medium">{copy.headline || "-"}</dd><dt className="text-muted-foreground">副标题</dt><dd className="whitespace-pre-wrap break-words">{copy.subheadline || "-"}</dd><dt className="text-muted-foreground">利益点</dt><dd className="whitespace-pre-wrap break-words">{copy.benefit || "-"}</dd><dt className="text-muted-foreground">补充信息</dt><dd className="whitespace-pre-wrap break-words">{copy.supporting || "-"}</dd><dt className="text-muted-foreground">CTA</dt><dd className="whitespace-pre-wrap break-words">{copy.cta || "-"}</dd><dt className="text-muted-foreground">产品事实</dt><dd className="flex flex-wrap gap-1">{copy.product_facts.length > 0 ? copy.product_facts.map((fact) => <Badge key={fact.key} variant="outline">{fact.label}：{fact.copy_text}</Badge>) : "-"}</dd></dl>{!recommendation.exactTypeMatch && <p className="mt-3 border-t pt-2 text-muted-foreground">当前素材未识别出明确类型，按文案库默认策略排序。提交前可切换其他组合方案。</p>}</div>;
}

export function latestCandidateFeedback(events: { id: string; subject_id: string; event_type: string; decision: string; undo_of_id: string; created_at?: string }[]) {
  const undone = new Set(events.filter((event) => event.event_type === "undo" && event.undo_of_id).map((event) => event.undo_of_id));
  const latest = new Map<string, { id: string; decision: string; created_at: string }>();
  for (const event of events.filter((event) => event.event_type === "decision" && !undone.has(event.id))) {
    const current = latest.get(event.subject_id);
    if (!current || compareNewest(event, current) > 0) {
      latest.set(event.subject_id, { id: event.id, decision: event.decision, created_at: event.created_at ?? "" });
    }
  }
  return latest;
}

export function latestCompletedAnalyses(analyses: CreativeSourceAnalysis[]): Map<string, CreativeSourceAnalysis> {
  const latest = new Map<string, CreativeSourceAnalysis>();
  for (const analysis of analyses) {
    if (analysis.status !== "completed") continue;
    const current = latest.get(analysis.candidate_id);
    if (!current || analysis.analysis_version > current.analysis_version || (analysis.analysis_version === current.analysis_version && compareNewest(analysis, current) > 0)) {
      latest.set(analysis.candidate_id, analysis);
    }
  }
  return latest;
}

export function materialAnalysisState(candidate: Pick<CreativeMaterialCandidate, "id" | "analysis_status" | "analysis_error">, analyses: CreativeSourceAnalysis[]): MaterialAnalysisState {
  const completed = latestCompletedAnalyses(analyses).get(candidate.id);
  if (completed) return { ready: true, status: "completed", error: "", version: completed.analysis_version };

  const latest = analyses
    .filter((analysis) => analysis.candidate_id === candidate.id)
    .reduce<CreativeSourceAnalysis | undefined>((current, analysis) => !current || compareNewest(analysis, current) > 0 ? analysis : current, undefined);
  if (latest) return { ready: false, status: latest.status || "pending", error: latest.error_message || "", version: latest.analysis_version };

  const fallbackStatus = candidate.analysis_status || "pending";
  return { ready: fallbackStatus === "completed", status: fallbackStatus, error: candidate.analysis_error || "", version: 0 };
}

function compareNewest(left: { id: string; created_at?: string; completed_at?: string }, right: { id: string; created_at?: string; completed_at?: string }): number {
  const leftTime = Date.parse(left.completed_at || left.created_at || "") || 0;
  const rightTime = Date.parse(right.completed_at || right.created_at || "") || 0;
  if (leftTime !== rightTime) return leftTime - rightTime;
  return left.id.localeCompare(right.id);
}

function analysisBrief(analysis?: CreativeSourceAnalysis): CopyRecommendationBrief {
  const result = analysis?.result ?? {};
  return {
    theme: recordString(result, "theme"),
    theme_elements: recordStringArray(result, "theme_elements"),
    primary_benefit: recordString(result, "primary_benefit"),
    secondary_benefits: recordStringArray(result, "secondary_benefits"),
    benefit_value: recordString(result, "benefit_value"),
  };
}

export function customCopySnapshot(
  draft: OrderItemDraft,
  creativeType: CreativeCopySnapshot["creative_type"] = "num",
  library?: { id: string; published_version: number },
): CreativeCopySnapshot {
  const snapshot: CreativeCopySnapshot = { schema_version: 2, id: "", library_id: library?.id ?? "", library_version: library?.published_version ?? 0, recipe_id: "", recipe_key: "user-custom", creative_type: creativeType, headline: draft.customHeadline.trim(), subheadline: "", benefit: draft.customBenefit.trim(), supporting: "", cta: draft.customCta.trim(), legal_text: "", fragments: [], product_facts: [], recommendation: { score: 0, reasons: ["用户自定义"], matched_signals: [] }, status: "user_custom" };
  return snapshot;
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

export function recommendedCopyDecisionFeedbackInput({
  submissionKey,
  issueId,
  orderId,
  candidateId,
  recommendedCopyId,
  selectedCopyId,
  replacementReason,
}: {
  submissionKey: string;
  issueId: string;
  orderId: string;
  candidateId: string;
  recommendedCopyId: string;
  selectedCopyId: string;
  replacementReason: string;
}): CreateCreativeFeedbackRequest {
  const accepted = selectedCopyId === recommendedCopyId;
  const eventType = accepted ? "decision" : "replacement";
  const decision = accepted ? "accepted" : "replaced";
  return {
    idempotency_key: creativeFeedbackIdempotencyKey(submissionKey, "recommended_copy", recommendedCopyId, eventType, decision, candidateId),
    issue_id: issueId,
    subject_type: "recommended_copy",
    subject_id: recommendedCopyId,
    event_type: eventType,
    decision,
    reason_codes: accepted ? undefined : [replacementReason],
    context_snapshot: {
      candidate_id: candidateId,
      order_id: orderId,
      ...(accepted ? {} : { replacement_copy_id: selectedCopyId }),
    },
  };
}

export function candidateDecisionFeedbackInput(
  candidate: Pick<CreativeMaterialCandidate, "id" | "source_run_id" | "analysis_status">,
  decision: "selected" | "rejected",
  reasonCodes?: string[],
  comment?: string,
  idempotencyKey?: string,
): CreateCreativeFeedbackRequest {
  return {
    idempotency_key: idempotencyKey,
    issue_id: "",
    subject_type: "candidate",
    subject_id: candidate.id,
    event_type: "decision",
    decision,
    reason_codes: reasonCodes,
    comment,
    context_snapshot: { crawl_run_id: candidate.source_run_id, analysis_status: candidate.analysis_status },
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

function recordStringArray(record: Record<string, unknown> | undefined, key: string): string[] {
  const value = record?.[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function MaterialPreview({ candidate, analysis, onClose, onRetry, retrying, onRetryAnalysis, retryingAnalysis }: { candidate: CreativeMaterialCandidate | null; analysis?: CreativeSourceAnalysis; onClose: () => void; onRetry: (candidateId: string) => void; retrying: boolean; onRetryAnalysis: (candidateId: string) => void; retryingAnalysis: boolean }) {
  const source = candidate ? candidateSource(candidate) : "";
  const original = candidate ? resolvePublicFileUrl(candidate.original_url) : "";
  const details = candidate ? materialCandidateDisplayDetails(candidate) : null;
  return <Dialog open={candidate !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-h-[94vh] overflow-hidden p-0 sm:max-w-[min(94vw,1100px)]">
    <DialogHeader className="border-b px-4 py-3 pr-12"><DialogTitle className="truncate text-sm">{candidate?.title || "素材详情"}</DialogTitle><DialogDescription>{candidate?.competitor || sourceLabel(candidate?.connector_id ?? "")}</DialogDescription></DialogHeader>
    <div className="grid max-h-[82vh] overflow-y-auto md:grid-cols-[minmax(0,1fr)_300px]">
      <div className="flex min-h-80 items-center justify-center bg-black p-3">{source ? <img src={source} alt={candidate?.title || "素材"} width={1600} height={1200} className="max-h-[72vh] w-full object-contain" /> : <ImageIcon className="h-8 w-8 text-white/50" />}</div>
      <div className="space-y-5 p-4 text-sm">
        <section aria-labelledby="material-analysis-title" className="border-b pb-5">
          <div className="flex flex-wrap items-center gap-2"><Sparkles className="h-4 w-4" /><h3 id="material-analysis-title" className="font-medium">参考分析</h3>{analysis && <Badge variant="outline">已分析 v{analysis.analysis_version}</Badge>}</div>
          {analysis ? <><p className="mt-3 break-words text-sm">{analysis.summary || "分析已完成"}</p><AnalysisFacts analysis={analysis} /></> : <p className="mt-2 text-xs text-muted-foreground">该素材还没有完成参考分析。</p>}
		  {candidate?.analysis_status === "failed" && <Button className="mt-3" size="sm" variant="outline" disabled={retryingAnalysis} onClick={() => onRetryAnalysis(candidate.id)}><RefreshCw className={retryingAnalysis ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重新分析</Button>}
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
        <MetadataRow label="标签" value={details?.tags ?? MATERIAL_SOURCE_MISSING} />
        <MetadataRow label="备注" value={details?.note ?? MATERIAL_SOURCE_MISSING} />
        {candidate?.archive_status === "failed" && <div className="border border-destructive/30 bg-destructive/5 p-3"><p className="flex items-center gap-2 text-sm font-medium text-destructive"><AlertTriangle className="h-4 w-4" />稳定归档失败</p><p className="mt-2 break-words text-xs text-muted-foreground">{archiveErrorLabel(candidate.archive_error)}</p><Button className="mt-3" size="sm" variant="outline" disabled={retrying} onClick={() => onRetry(candidate.id)}><RefreshCw className={retrying ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重新归档</Button></div>}
        <div className="flex flex-wrap gap-2 border-t pt-4">
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
        area_names: ["Indonesia"],
        language_names: ["Indonesian"],
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
