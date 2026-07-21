"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { strToU8, unzipSync, zipSync } from "fflate";
import {
  ArrowLeft,
  Check,
  Download,
  ExternalLink,
  ImageIcon,
  Maximize2,
  RotateCcw,
  Search,
  Sparkles,
  Video,
  X,
} from "lucide-react";
import type {
  CreateCreativeEditFeedbackRequest,
  CreativeEditJob,
  CreativeMaterialCandidate,
  Issue,
} from "@multica/core/types";
import { api } from "@multica/core/api";
import { creativeKeys, creativeMaterialsOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";
import { CreativeQcAttemptHistory, extractCreativeQcAttemptRounds } from "./creative-qc-attempt-history";
import { CreativeVariantFeedback } from "./creative-variant-feedback";

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

const STATUS_LABEL: Record<string, string> = {
  new: "待选",
  selected: "已选择",
  rejected: "不采用",
  sent_to_edit: "修图中",
  edited: "已出图",
  approved: "已确认",
  archived: "已归档",
};

const creativeEditPromptDraftKey = (workspaceId: string, issueId: string) =>
  `multica:creative-edit-prompt:${workspaceId}:${issueId}`;

const creativeEditViewedJobsKey = (workspaceId: string, issueId: string) =>
  `multica:creative-edit-viewed:${workspaceId}:${issueId}`;

function readCreativeEditPromptDraft(key: string) {
  if (typeof window === "undefined") return "";
  try {
    return window.sessionStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function writeCreativeEditPromptDraft(key: string, value: string) {
  if (typeof window === "undefined") return;
  try {
    if (value) window.sessionStorage.setItem(key, value);
    else window.sessionStorage.removeItem(key);
  } catch {
    // The input remains usable when session storage is unavailable.
  }
}

function readCreativeEditViewedJobs(key: string) {
  if (typeof window === "undefined") return new Set<string>();
  try {
    const value = JSON.parse(window.localStorage.getItem(key) ?? "[]");
    return new Set(Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : []);
  } catch {
    return new Set<string>();
  }
}

function writeCreativeEditViewedJobs(key: string, jobIDs: Set<string>) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, JSON.stringify([...jobIDs]));
  } catch {
    // Viewing the result board does not depend on local storage being available.
  }
}

function creativeEditJobHistory(jobs: CreativeEditJob[]) {
  return [...jobs]
    .sort((left, right) => Date.parse(left.created_at) - Date.parse(right.created_at))
    .map((job, index) => ({ job, sequence: index + 1 }))
    .reverse();
}

function formatCreativeEditJobTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

function creativePreviewEmptyMessage(errorMessage: string) {
  if (/service restarted/i.test(errorMessage)) {
    return "服务重启导致本次任务未完成，未产生可查看的生成图。";
  }
  if (/no conceptual variant/i.test(errorMessage)) {
    return "本次生成图均未通过质检，详细原因可在看板的 QC 每轮记录查看。";
  }
  return "本次未产生可查看的生成图；详细原因可在看板的 QC 每轮记录查看。";
}

async function downloadSelectedCreativeAssets(
  issueID: string,
  jobID: string,
  assets: { id: string; asset_url: string }[],
  includeOriginal: boolean,
) {
  const source = await api.downloadCreativeEditJob(issueID, jobID);
  const entries = unzipSync(new Uint8Array(await source.blob.arrayBuffer()));
  const selectedFilenames = new Set(assets.map((asset) => creativeAssetDownloadName(asset.asset_url)).filter(Boolean));
  const selectedEntries: Record<string, Uint8Array> = {};
  for (const [entryName, data] of Object.entries(entries)) {
    if (entryName === "manifest.json") continue;
    const fileName = entryName.split("/").at(-1) ?? "";
    if ((includeOriginal && entryName.includes("/original/")) || selectedFilenames.has(fileName)) {
      selectedEntries[entryName] = data;
    }
  }
  selectedEntries["manifest.json"] = strToU8(JSON.stringify({
    job_id: jobID,
    selection: { include_original: includeOriginal, asset_ids: assets.map((asset) => asset.id) },
    asset_count: assets.length,
  }, null, 2));
  const filename = source.filename.replace(/\.zip$/i, "_SELECTED.zip");
  const archive = zipSync(selectedEntries, { level: 6 });
  const copy = new Uint8Array(archive.byteLength);
  copy.set(archive);
  return { blob: new Blob([copy.buffer as ArrayBuffer], { type: "application/zip" }), filename };
}

function creativeAssetDownloadName(assetURL: string) {
  try {
    return decodeURIComponent(new URL(assetURL).pathname.split("/").at(-1) ?? "");
  } catch {
    return "";
  }
}

function creativeEditAssetPreviewURL(issueID: string, assetID: string) {
  if (!assetID) return "";
  return `/api/issues/${encodeURIComponent(issueID)}/creative-edit-assets/${encodeURIComponent(assetID)}/preview`;
}

