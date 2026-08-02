"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Download,
  ExternalLink,
  FileUp,
  ImageIcon,
  Library,
  Maximize2,
  Package,
  RefreshCw,
  RotateCcw,
  Search,
  Settings2,
  Sparkles,
  Target,
  Video,
  X,
} from "lucide-react";
import { zipSync } from "fflate";
import type {
  CreativeCopyEntry,
  CreativeCopyEntryInput,
  CreativeDelivery,
  CreativeIssueItem,
  CreativeMaterialCandidate,
  Issue,
} from "@multica/core/types";
import { api } from "@multica/core/api";
import {
  creativeCopyEntriesOptions,
  creativeKeys,
  creativeMaterialLibraryOptions,
  creativeMaterialsOptions,
  creativeResourcesOptions,
} from "@multica/core/creative";
import { credentialProfilesOptions } from "@multica/core/credential";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { issueAttachmentsOptions, issueKeys, issueTimelineOptions } from "@multica/core/issues/queries";
import { squadListOptions } from "@multica/core/workspace/queries";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";

type CandidateFilter = {
  status: string;
  competitor: string;
  assetType: string;
  media: string;
  area: string;
  language: string;
  platform: string;
  text: string;
};

type MediaPreviewItem = {
  url: string;
  posterUrl?: string;
  title: string;
  subtitle?: string;
  assetType?: string;
  openUrl?: string;
};

type ContextDraft = {
  market_pack_id: string;
  squad_id: string;
};

type CopyDraft = Pick<CreativeCopyEntryInput, "headline" | "subheadline" | "benefit" | "cta" | "legal_text">;
type CreativeBriefDraft = CreativeIssueItem["creative_brief"];
type CreativeDeliveryAsset = { id: string; filename: string; url: string; download_url?: string | null; markdown_url?: string | null };
type CreativeAdjustmentTarget = {
  candidateId: string;
  asset: CreativeDeliveryAsset;
  groupAssets: CreativeDeliveryAsset[];
  delivery?: CreativeDelivery;
  groupDeliveries: CreativeDelivery[];
  variant?: number | null;
};

const DEFAULT_FILTER: CandidateFilter = {
  status: "all",
  competitor: "all",
  assetType: "all",
  media: "all",
  area: "all",
  language: "all",
  platform: "all",
  text: "",
};

const EMPTY_CONTEXT: ContextDraft = { market_pack_id: "", squad_id: "" };
const EMPTY_COPY: CopyDraft = { headline: "", subheadline: "", benefit: "", cta: "", legal_text: "" };
const EMPTY_BRIEF: CreativeBriefDraft = {
  theme: "", theme_elements: [], primary_benefit: "", secondary_benefits: [],
  benefit_value: "", evidence: [], detected_text: [], visual_type: "",
  analysis_summary: "", status: "", source: "", confidence: null,
  analysis_issue_id: "",
};
const STATUS_LABEL: Record<string, string> = { new: "待选", selected: "已选择", rejected: "不采用", archived: "已归档" };