export function CreativeMaterialPool({ issue }: { issue: Issue }) {
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const [filter, setFilter] = useState<CandidateFilter>(DEFAULT_FILTER);
  const [editPrompt, setEditPrompt] = useState("");
  const editPromptDraftKey = creativeEditPromptDraftKey(wsId, issue.id);
  const editViewedJobsKey = creativeEditViewedJobsKey(wsId, issue.id);
  const [activeJobId, setActiveJobId] = useState("");
  const [activeCandidateId, setActiveCandidateId] = useState("");
  const [viewedJobIDs, setViewedJobIDs] = useState<Set<string>>(() => readCreativeEditViewedJobs(editViewedJobsKey));
  const [previewItem, setPreviewItem] = useState<MediaPreviewItem | null>(null);
  const [candidateOrder, setCandidateOrder] = useState<Record<string, number>>({});
  const [poolPreviewOpen, setPoolPreviewOpen] = useState(false);
  const [resultPreviewOpen, setResultPreviewOpen] = useState(false);

  const workflowEnabled = issue.metadata?.workflow === "creative_material";
  const query = useQuery(creativeMaterialsOptions(wsId, issue.id));
  const data = query.data;
  const visible = workflowEnabled
    || !!data?.enabled
    || (data?.candidates.length ?? 0) > 0
    || (data?.edit_jobs.length ?? 0) > 0;

  const updateCandidate = useMutation({
    mutationFn: (input: { candidateId: string; status: string }) =>
      api.updateCreativeMaterialCandidate(issue.id, input.candidateId, { status: input.status }),
    onSuccess: (next) => {
      qc.setQueryData(creativeKeys.issue(wsId, issue.id), next);
    },
    onError: () => toast.error("候选状态更新失败"),
  });

  const requestCreativeAgent = useMutation({
    mutationFn: async (candidateIds: string[]) => {
      const agents = await api.listAgents({ workspace_id: wsId });
      const agent = agents.find((item) => item.name.trim() === "修图智能体")
        ?? agents.find((item) => item.name.includes("修图"));
      if (!agent) throw new Error("修图智能体未配置");
      const direction = editPrompt.trim();
      const content = [
        `[@${agent.name}](mention://agent/${agent.id}) 请处理当前已勾选的 ${candidateIds.length} 张素材。`,
        direction ? `补充要求：${direction}` : "请根据参考图判断主利益点，并按当前市场 profile、已确认文案和品牌规则生成修图。",
      ].join("\n\n");
      return api.createComment(issue.id, content);
    },
    onSuccess: () => {
      toast.success("已交给修图智能体");
      setEditPrompt("");
      writeCreativeEditPromptDraft(editPromptDraftKey, "");
    },
    onError: (error) => toast.error(error instanceof Error && error.message.includes("未配置") ? error.message : "无法交给修图智能体"),
  });

  const syncEditJob = useMutation({
    mutationFn: (jobId: string) => api.syncCreativeEditJob(issue.id, jobId),
    onSuccess: (next) => {
      qc.setQueryData(creativeKeys.issue(wsId, issue.id), next);
      toast.success("已同步修图结果");
    },
    onError: () => toast.error("同步修图任务失败"),
  });

  const downloadEditJob = useMutation({
    mutationFn: (input: { jobId: string; assetIDs?: string[]; includeOriginal?: boolean }) =>
      api.downloadCreativeEditJob(issue.id, input.jobId, {
        assetIds: input.assetIDs,
        includeOriginal: input.includeOriginal,
      }),
    onSuccess: ({ blob, filename }) => {
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = filename;
      anchor.style.display = "none";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    },
    onError: () => toast.error("素材包下载失败"),
  });

  const downloadSelectedEditJob = useMutation({
    mutationFn: (input: { jobId: string; assets: { id: string; asset_url: string }[]; includeOriginal: boolean }) =>
      downloadSelectedCreativeAssets(issue.id, input.jobId, input.assets, input.includeOriginal),
    onSuccess: ({ blob, filename }) => {
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = filename;
      anchor.style.display = "none";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    },
    onError: () => toast.error("选中素材打包失败"),
  });

  const submitVariantFeedback = useMutation({
    mutationFn: (input: {
      jobId: string;
      variantId: string;
      feedback: CreateCreativeEditFeedbackRequest;
    }) => api.createCreativeEditFeedback(
      issue.id,
      input.jobId,
      input.variantId,
      input.feedback,
    ),
    onSuccess: (next) => {
      qc.setQueryData(creativeKeys.issue(wsId, issue.id), next);
      toast.success("反馈已记录");
    },
    onError: () => toast.error("无法保存反馈"),
  });

  const candidates = data?.candidates ?? [];

  useEffect(() => {
    setEditPrompt(readCreativeEditPromptDraft(editPromptDraftKey));
  }, [editPromptDraftKey]);

  useEffect(() => {
    setViewedJobIDs(readCreativeEditViewedJobs(editViewedJobsKey));
  }, [editViewedJobsKey]);

  useEffect(() => {
    setCandidateOrder({});
  }, [issue.id]);

  useEffect(() => {
    if (candidates.length === 0) return;
    setCandidateOrder((current) => {
      let next = current;
      let maxOrder = Math.max(-1, ...Object.values(current));
      for (const candidate of candidates) {
        if (next[candidate.id] == null) {
          if (next === current) next = { ...current };
          next[candidate.id] = ++maxOrder;
        }
      }
      return next;
    });
  }, [candidates]);

  const displayCandidates = useMemo(
    () => [...candidates].sort((left, right) =>
      (candidateOrder[left.id] ?? 0) - (candidateOrder[right.id] ?? 0),
    ),
    [candidateOrder, candidates],
  );
  const selectedCandidates = useMemo(
    () => candidates.filter((candidate) => candidate.status === "selected"),
    [candidates],
  );
  const candidateByID = useMemo(() => {
    const map = new Map<string, CreativeMaterialCandidate>();
    for (const candidate of candidates) map.set(candidate.id, candidate);
    return map;
  }, [candidates]);

  const filteredCandidates = useMemo(
    () => displayCandidates.filter((candidate) => candidateMatchesFilter(candidate, filter)),
    [displayCandidates, filter],
  );
  const hasActiveFilter = !candidateFilterEquals(filter, DEFAULT_FILTER);

  const latestJob = data?.edit_jobs[0];
  const activeJob = useMemo(
    () => data?.edit_jobs.find((job) => job.id === activeJobId) ?? latestJob,
    [activeJobId, data?.edit_jobs, latestJob],
  );
  const activeJobCandidates = activeJob?.candidate_ids ?? [];
  useEffect(() => {
    if (!activeJob && activeJobId) setActiveJobId("");
    if (activeJob && activeJob.id !== activeJobId) setActiveJobId(activeJob.id);
  }, [activeJob, activeJobId]);

  useEffect(() => {
    if (!activeJob) return;
    const nextCandidateID = activeJob.candidate_ids.includes(activeCandidateId)
      ? activeCandidateId
      : activeJob.candidate_ids[0] ?? "";
    if (nextCandidateID !== activeCandidateId) setActiveCandidateId(nextCandidateID);
  }, [activeCandidateId, activeJob]);

  useEffect(() => {
    if (!activeJob || viewedJobIDs.has(activeJob.id)) return;
    setViewedJobIDs((current) => {
      if (current.has(activeJob.id)) return current;
      const next = new Set(current).add(activeJob.id);
      writeCreativeEditViewedJobs(editViewedJobsKey, next);
      return next;
    });
  }, [activeJob, editViewedJobsKey, viewedJobIDs]);

  if (!visible && !query.isLoading) return null;
  if (!visible && query.isLoading) return null;

  return (
    <section className="rounded-lg border bg-background">
      <div className="border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-sm font-semibold">素材候选池</h2>
            <Badge variant="outline">{data?.summary.total ?? 0} 条素材</Badge>
            <Badge variant="outline">已选 {selectedCandidates.length}</Badge>
            {(data?.edit_jobs.length ?? 0) > 0 && (
              <Badge variant="outline">出图记录 {data?.edit_jobs.length}</Badge>
            )}
          </div>
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/20 px-4 py-3">
        <div className="min-w-60 flex-1">
          <Textarea
            value={editPrompt}
            onChange={(event) => {
              const value = event.target.value;
              setEditPrompt(value);
              writeCreativeEditPromptDraft(editPromptDraftKey, value);
            }}
            placeholder="给修图智能体的补充要求，可不填"
            className="h-9 min-h-9 resize-none text-xs"
          />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={candidates.length === 0}
            onClick={() => setPoolPreviewOpen(true)}
          >
            <Maximize2 className="h-4 w-4" />
            预览候选池
          </Button>
          <Button
            size="sm"
            disabled={selectedCandidates.length === 0 || requestCreativeAgent.isPending}
            onClick={() => requestCreativeAgent.mutate(selectedCandidates.map((candidate) => candidate.id))}
          >
            <Sparkles className="h-4 w-4" />
            {requestCreativeAgent.isPending ? "正在交办" : "交给修图智能体"}
          </Button>
        </div>
      </div>

      <div className="space-y-4 p-4">
        <CandidateFilters
          candidates={candidates}
          filter={filter}
          onChange={setFilter}
        />

        <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
          <span>当前显示 {filteredCandidates.length} / {candidates.length} 条</span>
          {hasActiveFilter && (
            <button
              type="button"
              className="inline-flex items-center gap-1 hover:text-foreground"
              onClick={() => setFilter(DEFAULT_FILTER)}
            >
              <RotateCcw className="h-3.5 w-3.5" />
              显示全部
            </button>
          )}
        </div>

        {filteredCandidates.length === 0 ? (
          <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
            当前筛选下没有候选素材。
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {filteredCandidates.map((candidate) => (
              <CandidateCard
                key={candidate.id}
                candidate={candidate}
                busy={updateCandidate.isPending}
                onStatus={(status) => updateCandidate.mutate({ candidateId: candidate.id, status })}
                onPreview={setPreviewItem}
              />
            ))}
          </div>
        )}

        {activeJob && (
          <EditResultBoard
            jobs={data?.edit_jobs ?? []}
            job={activeJob}
            activeCandidateId={activeCandidateId}
            activeJobCandidates={activeJobCandidates}
            candidateByID={candidateByID}
            viewedJobIDs={viewedJobIDs}
            onJobChange={setActiveJobId}
            onCandidateChange={setActiveCandidateId}
            onSyncJob={(jobId) => syncEditJob.mutate(jobId)}
            onDownloadJob={(jobId) => downloadEditJob.mutate({ jobId })}
            syncingJobId={syncEditJob.isPending ? syncEditJob.variables : undefined}
            downloadingJobId={downloadEditJob.isPending ? downloadEditJob.variables?.jobId : undefined}
            submittingFeedbackVariantId={submitVariantFeedback.isPending
              ? submitVariantFeedback.variables?.variantId
              : undefined}
            onSubmitFeedback={async (jobId, variantId, feedback) => {
              await submitVariantFeedback.mutateAsync({ jobId, variantId, feedback });
            }}
            onOpenLargePreview={() => setResultPreviewOpen(true)}
            onPreview={setPreviewItem}
          />
        )}
      </div>
      <CandidatePoolPreviewDialog
        open={poolPreviewOpen}
        onOpenChange={setPoolPreviewOpen}
        candidates={displayCandidates}
        filter={filter}
        onFilterChange={setFilter}
        selectedCount={selectedCandidates.length}
        busy={updateCandidate.isPending}
        onStatus={(candidateId, status) => updateCandidate.mutate({ candidateId, status })}
        onPreview={setPreviewItem}
      />
      {activeJob && (
        <CreativeResultPreviewDialog
          open={resultPreviewOpen}
          onOpenChange={setResultPreviewOpen}
          job={activeJob}
          activeCandidateId={activeCandidateId}
          candidateByID={candidateByID}
          onPreview={setPreviewItem}
          downloading={downloadSelectedEditJob.isPending}
          onDownloadSelected={(assetIDs, includeOriginal) => {
            const assets = activeJob.variants
              .flatMap((variant) => variant.assets)
              .filter((asset) => assetIDs.includes(asset.id))
              .map((asset) => ({ id: asset.id, asset_url: asset.asset_url }));
            downloadSelectedEditJob.mutate({ jobId: activeJob.id, assets, includeOriginal });
          }}
        />
      )}
      <MediaPreviewDialog
        item={previewItem}
        onOpenChange={(open) => {
          if (!open) setPreviewItem(null);
        }}
      />
    </section>
  );
}