export function CreativeMaterialPool({ issue }: { issue: Issue }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { upload, uploading } = useFileUpload(api);
  const [filter, setFilter] = useState<CandidateFilter>(DEFAULT_FILTER);
  const [candidatePoolExpanded, setCandidatePoolExpanded] = useState(true);
  const [resultBoardExpanded, setResultBoardExpanded] = useState(true);
  const [poolPreviewOpen, setPoolPreviewOpen] = useState(false);
  const [resultPreviewOpen, setResultPreviewOpen] = useState(false);
  const [libraryOpen, setLibraryOpen] = useState(false);
  const [previewItem, setPreviewItem] = useState<MediaPreviewItem | null>(null);
  const [copyCandidateId, setCopyCandidateId] = useState("");
  const [adjustmentTarget, setAdjustmentTarget] = useState<CreativeAdjustmentTarget | null>(null);
  const [adjustmentOpen, setAdjustmentOpen] = useState(false);
  const [activeAssetId, setActiveAssetId] = useState("");
  const [contextDraft, setContextDraft] = useState<ContextDraft>(EMPTY_CONTEXT);

  const materials = useQuery(creativeMaterialsOptions(wsId, issue.id));
  const attachments = useQuery(issueAttachmentsOptions(issue.id));
  const timeline = useQuery(issueTimelineOptions(issue.id));
  const resources = useQuery(creativeResourcesOptions(wsId));
  const squads = useQuery(squadListOptions(wsId));
  const profiles = useQuery(credentialProfilesOptions(wsId));
  const library = useQuery(creativeMaterialLibraryOptions(wsId));
  const candidates = useMemo(() => materials.data?.candidates ?? [], [materials.data?.candidates]);
  const selectedCandidates = candidates.filter((candidate) => candidate.status === "selected");
  const expectedDeliveryCount = selectedCandidates.length * 9;
  const marketPacks = (resources.data?.resources ?? []).filter((resource) => resource.kind === "market_pack" && resource.status === "published");
  const currentContext = materials.data?.context;
  const defaultMarketPackId = marketPacks[0]?.id ?? "";
  const defaultSquadId = squads.data?.[0]?.id ?? "";

  useEffect(() => {
    setContextDraft({
      market_pack_id: currentContext?.market_pack_id || defaultMarketPackId,
      squad_id: currentContext?.squad_id || defaultSquadId,
    });
  }, [currentContext?.market_pack_id, currentContext?.squad_id, defaultMarketPackId, defaultSquadId]);

  const activeMarketPack = marketPacks.find((resource) => resource.id === contextDraft.market_pack_id);
  const copyLibraryId = stringConfig(activeMarketPack?.config, "copy_library_id");
  const benefitOptions = stringArrayConfig(activeMarketPack?.config, "benefit_taxonomy");
  const themeOptions = stringArrayConfig(activeMarketPack?.config, "theme_presets");
  const copyEntries = useQuery(creativeCopyEntriesOptions(wsId, copyLibraryId));
  const itemByCandidate = useMemo(() => new Map((materials.data?.items ?? []).map((item) => [item.candidate_id, item])), [materials.data?.items]);
  const filteredCandidates = useMemo(() => candidates.filter((candidate) => candidateMatchesFilter(candidate, filter)), [candidates, filter]);
  const deliveries = useMemo(() => materials.data?.deliveries ?? [], [materials.data?.deliveries]);
  const deliveryByAttachment = useMemo(() => new Map(deliveries.map((delivery) => [delivery.final_attachment_id, delivery])), [deliveries]);
  const finalAssets = useMemo(() => (attachments.data ?? []).filter((attachment) => attachment.content_type.startsWith("image/") && (isCreativeDeliveryFilename(attachment.filename) || deliveryByAttachment.has(attachment.id))), [attachments.data, deliveryByAttachment]);
  const resultCandidateByAttachment = useMemo(() => {
    const result = new Map<string, string>();
		for (const delivery of deliveries) result.set(delivery.final_attachment_id, delivery.candidate_id);
    for (const entry of timeline.data ?? []) {
      if (entry.type !== "comment" || !entry.content) continue;
      const candidateId = candidateIdFromResultComment(entry.content);
      if (!candidateId) continue;
			for (const attachment of entry.attachments ?? []) {
				if (!result.has(attachment.id)) result.set(attachment.id, candidateId);
			}
    }
    return result;
  }, [deliveries, timeline.data]);
  const activeAsset = finalAssets.find((asset) => asset.id === activeAssetId) ?? finalAssets[0];
  const latestCrawl = materials.data?.crawl_runs[0];
  const contextReady = Boolean(currentContext?.market_pack_id && currentContext?.squad_id);
  const briefReadyItems = selectedCandidates.filter((candidate) => itemByCandidate.get(candidate.id)?.creative_brief.primary_benefit);
  const copyReadyItems = selectedCandidates.filter((candidate) => itemByCandidate.get(candidate.id)?.copy_entry_id);
  const readyItems = selectedCandidates.filter((candidate) => {
    const item = itemByCandidate.get(candidate.id);
    return Boolean(item?.copy_entry_id && item.creative_brief.primary_benefit);
  });
  const briefsToAnalyze = selectedCandidates.filter((candidate) => {
    const brief = itemByCandidate.get(candidate.id)?.creative_brief;
    return !brief?.primary_benefit && brief?.status !== "requested";
  });

  const refreshMaterials = () => queryClient.invalidateQueries({ queryKey: creativeKeys.issue(wsId, issue.id) });
  const updateCandidate = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) => api.updateCreativeMaterialCandidate(issue.id, id, { status }),
    onSuccess: (data) => queryClient.setQueryData(creativeKeys.issue(wsId, issue.id), data),
    onError: () => toast.error("候选状态更新失败"),
  });
  const saveContext = useMutation({
    mutationFn: () => api.putCreativeIssueContext(issue.id, contextDraft),
    onSuccess: () => { refreshMaterials(); toast.success("本 Issue 已固定资源快照"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "资源上下文保存失败"),
  });
  const assignCopy = useMutation({
    mutationFn: ({ candidateId, copyEntryId }: { candidateId: string; copyEntryId: string }) => api.putCreativeItemCopy(issue.id, candidateId, copyEntryId),
    onSuccess: (_, variables) => {
      refreshMaterials();
      const next = selectedCandidates.find((candidate) => candidate.id !== variables.candidateId && !itemByCandidate.get(candidate.id)?.copy_entry_id);
      setCopyCandidateId(next?.id ?? variables.candidateId);
      toast.success(next ? "文案已固定，已切到下一张" : "该图文案已固定");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "文案选择失败"),
  });
  const saveBrief = useMutation({
    mutationFn: ({ candidateId, brief }: { candidateId: string; brief: CreativeBriefDraft }) => api.putCreativeItemBrief(issue.id, candidateId, brief),
    onSuccess: () => { refreshMaterials(); toast.success("创意简报已保存，文案推荐已更新"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "创意简报保存失败"),
  });
  const requestAnalysis = useMutation({
    mutationFn: async (targets: CreativeMaterialCandidate[]) => {
      if (!currentContext?.squad_id) throw new Error("先固定市场资源包和执行小队");
      await mapWithConcurrency(targets, 4, async (candidate) => {
        const current = itemByCandidate.get(candidate.id)?.creative_brief ?? EMPTY_BRIEF;
        const analysisIssue = await api.createIssue({
          title: `素材理解 · ${candidate.title || candidate.competitor || candidate.id}`,
          description: `目标候选池 Issue：${issue.id}\n候选 ID：${candidate.id}\n市场资源包：${currentContext.market_pack_id}\n\n使用广告参考分析 Skill 读取候选图真实像素和文字，分别识别视觉主题、主题元素、主利益点、辅助利益点、具体数值和证据。将结构化结果回写目标候选图；主题不能替代金融利益点，采集标题和标签只能作为弱辅助。`,
          parent_issue_id: issue.id,
          assignee_type: "squad",
          assignee_id: currentContext.squad_id,
          status: "todo",
        });
        await api.putCreativeItemBrief(issue.id, candidate.id, {
          ...current,
          status: "requested",
          source: current.source === "user" || current.source === "mixed" ? "mixed" : "ai",
          analysis_issue_id: analysisIssue.id,
        });
      });
      return targets.length;
    },
    onSuccess: (count) => {
      refreshMaterials();
      queryClient.invalidateQueries({ queryKey: issueKeys.children(wsId, issue.id) });
      toast.success(`已委派 ${count} 张素材理解任务`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法发起素材理解"),
  });
  const createCustomCopy = useMutation({
    mutationFn: async ({ candidateId, value }: { candidateId: string; value: CopyDraft }) => {
      if (!copyLibraryId) throw new Error("市场资源包尚未绑定文案库");
      const externalKey = `issue-${issue.id}-candidate-${candidateId}-${Date.now()}`;
      await api.importCreativeCopyEntries(copyLibraryId, {
        mode: "upsert",
        source_filename: "Issue 在线编辑",
        mapping: {},
        entries: [{ ...value, external_key: externalKey, copy_role: "issue_override", market: stringConfig(activeMarketPack?.config, "market"), locale: stringConfig(activeMarketPack?.config, "locale"), tags: ["issue-override"], status: "approved", metadata: { issue_id: issue.id, candidate_id: candidateId } }],
      });
      const latest = await api.listCreativeCopyEntries(copyLibraryId);
      const created = latest.entries.find((entry) => entry.external_key === externalKey);
      if (!created) throw new Error("新文案保存后未能读取");
      return api.putCreativeItemCopy(issue.id, candidateId, created.id);
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.copyEntries(wsId, copyLibraryId) });
      refreshMaterials();
      const next = selectedCandidates.find((candidate) => candidate.id !== variables.candidateId && !itemByCandidate.get(candidate.id)?.copy_entry_id);
      setCopyCandidateId(next?.id ?? variables.candidateId);
      toast.success(next ? "自定义文案已保存，已切到下一张" : "本图自定义文案已保存并选中");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "自定义文案保存失败"),
  });
  const importFromLibrary = useMutation({
    mutationFn: (selected: CreativeMaterialCandidate[]) => api.importCreativeMaterials(issue.id, { connector_id: "workspace-library", query_summary: "从工作区素材库导入", materials: selected.map(candidateToImport) }),
    onSuccess: () => { refreshMaterials(); setLibraryOpen(false); toast.success("素材已加入候选池"); },
    onError: () => toast.error("素材库导入失败"),
  });
  const uploadCandidate = useMutation({
    mutationFn: async (file: File) => {
      const uploaded = await upload(file);
      if (!uploaded) throw new Error("文件上传失败");
      return api.importCreativeMaterials(issue.id, { connector_id: "manual-upload", query_summary: "人工上传", materials: [{ external_id: uploaded.id, dedupe_key: `attachment:${uploaded.id}`, competitor: "人工上传", title: file.name, asset_type: "image", preview_url: uploaded.markdownLink, resource_url: uploaded.markdownLink, original_url: uploaded.markdownLink, raw: { attachment_id: uploaded.id } }] });
    },
    onSuccess: () => { refreshMaterials(); toast.success("图片已加入候选池"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "图片导入失败"),
  });
  const requestCrawl = useMutation({
    mutationFn: async () => {
      if (!contextReady) throw new Error("先固定市场资源包和执行小队");
      const profile = (profiles.data?.profiles ?? []).find((item) => item.connector_id === "appgrowing" && item.status === "active");
      if (!profile) throw new Error("AppGrowing 尚未授权，请前往设置 - 集成绑定");
      return api.createIssue({
        title: `真实素材采集 · ${issue.identifier}`,
        description: `目标候选池 Issue：${issue.id}\n连接器：AppGrowing\n授权配置：${profile.id}\n\n读取父 Issue 的业务筛选条件，使用 AppGrowing 素材采集 Skill 创建并委派采集子 Issue。真实结果只导入父 Issue 原生候选池。`,
        parent_issue_id: issue.id,
        assignee_type: "squad",
        assignee_id: currentContext!.squad_id,
        status: "todo",
      });
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: issueKeys.children(wsId, issue.id) }); toast.success("Leader 已收到真实采集请求"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法发起采集"),
  });
  const startCreative = useMutation({
    mutationFn: async () => {
      if (!currentContext) throw new Error("先固定资源上下文");
      const targets = selectedCandidates.filter((candidate) => {
        const item = itemByCandidate.get(candidate.id);
        return item?.copy_entry_id && !item.work_issue_id;
      });
      if (targets.length === 0) throw new Error("没有待创建的逐图任务");
      await mapWithConcurrency(targets, 4, async (candidate) => {
        const item = itemByCandidate.get(candidate.id)!;
        const copy = item.copy_snapshot as Partial<CreativeCopyEntry>;
        const brief = item.creative_brief;
        const created = await api.createIssue({
          title: `创意图 · ${candidate.title || candidate.competitor || candidate.id} · r${item.revision}`,
          description: `父候选池：${issue.id}\n候选 ID：${candidate.id}\n文案记录：${item.copy_entry_id} · v${copy.version ?? 1}\n创意组合：${creativeBriefLabel(brief)}\n主利益点：${brief.primary_benefit}${brief.benefit_value ? ` · ${brief.benefit_value}` : ""}\n市场资源包：${currentContext.market_pack_id}\n执行小队：${currentContext.squad_id}\n修订：r${item.revision}\n\n交付契约：为本候选生成 V01、V02、V03 三个有明确视觉差异的创意变体；每个变体原生交付 1080x1080、1200x628、800x1000 三个尺寸，共 9 张最终成图。先并发生成三个 1080x1080 创意母版；每个母版通过后，立即以该母版为第一参考并发原生重排横版和竖版。尺寸不是变体，禁止由一个尺寸裁切、加边或拉伸得到另外两个尺寸。\n\n使用父 Issue 固定快照中的创意简报、文案和资源。主题控制视觉表达，主利益点控制信息层级；Leader 一次创建所有已满足依赖的专业子 issue，按变体隔离证据和返工，只把 3 个变体各自验收通过的三尺寸成图发布回父 Issue。`,
          parent_issue_id: issue.id,
          assignee_type: "squad",
          assignee_id: currentContext.squad_id,
          status: "todo",
        });
        await api.putCreativeItemWorkIssue(issue.id, candidate.id, created.id);
      });
      return targets.length;
    },
    onSuccess: (count) => { refreshMaterials(); queryClient.invalidateQueries({ queryKey: issueKeys.children(wsId, issue.id) }); toast.success(`已创建 ${count} 套创意任务，每套将交付 3 个创意 × 3 个尺寸`); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法开始修图"),
  });

  const workflowEnabled = issue.metadata["workflow"] === "creative_material" || candidates.length > 0 || finalAssets.length > 0;
  if (!workflowEnabled) return null;

  return <section className="space-y-4 border-y py-5">
    <div className="border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
        <button type="button" className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={() => setCandidatePoolExpanded((value) => !value)}><ChevronRight className={cn("h-4 w-4 shrink-0 text-muted-foreground transition-transform", candidatePoolExpanded && "rotate-90")} /><span className="text-sm font-semibold">素材候选池</span><Badge variant="outline">{candidates.length} 张</Badge><Badge variant="outline">已选 {selectedCandidates.length}</Badge></button>
        <div className="flex flex-wrap items-center gap-2"><input ref={fileInputRef} type="file" accept="image/*" className="hidden" onChange={(event) => { const file = event.target.files?.[0]; if (file) uploadCandidate.mutate(file); event.currentTarget.value = ""; }} /><Button size="sm" variant="outline" onClick={() => requestCrawl.mutate()} disabled={requestCrawl.isPending}><RefreshCw className={cn("h-4 w-4", requestCrawl.isPending && "animate-spin")} />采集素材</Button><Button size="sm" variant="outline" onClick={() => setLibraryOpen(true)}><Library className="h-4 w-4" />素材库</Button><Button size="icon-sm" variant="outline" title="上传图片" onClick={() => fileInputRef.current?.click()} disabled={uploading || uploadCandidate.isPending}><FileUp className="h-4 w-4" /></Button><Button size="icon-sm" variant="outline" title="放大候选池" onClick={() => setPoolPreviewOpen(true)} disabled={candidates.length === 0}><Maximize2 className="h-4 w-4" /></Button></div>
      </div>
      {candidatePoolExpanded && <div className="border-t">
        <div className="grid gap-3 border-b bg-muted/20 px-4 py-3 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_auto] lg:items-end"><Field label="市场资源包"><NativeSelect value={contextDraft.market_pack_id} onChange={(event) => setContextDraft({ ...contextDraft, market_pack_id: event.target.value })}><NativeSelectOption value="">选择已发布资源包</NativeSelectOption>{marketPacks.map((resource) => <NativeSelectOption key={resource.id} value={resource.id}>{resource.name} · v{resource.published_version}</NativeSelectOption>)}</NativeSelect></Field><Field label="执行小队"><NativeSelect value={contextDraft.squad_id} onChange={(event) => setContextDraft({ ...contextDraft, squad_id: event.target.value })}><NativeSelectOption value="">选择小队</NativeSelectOption>{(squads.data ?? []).map((squad) => <NativeSelectOption key={squad.id} value={squad.id}>{squad.name}</NativeSelectOption>)}</NativeSelect></Field><Button size="sm" onClick={() => saveContext.mutate()} disabled={saveContext.isPending || !contextDraft.market_pack_id || !contextDraft.squad_id}><Settings2 className="h-4 w-4" />{contextReady ? "更新快照" : "固定资源"}</Button></div>
        {selectedCandidates.length > 0 && <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 border-b bg-background/95 px-4 py-3 shadow-sm backdrop-blur"><div><p className="text-sm font-medium">已选 {selectedCandidates.length} 张 · 已识别利益点 {briefReadyItems.length} 张 · 已定文案 {copyReadyItems.length} 张</p><p className="mt-0.5 text-xs text-muted-foreground">每张图交付 3 个创意 × 3 个尺寸，预计 {expectedDeliveryCount} 张；文案与返工按素材和创意独立保存。</p></div><div className="flex flex-wrap gap-2">{briefsToAnalyze.length > 0 && <Button size="sm" variant="outline" onClick={() => requestAnalysis.mutate(briefsToAnalyze)} disabled={requestAnalysis.isPending || !contextReady}><Sparkles className={cn("h-4 w-4", requestAnalysis.isPending && "animate-pulse")} />识别创意 ({briefsToAnalyze.length})</Button>}<Button size="sm" variant="outline" onClick={() => setCopyCandidateId(selectedCandidates.find((candidate) => !itemByCandidate.get(candidate.id)?.creative_brief.primary_benefit || !itemByCandidate.get(candidate.id)?.copy_entry_id)?.id ?? selectedCandidates[0]?.id ?? "")}><Settings2 className="h-4 w-4" />逐图配置</Button><Button size="sm" onClick={() => startCreative.mutate()} disabled={startCreative.isPending || !contextReady || readyItems.length !== selectedCandidates.length}>{startCreative.isPending ? "正在创建" : `生成 ${selectedCandidates.length} 套创意`}</Button></div></div>}
        <div className="px-4 py-4"><div className="mb-3 flex flex-wrap items-center justify-between gap-3"><p className="text-xs text-muted-foreground">{latestCrawl ? `${latestCrawl.query_summary} · 新增 ${latestCrawl.imported_count}` : "等待真实采集、素材库导入或人工上传"}</p>{!candidateFilterEquals(filter, DEFAULT_FILTER) && <button type="button" className="inline-flex items-center gap-1 text-xs text-muted-foreground" onClick={() => setFilter(DEFAULT_FILTER)}><RotateCcw className="h-3.5 w-3.5" />显示全部</button>}</div><CandidateFilters candidates={candidates} filter={filter} onChange={setFilter} /><div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">{filteredCandidates.map((candidate) => <CandidateCard key={candidate.id} candidate={candidate} item={itemByCandidate.get(candidate.id)} busy={updateCandidate.isPending} onStatus={(status) => updateCandidate.mutate({ id: candidate.id, status })} onPreview={setPreviewItem} onCopy={() => setCopyCandidateId(candidate.id)} />)}</div>{filteredCandidates.length === 0 && <div className="mt-4 flex min-h-36 items-center justify-center border border-dashed text-sm text-muted-foreground">候选池暂无匹配素材</div>}</div>
      </div>}
    </div>

    <IssueResultBoard archiveName={`${issue.identifier}-修图结果`} assets={finalAssets} activeAsset={activeAsset} candidates={candidates} candidateByAttachment={resultCandidateByAttachment} deliveryByAttachment={deliveryByAttachment} expanded={resultBoardExpanded} onExpandedChange={setResultBoardExpanded} onAssetChange={setActiveAssetId} onPreview={setPreviewItem} onOpenBoardPreview={() => setResultPreviewOpen(true)} onAdjust={(target) => { setAdjustmentTarget(target); setAdjustmentOpen(true); }} />

    <CandidatePoolPreviewDialog open={poolPreviewOpen} onOpenChange={setPoolPreviewOpen} candidates={candidates} items={materials.data?.items ?? []} filter={filter} onFilterChange={setFilter} selectedCount={selectedCandidates.length} busy={updateCandidate.isPending} onStatus={(candidateId, status) => updateCandidate.mutate({ id: candidateId, status })} onPreview={setPreviewItem} onCopy={(candidateId) => { setPoolPreviewOpen(false); setCopyCandidateId(candidateId); }} />
    <ResultBoardPreviewDialog open={resultPreviewOpen} onOpenChange={setResultPreviewOpen} archiveName={`${issue.identifier}-修图结果`} assets={finalAssets} activeAsset={activeAsset} candidates={candidates} candidateByAttachment={resultCandidateByAttachment} deliveryByAttachment={deliveryByAttachment} onAssetChange={setActiveAssetId} onPreview={setPreviewItem} onAdjust={(target) => { setResultPreviewOpen(false); setAdjustmentTarget(target); setAdjustmentOpen(true); }} />
    <MaterialLibraryDialog open={libraryOpen} onOpenChange={setLibraryOpen} candidates={library.data?.candidates ?? []} existingIds={new Set(candidates.map((candidate) => candidate.id))} busy={importFromLibrary.isPending} onImport={(selected) => importFromLibrary.mutate(selected)} />
    <CopyPickerDialog candidates={selectedCandidates} activeCandidateId={copyCandidateId} entries={copyEntries.data?.entries ?? []} items={materials.data?.items ?? []} benefitOptions={benefitOptions} themeOptions={themeOptions} busy={assignCopy.isPending || createCustomCopy.isPending || saveBrief.isPending} analysisBusy={requestAnalysis.isPending} onCandidateId={setCopyCandidateId} onClose={() => setCopyCandidateId("")} onPick={(candidateId, copyEntryId) => assignCopy.mutate({ candidateId, copyEntryId })} onCustom={(candidateId, value) => createCustomCopy.mutate({ candidateId, value })} onBrief={(candidateId, brief) => saveBrief.mutate({ candidateId, brief })} onAnalyze={(candidate) => requestAnalysis.mutate([candidate])} />
		<AdjustmentDialog open={adjustmentOpen} onOpenChange={setAdjustmentOpen} issue={issue} candidates={selectedCandidates} target={adjustmentTarget} context={currentContext} items={materials.data?.items ?? []} onCreated={() => { setAdjustmentOpen(false); refreshMaterials(); queryClient.invalidateQueries({ queryKey: issueKeys.children(wsId, issue.id) }); }} />
    <MediaPreviewDialog item={previewItem} onOpenChange={(open) => !open && setPreviewItem(null)} />
  </section>;
}

function CandidatePoolPreviewDialog({ open, onOpenChange, candidates, items, filter, onFilterChange, selectedCount, busy, onStatus, onPreview, onCopy }: { open: boolean; onOpenChange: (open: boolean) => void; candidates: CreativeMaterialCandidate[]; items: CreativeIssueItem[]; filter: CandidateFilter; onFilterChange: (filter: CandidateFilter) => void; selectedCount: number; busy: boolean; onStatus: (candidateId: string, status: string) => void; onPreview: (item: MediaPreviewItem) => void; onCopy: (candidateId: string) => void }) {
  const filtered = candidates.filter((candidate) => candidateMatchesFilter(candidate, filter));
  const itemByCandidate = new Map(items.map((item) => [item.candidate_id, item]));
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="grid h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(98vw,1440px)]"><DialogHeader className="border-b px-5 py-4 pr-12"><DialogTitle>素材候选池</DialogTitle><div className="mt-2 flex gap-2"><Badge variant="outline">{candidates.length} 张</Badge><Badge variant="outline">已选 {selectedCount}</Badge></div><div className="mt-4"><CandidateFilters candidates={candidates} filter={filter} onChange={onFilterChange} /></div></DialogHeader><div className="min-h-0 overflow-y-auto bg-muted/20 p-5"><div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{filtered.map((candidate) => <CandidateCard key={candidate.id} candidate={candidate} item={itemByCandidate.get(candidate.id)} busy={busy} onStatus={(status) => onStatus(candidate.id, status)} onPreview={onPreview} onCopy={() => onCopy(candidate.id)} />)}</div></div></DialogContent></Dialog>;
}

type ResultBoardProps = {
  archiveName: string;
  assets: CreativeDeliveryAsset[];
  activeAsset?: CreativeDeliveryAsset;
  candidates: CreativeMaterialCandidate[];
  candidateByAttachment: ReadonlyMap<string, string>;
  deliveryByAttachment: ReadonlyMap<string, CreativeDelivery>;
  onAssetChange: (id: string) => void;
  onPreview: (item: MediaPreviewItem) => void;
  onAdjust: (target: CreativeAdjustmentTarget) => void;
};

export function ResultBoardPreviewDialog({
  open,
  onOpenChange,
  ...boardProps
}: ResultBoardProps & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const groups = groupCreativeDeliveries(boardProps.assets);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(98vw,1560px)]">
        <DialogHeader className="border-b px-5 py-4 pr-14">
          <div className="flex flex-wrap items-center gap-2">
            <DialogTitle>修图结果看板预览</DialogTitle>
            <Badge variant="outline">{groups.length} 张创意</Badge>
            <Badge variant="outline">
              {groups.flatMap((group) => group.assets).length} 个最新尺寸
            </Badge>
          </div>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto lg:overflow-hidden">
          <IssueResultBoard
            {...boardProps}
            expanded
            onExpandedChange={() => undefined}
            previewMode
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CandidateFilters({ candidates, filter, onChange }: { candidates: CreativeMaterialCandidate[]; filter: CandidateFilter; onChange: (filter: CandidateFilter) => void }) {
  const set = (key: keyof CandidateFilter, value: string) => onChange({ ...filter, [key]: value });
  return <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8"><FilterSelect label="状态" value={filter.status} values={["new", "selected", "rejected"]} labels={STATUS_LABEL} onChange={(value) => set("status", value)} /><FilterSelect label="竞品" value={filter.competitor} values={uniqueOptions(candidates.map((candidate) => candidate.competitor))} onChange={(value) => set("competitor", value)} /><FilterSelect label="类型" value={filter.assetType} values={uniqueOptions(candidates.map((candidate) => candidate.asset_type))} onChange={(value) => set("assetType", value)} /><FilterSelect label="媒体" value={filter.media} values={uniqueOptions(candidates.flatMap((candidate) => candidate.media_names))} onChange={(value) => set("media", value)} /><FilterSelect label="地区" value={filter.area} values={uniqueOptions(candidates.flatMap((candidate) => candidate.area_names))} onChange={(value) => set("area", value)} /><FilterSelect label="语言" value={filter.language} values={uniqueOptions(candidates.flatMap((candidate) => candidate.language_names))} onChange={(value) => set("language", value)} /><FilterSelect label="设备" value={filter.platform} values={uniqueOptions(candidates.flatMap((candidate) => candidate.platform_names))} onChange={(value) => set("platform", value)} /><label className="grid gap-1 text-xs font-medium text-muted-foreground">搜索<div className="relative"><Search className="absolute left-2 top-2 h-4 w-4" /><Input value={filter.text} onChange={(event) => set("text", event.target.value)} className="h-8 pl-7" /></div></label></div>;
}

function FilterSelect({ label, value, values, labels = {}, onChange }: { label: string; value: string; values: string[]; labels?: Record<string, string>; onChange: (value: string) => void }) {
  return <label className="grid gap-1 text-xs font-medium text-muted-foreground">{label}<NativeSelect size="sm" value={value} onChange={(event) => onChange(event.target.value)}><NativeSelectOption value="all">全部</NativeSelectOption>{values.map((item) => <NativeSelectOption key={item} value={item}>{labels[item] ?? item}</NativeSelectOption>)}</NativeSelect></label>;
}

function CandidateCard({ candidate, item, busy, onStatus, onPreview, onCopy }: { candidate: CreativeMaterialCandidate; item?: CreativeIssueItem; busy: boolean; onStatus: (status: string) => void; onPreview: (item: MediaPreviewItem) => void; onCopy?: () => void }) {
  const mediaURL = firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url);
  const previewURL = candidate.asset_type === "video" ? firstNonEmpty(candidate.resource_url, mediaURL) : mediaURL;
  const selected = candidate.status === "selected";
  const rejected = candidate.status === "rejected";
  const copy = item?.copy_snapshot as Partial<CreativeCopyEntry> | undefined;
  const brief = item?.creative_brief;
  return <article className={cn("group overflow-hidden border bg-background", selected && "border-emerald-600 ring-1 ring-emerald-600/20", rejected && "opacity-55")}><button type="button" className="relative block aspect-[4/3] w-full bg-muted" onClick={() => onPreview({ url: previewURL, posterUrl: candidate.poster_url || candidate.preview_url, title: candidate.title || candidate.competitor || "素材预览", subtitle: candidate.competitor, assetType: candidate.asset_type, openUrl: firstNonEmpty(candidate.resource_url, candidate.original_url) })}><MediaPreview url={previewURL} posterUrl={candidate.poster_url || candidate.preview_url} alt={candidate.title || candidate.competitor} assetType={candidate.asset_type} compact /><Badge variant="secondary" className="absolute left-2 top-2 bg-background/90">{candidate.asset_type === "video" ? <Video className="h-3 w-3" /> : <ImageIcon className="h-3 w-3" />}{candidate.asset_type === "video" ? "视频" : "图片"}</Badge>{selected && <span className="absolute right-2 top-2 inline-flex h-7 w-7 items-center justify-center rounded-full bg-emerald-600 text-white"><Check className="h-4 w-4" /></span>}</button><div className="space-y-3 p-3"><div><h3 className="truncate text-sm font-semibold">{candidate.competitor || "未命名竞品"}</h3><p className="mt-1 line-clamp-2 min-h-9 text-sm text-muted-foreground">{candidate.title || "未返回标题"}</p></div><div className="grid grid-cols-2 gap-2 bg-muted/50 p-2 text-xs"><Metric label="投放天数" value={formatDuration(candidate.duration_days)} /><Metric label="曝光估算" value={formatImpression(candidate.impression_estimate)} /></div>{selected && <button type="button" onClick={onCopy} className="w-full border-l-2 border-emerald-600 bg-muted/30 px-3 py-2 text-left"><span className="flex items-center gap-1 text-[10px] text-muted-foreground"><Target className="h-3 w-3" />创意组合{brief?.status === "requested" && " · 识别中"}</span><span className="mt-0.5 block truncate text-sm font-medium">{brief?.primary_benefit ? creativeBriefLabel(brief) : "识别或填写主利益点"}</span><span className="mt-1 block truncate text-xs text-muted-foreground">{copy?.headline || "选择或编辑文案"}</span></button>}<div className="flex items-center justify-between"><Button size="sm" variant={selected ? "outline" : "default"} disabled={busy} onClick={() => onStatus(selected ? "new" : "selected")}>{selected ? "取消选择" : "选择素材"}</Button><Button size="sm" variant="ghost" disabled={busy} onClick={() => onStatus(rejected ? "new" : "rejected")}><X className="h-4 w-4" />{rejected ? "恢复" : "不采用"}</Button></div></div></article>;
}

function MaterialLibraryDialog({ open, onOpenChange, candidates, existingIds, busy, onImport }: { open: boolean; onOpenChange: (open: boolean) => void; candidates: CreativeMaterialCandidate[]; existingIds: Set<string>; busy: boolean; onImport: (selected: CreativeMaterialCandidate[]) => void }) {
  const [selected, setSelected] = useState<string[]>([]);
  useEffect(() => { if (!open) setSelected([]); }, [open]);
  const available = candidates.filter((candidate) => !existingIds.has(candidate.id));
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-w-5xl"><DialogHeader><DialogTitle>从工作区素材库导入</DialogTitle></DialogHeader><div className="grid max-h-[62vh] grid-cols-2 gap-px overflow-y-auto border bg-border md:grid-cols-3 lg:grid-cols-4">{available.map((candidate) => { const checked = selected.includes(candidate.id); return <button key={candidate.id} type="button" onClick={() => setSelected(checked ? selected.filter((id) => id !== candidate.id) : [...selected, candidate.id])} className={cn("bg-background text-left", checked && "ring-2 ring-inset ring-emerald-600")}><div className="aspect-[4/3] bg-muted"><MediaPreview url={firstNonEmpty(candidate.archived_url, candidate.preview_url)} alt={candidate.title || candidate.competitor} compact /></div><div className="p-2"><p className="truncate text-sm font-medium">{candidate.title || candidate.competitor}</p><p className="truncate text-xs text-muted-foreground">{candidate.competitor}</p></div></button>; })}</div><DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={selected.length === 0 || busy} onClick={() => onImport(available.filter((candidate) => selected.includes(candidate.id)))}>导入 {selected.length} 张</Button></DialogFooter></DialogContent></Dialog>;
}

function CopyPickerDialog({ candidates, activeCandidateId, entries, items, benefitOptions, themeOptions, busy, analysisBusy, onCandidateId, onClose, onPick, onCustom, onBrief, onAnalyze }: { candidates: CreativeMaterialCandidate[]; activeCandidateId: string; entries: CreativeCopyEntry[]; items: CreativeIssueItem[]; benefitOptions: string[]; themeOptions: string[]; busy: boolean; analysisBusy: boolean; onCandidateId: (id: string) => void; onClose: () => void; onPick: (candidateId: string, copyEntryId: string) => void; onCustom: (candidateId: string, value: CopyDraft) => void; onBrief: (candidateId: string, value: CreativeBriefDraft) => void; onAnalyze: (candidate: CreativeMaterialCandidate) => void }) {
  const [mode, setMode] = useState<"library" | "custom">("library");
  const [search, setSearch] = useState("");
  const [draft, setDraft] = useState<CopyDraft>(EMPTY_COPY);
  const [briefDraft, setBriefDraft] = useState<CreativeBriefDraft>(EMPTY_BRIEF);
  const [briefEditing, setBriefEditing] = useState(false);
  const activeIndex = Math.max(0, candidates.findIndex((candidate) => candidate.id === activeCandidateId));
  const candidate = candidates[activeIndex];
  const selected = items.find((item) => item.candidate_id === candidate?.id);
  const currentBrief = selected?.creative_brief ?? EMPTY_BRIEF;
  const recommendations = useMemo(() => candidate ? recommendCopyEntries(candidate, entries, currentBrief) : [], [candidate, entries, currentBrief]);
  const visible = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase();
    if (!needle) return recommendations;
    return recommendations.filter(({ entry }) => [entry.external_key, entry.headline, entry.subheadline, entry.benefit, entry.cta, entry.copy_role, ...entry.tags].join(" ").toLocaleLowerCase().includes(needle));
  }, [recommendations, search]);
  useEffect(() => {
    if (!candidate) return;
    const copy = selected?.copy_snapshot as Partial<CreativeCopyEntry> | undefined;
    setDraft({ headline: copy?.headline ?? "", subheadline: copy?.subheadline ?? "", benefit: copy?.benefit ?? "", cta: copy?.cta ?? "", legal_text: copy?.legal_text ?? "" });
    setBriefDraft({ ...currentBrief, theme_elements: [...currentBrief.theme_elements], secondary_benefits: [...currentBrief.secondary_benefits], evidence: [...currentBrief.evidence], detected_text: [...currentBrief.detected_text] });
    setBriefEditing(false);
    setMode("library");
    setSearch("");
  }, [candidate, currentBrief, selected]);
  const move = (offset: number) => {
    if (!candidates.length) return;
    onCandidateId(candidates[(activeIndex + offset + candidates.length) % candidates.length]?.id ?? "");
  };
  const saveCurrentBrief = () => {
    if (!candidate || !briefDraft.primary_benefit.trim()) return;
    onBrief(candidate.id, {
      ...briefDraft,
      theme: briefDraft.theme.trim(),
      theme_elements: uniqueOptions(briefDraft.theme_elements),
      primary_benefit: briefDraft.primary_benefit.trim(),
      secondary_benefits: uniqueOptions(briefDraft.secondary_benefits),
      benefit_value: briefDraft.benefit_value.trim(),
      evidence: uniqueOptions(briefDraft.evidence),
      detected_text: uniqueOptions(briefDraft.detected_text),
      visual_type: briefDraft.visual_type.trim(),
      analysis_summary: briefDraft.analysis_summary.trim(),
      status: "confirmed",
      source: currentBrief.source === "ai" || currentBrief.source === "mixed" ? "mixed" : "user",
    });
    setBriefEditing(false);
  };
  return <Dialog open={Boolean(candidate && activeCandidateId)} onOpenChange={(open) => !open && onClose()}><DialogContent className="grid h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(98vw,1560px)]">
    <DialogHeader className="flex-row items-center justify-between border-b px-5 py-4 pr-14"><div><DialogTitle>逐图配置创意与文案</DialogTitle><p className="mt-1 text-xs text-muted-foreground">第 {activeIndex + 1} / {candidates.length} 张 · 已识别利益点 {items.filter((item) => candidates.some((candidate) => candidate.id === item.candidate_id) && item.creative_brief.primary_benefit).length} 张 · 已定文案 {items.filter((item) => candidates.some((candidate) => candidate.id === item.candidate_id) && item.copy_entry_id).length} 张</p></div><div className="flex gap-2"><Button size="icon-sm" variant="outline" title="上一张" onClick={() => move(-1)}><ChevronLeft className="h-4 w-4" /></Button><Button size="icon-sm" variant="outline" title="下一张" onClick={() => move(1)}><ChevronRight className="h-4 w-4" /></Button></div></DialogHeader>
    <div className="grid min-h-0 lg:grid-cols-[180px_minmax(320px,1fr)_minmax(460px,560px)]">
      <div className="min-h-0 overflow-y-auto border-r bg-muted/15 p-2"><div className="space-y-2">{candidates.map((item, index) => { const itemCopy = items.find((value) => value.candidate_id === item.id); return <button key={item.id} type="button" onClick={() => onCandidateId(item.id)} className={cn("grid w-full grid-cols-[52px_minmax(0,1fr)] gap-2 border bg-background p-2 text-left", item.id === candidate?.id && "border-emerald-600 ring-1 ring-emerald-600/20")}><div className="aspect-square overflow-hidden bg-muted"><MediaPreview url={firstNonEmpty(item.archived_url, item.preview_url, item.poster_url)} alt={item.title || item.competitor} compact /></div><div className="min-w-0"><p className="truncate text-xs font-medium">{index + 1}. {item.title || item.competitor}</p><p className={cn("mt-1 truncate text-[11px]", itemCopy?.creative_brief.primary_benefit ? "text-emerald-700" : "text-muted-foreground")}>{itemCopy?.creative_brief.primary_benefit ? creativeBriefLabel(itemCopy.creative_brief) : itemCopy?.creative_brief.status === "requested" ? "识别中" : "待定利益点"}</p><p className="mt-0.5 truncate text-[11px] text-muted-foreground">{itemCopy?.copy_entry_id ? (itemCopy.copy_snapshot as Partial<CreativeCopyEntry>).headline || "已定文案" : "待定文案"}</p></div></button>; })}</div></div>
      <div className="flex min-h-0 flex-col border-r bg-black"><div className="flex min-h-0 flex-1 items-center justify-center p-4">{candidate && <MediaPreview url={firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url)} posterUrl={candidate.poster_url} alt={candidate.title || candidate.competitor} assetType={candidate.asset_type} />}</div><div className="border-t border-white/15 bg-black px-4 py-3 text-white"><p className="truncate text-sm font-medium">{candidate?.title || candidate?.competitor}</p><p className="mt-1 truncate text-xs text-white/60">{candidate?.competitor}</p></div></div>
      <div className="grid min-h-0 grid-rows-[auto_auto_minmax(0,1fr)_auto] bg-background">
        <div className="border-b bg-muted/20 px-4 py-3">{briefEditing ? <div className="space-y-3"><div className="flex items-center justify-between gap-3"><div><p className="text-sm font-semibold">创意简报</p><p className="text-xs text-muted-foreground">主题控制视觉，主利益点控制文案与信息层级。</p></div><Button size="sm" variant="ghost" onClick={() => setBriefEditing(false)}>取消</Button></div><datalist id="creative-theme-options">{themeOptions.map((value) => <option key={value} value={value} />)}</datalist><datalist id="creative-benefit-options">{benefitOptions.map((value) => <option key={value} value={value} />)}</datalist><div className="grid gap-2 sm:grid-cols-2"><Field label="视觉主题"><Input list="creative-theme-options" value={briefDraft.theme} placeholder="如：世界杯 / 足球赛事" onChange={(event) => setBriefDraft({ ...briefDraft, theme: event.target.value })} /></Field><Field label="主利益点"><Input list="creative-benefit-options" value={briefDraft.primary_benefit} placeholder="如：费用减免" onChange={(event) => setBriefDraft({ ...briefDraft, primary_benefit: event.target.value })} /></Field><Field label="主题元素"><Input value={briefDraft.theme_elements.join("、")} placeholder="球场、足球、欢呼" onChange={(event) => setBriefDraft({ ...briefDraft, theme_elements: splitBriefList(event.target.value) })} /></Field><Field label="辅助利益点"><Input value={briefDraft.secondary_benefits.join("、")} placeholder="低利率、灵活期限" onChange={(event) => setBriefDraft({ ...briefDraft, secondary_benefits: splitBriefList(event.target.value) })} /></Field><Field label="关键数值"><Input value={briefDraft.benefit_value} placeholder="如：Biaya turun 25%" onChange={(event) => setBriefDraft({ ...briefDraft, benefit_value: event.target.value })} /></Field><Field label="画面类型"><Input value={briefDraft.visual_type} placeholder="如：主题活动海报" onChange={(event) => setBriefDraft({ ...briefDraft, visual_type: event.target.value })} /></Field><Field label="识别证据" wide><Textarea rows={2} value={briefDraft.evidence.join("\n")} placeholder="每行一条图片中的文字或视觉证据" onChange={(event) => setBriefDraft({ ...briefDraft, evidence: splitBriefList(event.target.value) })} /></Field></div><div className="flex justify-end"><Button size="sm" disabled={busy || !briefDraft.primary_benefit.trim()} onClick={saveCurrentBrief}>确认创意简报</Button></div></div> : <div className="flex items-start justify-between gap-3"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><Target className="h-4 w-4 text-emerald-700" /><span className="text-sm font-semibold">{currentBrief.primary_benefit ? creativeBriefLabel(currentBrief) : currentBrief.status === "requested" ? "正在识别主题与利益点" : "尚未识别主利益点"}</span>{currentBrief.status === "draft" && <Badge variant="outline">AI 待确认</Badge>}{currentBrief.status === "confirmed" && <Badge variant="outline">已确认</Badge>}</div>{currentBrief.evidence.length > 0 && <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">依据：{currentBrief.evidence.join("；")}</p>}</div><div className="flex shrink-0 gap-1"><Button size="sm" variant="ghost" disabled={analysisBusy || !candidate || currentBrief.status === "requested"} onClick={() => candidate && onAnalyze(candidate)}><Sparkles className="h-4 w-4" />{currentBrief.status === "requested" ? "识别中" : currentBrief.primary_benefit ? "重新识别" : "AI 识别"}</Button><Button size="sm" variant="outline" onClick={() => setBriefEditing(true)}>编辑</Button></div></div>}</div>
        <div className="flex border-b px-4"><button type="button" className={cn("border-b-2 px-4 py-3 text-sm", mode === "library" ? "border-emerald-600 font-medium" : "border-transparent text-muted-foreground")} onClick={() => setMode("library")}>文案库推荐</button><button type="button" className={cn("border-b-2 px-4 py-3 text-sm", mode === "custom" ? "border-emerald-600 font-medium" : "border-transparent text-muted-foreground")} onClick={() => setMode("custom")}>精准编辑</button></div>
        {mode === "library" ? <div className="min-h-0 overflow-y-auto p-4">{!currentBrief.primary_benefit && <div className="mb-3 border-l-2 border-amber-500 bg-amber-50/60 px-3 py-2 text-xs text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">先识别或填写主利益点，才能得到可靠的文案匹配；当前只按通用可用性排序。</div>}<div className="relative"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input className="pl-8" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索标题、卖点、职责或标签" /></div><div className="mt-3 divide-y border">{visible.map(({ entry, reasons }, index) => <button key={entry.id} type="button" onClick={() => candidate && onPick(candidate.id, entry.id)} disabled={busy} className="block w-full px-4 py-3 text-left hover:bg-muted/40"><div className="flex flex-wrap items-center gap-2"><span className="text-sm font-semibold">{entry.headline || entry.subheadline || entry.external_key}</span>{index < 3 && currentBrief.primary_benefit && reasons.length > 0 && <Badge variant="outline">利益点匹配</Badge>}{selected?.copy_entry_id === entry.id && <Badge>当前</Badge>}</div><p className="mt-1 line-clamp-3 whitespace-pre-line text-sm text-muted-foreground">{entry.subheadline || entry.benefit || entry.cta || "无补充文案"}</p>{reasons.length > 0 && <div className="mt-2 flex flex-wrap gap-1">{reasons.map((reason) => <span key={reason} className="bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{reason}</span>)}</div>}</button>)}{visible.length === 0 && <div className="px-4 py-10 text-center text-sm text-muted-foreground">没有匹配文案</div>}</div></div> : <div className="min-h-0 overflow-y-auto p-4"><div className="grid gap-3 sm:grid-cols-2"><Field label="主标题" wide><Input value={draft.headline} onChange={(event) => setDraft({ ...draft, headline: event.target.value })} /></Field><Field label="副标题" wide><Input value={draft.subheadline} onChange={(event) => setDraft({ ...draft, subheadline: event.target.value })} /></Field><Field label="卖点" wide><Textarea rows={5} value={draft.benefit} onChange={(event) => setDraft({ ...draft, benefit: event.target.value })} /></Field><Field label="CTA"><Input value={draft.cta} onChange={(event) => setDraft({ ...draft, cta: event.target.value })} /></Field><Field label="合规文字" wide><Textarea rows={4} value={draft.legal_text} onChange={(event) => setDraft({ ...draft, legal_text: event.target.value })} /></Field></div></div>}
        <DialogFooter className="border-t px-4 py-3"><Button variant="outline" onClick={onClose}>完成</Button>{mode === "custom" && <Button disabled={busy || !candidate || !draft.headline.trim()} onClick={() => candidate && onCustom(candidate.id, draft)}>保存并用于本图</Button>}</DialogFooter>
      </div>
    </div>
  </DialogContent></Dialog>;
}

export function IssueResultBoard({
  archiveName,
  assets,
  activeAsset,
  candidates,
  candidateByAttachment,
  deliveryByAttachment,
  expanded,
  onExpandedChange,
  onAssetChange,
  onPreview,
  onOpenBoardPreview,
  onAdjust,
  previewMode = false,
}: ResultBoardProps & {
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onOpenBoardPreview?: () => void;
  previewMode?: boolean;
}) {
  const [downloading, setDownloading] = useState<"all" | "group" | "">("");
  const groups = groupCreativeDeliveries(assets);
  const latestAssets = groups.flatMap((group) => group.assets);
  const activeGroup =
    groups.find((group) =>
      group.assets.some((asset) => asset.id === activeAsset?.id),
    ) ?? groups[0];
  const groupAssets = activeGroup?.assets ?? [];
  const groupDeliveries = groupAssets.flatMap((asset) => {
    const delivery = deliveryByAttachment.get(asset.id);
    return delivery ? [delivery] : [];
  });
  const currentAsset =
    groupAssets.find((asset) => asset.id === activeAsset?.id) ?? groupAssets[0];
  const candidateId = activeGroup
    ? candidateIdForResultGroup(activeGroup, candidateByAttachment, candidates)
    : "";
  const sourceCandidate = candidates.find(
    (candidate) => candidate.id === candidateId,
  );
  const sourceURL = sourceCandidate
    ? firstNonEmpty(
        sourceCandidate.archived_url,
        sourceCandidate.preview_url,
        sourceCandidate.poster_url,
        sourceCandidate.resource_url,
      )
    : "";
  const previewResult = () =>
    currentAsset &&
    onPreview({
      url: currentAsset.markdown_url || currentAsset.url,
      title: currentAsset.filename,
      assetType: "image",
      openUrl: currentAsset.download_url || currentAsset.url,
    });
  const previewSource = () =>
    sourceCandidate &&
    onPreview({
      url: sourceURL,
      posterUrl: sourceCandidate.poster_url,
      title: sourceCandidate.title || sourceCandidate.competitor || "原始素材",
      subtitle: sourceCandidate.competitor,
      assetType: sourceCandidate.asset_type,
      openUrl: firstNonEmpty(
        sourceCandidate.original_url,
        sourceCandidate.resource_url,
      ),
    });
  const downloadZip = async (scope: "all" | "group") => {
    setDownloading(scope);
    try {
      if (scope === "all")
        await downloadCreativeZip(
          groups.flatMap((group) =>
            group.assets.map((asset) => ({ ...asset, folder: group.label })),
          ),
          archiveName,
        );
      else if (activeGroup)
        await downloadCreativeZip(
          activeGroup.assets.map((asset) => ({
            ...asset,
            folder: activeGroup.label,
          })),
          `${archiveName}-${activeGroup.label}`,
        );
    } finally {
      setDownloading("");
    }
  };
  return (
    <div
      className={cn(
        "border bg-background",
        previewMode &&
          "min-h-full border-0 lg:grid lg:h-full lg:min-h-0 lg:grid-rows-[auto_minmax(0,1fr)]",
      )}
    >
      <div
        className={cn(
          "flex flex-wrap items-center justify-between gap-3 px-4 py-3",
          expanded && "border-b",
        )}
      >
        {previewMode ? (
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs text-muted-foreground">
              {currentAsset?.filename || "选择创意和尺寸查看大图对比"}
            </p>
          </div>
        ) : (
          <button
            type="button"
            className="flex min-w-0 flex-1 items-center gap-2 text-left"
            onClick={() => onExpandedChange(!expanded)}
          >
            <ChevronRight
              className={cn(
                "h-4 w-4 text-muted-foreground transition-transform",
                expanded && "rotate-90",
              )}
            />
            <span className="text-sm font-semibold">修图结果看板</span>
            <Badge variant="outline">{groups.length} 张创意</Badge>
            <Badge variant="outline">{latestAssets.length} 个最新尺寸</Badge>
          </button>
        )}
        {currentAsset && (
          <div className="flex flex-wrap gap-2">
            {!previewMode && onOpenBoardPreview && (
              <Button
                size="icon-sm"
                variant="outline"
                title="放大修图看板"
                onClick={onOpenBoardPreview}
              >
                <Maximize2 className="h-4 w-4" />
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              disabled={Boolean(downloading)}
              onClick={() => void downloadZip("all")}
            >
              <Package className="h-4 w-4" />
              {downloading === "all" ? "打包中" : "下载全部 ZIP"}
            </Button>
            <Button
              size="icon-sm"
              variant="outline"
              title="下载当前尺寸"
              onClick={() => void downloadCreativeAssets([currentAsset])}
            >
              <Download className="h-4 w-4" />
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={Boolean(downloading)}
              onClick={() => void downloadZip("group")}
            >
              <Download className="h-4 w-4" />
              {downloading === "group" ? "打包中" : "本创意 ZIP"}
            </Button>
            {candidateId && (
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  onAdjust({
                    candidateId,
                    asset: currentAsset,
                    groupAssets,
                    delivery: deliveryByAttachment.get(currentAsset.id),
                    groupDeliveries,
                    variant: activeGroup?.variant ?? null,
                  })
                }
              >
                调整当前创意
              </Button>
            )}
          </div>
        )}
      </div>
      {expanded &&
        (assets.length ? (
          <div
            className={cn(
              "grid min-h-80 lg:grid-cols-[240px_minmax(0,1fr)]",
              previewMode && "lg:h-full lg:min-h-0",
            )}
          >
            <div
              className={cn(
                "border-r",
                previewMode && "lg:min-h-0 lg:overflow-y-auto",
              )}
            >
              {groups.map((group) => {
                const groupCandidateId = candidateIdForResultGroup(
                  group,
                  candidateByAttachment,
                  candidates,
                );
                const groupCandidate = candidates.find(
                  (candidate) => candidate.id === groupCandidateId,
                );
                return (
                  <button
                    key={group.key}
                    type="button"
                    className={cn(
                      "block w-full border-b px-3 py-3 text-left",
                      group.key === activeGroup?.key &&
                        "bg-muted/50 shadow-[inset_2px_0_0_hsl(var(--primary))]",
                    )}
                    onClick={() => onAssetChange(group.assets[0]?.id ?? "")}
                  >
                    <p className="truncate text-sm font-medium">
                      {group.variant
                        ? `创意 ${group.variant} · V${String(group.variant).padStart(2, "0")}`
                        : groupCandidate?.title || group.label}
                    </p>
                    <p className="mt-1 truncate text-xs text-muted-foreground">
                      {group.variant
                        ? groupCandidate?.title ||
                          groupCandidate?.competitor ||
                          `${group.assets.length} 个尺寸`
                        : groupCandidate?.competitor ||
                          `${group.assets.length} 个尺寸`}
                    </p>
                  </button>
                );
              })}
            </div>
            <div
              className={cn(
                "min-w-0",
                previewMode &&
                  "lg:grid lg:min-h-0 lg:grid-rows-[auto_minmax(0,1fr)_auto]",
              )}
            >
              <div className="flex gap-2 overflow-x-auto border-b p-3">
                {groupAssets.map((asset) => (
                  <button
                    key={asset.id}
                    type="button"
                    className={cn(
                      "shrink-0 border px-3 py-2 text-xs",
                      asset.id === currentAsset?.id &&
                        "border-emerald-600 bg-emerald-600/5",
                    )}
                    onClick={() => onAssetChange(asset.id)}
                  >
                    <span className="font-semibold">
                      {creativeDeliveryInfo(asset.filename)?.size}
                    </span>
                    <span className="ml-2 text-muted-foreground">通过</span>
                  </button>
                ))}
              </div>
              <div
                className={cn(
                  "grid min-h-[420px]",
                  previewMode && "lg:min-h-0",
                  sourceCandidate && sourceURL
                    ? "md:grid-cols-2"
                    : "grid-cols-1",
                )}
              >
                {sourceCandidate && sourceURL && (
                  <button
                    type="button"
                    className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] border-r text-left"
                    onClick={previewSource}
                    title="查看竞品原图大图"
                  >
                    <span className="flex items-center justify-between gap-2 border-b bg-muted/30 px-4 py-2 text-xs font-medium">
                      竞品原图
                      <Maximize2 className="h-3.5 w-3.5 text-muted-foreground" />
                    </span>
                    <span
                      className={cn(
                        "flex items-center justify-center bg-black p-4",
                        previewMode ? "h-full min-h-[320px]" : "h-[min(58vh,620px)]",
                      )}
                    >
                      <MediaPreview
                        url={sourceURL}
                        posterUrl={sourceCandidate.poster_url}
                        alt={
                          sourceCandidate.title || sourceCandidate.competitor
                        }
                        assetType={sourceCandidate.asset_type}
                        compact
                      />
                    </span>
                  </button>
                )}
                <button
                  type="button"
                  className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] text-left"
                  onClick={previewResult}
                  title="查看修图结果大图"
                >
                  <span className="flex items-center justify-between gap-2 border-b bg-muted/30 px-4 py-2 text-xs font-medium">
                    <span>
                      修图结果
                      {activeGroup?.variant ? ` · 创意 ${activeGroup.variant}` : ""} ·{" "}
                      {creativeDeliveryInfo(currentAsset?.filename ?? "")?.size}
                    </span>
                    <Maximize2 className="h-3.5 w-3.5 text-muted-foreground" />
                  </span>
                  <span
                    className={cn(
                      "flex items-center justify-center bg-black p-4",
                      previewMode ? "h-full min-h-[320px]" : "h-[min(58vh,620px)]",
                    )}
                  >
                    {currentAsset && (
                      <MediaPreview
                        url={currentAsset.markdown_url || currentAsset.url}
                        alt={currentAsset.filename}
                        compact
                      />
                    )}
                  </span>
                </button>
              </div>
              <div className="border-t px-4 py-3">
                <p className="break-all text-sm font-medium">
                  {currentAsset?.filename}
                </p>
              </div>
            </div>
          </div>
        ) : (
          <div className="flex min-h-48 items-center justify-center text-sm text-muted-foreground">
            Leader 发布验收通过的成图后会显示在这里
          </div>
        ))}
    </div>
  );
}