function CandidatePoolPreviewDialog({
  open,
  onOpenChange,
  candidates,
  filter,
  onFilterChange,
  selectedCount,
  busy,
  onStatus,
  onPreview,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  candidates: CreativeMaterialCandidate[];
  filter: CandidateFilter;
  onFilterChange: (filter: CandidateFilter) => void;
  selectedCount: number;
  busy: boolean;
  onStatus: (candidateId: string, status: string) => void;
  onPreview: (item: MediaPreviewItem) => void;
}) {
  const filteredCandidates = useMemo(
    () => candidates.filter((candidate) => candidateMatchesFilter(candidate, filter)),
    [candidates, filter],
  );
  const hasActiveFilter = !candidateFilterEquals(filter, DEFAULT_FILTER);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(98vw,1440px)]">
        <DialogHeader className="border-b px-5 py-4 pr-12">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <DialogTitle className="text-lg">素材候选池预览</DialogTitle>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <Badge variant="outline">{candidates.length} 条素材</Badge>
                <Badge variant="outline">当前显示 {filteredCandidates.length}</Badge>
                <Badge variant="outline">已选 {selectedCount}</Badge>
              </div>
            </div>
            {hasActiveFilter && (
              <button
                type="button"
                className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                onClick={() => onFilterChange(DEFAULT_FILTER)}
              >
                <RotateCcw className="h-3.5 w-3.5" />
                显示全部
              </button>
            )}
          </div>
          <div className="mt-4">
            <CandidateFilters
              candidates={candidates}
              filter={filter}
              onChange={onFilterChange}
            />
          </div>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto bg-muted/30 p-5">
          {filteredCandidates.length === 0 ? (
            <div className="flex h-64 items-center justify-center rounded-lg border border-dashed bg-background text-sm text-muted-foreground">
              当前筛选下没有候选素材。
            </div>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {filteredCandidates.map((candidate) => (
                <CandidatePreviewCard
                  key={candidate.id}
                  candidate={candidate}
                  busy={busy}
                  onStatus={(status) => onStatus(candidate.id, status)}
                  onPreview={onPreview}
                />
              ))}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CandidateFilters({
  candidates,
  filter,
  onChange,
}: {
  candidates: CreativeMaterialCandidate[];
  filter: CandidateFilter;
  onChange: (filter: CandidateFilter) => void;
}) {
  const competitors = uniqueOptions(candidates.map((candidate) => candidate.competitor));
  const assetTypes = uniqueOptions(candidates.map((candidate) => candidate.asset_type));
  const medias = uniqueOptions(candidates.flatMap((candidate) => candidate.media_names));
  const areas = uniqueOptions(candidates.flatMap((candidate) => candidate.area_names));
  const languages = uniqueOptions(candidates.flatMap((candidate) => candidate.language_names));
  const platforms = uniqueOptions(candidates.flatMap((candidate) => candidate.platform_names));

  const set = (key: keyof CandidateFilter, value: string) =>
    onChange({ ...filter, [key]: value });

  return (
    <div className="grid gap-2 md:grid-cols-4 xl:grid-cols-8">
      <FilterSelect label="状态" value={filter.status} values={["new", "selected", "sent_to_edit", "edited", "rejected", "approved", "archived"]} labels={STATUS_LABEL} onChange={(value) => set("status", value)} />
      <FilterSelect label="竞品" value={filter.competitor} values={competitors} onChange={(value) => set("competitor", value)} />
      <FilterSelect label="类型" value={filter.assetType} values={assetTypes} labels={{ image: "图片", video: "视频", unknown: "未知" }} onChange={(value) => set("assetType", value)} />
      <FilterSelect label="媒体" value={filter.media} values={medias} onChange={(value) => set("media", value)} />
      <FilterSelect label="地区" value={filter.area} values={areas} onChange={(value) => set("area", value)} />
      <FilterSelect label="语言" value={filter.language} values={languages} onChange={(value) => set("language", value)} />
      <FilterSelect label="设备" value={filter.platform} values={platforms} onChange={(value) => set("platform", value)} />
      <label className="grid gap-1 text-xs font-medium text-muted-foreground">
        标题搜索
        <div className="relative">
          <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={filter.text}
            onChange={(event) => set("text", event.target.value)}
            className="h-8 pl-7 text-sm"
          />
        </div>
      </label>
    </div>
  );
}

function FilterSelect({
  label,
  value,
  values,
  labels = {},
  onChange,
}: {
  label: string;
  value: string;
  values: string[];
  labels?: Record<string, string>;
  onChange: (value: string) => void;
}) {
  return (
    <label className="grid gap-1 text-xs font-medium text-muted-foreground">
      {label}
      <NativeSelect
        size="sm"
        value={value}
        className="w-full"
        onChange={(event) => onChange(event.target.value)}
      >
        <NativeSelectOption value="all">全部</NativeSelectOption>
        {values.map((item) => (
          <NativeSelectOption key={item} value={item}>
            {labels[item] ?? item}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </label>
  );
}

function CandidateCard({
  candidate,
  busy,
  onStatus,
  onPreview,
}: {
  candidate: CreativeMaterialCandidate;
  busy: boolean;
  onStatus: (status: string) => void;
  onPreview: (item: MediaPreviewItem) => void;
}) {
  const mediaURL = firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url);
  const previewURL = candidate.asset_type === "video"
    ? firstNonEmpty(candidate.resource_url, mediaURL)
    : mediaURL;
  const openURL = firstNonEmpty(candidate.resource_url, candidate.preview_url, candidate.poster_url, candidate.original_url);
  const isSelected = candidate.status === "selected";
  const isRejected = candidate.status === "rejected";

  return (
    <article
      className={cn(
        "group overflow-hidden rounded-lg border bg-card transition-colors",
        isSelected && "border-emerald-500/70 ring-1 ring-emerald-500/20",
        isRejected && "opacity-55",
        !isRejected && "hover:border-foreground/20",
      )}
      role="button"
      tabIndex={0}
      aria-pressed={isSelected}
      onClick={() => onStatus(isSelected ? "new" : "selected")}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onStatus(isSelected ? "new" : "selected");
        }
      }}
    >
      <div className="relative aspect-[4/3] bg-muted">
        <MediaPreview
          url={previewURL}
          posterUrl={candidate.poster_url || candidate.preview_url}
          alt={candidate.title || candidate.competitor}
          assetType={candidate.asset_type}
          compact
        />
        {previewURL && (
          <button
            type="button"
            className="absolute bottom-2 right-2 inline-flex h-8 w-8 items-center justify-center rounded-md bg-background/90 text-foreground opacity-0 shadow-sm ring-1 ring-border transition hover:bg-background group-hover:opacity-100 focus:opacity-100 focus-visible:ring-2 focus-visible:ring-primary"
            onClick={(event) => {
              event.stopPropagation();
              onPreview({
              url: previewURL,
              posterUrl: candidate.poster_url || candidate.preview_url,
              title: candidate.title || candidate.competitor || "素材预览",
              subtitle: candidate.competitor,
              assetType: candidate.asset_type,
              openUrl: openURL,
              });
            }}
            aria-label="放大预览素材"
          >
            <Maximize2 className="h-4 w-4" />
          </button>
        )}
        <Badge variant="secondary" className="absolute left-2 top-2 bg-background/90">
          {candidate.asset_type === "video" ? <Video className="h-3 w-3" /> : <ImageIcon className="h-3 w-3" />}
          {candidate.asset_type === "video" ? "视频" : "图片"}
        </Badge>
        {isSelected && (
          <span className="absolute right-2 top-2 inline-flex h-7 w-7 items-center justify-center rounded-full bg-emerald-500 text-white shadow-sm">
            <Check className="h-4 w-4" />
          </span>
        )}
        {isRejected && (
          <Badge variant="secondary" className="absolute right-2 top-2 bg-background/90 text-muted-foreground">
            不采用
          </Badge>
        )}
      </div>
      <div className="space-y-3 p-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">{candidate.competitor || "未命名竞品"}</h3>
          <p className="mt-1 line-clamp-2 min-h-9 text-sm text-muted-foreground">
            {candidate.title || "未返回标题"}
          </p>
        </div>
        <div className="grid grid-cols-2 gap-2 rounded-md bg-muted/50 p-2 text-xs">
          <Metric label="投放天数" value={formatDuration(candidate.duration_days)} />
          <Metric label="曝光估算" value={formatImpression(candidate.impression_estimate)} />
        </div>
        <div className="space-y-1">
          <MetaChips label="媒体" values={candidate.media_names} limit={3} />
          <MetaChips label="地区" values={candidate.area_names} limit={2} />
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">
            {isSelected ? "已选中" : "点卡片选择"}
          </span>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            className={cn(
              "h-7 px-2 text-xs text-muted-foreground hover:text-foreground",
              !isRejected && "opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100",
            )}
            onClick={(event) => {
              event.stopPropagation();
              onStatus(isRejected ? "new" : "rejected");
            }}
          >
            <X className="h-4 w-4" />
            {isRejected ? "恢复" : "不采用"}
          </Button>
        </div>
      </div>
    </article>
  );
}

function CandidatePreviewCard({
  candidate,
  busy,
  onStatus,
  onPreview,
}: {
  candidate: CreativeMaterialCandidate;
  busy: boolean;
  onStatus: (status: string) => void;
  onPreview: (item: MediaPreviewItem) => void;
}) {
  const mediaURL = firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url);
  const previewURL = candidate.asset_type === "video"
    ? firstNonEmpty(candidate.resource_url, mediaURL)
    : mediaURL;
  const openURL = firstNonEmpty(candidate.resource_url, candidate.preview_url, candidate.poster_url, candidate.original_url);
  const isSelected = candidate.status === "selected";
  const isRejected = candidate.status === "rejected";

  return (
    <article
      className={cn(
        "group overflow-hidden rounded-lg border bg-background shadow-sm transition-colors",
        isSelected && "border-emerald-500/70 ring-1 ring-emerald-500/20",
        isRejected && "opacity-55",
        !isRejected && "hover:border-foreground/20",
      )}
      role="button"
      tabIndex={0}
      aria-pressed={isSelected}
      onClick={() => onStatus(isSelected ? "new" : "selected")}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onStatus(isSelected ? "new" : "selected");
        }
      }}
    >
      <div className="relative aspect-[4/3] w-full bg-muted">
        <MediaPreview
          url={previewURL}
          posterUrl={candidate.poster_url || candidate.preview_url}
          alt={candidate.title || candidate.competitor}
          assetType={candidate.asset_type}
          compact
        />
        <Badge variant="secondary" className="absolute left-3 top-3 bg-background/90">
          {candidate.asset_type === "video" ? <Video className="h-3 w-3" /> : <ImageIcon className="h-3 w-3" />}
          {candidate.asset_type === "video" ? "视频" : "图片"}
        </Badge>
        {isSelected && (
          <span className="absolute right-3 top-3 inline-flex h-8 w-8 items-center justify-center rounded-full bg-emerald-500 text-white shadow-sm">
            <Check className="h-4 w-4" />
          </span>
        )}
        {isRejected && (
          <Badge variant="secondary" className="absolute right-3 top-3 bg-background/90 text-muted-foreground">
            不采用
          </Badge>
        )}
        {previewURL && (
          <button
            type="button"
            className="absolute bottom-3 right-3 inline-flex h-8 w-8 items-center justify-center rounded-md bg-background/90 text-foreground opacity-0 shadow-sm ring-1 ring-border transition hover:bg-background group-hover:opacity-100 focus:opacity-100 focus-visible:ring-2 focus-visible:ring-primary"
            onClick={(event) => {
              event.stopPropagation();
              if (!previewURL) return;
          onPreview({
            url: previewURL,
            posterUrl: candidate.poster_url || candidate.preview_url,
            title: candidate.title || candidate.competitor || "素材预览",
            subtitle: candidate.competitor,
            assetType: candidate.asset_type,
            openUrl: openURL,
          });
            }}
            aria-label="放大预览素材"
          >
            <Maximize2 className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
      <div className="space-y-3 p-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate text-sm font-semibold">{candidate.competitor || "未命名竞品"}</h3>
            <p className="mt-1 line-clamp-2 min-h-10 text-sm text-muted-foreground">
              {candidate.title || "未返回标题"}
            </p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2 rounded-md bg-muted/50 p-2 text-xs">
          <Metric label="投放天数" value={formatDuration(candidate.duration_days)} />
          <Metric label="曝光估算" value={formatImpression(candidate.impression_estimate)} />
        </div>
        <div className="grid gap-1">
          <MetaChips label="媒体" values={candidate.media_names} limit={5} />
          <MetaChips label="地区" values={candidate.area_names} limit={4} />
          <MetaChips label="语言" values={candidate.language_names} limit={3} />
          <MetaChips label="设备" values={candidate.platform_names} limit={3} />
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">
            {isSelected ? "已选中" : "点卡片选择"}
          </span>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            className={cn(
              "h-7 px-2 text-xs text-muted-foreground hover:text-foreground",
              !isRejected && "opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100",
            )}
            onClick={(event) => {
              event.stopPropagation();
              onStatus(isRejected ? "new" : "rejected");
            }}
          >
            <X className="h-4 w-4" />
            {isRejected ? "恢复" : "不采用"}
          </Button>
        </div>
      </div>
    </article>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-muted-foreground">{label}</div>
      <div className="font-semibold">{value}</div>
    </div>
  );
}