function AdjustmentDialog({ open, onOpenChange, issue, candidates, target, context, items, onCreated }: { open: boolean; onOpenChange: (open: boolean) => void; issue: Issue; candidates: CreativeMaterialCandidate[]; target: CreativeAdjustmentTarget | null; context: { squad_id: string } | null | undefined; items: CreativeIssueItem[]; onCreated: () => void }) {
  const [text, setText] = useState("");
	const [scope, setScope] = useState<"size" | "variant">("size");
  const candidate = candidates.find((value) => value.id === target?.candidateId);
  const currentDelivery = creativeDeliveryInfo(target?.asset.filename ?? "");
	const currentSize = target?.delivery?.size ?? currentDelivery?.size ?? "当前尺寸";
  const currentVariant = target?.variant ?? (currentDelivery ? creativeVariantBranch(currentDelivery.branch).variant : null);
  const variantLabel = currentVariant ? `创意 ${currentVariant}` : "当前创意";
  const sourceURL = candidate ? firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url) : "";
	useEffect(() => { if (open) { setText(""); setScope("size"); } }, [open, target?.asset.id]);
  const create = useMutation({ mutationFn: async () => {
    if (!target) throw new Error("没有选中待调整成图");
    const item = items.find((value) => value.candidate_id === target.candidateId);
    if (!context?.squad_id || !item?.work_issue_id) throw new Error("该图尚未建立协作 Issue");
		if (!currentVariant) throw new Error("当前成图缺少创意变体编号");
		const affected = scope === "size" ? [target.asset] : target.groupAssets;
		const affectedDeliveries = scope === "size"
			? (target.delivery ? [target.delivery] : [])
			: target.groupDeliveries;
		if (affectedDeliveries.length !== affected.length) throw new Error("当前成图尚未登记交付映射，请刷新后重试");
		const adjustment = await api.createCreativeAdjustment(issue.id, target.candidateId, {
			variant: currentVariant,
			scope,
			...(scope === "size" ? { size: currentSize as CreativeDelivery["size"] } : {}),
			instruction: text.trim(),
			target_attachment_ids: affected.map((asset) => asset.id),
			base_attachment_ids: affectedDeliveries.map((delivery) => delivery.base_attachment_id).filter(Boolean),
		});
		const created = await api.createIssue({
			title: `V${String(currentVariant).padStart(2, "0")} / ${scope === "size" ? currentSize : "三尺寸"} 精准调整 · R${adjustment.revision}`,
			description: `目标创意：${variantLabel}\n调整范围：${scope === "size" ? `仅 ${currentSize}` : "当前创意的三个尺寸"}\n\n用户反馈：\n${text.trim()}`,
      parent_issue_id: item.work_issue_id,
      assignee_type: "squad",
      assignee_id: context.squad_id,
      status: "todo",
			metadata: {
				workflow: "creative_adjustment",
				creative_adjustment_id: adjustment.id,
				creative_candidate_id: target.candidateId,
				creative_work_issue_id: item.work_issue_id,
				creative_variant: currentVariant,
				creative_scope: scope,
				...(scope === "size" ? { creative_size: currentSize } : {}),
				creative_revision: adjustment.revision,
				creative_target_attachment_ids: affected.map((asset) => asset.id).join(","),
				creative_base_attachment_ids: affectedDeliveries.map((delivery) => delivery.base_attachment_id).filter(Boolean).join(","),
			},
    });
		await api.bindCreativeAdjustmentIssue(issue.id, target.candidateId, adjustment.id, created.id);
		return created;
  }, onSuccess: () => { toast.success("调整请求已交给 Leader"); onCreated(); }, onError: (error) => toast.error(error instanceof Error ? error.message : "无法创建调整任务") });
	return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-[min(94vw,1040px)]"><DialogHeader><DialogTitle>精准调整成图</DialogTitle><p className="text-xs text-muted-foreground">单尺寸只重做这一张；整创意会重做当前 V0{currentVariant ?? "-"} 的三个尺寸。历史版本继续保留。</p></DialogHeader>{target && <div className={cn("grid overflow-hidden border bg-muted/10", sourceURL ? "md:grid-cols-2" : "grid-cols-1")}>{sourceURL && <div className="grid grid-rows-[auto_260px] border-r"><div className="border-b px-3 py-2 text-xs font-medium">竞品原图</div><div className="flex items-center justify-center p-3"><MediaPreview url={sourceURL} posterUrl={candidate?.poster_url} alt={candidate?.title || "竞品原图"} assetType={candidate?.asset_type} compact /></div></div>}<div className="grid grid-rows-[auto_260px]"><div className="border-b px-3 py-2 text-xs font-medium">当前成图 · {currentSize}</div><div className="flex items-center justify-center p-3"><MediaPreview url={target.asset.markdown_url || target.asset.url} alt={target.asset.filename} compact /></div></div></div>}<div className="grid gap-3 sm:grid-cols-2"><Field label="调整范围"><NativeSelect value={scope} onChange={(event) => setScope(event.target.value as "size" | "variant")}><NativeSelectOption value="size">仅当前尺寸 · {currentSize}</NativeSelectOption><NativeSelectOption value="variant">当前创意 V{String(currentVariant ?? "-").padStart(2, "0")} · 三尺寸</NativeSelectOption></NativeSelect></Field><Field label="基准文件"><Input value={target?.asset.filename ?? ""} readOnly /></Field><Field label="调整内容" wide><Textarea rows={6} value={text} onChange={(event) => setText(event.target.value)} placeholder="例如：四角文案被深色背景挡住，改成浅色高对比底；中央构图和另外两个尺寸不变。" /></Field></div><DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={create.isPending || !target || !text.trim()} onClick={() => create.mutate()}>{create.isPending ? "正在创建" : "交给 Leader"}</Button></DialogFooter></DialogContent></Dialog>;
}

function Field({ label, children, wide = false }: { label: string; children: React.ReactNode; wide?: boolean }) { return <div className={cn("space-y-1.5", wide && "sm:col-span-2")}><Label className="text-xs text-muted-foreground">{label}</Label>{children}</div>; }
function MediaPreview({ url, alt, assetType, posterUrl, compact = false }: { url: string; alt: string; assetType?: string; posterUrl?: string; compact?: boolean }) { if (!url) return <div className="flex h-full w-full items-center justify-center text-muted-foreground"><ImageIcon className="h-8 w-8" /></div>; if (assetType === "video" || isVideoURL(url)) return <video src={url} poster={posterUrl} controls={!compact} muted playsInline preload="metadata" className="h-full w-full object-contain" />; return <img src={url} alt={alt} className="h-full w-full object-contain" />; }
function MediaPreviewDialog({ item, onOpenChange }: { item: MediaPreviewItem | null; onOpenChange: (open: boolean) => void }) { return <Dialog open={!!item} onOpenChange={onOpenChange}><DialogContent className="max-h-[94vh] gap-0 overflow-hidden p-0 sm:max-w-[min(96vw,1280px)]"><DialogHeader className="border-b px-4 py-3 pr-12"><DialogTitle className="truncate text-sm">{item?.subtitle ? `${item.subtitle} - ${item.title}` : item?.title}</DialogTitle>{item?.openUrl && <a href={item.openUrl} target="_blank" rel="noopener noreferrer" className="mt-1 inline-flex items-center gap-1 text-xs"><ExternalLink className="h-3.5 w-3.5" />打开原始素材</a>}</DialogHeader><div className="flex h-[min(78vh,820px)] items-center justify-center bg-black p-3">{item && <MediaPreview url={item.url} posterUrl={item.posterUrl} alt={item.title} assetType={item.assetType} />}</div></DialogContent></Dialog>; }
function Metric({ label, value }: { label: string; value: string }) { return <div><div className="text-muted-foreground">{label}</div><div className="font-semibold">{value}</div></div>; }
function candidateMatchesFilter(candidate: CreativeMaterialCandidate, filter: CandidateFilter) { if (filter.status !== "all" && candidate.status !== filter.status) return false; if (filter.competitor !== "all" && candidate.competitor !== filter.competitor) return false; if (filter.assetType !== "all" && candidate.asset_type !== filter.assetType) return false; if (filter.media !== "all" && !candidate.media_names.includes(filter.media)) return false; if (filter.area !== "all" && !candidate.area_names.includes(filter.area)) return false; if (filter.language !== "all" && !candidate.language_names.includes(filter.language)) return false; if (filter.platform !== "all" && !candidate.platform_names.includes(filter.platform)) return false; const text = filter.text.trim().toLowerCase(); return !text || [candidate.title, candidate.competitor, ...candidate.media_names, ...candidate.area_names, ...candidate.language_names, ...candidate.platform_names].join(" ").toLowerCase().includes(text); }
function candidateFilterEquals(left: CandidateFilter, right: CandidateFilter) { return left.status === right.status && left.competitor === right.competitor && left.assetType === right.assetType && left.media === right.media && left.area === right.area && left.language === right.language && left.platform === right.platform && left.text === right.text; }
function candidateToImport(candidate: CreativeMaterialCandidate) { return { external_id: candidate.external_id, dedupe_key: candidate.dedupe_key, competitor: candidate.competitor, title: candidate.title, asset_type: candidate.asset_type, preview_url: firstNonEmpty(candidate.archived_url, candidate.preview_url), resource_url: candidate.resource_url, poster_url: candidate.poster_url, original_url: candidate.original_url, media_names: candidate.media_names, area_names: candidate.area_names, language_names: candidate.language_names, platform_names: candidate.platform_names, raw: { source_candidate_id: candidate.id } }; }
function stringConfig(config: Record<string, unknown> | undefined, key: string) { const value = config?.[key]; return typeof value === "string" ? value : ""; }
function stringArrayConfig(config: Record<string, unknown> | undefined, key: string) { const value = config?.[key]; return Array.isArray(value) ? uniqueOptions(value.filter((item): item is string => typeof item === "string")) : []; }
function uniqueOptions(values: string[]) { return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b)); }
function splitBriefList(value: string) { return uniqueOptions(value.split(/[\n,，、;；]+/)); }
function creativeBriefLabel(brief: CreativeBriefDraft) { return [brief.theme, brief.primary_benefit].filter(Boolean).join(" × ") || "待配置"; }
async function mapWithConcurrency<T>(items: T[], concurrency: number, task: (item: T) => Promise<void>) {
  const queue = [...items];
  await Promise.all(Array.from({ length: Math.min(concurrency, queue.length) }, async () => {
    while (queue.length > 0) {
      const item = queue.shift();
      if (item !== undefined) await task(item);
    }
  }));
}
function firstNonEmpty(...values: string[]) { return values.find((value) => value.trim() !== "")?.trim() ?? ""; }
function isVideoURL(url: string) { return /\.(mp4|mov|webm|m3u8)(?:[?#].*)?$/i.test(url); }
function formatDuration(value: number | null) { return value == null ? "-" : `${Math.round(value).toLocaleString()} 天`; }
function formatImpression(value: number | null) { if (value == null) return "-"; if (value >= 1_000_000) return `${trimNumber(value / 1_000_000)}M`; if (value >= 1_000) return `${trimNumber(value / 1_000)}K`; return Math.round(value).toLocaleString(); }
function trimNumber(value: number) { return value.toFixed(value >= 10 ? 0 : 1).replace(/\.0$/, ""); }

const COPY_INTENT_RULES = [
  { key: "repayment_plan", label: "还款/分期", pattern: /还款|分期|期限|cicilan|angsuran|tenor|repayment|pelunasan/i },
  { key: "rate_down", label: "利率", pattern: /低利率|降息|利率|bunga|suku bunga|interest|rate down|0[,.]0\d\s*%/i },
  { key: "interest_free", label: "免息", pattern: /免息|0\s*%|bebas bunga|interest free/i },
  { key: "fee_reduction", label: "费用减免", pattern: /降费|费用减免|减免|biaya|uang muka|fee|potongan/i },
  { key: "limit_amount", label: "额度", pattern: /额度|limit|jumlah pinjaman|pencairan|dana|rp\s*\d|juta/i },
  { key: "fast_disbursement", label: "快速放款", pattern: /快速放款|到账|秒批|cepat cair|pencairan cepat|cair dalam|menit/i },
  { key: "easy_application", label: "低门槛", pattern: /低门槛|易申请|mudah|tanpa jaminan|cukup ktp|syarat/i },
  { key: "app_interface", label: "App 界面", pattern: /app|phone|screen|interface|halaman|beranda|whatsapp/i },
  { key: "comparison", label: "对比", pattern: /comparison|perbandingan|bandingkan/i },
] as const;

export function recommendCopyEntries(candidate: CreativeMaterialCandidate, entries: CreativeCopyEntry[], brief: CreativeBriefDraft = EMPTY_BRIEF) {
  const hasPrimaryBenefit = Boolean(brief.primary_benefit.trim());
  const primaryText = [brief.primary_benefit, brief.benefit_value].join(" ").toLocaleLowerCase();
  const secondaryText = brief.secondary_benefits.join(" ").toLocaleLowerCase();
  const primaryIntents = COPY_INTENT_RULES.filter((rule) => rule.pattern.test(primaryText) || primaryText.includes(rule.key));
  const secondaryIntents = COPY_INTENT_RULES.filter((rule) => rule.pattern.test(secondaryText) || secondaryText.includes(rule.key));
  const benefitTokens = signalTokens([brief.primary_benefit, ...brief.secondary_benefits, brief.benefit_value].join(" "));
  const themeTokens = signalTokens([brief.theme, ...brief.theme_elements].join(" "));
  const fallbackText = [candidate.title, ...candidate.tags, ...candidate.media_names].join(" ").toLocaleLowerCase();
  const fallbackIntents = hasPrimaryBenefit ? [] : COPY_INTENT_RULES.filter((rule) => rule.pattern.test(fallbackText));
  return entries.filter((entry) => entry.status === "approved").map((entry) => {
    const entryText = [entry.headline, entry.subheadline, entry.benefit, entry.cta, entry.copy_role, ...entry.tags, JSON.stringify(entry.metadata)].join(" ").toLocaleLowerCase();
    const matches = (rule: (typeof COPY_INTENT_RULES)[number]) => entryText.includes(rule.key) || entryText.includes(rule.key.replaceAll("_", " ")) || rule.pattern.test(entryText);
    const matchedPrimary = primaryIntents.filter(matches);
    const matchedSecondary = secondaryIntents.filter(matches);
    const matchedFallback = fallbackIntents.filter(matches);
    const benefitOverlap = benefitTokens.filter((token) => entryText.includes(token)).slice(0, 3);
    const themeOverlap = themeTokens.filter((token) => entryText.includes(token)).slice(0, 2);
    const conciseScore = Math.max(0, 8 - Math.round([entry.headline, entry.subheadline, entry.benefit, entry.cta].join(" ").length / 50));
    const score = matchedPrimary.length * 100 + matchedSecondary.length * 30 + benefitOverlap.length * 18 + themeOverlap.length * 8 + matchedFallback.length * 8 + conciseScore;
    const reasons = hasPrimaryBenefit
      ? [...matchedPrimary.map((rule) => `主利益点：${rule.label}`), ...matchedSecondary.map((rule) => `辅助利益点：${rule.label}`), ...benefitOverlap.map((token) => `利益点词：${token}`), ...themeOverlap.map((token) => `主题词：${token}`)].slice(0, 4)
      : matchedFallback.map((rule) => `采集信息兜底：${rule.label}`).slice(0, 2);
    return { entry, score, reasons };
  }).sort((left, right) => right.score - left.score || Date.parse(right.entry.updated_at) - Date.parse(left.entry.updated_at) || left.entry.external_key.localeCompare(right.entry.external_key));
}

function signalTokens(value: string) {
  const stop = new Set(["adakami", "easycash", "kredit", "pintar", "adapundi", "bantusaku", "rupiah", "cepat", "julo", "indonesia", "image", "video"]);
  return [...new Set(value.toLocaleLowerCase().split(/[^\p{L}\p{N}%]+/u).filter((token) => token.length >= 4 && !stop.has(token)))];
}

const CREATIVE_CANDIDATE_ID = /候选(?:\s*ID|\s*素材)\s*[：:]\s*`?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`?/i;

export function candidateIdFromResultComment(content: string) {
  return content.match(CREATIVE_CANDIDATE_ID)?.[1] ?? "";
}

function candidateIdForResultGroup<T extends { id: string; filename: string }>(group: { key: string; assets: T[] }, candidateByAttachment: ReadonlyMap<string, string>, candidates: CreativeMaterialCandidate[]) {
  for (const asset of group.assets) {
    const candidateId = candidateByAttachment.get(asset.id);
    if (candidateId) return candidateId;
  }
  return candidates.find((candidate) => group.key.includes(candidate.id))?.id ?? "";
}

const LEGACY_CREATIVE_DELIVERY_FILENAME = /^(?:(.+)__)?(?:prime-)?(1080x1080|1200x628|800x1000)\.png$/i;
const VERSIONED_CREATIVE_DELIVERY_FILENAME = /^(.+)_(1080x1080|1200x628|800x1000)_v(\d+)\.png$/i;

export function creativeDeliveryInfo(filename: string) {
  const legacy = filename.match(LEGACY_CREATIVE_DELIVERY_FILENAME);
  if (legacy?.[2]) return { branch: legacy[1] || "current", size: legacy[2], revision: null };
  const versioned = filename.match(VERSIONED_CREATIVE_DELIVERY_FILENAME);
  return versioned?.[1] && versioned[2] && versioned[3]
    ? { branch: versioned[1], size: versioned[2], revision: Number(versioned[3]) }
    : null;
}

export function isCreativeDeliveryFilename(filename: string) { return creativeDeliveryInfo(filename) !== null; }
export function groupCreativeDeliveries<T extends { id: string; filename: string }>(assets: T[]) {
  const groups = new Map<string, { key: string; label: string; setKey: string; variant: number | null; bySize: Map<string, { asset: T; revision: number }> }>();
  for (const asset of assets) {
    const info = creativeDeliveryInfo(asset.filename);
    if (!info) continue;
    const variantInfo = creativeVariantBranch(info.branch);
    const group = groups.get(info.branch) ?? {
      key: info.branch,
      label: info.branch === "current" ? "本轮交付" : info.branch,
      setKey: variantInfo.setKey,
      variant: variantInfo.variant,
      bySize: new Map<string, { asset: T; revision: number }>(),
    };
    const revision = info.revision ?? 0;
    const current = group.bySize.get(info.size);
    if (!current || revision >= current.revision) {
      group.bySize.set(info.size, { asset, revision });
    }
    groups.set(info.branch, group);
  }
  return [...groups.values()].map((group) => ({
    key: group.key,
    label: group.label,
    setKey: group.setKey,
    variant: group.variant,
    assets: [...group.bySize.values()].map(({ asset }) => asset).sort((left, right) => (
      creativeDeliveryInfo(left.filename)?.size ?? ""
    ).localeCompare(creativeDeliveryInfo(right.filename)?.size ?? "")),
  }));
}

function creativeVariantBranch(branch: string) {
  const match = branch.match(/^(.+)_V(\d{2})$/i);
  return match?.[1] && match[2]
    ? { setKey: match[1], variant: Number(match[2]) }
    : { setKey: branch, variant: null };
}
async function downloadCreativeAssets(assets: { filename: string; download_url?: string | null; url: string }[]) { try { for (const asset of assets) { const response = await fetch(asset.download_url || asset.url, { credentials: "include" }); if (!response.ok) throw new Error(asset.filename); const href = URL.createObjectURL(await response.blob()); const anchor = document.createElement("a"); anchor.href = href; anchor.download = asset.filename; document.body.appendChild(anchor); anchor.click(); anchor.remove(); URL.revokeObjectURL(href); } } catch { toast.error("下载交付图失败，请稍后重试"); } }

async function downloadCreativeZip(assets: (CreativeDeliveryAsset & { folder: string })[], archiveName: string) {
  try {
    const files: Record<string, Uint8Array> = {};
    await Promise.all(assets.map(async (asset) => {
      const response = await fetch(asset.download_url || asset.url, { credentials: "include" });
      if (!response.ok) throw new Error(asset.filename);
      files[`${safeArchiveName(asset.folder)}/${safeArchiveName(asset.filename)}`] = new Uint8Array(await response.arrayBuffer());
    }));
    const archive = zipSync(files, { level: 0 });
    const archiveBuffer = new ArrayBuffer(archive.byteLength);
    new Uint8Array(archiveBuffer).set(archive);
    const href = URL.createObjectURL(new Blob([archiveBuffer], { type: "application/zip" }));
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.download = `${safeArchiveName(archiveName)}.zip`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(href), 1000);
  } catch {
    toast.error("打包下载失败，请稍后重试");
  }
}

function safeArchiveName(value: string) {
  const printable = [...value].map((character) => character.charCodeAt(0) < 32 ? "_" : character).join("");
  return printable.replace(/[<>:"/\\|?*]/g, "_").replace(/\s+/g, " ").trim() || "creative-assets";
}