function MetaChips({
  label,
  values,
  limit,
}: {
  label: string;
  values: string[];
  limit: number;
}) {
  if (values.length === 0) return null;
  const shown = values.slice(0, limit);
  const hidden = values.length - shown.length;
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground">
      <span className="mr-1 shrink-0">{label}</span>
      {shown.map((value) => (
        <span
          key={value}
          className="max-w-24 truncate rounded-md bg-muted px-1.5 py-0.5 text-muted-foreground"
          title={value}
        >
          {value}
        </span>
      ))}
      {hidden > 0 && (
        <span className="rounded-md bg-muted px-1.5 py-0.5 text-muted-foreground">
          +{hidden}
        </span>
      )}
    </div>
  );
}

function CreativeResultPreviewDialog({
  open,
  onOpenChange,
  job,
  activeCandidateId,
  candidateByID,
  onPreview,
  downloading,
  onDownloadSelected,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  job: CreativeEditJob;
  activeCandidateId: string;
  candidateByID: Map<string, CreativeMaterialCandidate>;
  onPreview: (item: MediaPreviewItem) => void;
  downloading: boolean;
  onDownloadSelected: (assetIDs: string[], includeOriginal: boolean) => void;
}) {
  const candidate = candidateByID.get(activeCandidateId);
  const sourceURL = candidate
    ? firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url)
    : "";
  const variants = job.variants.filter((variant) => variant.candidate_id === activeCandidateId);
  const expectedVariantCount = creativeEditExpectedVariantCount(job);
  const [detailVariantID, setDetailVariantID] = useState("");
  const detailVariant = variants.find((variant) => variant.id === detailVariantID);
  const variantPreviews = variants.map((variant) => ({
    variant,
    asset: variant.assets.find((asset) => asset.label === "1080x1080") ?? variant.assets[0],
  })).filter((item): item is { variant: typeof variants[number]; asset: NonNullable<typeof variants[number]["assets"][number]> } => !!item.asset);
  const deliveryAssets = variants.flatMap((variant) => variant.assets.map((asset) => ({ variant, asset })));
  const [selectedAssetIDs, setSelectedAssetIDs] = useState<Set<string>>(new Set());
  const [includeOriginal, setIncludeOriginal] = useState(true);
  const qcAttempts = extractCreativeQcAttemptRounds(job.process_data, activeCandidateId)
    .flatMap((round) => round.attempts.map((attempt) => ({ round, attempt })))
    .filter(({ attempt }) => !!attempt.imageUrl);

  useEffect(() => {
    setDetailVariantID("");
    setSelectedAssetIDs(new Set(deliveryAssets.map(({ asset }) => asset.id)));
    setIncludeOriginal(true);
  }, [open, job.id, activeCandidateId]);

  const toggleAssets = (assetIDs: string[], checked: boolean) => {
    setSelectedAssetIDs((current) => {
      const next = new Set(current);
      for (const assetID of assetIDs) {
        if (checked) next.add(assetID);
        else next.delete(assetID);
      }
      return next;
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(98vw,1440px)]">
        <DialogHeader className="border-b px-5 py-4 pr-12">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                {detailVariant && (
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    title="返回变体列表"
                    aria-label="返回变体列表"
                    onClick={() => setDetailVariantID("")}
                  >
                    <ArrowLeft className="h-4 w-4" />
                  </Button>
                )}
                <DialogTitle className="text-lg">
                  {detailVariant ? `变体 ${detailVariant.variant_index} 尺寸预览` : "修图结果预览"}
                </DialogTitle>
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <Badge variant="outline">{candidate?.competitor || "当前素材"}</Badge>
                <Badge variant="outline">已完成 {variants.length}/{expectedVariantCount} 变体</Badge>
                <Badge variant="outline">每个变体 3 个尺寸</Badge>
                {qcAttempts.length > 0 && <Badge variant="outline">QC 尝试 {qcAttempts.length} 张</Badge>}
                <Badge variant="outline">已选 {selectedAssetIDs.size} 张</Badge>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={selectedAssetIDs.size === 0 || downloading}
                  onClick={() => onDownloadSelected([...selectedAssetIDs], includeOriginal)}
                >
                  <Download className="h-4 w-4" />
                  {downloading ? "正在打包" : "下载已选"}
                </Button>
              </div>
            </div>
          </div>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto bg-muted/30 p-5">
          {detailVariant ? (
            <div className="grid items-start gap-4 md:grid-cols-2 xl:grid-cols-3">
              {detailVariant.assets.map((asset) => (
                <article key={asset.id} className="group overflow-hidden rounded-lg border bg-card">
                  <button
                    type="button"
                    className="relative block w-full cursor-zoom-in bg-muted text-left outline-none ring-inset focus-visible:ring-2 focus-visible:ring-primary"
                    style={{ aspectRatio: `${asset.width} / ${asset.height}` }}
                    onClick={() => asset.asset_url && onPreview({
                      url: creativeEditAssetPreviewURL(job.issue_id, asset.id),
                      title: `变体 ${detailVariant.variant_index} - ${asset.label || `${asset.width}x${asset.height}`}`,
                      subtitle: candidate?.competitor,
                      assetType: asset.content_type.startsWith("video/") ? "video" : "image",
                      openUrl: asset.asset_url,
                    })}
                  >
                    <MediaPreview url={creativeEditAssetPreviewURL(job.issue_id, asset.id)} alt={asset.label || `${asset.width}x${asset.height}`} compact />
                    <span className="absolute left-2 top-2" onClick={(event) => event.stopPropagation()}>
                      <Checkbox
                        checked={selectedAssetIDs.has(asset.id)}
                        aria-label={`选择变体 ${detailVariant.variant_index} 的 ${asset.label}`}
                        className="bg-background/90"
                        onCheckedChange={(checked) => toggleAssets([asset.id], checked === true)}
                      />
                    </span>
                  </button>
                  <div className="flex items-center justify-between gap-2 p-3 text-sm">
                    <span className="font-semibold">{asset.label || `${asset.width}x${asset.height}`}</span>
                    <span className="text-xs text-muted-foreground">变体 {detailVariant.variant_index}</span>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <>
              <div className="grid items-start gap-4 md:grid-cols-2 xl:grid-cols-3">
            <article className="group overflow-hidden rounded-lg border bg-card">
              <div className="relative aspect-[4/3] bg-muted">
                {sourceURL ? (
                  <button
                    type="button"
                    className="h-full w-full cursor-zoom-in outline-none ring-inset focus-visible:ring-2 focus-visible:ring-primary"
                    onClick={() => onPreview({
                      url: sourceURL,
                      posterUrl: candidate?.poster_url || candidate?.preview_url,
                      title: candidate?.title || candidate?.competitor || "原始参考图",
                      subtitle: candidate?.competitor,
                      assetType: candidate?.asset_type,
                      openUrl: firstNonEmpty(candidate?.resource_url || "", candidate?.preview_url || "", candidate?.poster_url || "", candidate?.original_url || ""),
                    })}
                  >
                    <MediaPreview
                      url={sourceURL}
                      posterUrl={candidate?.poster_url || candidate?.preview_url}
                      alt={candidate?.title || candidate?.competitor || "原始参考图"}
                      assetType={candidate?.asset_type}
                      compact
                    />
                  </button>
                ) : (
                  <div className="flex h-full items-center justify-center text-muted-foreground"><ImageIcon className="h-8 w-8" /></div>
                )}
                <span className="absolute left-2 top-2" onClick={(event) => event.stopPropagation()}>
                  <Checkbox checked={includeOriginal} aria-label="下载时包含原始参考图" className="bg-background/90" onCheckedChange={(checked) => setIncludeOriginal(checked === true)} />
                </span>
                <Badge variant="secondary" className="absolute right-2 top-2 bg-background/90">原始参考图</Badge>
              </div>
              <div className="flex items-center justify-between gap-2 p-3">
                <span className="truncate text-sm font-semibold">{candidate?.competitor || "素材"}</span>
                <Maximize2 className="h-4 w-4 shrink-0 text-muted-foreground" />
              </div>
            </article>
            {variantPreviews.length === 0 && qcAttempts.length === 0 && (
              <div className="col-span-full flex min-h-48 flex-col items-center justify-center border border-dashed bg-background px-6 text-center">
                <ImageIcon className="h-8 w-8 text-muted-foreground" />
                <p className="mt-3 text-sm font-semibold">本次未产出可预览的生成图</p>
                <p className="mt-1 max-w-xl text-xs leading-5 text-muted-foreground">
                  {creativePreviewEmptyMessage(job.error_message)}
                </p>
              </div>
            )}
            {variantPreviews.map(({ variant, asset }) => {
              const variantAssetIDs = variant.assets.map((item) => item.id);
              const allSelected = variantAssetIDs.every((assetID) => selectedAssetIDs.has(assetID));
              return (
              <article key={variant.id} className="group overflow-hidden rounded-lg border bg-card transition-colors hover:border-foreground/20">
                <button
                  type="button"
                  className="relative block w-full cursor-pointer bg-muted text-left outline-none ring-inset focus-visible:ring-2 focus-visible:ring-primary"
                  style={{ aspectRatio: `${asset.width} / ${asset.height}` }}
                  onClick={() => setDetailVariantID(variant.id)}
                >
                  {asset.asset_url ? (
                    <MediaPreview url={creativeEditAssetPreviewURL(job.issue_id, asset.id)} alt={asset.label || `${asset.width}x${asset.height}`} compact />
                  ) : (
                    <span className="flex h-full items-center justify-center text-muted-foreground"><ImageIcon className="h-8 w-8" /></span>
                  )}
                  <span className="absolute left-2 top-2" onClick={(event) => event.stopPropagation()}>
                    <Checkbox checked={allSelected} aria-label={`选择变体 ${variant.variant_index} 的全部尺寸`} className="bg-background/90" onCheckedChange={(checked) => toggleAssets(variantAssetIDs, checked === true)} />
                  </span>
                  <Badge variant="secondary" className="absolute right-2 top-2 bg-background/90">变体 {variant.variant_index}</Badge>
                  <span className="absolute bottom-2 right-2 inline-flex h-8 w-8 items-center justify-center rounded-md bg-background/90 text-foreground opacity-0 shadow-sm ring-1 ring-border transition group-hover:opacity-100 group-focus-within:opacity-100">
                    <Maximize2 className="h-4 w-4" />
                  </span>
                </button>
                <div className="flex items-center justify-between gap-2 p-3 text-sm">
                  <span className="font-semibold">变体 {variant.variant_index}</span>
                  <span className="text-xs text-muted-foreground">{variant.assets.length} 个尺寸</span>
                </div>
              </article>
              );
            })}
              </div>
              {qcAttempts.length > 0 && (
                <section className="mt-6 border-t pt-5">
                  <div className="mb-3">
                    <div className="text-sm font-semibold">QC 过程图</div>
                    <p className="mt-1 text-xs text-muted-foreground">仅用于回看失败原因，不属于可交付成图。</p>
                  </div>
                  <div className="grid items-start gap-4 md:grid-cols-2 xl:grid-cols-3">
            {qcAttempts.map(({ round, attempt }) => (
              <article key={`qc:${attempt.key}`} className="group overflow-hidden rounded-lg border border-dashed bg-card">
                <button
                  type="button"
                  className="relative block aspect-[4/3] w-full cursor-zoom-in bg-muted text-left outline-none ring-inset focus-visible:ring-2 focus-visible:ring-primary"
                  onClick={() => onPreview({
                    url: attempt.imageUrl,
                    title: `第 ${round.round} 轮 · 概念 ${attempt.conceptIndex} · ${attempt.size}`,
                    subtitle: candidate?.competitor,
                    assetType: "image",
                    openUrl: attempt.imageUrl,
                  })}
                >
                  <MediaPreview url={attempt.imageUrl} alt={`第 ${round.round} 轮概念 ${attempt.conceptIndex} ${attempt.size} 尝试图`} compact />
                  <Badge variant="secondary" className="absolute left-2 top-2 bg-background/90">QC 尝试</Badge>
                  <span className={cn(
                    "absolute right-2 top-2 rounded-md bg-background/90 px-2 py-1 text-xs font-medium",
                    attempt.verdict === "pass" ? "text-emerald-700 dark:text-emerald-300" : "text-destructive",
                  )}>
                    {attempt.verdict === "pass" ? "通过" : attempt.verdict === "generation_error" ? "生成失败" : "未通过"}
                  </span>
                </button>
                <div className="flex items-center justify-between gap-2 p-3 text-sm">
                  <span className="font-semibold">第 {round.round} 轮 · {attempt.size}</span>
                  <span className="text-xs text-muted-foreground">概念 {attempt.conceptIndex}</span>
                </div>
              </article>
            ))}
                  </div>
                </section>
              )}
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function EditResultBoard({
  jobs,
  job,
  activeCandidateId,
  activeJobCandidates,
  candidateByID,
  viewedJobIDs,
  onJobChange,
  onCandidateChange,
  onSyncJob,
  onDownloadJob,
  syncingJobId,
  downloadingJobId,
  submittingFeedbackVariantId,
  onSubmitFeedback,
  onOpenLargePreview,
  onPreview,
}: {
  jobs: CreativeEditJob[];
  job: CreativeEditJob;
  activeCandidateId: string;
  activeJobCandidates: string[];
  candidateByID: Map<string, CreativeMaterialCandidate>;
  viewedJobIDs: Set<string>;
  onJobChange: (id: string) => void;
  onCandidateChange: (id: string) => void;
  onSyncJob: (id: string) => void;
  onDownloadJob: (id: string) => void;
  syncingJobId?: string;
  downloadingJobId?: string;
  submittingFeedbackVariantId?: string;
  onSubmitFeedback: (
    jobId: string,
    variantId: string,
    feedback: CreateCreativeEditFeedbackRequest,
  ) => Promise<void>;
  onOpenLargePreview: () => void;
  onPreview: (item: MediaPreviewItem) => void;
}) {
  const variants = job.variants.filter((variant) => variant.candidate_id === activeCandidateId);
  const activeCandidate = candidateByID.get(activeCandidateId);
  const activeCandidateURL = activeCandidate
    ? firstNonEmpty(activeCandidate.archived_url, activeCandidate.preview_url, activeCandidate.poster_url, activeCandidate.resource_url)
    : "";
  const isSyncing = syncingJobId === job.id;
  const hasResults = variants.length > 0;
  const expectedVariantCount = creativeEditExpectedVariantCount(job);
  const completedVariantCount = variants.length;
  const isPartialResult = completedVariantCount > 0 && completedVariantCount < expectedVariantCount;
  const canDownload = job.variants.length > 0
    && new Set(["completed", "partial", "failed"]).has(job.status);
  const history = creativeEditJobHistory(jobs);

  return (
    <div className="rounded-lg border bg-muted/20">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-3 py-3">
        <div>
          <h3 className="text-sm font-semibold">修图结果看板</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {hasResults
              ? "下方用于对照原图并反馈变体；三种尺寸可在“预览成图”中查看。"
              : "任务已提交，结果会自动更新。"}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {hasResults ? (
            <Badge variant={isPartialResult ? "secondary" : "outline"}>
              已完成 {completedVariantCount}/{expectedVariantCount} 变体
            </Badge>
          ) : (
            <Badge variant="outline">{creativeEditJobStatusLabel(job.status)}</Badge>
          )}
          {canDownload && (
            <Button
              size="sm"
              variant="outline"
              title="下载全部选中原图，以及每个变体的 1080x1080、800x1000、1200x628 成图"
              disabled={downloadingJobId === job.id}
              onClick={() => onDownloadJob(job.id)}
            >
              <Download className="h-4 w-4" />
              {downloadingJobId === job.id ? "正在打包" : "全部下载"}
            </Button>
          )}
          <Button
            size="sm"
            variant="outline"
            onClick={onOpenLargePreview}
          >
            <Maximize2 className="h-4 w-4" />
            预览成图
          </Button>
        </div>
      </div>
      <div className="border-b bg-background/60 px-3 py-2">
        <div className="flex items-center gap-2 overflow-x-auto pb-1" role="tablist" aria-label="出图记录">
          {history.map(({ job: item, sequence }) => {
            const isActive = item.id === job.id;
            const isViewed = viewedJobIDs.has(item.id);
            return (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={isActive}
                className={cn(
                  "relative flex shrink-0 items-center gap-2 rounded-md border px-2.5 py-2 text-left text-xs outline-none transition-colors hover:border-primary/50 focus-visible:ring-2 focus-visible:ring-primary",
                  isActive ? "border-primary bg-primary/5 ring-1 ring-primary" : "bg-background",
                )}
                onClick={() => onJobChange(item.id)}
              >
                {!isViewed && <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-primary" aria-label="未查看" />}
                <span className="font-semibold">出图 {sequence}</span>
                <span className="text-muted-foreground">{formatCreativeEditJobTime(item.created_at)}</span>
                <Badge variant={item.status === "failed" ? "destructive" : item.status === "partial" ? "secondary" : "outline"} className="h-5 px-1.5 text-[10px]">
                  {creativeEditJobStatusLabel(item.status)}
                </Badge>
              </button>
            );
          })}
        </div>
      </div>
      {activeJobCandidates.length > 1 && (
        <div className="border-b bg-background/40 px-3 py-2.5">
          <div className="flex items-center gap-3 overflow-x-auto pb-1" role="tablist" aria-label="本批素材">
            <span className="shrink-0 text-xs font-medium text-muted-foreground">本批素材</span>
            {activeJobCandidates.map((candidateID, index) => {
              const candidate = candidateByID.get(candidateID);
              const candidateURL = candidate
                ? firstNonEmpty(candidate.archived_url, candidate.preview_url, candidate.poster_url, candidate.resource_url)
                : "";
              const candidateVariants = job.variants.filter((variant) => variant.candidate_id === candidateID).length;
              const isActive = candidateID === activeCandidateId;
              return (
                <button
                  key={candidateID}
                  type="button"
                  role="tab"
                  aria-selected={isActive}
                  className={cn(
                    "flex w-48 shrink-0 items-center gap-2 rounded-md border p-1.5 text-left outline-none transition-colors hover:border-primary/50 focus-visible:ring-2 focus-visible:ring-primary",
                    isActive ? "border-primary bg-primary/5 ring-1 ring-primary" : "bg-background",
                  )}
                  onClick={() => onCandidateChange(candidateID)}
                >
                  <span className="flex h-10 w-10 shrink-0 overflow-hidden rounded-sm bg-muted">
                    {candidateURL ? (
                      <MediaPreview url={candidateURL} alt={candidate?.title || `素材 ${index + 1}`} compact />
                    ) : (
                      <span className="flex h-full w-full items-center justify-center text-muted-foreground"><ImageIcon className="h-4 w-4" /></span>
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs font-semibold">素材 {index + 1} · {candidate?.competitor || "未知来源"}</span>
                    <span className="block truncate text-[11px] text-muted-foreground">{candidateVariants}/{expectedVariantCount} 变体</span>
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}
      <div className="grid gap-3 p-3 lg:grid-cols-[260px_1fr]">
        <div className="rounded-md border bg-background p-3">
          <div className="text-xs text-muted-foreground">当前素材</div>
          <div className="mt-1 text-sm font-semibold">{activeCandidate?.competitor || "素材"}</div>
          <p className="mt-1 line-clamp-3 text-xs text-muted-foreground">{activeCandidate?.title || "未返回标题"}</p>
          <div className="mt-3 aspect-[4/3] overflow-hidden rounded-md bg-muted">
            {activeCandidate && activeCandidateURL ? (
              <button
                type="button"
                className="group/media relative h-full w-full cursor-zoom-in outline-none ring-inset focus-visible:ring-2 focus-visible:ring-primary"
                onClick={() => onPreview({
                  url: activeCandidateURL,
                  posterUrl: activeCandidate.poster_url || activeCandidate.preview_url,
                  title: activeCandidate.title || activeCandidate.competitor || "素材预览",
                  subtitle: activeCandidate.competitor,
                  assetType: activeCandidate.asset_type,
                  openUrl: firstNonEmpty(activeCandidate.resource_url, activeCandidate.preview_url, activeCandidate.poster_url, activeCandidate.original_url),
                })}
              >
                <MediaPreview
                  url={activeCandidateURL}
                  posterUrl={activeCandidate.poster_url || activeCandidate.preview_url}
                  alt={activeCandidate.title || activeCandidate.competitor}
                  assetType={activeCandidate.asset_type}
                  compact
                />
                <span className="absolute bottom-2 right-2 inline-flex items-center gap-1 rounded-md bg-background/90 px-2 py-1 text-xs font-medium opacity-0 shadow-sm ring-1 ring-border transition group-hover/media:opacity-100">
                  <Maximize2 className="h-3.5 w-3.5" />
                  预览
                </span>
              </button>
            ) : (
              <div className="flex h-full items-center justify-center text-muted-foreground">
                <ImageIcon className="h-7 w-7" />
              </div>
            )}
          </div>
        </div>
        <div className="space-y-3">
          {!hasResults ? (
            <CreativeEditJobPendingPanel
              job={job}
              isSyncing={isSyncing}
              onSync={() => onSyncJob(job.id)}
            />
          ) : (
            <>
              <div className="rounded-md border bg-background p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-semibold">全部已完成变体</div>
                    <p className="mt-1 text-xs text-muted-foreground">选择变体反馈；尺寸在预览成图中查看。</p>
                  </div>
                  {isPartialResult && (
                    <Badge variant="secondary">第 {completedVariantCount + 1} 个变体未完成</Badge>
                  )}
                </div>
              </div>
              <div className="grid gap-3 xl:grid-cols-2">
              {variants.map((variant) => {
                const previewAsset = variant.assets.find((asset) => asset.label === "1080x1080") ?? variant.assets[0];
                return (
                <section key={variant.id} className="rounded-md border bg-background p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <div className="text-sm font-semibold">变体 {variant.variant_index}</div>
                      {variant.description && <p className="mt-1 text-xs text-muted-foreground">{variant.description}</p>}
                    </div>
                    <Badge variant="outline">质检 {variant.qc_status || "待处理"}</Badge>
                  </div>
                  <button
                    type="button"
                    className="group mt-3 block w-full overflow-hidden rounded-md border bg-muted text-left outline-none transition-colors hover:border-primary/50 focus-visible:ring-2 focus-visible:ring-primary"
                    onClick={() => previewAsset?.asset_url && onPreview({
                      url: creativeEditAssetPreviewURL(issue.id, previewAsset.id),
                      title: `变体 ${variant.variant_index} - ${previewAsset.label || `${previewAsset.width}x${previewAsset.height}`}`,
                      subtitle: activeCandidate?.competitor,
                      assetType: previewAsset.content_type.startsWith("video/") ? "video" : "image",
                      openUrl: previewAsset.asset_url,
                    })}
                  >
                    <div className="relative aspect-square bg-muted">
                      {previewAsset?.asset_url ? <MediaPreview url={creativeEditAssetPreviewURL(issue.id, previewAsset.id)} alt={`变体 ${variant.variant_index}`} compact /> : (
                        <div className="flex h-full items-center justify-center text-muted-foreground"><ImageIcon className="h-7 w-7" /></div>
                      )}
                      <span className="absolute bottom-2 right-2 inline-flex items-center rounded-md bg-background/90 p-1.5 opacity-0 shadow-sm ring-1 ring-border transition group-hover:opacity-100">
                        <Maximize2 className="h-3.5 w-3.5" />
                      </span>
                    </div>
                  </button>
                  <div className="mt-3">
                    <CreativeVariantFeedback
                      variant={variant}
                      submitting={submittingFeedbackVariantId === variant.id}
                      onSubmit={(feedback) => onSubmitFeedback(job.id, variant.id, feedback)}
                    />
                  </div>
                </section>
                );
              })}
              </div>
            </>
          )}
          <CreativeQcAttemptHistory
            processData={job.process_data}
            candidateId={activeCandidateId}
            candidateName={activeCandidate?.competitor}
            onPreview={onPreview}
          />
        </div>
      </div>
    </div>
  );
}

function CreativeEditJobPendingPanel({
  job,
  isSyncing,
  onSync,
}: {
  job: CreativeEditJob;
  isSyncing: boolean;
  onSync: () => void;
}) {
  const progress = clampPercent(job.progress);
  const title = creativeEditJobPanelTitle(job.status);
  const isTerminal = new Set(["completed", "partial", "failed"]).has(job.status);

  return (
    <div className="rounded-md border bg-background p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold">{title}</h4>
            <Badge variant="outline">{creativeEditJobStatusLabel(job.status)}</Badge>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {isTerminal
              ? "最终成图可能为空，但已生成的尝试图和每轮 QC 结论仍会保留在下方。"
              : "后台会自动同步；离开页面或服务重启后，任务仍会继续。"}
          </p>
        </div>
        <Button
          size="icon-sm"
          variant="ghost"
          disabled={isSyncing}
          onClick={onSync}
          title="立即同步"
          aria-label="立即同步"
        >
          <RotateCcw className={cn("h-4 w-4", isSyncing && "animate-spin")} />
        </Button>
      </div>
      <div className="mt-4">
        <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
          <span>{job.stage || "等待结果"}</span>
          <span>{progress}%</span>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-primary transition-all"
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{createCreativeJobToolNameText()}</span>
        {job.external_job_id && (
          <span className="max-w-full truncate">job {job.external_job_id}</span>
        )}
      </div>
      {job.error_message && (
        <p className="mt-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {job.error_message}
        </p>
      )}
    </div>
  );
}

function MediaPreview({
  url,
  alt,
  assetType,
  posterUrl,
  compact = false,
}: {
  url: string;
  alt: string;
  assetType?: string;
  posterUrl?: string;
  compact?: boolean;
}) {
  if (!url) {
    return (
      <div className="flex h-full w-full items-center justify-center text-muted-foreground">
        <ImageIcon className="h-8 w-8" />
      </div>
    );
  }
  if (assetType === "video" || isVideoURL(url)) {
    return (
      <video
        src={url}
        poster={posterUrl}
        controls={!compact}
        muted
        playsInline
        preload="metadata"
        className="h-full w-full object-contain"
      />
    );
  }
  return <img src={url} alt={alt} className="h-full w-full object-contain" />;
}

function MediaPreviewDialog({
  item,
  onOpenChange,
}: {
  item: MediaPreviewItem | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={!!item} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[94vh] gap-0 overflow-hidden p-0 sm:max-w-[min(96vw,1280px)]">
        <DialogHeader className="border-b px-4 py-3 pr-12">
          <DialogTitle className="truncate text-sm">
            {item?.subtitle ? `${item.subtitle} - ${item.title}` : (item?.title ?? "素材预览")}
          </DialogTitle>
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            {item?.assetType && (
              <Badge variant="outline">{item.assetType === "video" ? "视频" : "图片"}</Badge>
            )}
            {item?.openUrl && (
              <a
                href={item.openUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-foreground hover:underline"
              >
                <ExternalLink className="h-3.5 w-3.5" />
                打开原始素材
              </a>
            )}
          </div>
        </DialogHeader>
        <div className="flex h-[min(78vh,820px)] items-center justify-center bg-black p-3">
          {item && (
            <MediaPreview
              url={item.url}
              posterUrl={item.posterUrl}
              alt={item.title}
              assetType={item.assetType}
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function candidateMatchesFilter(candidate: CreativeMaterialCandidate, filter: CandidateFilter) {
  if (filter.status !== "all" && candidate.status !== filter.status) return false;
  if (filter.competitor !== "all" && candidate.competitor !== filter.competitor) return false;
  if (filter.assetType !== "all" && candidate.asset_type !== filter.assetType) return false;
  if (filter.media !== "all" && !candidate.media_names.includes(filter.media)) return false;
  if (filter.area !== "all" && !candidate.area_names.includes(filter.area)) return false;
  if (filter.language !== "all" && !candidate.language_names.includes(filter.language)) return false;
  if (filter.platform !== "all" && !candidate.platform_names.includes(filter.platform)) return false;
  const text = filter.text.trim().toLowerCase();
  if (text) {
    const haystack = [
      candidate.title,
      candidate.competitor,
      candidate.media_names.join(" "),
      candidate.area_names.join(" "),
      candidate.language_names.join(" "),
      candidate.platform_names.join(" "),
    ].join(" ").toLowerCase();
    if (!haystack.includes(text)) return false;
  }
  return true;
}

function candidateFilterEquals(left: CandidateFilter, right: CandidateFilter) {
  return left.status === right.status
    && left.competitor === right.competitor
    && left.assetType === right.assetType
    && left.media === right.media
    && left.area === right.area
    && left.language === right.language
    && left.platform === right.platform
    && left.text === right.text;
}

function uniqueOptions(values: string[]) {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b));
}

function firstNonEmpty(...values: string[]) {
  return values.find((value) => value.trim() !== "")?.trim() ?? "";
}

function isVideoURL(url: string) {
  return /\.(mp4|mov|webm|m3u8)(?:[?#].*)?$/i.test(url);
}

function creativeEditExpectedVariantCount(job: CreativeEditJob) {
  const rules = parseCreativeEditRules(job.rules);
  const value = rules?.variant_count;
  if (typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= 6) return value;
  return 3;
}

function parseCreativeEditRules(rules: unknown): Record<string, unknown> | null {
  if (rules && typeof rules === "object" && !Array.isArray(rules)) {
    return rules as Record<string, unknown>;
  }
  if (typeof rules !== "string") return null;
  try {
    const parsed: unknown = JSON.parse(rules);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : null;
  } catch {
    return null;
  }
}

function creativeEditJobStatusLabel(status: string) {
  switch (status) {
    case "queued":
      return "排队中";
    case "running":
      return "生成中";
    case "completed":
      return "已完成";
    case "partial":
      return "部分完成";
    case "failed":
      return "失败";
    default:
      return status || "等待同步";
  }
}

function creativeEditJobPanelTitle(status: string) {
  switch (status) {
    case "queued": return "等待生成";
    case "running": return "生成中";
    case "completed": return "没有通过 QC 的最终成图";
    case "partial": return "部分成图通过 QC";
    case "failed": return "任务失败";
    default: return "任务状态";
  }
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, Math.round(value)));
}

function createCreativeJobToolNameText() {
  return "create_creative_job -> get_creative_job";
}

function formatDuration(value: number | null) {
  if (value == null) return "-";
  return `${Math.round(value).toLocaleString()} 天`;
}

function formatImpression(value: number | null) {
  if (value == null) return "-";
  if (value >= 1_000_000) return `${trimNumber(value / 1_000_000)}M`;
  if (value >= 1_000) return `${trimNumber(value / 1_000)}K`;
  return Math.round(value).toLocaleString();
}

function trimNumber(value: number) {
  return value.toFixed(value >= 10 ? 0 : 1).replace(/\.0$/, "");
}
