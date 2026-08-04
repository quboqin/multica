"use client";

import { cloneElement, useEffect, useId, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Archive,
  BookOpenText,
  Check,
	CheckCircle2,
  FileSpreadsheet,
  FolderOpen,
  Globe2,
  Layers3,
  Plus,
  QrCode,
  RefreshCw,
  Save,
  Search,
  Settings2,
  Upload,
} from "lucide-react";
import { api } from "@multica/core/api";
import {
  creativeCopyEntriesOptions,
  creativeFeedbackOptions,
  creativeKeys,
  creativeOrderOptions,
  creativeOrdersOptions,
  creativeMaterialLibraryOptions,
  creativeResourcesOptions,
} from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type {
  CreativeCopyEntry,
  CreativeCopyEntryInput,
  CreativeOrder,
  CreativeOrderAsset,
  CreativeOrderWorkflowFailure,
  CreativeOrderVariant,
  CreativeResource,
  CreativeResourceKind,
} from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
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
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";
import { PageHeader } from "../../layout/page-header";
import { useNavigation } from "../../navigation";
import { readCopySpreadsheet, type SpreadsheetData } from "../lib/xlsx-copy-import";
import { CreativeMaterialLibrary } from "./creative-material-library";
import { CreativeCollectionPlans } from "./creative-collection-plans";
import { CreativeComparisonWorkspace, type CreativeAnnotationDraft } from "./creative-comparison-workspace";
import { MarketResourceFiles } from "./market-resource-files";

type CopyField = keyof Pick<
  CreativeCopyEntryInput,
  "external_key" | "headline" | "subheadline" | "benefit" | "cta" | "legal_text" |
  "copy_role" | "market" | "locale" | "tags" | "status"
>;

const COPY_FIELDS: { key: CopyField; label: string; required?: boolean }[] = [
  { key: "external_key", label: "文案编号" },
  { key: "headline", label: "主标题", required: true },
  { key: "subheadline", label: "副标题" },
  { key: "benefit", label: "卖点" },
  { key: "cta", label: "CTA" },
  { key: "legal_text", label: "条款" },
  { key: "copy_role", label: "文案职责" },
  { key: "market", label: "市场" },
  { key: "locale", label: "语言" },
  { key: "tags", label: "标签" },
  { key: "status", label: "状态" },
];

const PRIME_ROLE_LABELS: Record<string, string> = {
  prime_square: "Prime 方图模板",
  prime_landscape: "Prime 横图模板",
  prime_portrait: "Prime 竖图模板",
};

const EMPTY_COPY: CreativeCopyEntryInput = {
  external_key: "",
  headline: "",
  subheadline: "",
  benefit: "",
  cta: "",
  legal_text: "",
  copy_role: "",
  market: "",
  locale: "",
  tags: [],
  status: "draft",
  metadata: {},
};

type CreativeStudioTab = "discovery" | "orders" | "copy" | "market";

function creativeStudioTab(searchParams: URLSearchParams): CreativeStudioTab {
  if (searchParams.get("order")) return "orders";
  const tab = searchParams.get("tab");
  return tab === "orders" || tab === "copy" || tab === "market" ? tab : "discovery";
}

export function creativeStudioPath(
  pathname: string,
  searchParams: URLSearchParams,
  tab: CreativeStudioTab,
  orderId = "",
): string {
  const next = new URLSearchParams(searchParams);
  next.delete("tab");
  next.delete("order");
  if (tab !== "discovery") next.set("tab", tab);
  if (tab === "orders" && orderId) next.set("order", orderId);
  const query = next.toString();
  return query ? `${pathname}?${query}` : pathname;
}

export function CreativeStudioPage() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const navigation = useNavigation();
  const routeOrderId = navigation.searchParams.get("order") ?? "";
  const routeTab = creativeStudioTab(navigation.searchParams);
  const [tab, setTab] = useState<CreativeStudioTab>(routeTab);
  const [selectedOrderId, setSelectedOrderId] = useState(routeOrderId);
  const [createKind, setCreateKind] = useState<CreativeResourceKind | null>(null);
  const resources = useQuery(creativeResourcesOptions(wsId));
  const allResources = resources.data?.resources ?? [];

  useEffect(() => {
    setTab(routeTab);
    setSelectedOrderId(routeOrderId);
  }, [routeOrderId, routeTab]);

  const navigateStudio = (nextTab: CreativeStudioTab, orderId = "", mode: "push" | "replace" = "replace") => {
    setTab(nextTab);
    setSelectedOrderId(nextTab === "orders" ? orderId : "");
    const path = creativeStudioPath(navigation.pathname, navigation.searchParams, nextTab, orderId);
    if (mode === "push") navigation.push(path);
    else navigation.replace(path);
  };

  const changeTab = (value: string) => {
    const nextTab: CreativeStudioTab = value === "orders" || value === "copy" || value === "market" ? value : "discovery";
    navigateStudio(nextTab, nextTab === "orders" ? selectedOrderId : "");
  };

  const openOrder = (orderId: string) => navigateStudio("orders", orderId, "push");

  const refreshResources = () => queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
  const archiveResource = useMutation({
    mutationFn: (id: string) => api.archiveCreativeResource(id),
    onSuccess: () => { refreshResources(); toast.success("资源已归档"); },
    onError: () => toast.error("无法归档资源"),
  });

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex min-w-0 items-center gap-2">
          <Settings2 className="h-4 w-4 text-emerald-700" />
          <h1 className="text-sm font-medium">创意工厂</h1>
          <span className="hidden text-xs text-muted-foreground md:inline">维护素材、文案和市场资源</span>
        </div>
        <Badge variant="outline">{allResources.length} 个可配置资源</Badge>
      </PageHeader>

      <Tabs value={tab} onValueChange={changeTab} className="flex min-h-0 flex-1 flex-col">
        <div className="border-b px-5 py-2">
          <TabsList className="h-8 overflow-x-auto">
            <TabsTrigger value="discovery"><FolderOpen className="h-3.5 w-3.5" />采集运行 / 素材发现</TabsTrigger>
            <TabsTrigger value="orders"><Layers3 className="h-3.5 w-3.5" />创意订单 / 交付</TabsTrigger>
            <TabsTrigger value="copy"><BookOpenText className="h-3.5 w-3.5" />文案库</TabsTrigger>
            <TabsTrigger value="market"><Globe2 className="h-3.5 w-3.5" />市场资源包</TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="discovery" className="min-h-0 flex-1 overflow-y-auto p-5">
          <CreativeDiscoveryWorkspace onOrderCreated={openOrder} />
        </TabsContent>
        <TabsContent value="orders" className="min-h-0 flex-1 overflow-y-auto p-5"><CreativeOrdersWorkspace selectedOrderId={selectedOrderId} onSelectOrder={openOrder} onBack={() => navigateStudio("orders")} /></TabsContent>
        <TabsContent value="copy" className="min-h-0 flex-1 overflow-hidden p-5">
          <CopyLibraries
            resources={allResources.filter((resource) => resource.kind === "copy_library")}
            onCreate={() => setCreateKind("copy_library")}
            onArchive={(id) => archiveResource.mutate(id)}
          />
        </TabsContent>
        <TabsContent value="market" className="min-h-0 flex-1 overflow-hidden p-5">
          <ResourceEditor
            resources={allResources.filter((resource) => resource.kind === "market_pack")}
            copyLibraries={allResources.filter((resource) => resource.kind === "copy_library")}
            onCreate={() => setCreateKind("market_pack")}
            onArchive={(id) => archiveResource.mutate(id)}
          />
        </TabsContent>
      </Tabs>

      <CreateResourceDialog kind={createKind} onClose={() => setCreateKind(null)} onCreated={() => {
        setCreateKind(null);
        refreshResources();
      }} />
    </div>
  );
}

function CreativeDiscoveryWorkspace({ onOrderCreated }: { onOrderCreated: (orderId: string) => void }) {
  return <div className="mx-auto max-w-[1440px] space-y-4"><div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3"><div><h2 className="text-base font-semibold">采集运行与素材发现</h2><p className="mt-1 text-sm text-muted-foreground">浏览已归档的竞品参考；素材进入订单前保持独立于 issue。</p></div><Badge variant="outline">稳定原图优先</Badge></div><CreativeCollectionPlans /><CreativeMaterialLibrary onOrderCreated={onOrderCreated} /></div>;
}

function CreativeOrdersWorkspace({ selectedOrderId, onSelectOrder, onBack }: { selectedOrderId: string; onSelectOrder: (orderId: string) => void; onBack: () => void }) {
  const wsId = useWorkspaceId();
  const navigation = useNavigation();
  const workspacePaths = useWorkspacePaths();
  const orders = useQuery(creativeOrdersOptions(wsId));
  const creativeOrders = orders.data?.orders ?? [];
  if (selectedOrderId) return <CreativeOrderDetail orderId={selectedOrderId} onBack={onBack} />;
  return <div className="mx-auto max-w-[1440px] space-y-4"><div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3"><div><h2 className="text-base font-semibold">创意订单与交付</h2><p className="mt-1 text-sm text-muted-foreground">订单 API 是交付进度的来源；关联 issue 只用于协作摘要和跳转。</p></div><Badge variant="outline">{creativeOrders.length} 个订单</Badge></div><div className="grid gap-2">{creativeOrders.map((order) => <div key={order.id} className="grid min-h-16 grid-cols-[minmax(0,1fr)_auto] items-center gap-4 border bg-background px-4 py-3"><button type="button" className="min-w-0 text-left" onClick={() => onSelectOrder(order.id)}><span className="block truncate text-sm font-semibold">订单 {order.id.slice(0, 8)}</span><span className="mt-1 block text-[11px] text-muted-foreground">{order.trigger_evidence_kind || "人工创建"} · 更新于 {order.updated_at || "-"}</span></button><span className="flex items-center gap-2"><Badge variant="outline">{order.derived_status || order.status}</Badge>{order.issue_id && <Button size="sm" variant="outline" onClick={() => navigation.push(workspacePaths.issueDetail(order.issue_id))}>查看 issue 摘要</Button>}</span></div>)}</div>{!orders.isLoading && creativeOrders.length === 0 && <div className="flex min-h-64 items-center justify-center border border-dashed text-sm text-muted-foreground">暂无创意订单。采集后的素材会在创建订单后出现在这里。</div>}</div>;
}

function CreativeOrderDetail({ orderId, onBack }: { orderId: string; onBack: () => void }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const order = useQuery(creativeOrderOptions(wsId, orderId));
  const library = useQuery(creativeMaterialLibraryOptions(wsId));
  const feedback = useQuery(creativeFeedbackOptions(wsId, "variant"));
  const data = order.data;
  const assets = data?.items.flatMap((item) => item.variants.flatMap((variant) => variant.assets)) ?? [];
  const ids = [...new Set(assets.map((asset) => asset.attachment_id).filter(Boolean))];
  const attachments = useQuery({ queryKey: ["creative", wsId, "order-attachments", orderId, ids], queryFn: () => Promise.all(ids.map((id) => api.getAttachment(id))), enabled: ids.length > 0 });
  const byId = new Map((attachments.data ?? []).map((item) => [item.id, item]));
  const reviewAssets = selectCreativeReviewAssets(assets);
  const [activeAssetId, setActiveAssetId] = useState("");
  const [adjustOpen, setAdjustOpen] = useState(false);
  const [adjustment, setAdjustment] = useState("");
  const [adjustBusy, setAdjustBusy] = useState(false);
  const [retryingTaskId, setRetryingTaskId] = useState("");
  const defaultAsset = reviewAssets.find((asset) => asset.status === "completed") ?? reviewAssets[0];
  const active = reviewAssets.find((asset) => asset.id === activeAssetId) ?? defaultAsset;
  const variantById = new Map(data?.items.flatMap((item) => item.variants.map((variant) => [variant.id, { variant, item }] as const)) ?? []);
  const activeVariant = active ? variantById.get(active.variant_id) : undefined;
  const variantDecisions = useMemo(() => latestVariantFeedback(feedback.data?.events ?? []), [feedback.data?.events]);
  const acceptanceStatus = creativeOrderAcceptanceStatus(data, variantDecisions);
  const activeReadiness = activeVariant ? creativeVariantAcceptanceReadiness(activeVariant.variant) : { ready: false, status: "正在读取变体交付状态" };
  const activeAccepted = activeVariant ? variantDecisions.get(activeVariant.variant.id) === "accepted" : false;
  const source = library.data?.candidates.find((candidate) => candidate.id === activeVariant?.item.candidate_id);
  const generatedFor = (asset: CreativeOrderAsset) => assets.find((candidate) => candidate.status === "completed" && candidate.variant_id === asset.variant_id && candidate.size_key === asset.size_key && candidate.revision === asset.revision && candidate.stage === "generated");
  const attachmentURL = (asset?: CreativeOrderAsset) => {
    const attachment = asset ? byId.get(asset.attachment_id) : undefined;
    return resolvePublicFileUrl(attachment?.download_url || attachment?.url) ?? "";
  };
  const comparisonAssets = reviewAssets.filter((asset) => byId.has(asset.attachment_id)).map((asset) => ({ id: asset.id, label: `${asset.stage} · ${asset.size_key}`, finalUrl: attachmentURL(asset), baseUrl: attachmentURL(generatedFor(asset)), thumbnailUrl: byId.get(asset.attachment_id)?.url, size: asset.size_key, variant: variantById.get(asset.variant_id)?.variant.variant_key || "" }));

  const event = async (asset: CreativeOrderAsset, decision: "accepted" | "abandoned" | "downloaded") => {
    const variant = variantById.get(asset.variant_id)?.variant;
    if (!variant) return;
    if (decision === "accepted") {
      const readiness = creativeVariantAcceptanceReadiness(variant);
      if (!readiness.ready) {
        toast.error(readiness.status);
        return;
      }
    }
    try {
      if (decision === "downloaded") {
        await api.createCreativeFeedback({ issue_id: data?.issue_id ?? "", subject_type: "asset", subject_id: asset.id, event_type: "viewed", decision: "", context_snapshot: { action: "download", order_id: orderId, variant_id: asset.variant_id, size_key: asset.size_key, revision: asset.revision } });
        return;
      }
      await api.createCreativeFeedback({ issue_id: data?.issue_id ?? "", subject_type: "variant", subject_id: variant.id, event_type: "decision", decision, reason_codes: decision === "abandoned" ? ["other"] : [], comment: decision === "abandoned" ? "用户放弃当前变体" : "", context_snapshot: { order_id: orderId, asset_id: asset.id, size_key: asset.size_key, revision: asset.revision } });
      await Promise.all([
        order.refetch(),
        feedback.refetch(),
      ]);
      toast.success(decision === "accepted" ? "已接受当前变体" : "已记录放弃决定");
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法记录决定"); }
  };
  const annotation = async (asset: CreativeOrderAsset, draft: CreativeAnnotationDraft) => {
    try {
      await api.createCreativeFeedback({ issue_id: data?.issue_id ?? "", subject_type: "asset", subject_id: asset.id, event_type: "annotation", decision: "needs_revision", reason_codes: [assetFeedbackReason(draft.issueType)], comment: draft.comment, annotation: { id: crypto.randomUUID(), asset_id: asset.id, kind: draft.kind, issue_type: draft.issueType, x: draft.x, y: draft.y, width: draft.width, height: draft.height, scope: draft.scope, comment: draft.comment }, context_snapshot: { order_id: orderId, variant_id: asset.variant_id, size_key: asset.size_key, revision: asset.revision } });
      toast.success("问题标注已保存");
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法保存问题标注"); }
  };
  const submitAdjustment = async () => {
    if (!active || !data?.issue_id || !adjustment.trim()) return;
    setAdjustBusy(true);
    try {
      await api.createCreativeFeedback({ issue_id: data.issue_id, subject_type: "asset", subject_id: active.id, event_type: "decision", decision: "needs_revision", reason_codes: ["other"], comment: adjustment.trim(), context_snapshot: { order_id: orderId, variant_id: active.variant_id, size_key: active.size_key, revision: active.revision } });
      await api.createComment(data.issue_id, `用户在创意工厂提出调整：\n\n${adjustment.trim()}\n\n目标：${activeVariant?.variant.variant_key || active.variant_id} · ${active.size_key} · r${active.revision}。请按订单当前资产创建精准返工或直接改图 task，只处理受影响范围并保留其他已通过资产。`);
      setAdjustment(""); setAdjustOpen(false); toast.success("调整请求已提交给素材小队");
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法提交调整请求"); }
    finally { setAdjustBusy(false); }
  };

  const retryFailure = async (failure: CreativeOrderWorkflowFailure) => {
    if (!canRetryWorkflowFailure(failure)) return;
    setRetryingTaskId(failure.task_id);
    try {
      await api.retryFailedAgentTasksBySource(
        failure.agent_id,
        failure.trigger_evidence_kind,
        failure.trigger_evidence_ref_id,
      );
      await Promise.all([
        order.refetch(),
        queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) }),
      ]);
      toast.success("失败步骤已重新排队");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法重试失败步骤");
    } finally {
      setRetryingTaskId("");
    }
  };

  return <div className="mx-auto max-w-[1440px] space-y-4"><div className="flex items-center justify-between border-b pb-3"><div><Button size="sm" variant="ghost" onClick={onBack}>返回订单</Button><h2 className="mt-2 text-base font-semibold">订单 {orderId.slice(0, 8)}</h2></div><div className="flex items-center gap-2"><Badge variant="outline">{data?.derived_status || data?.status || "加载中"}</Badge><Badge variant={acceptanceStatus === "已验收" ? "default" : "secondary"}>{acceptanceStatus}</Badge></div></div>{data && <OrderProgress order={data} variantDecisions={variantDecisions} />}{data && <CreativeOrderFailureNotice failures={data.workflow_failures ?? []} retryingTaskId={retryingTaskId} onRetry={(failure) => void retryFailure(failure)} />}{active && attachmentURL(active) && <div className="h-[min(78vh,860px)] min-h-[620px] overflow-hidden border"><CreativeComparisonWorkspace source={{ label: source?.title || "原始素材", url: resolvePublicFileUrl(source?.archived_url || source?.preview_url) ?? "" }} result={{ id: active.id, label: `${activeVariant?.variant.variant_key || "结果"} · ${active.size_key}`, finalUrl: attachmentURL(active), baseUrl: attachmentURL(generatedFor(active)), size: active.size_key, variant: activeVariant?.variant.variant_key }} assets={comparisonAssets} onAssetChange={setActiveAssetId} onAdjust={() => setAdjustOpen(true)} onDecision={(decision) => void event(active, decision)} onAnnotation={(draft) => void annotation(active, draft)} acceptance={{ enabled: activeReadiness.ready && !activeAccepted, status: !activeReadiness.ready ? activeReadiness.status : activeAccepted ? "当前变体已验收" : activeReadiness.status }} /></div>}{data && !order.isLoading && reviewAssets.length === 0 && <div className="flex min-h-72 items-center justify-center border border-dashed px-6 text-center text-sm text-muted-foreground">{creativeOrderWaitingMessage(data.workflow_failures ?? [])}</div>}<Dialog open={adjustOpen} onOpenChange={setAdjustOpen}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>调整当前成图</DialogTitle><DialogDescription>{activeVariant?.variant.variant_key || "当前变体"} · {active?.size_key}。请求会记录到订单反馈，并通知小队 Leader 精准委派。</DialogDescription></DialogHeader><Textarea rows={5} value={adjustment} onChange={(event) => setAdjustment(event.target.value)} placeholder="说明需要改什么、必须保留什么，以及只影响当前尺寸还是整个变体..." /><DialogFooter><Button variant="outline" onClick={() => setAdjustOpen(false)}>取消</Button><Button disabled={adjustBusy || !adjustment.trim()} onClick={() => void submitAdjustment()}>{adjustBusy ? "正在提交" : "提交调整"}</Button></DialogFooter></DialogContent></Dialog></div>;
}

export function CreativeOrderFailureNotice({ failures, retryingTaskId, onRetry }: {
  failures: CreativeOrderWorkflowFailure[];
  retryingTaskId: string;
  onRetry: (failure: CreativeOrderWorkflowFailure) => void;
}) {
  if (failures.length === 0) return null;
  return <section className="border border-destructive/40 bg-destructive/5" role="alert" aria-label="创意流程失败">
    <div className="flex gap-3 border-b border-destructive/30 px-4 py-3"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" /><div><h3 className="text-sm font-semibold">创意流程需要处理</h3><p className="mt-1 text-xs text-muted-foreground">以下步骤已经失败，不会继续以“等待成图”隐藏。可重试任务会从原失败来源重新排队。</p></div></div>
    <div className="divide-y divide-destructive/20">{failures.map((failure) => {
      const retryable = canRetryWorkflowFailure(failure);
      const busy = retryingTaskId === failure.task_id;
      return <div key={failure.task_id || `${failure.workflow}:${failure.subject_id}:${failure.failed_at}`} className="px-4 py-3">
        <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="text-sm font-medium">失败步骤：{creativeWorkflowLabel(failure.workflow)}</span>{failure.item_key && <Badge variant="outline" title={failure.item_key}>对象 {failure.item_key.slice(0, 8)}</Badge>}</div><p className="mt-1 break-words text-sm text-destructive">{workflowFailureMessage(failure)}</p><p className="mt-1 break-words text-xs text-muted-foreground">范围：{failure.scope || "未提供"}{failure.failure_reason && failure.error !== failure.failure_reason ? ` · 分类：${failure.failure_reason}` : ""}</p></div>
        <div className="mt-3">{failure.retryable ? <Button size="sm" variant="outline" disabled={!retryable || busy || Boolean(retryingTaskId)} onClick={() => onRetry(failure)}><RefreshCw className={cn("h-4 w-4", busy && "animate-spin")} />{busy ? "正在重试" : retryable ? "重试失败步骤" : "缺少重试来源"}</Button> : <span className="text-xs text-muted-foreground">需要人工处理</span>}</div>
      </div>;
    })}</div>
  </section>;
}

export function creativeOrderWaitingMessage(failures: CreativeOrderWorkflowFailure[]): string {
  return failures.length > 0
    ? "失败步骤恢复后，首批成图会自动出现在这里。"
    : "正在等待首批成图。各变体完成后会立即出现在这里。";
}

function canRetryWorkflowFailure(failure: CreativeOrderWorkflowFailure): boolean {
  return failure.retryable && Boolean(failure.agent_id && failure.trigger_evidence_kind && failure.trigger_evidence_ref_id);
}

function workflowFailureMessage(failure: CreativeOrderWorkflowFailure): string {
  return failure.error || failure.failure_reason || "任务执行失败，未返回详细原因";
}

function creativeWorkflowLabel(workflow: string): string {
  const labels: Record<string, string> = {
    creative_reference_analysis: "参考分析",
    creative_plan: "创意方案",
    creative_production: "底图生成",
    creative_prime: "四角贴片",
    creative_qc_technical: "技术质检",
    creative_qc_visual: "视觉质检",
    creative_direct_edit: "直接改图",
  };
  return labels[workflow] || workflow || "未知步骤";
}

export function selectCreativeReviewAssets(assets: CreativeOrderAsset[]): CreativeOrderAsset[] {
  const selected = new Map<string, CreativeOrderAsset>();
  for (const asset of assets) {
    if (asset.status !== "completed" || !asset.attachment_id) continue;
    const key = `${asset.variant_id}:${asset.size_key}`;
    const current = selected.get(key);
    if (!current || compareCreativeAssets(asset, current) > 0) selected.set(key, asset);
  }
  return [...selected.values()].sort((left, right) =>
    left.variant_id.localeCompare(right.variant_id) || left.size_key.localeCompare(right.size_key),
  );
}

function compareCreativeAssets(left: CreativeOrderAsset, right: CreativeOrderAsset): number {
  if (left.revision !== right.revision) return left.revision - right.revision;
  const stageRank = (asset: CreativeOrderAsset) => asset.stage === "delivered" ? 3 : asset.stage === "primed" ? 2 : asset.stage === "generated" ? 1 : 0;
  const stageDifference = stageRank(left) - stageRank(right);
  if (stageDifference !== 0) return stageDifference;
  const updatedDifference = (Date.parse(left.updated_at) || 0) - (Date.parse(right.updated_at) || 0);
  return updatedDifference || left.id.localeCompare(right.id);
}

const REQUIRED_CREATIVE_DELIVERY_SIZES = ["1080x1080", "1200x628", "800x1000"] as const;

export function creativeVariantAcceptanceReadiness(variant: CreativeOrderVariant): { ready: boolean; status: string } {
  const revision = variant.revision;
  const completedSizes = (stage: "primed" | "delivered") => new Set(variant.assets
    .filter((asset) => asset.revision === revision && asset.stage === stage && asset.status === "completed" && asset.attachment_id)
    .map((asset) => asset.size_key)
    .filter((size) => REQUIRED_CREATIVE_DELIVERY_SIZES.includes(size as typeof REQUIRED_CREATIVE_DELIVERY_SIZES[number])));
  const primedSizes = completedSizes("primed");
  if (primedSizes.size < REQUIRED_CREATIVE_DELIVERY_SIZES.length) {
    return { ready: false, status: `等待 Prime：已完成 ${primedSizes.size}/${REQUIRED_CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  }

  const reportByLane = new Map(variant.qc_reports
    .filter((report) => report.revision === revision && (report.lane === "technical" || report.lane === "visual"))
    .map((report) => [report.lane, report.status]));
  const technical = reportByLane.get("technical") ?? "pending";
  const visual = reportByLane.get("visual") ?? "pending";
  if (technical === "failed" || visual === "failed") {
    return { ready: false, status: `QC 未通过：technical ${qcStatusLabel(technical)}，visual ${qcStatusLabel(visual)}` };
  }
  const qcComplete = (status: string) => status === "passed" || status === "warning";
  if (!qcComplete(technical) || !qcComplete(visual)) {
    return { ready: false, status: `等待双路 QC：technical ${qcStatusLabel(technical)}，visual ${qcStatusLabel(visual)}` };
  }

  const deliveredSizes = completedSizes("delivered");
  if (deliveredSizes.size < REQUIRED_CREATIVE_DELIVERY_SIZES.length) {
    return { ready: false, status: `等待正式交付：已完成 ${deliveredSizes.size}/${REQUIRED_CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  }
  if (variant.status !== "completed") {
    return { ready: false, status: `等待变体完成：当前状态 ${variant.status}` };
  }
  return { ready: true, status: "三尺寸、Prime 与双路 QC 均已完成，可以验收" };
}

function qcStatusLabel(status: string): string {
  if (status === "passed") return "通过";
  if (status === "warning") return "通过（有提醒）";
  if (status === "failed") return "失败";
  return "待完成";
}

function OrderProgress({ order, variantDecisions }: { order: CreativeOrder; variantDecisions: Map<string, string> }) { return <div className="divide-y border">{order.items.map((item) => <div key={item.id} className="p-3"><p className="break-words text-sm font-medium">{item.direction || `候选 ${item.candidate_id.slice(0, 8)}`}</p>{item.variants.map((variant) => {
  const readiness = creativeVariantAcceptanceReadiness(variant);
  const accepted = readiness.ready && variantDecisions.get(variant.id) === "accepted";
  return <div key={variant.id} className="mt-2 flex flex-wrap gap-2 text-xs"><Badge variant="outline">{variant.variant_key}</Badge><span>{variant.status} · QC {variant.qc_status}</span><span>{variant.assets.length} 个资产</span><Badge variant={accepted ? "default" : "secondary"}>{accepted ? "已验收" : "待验收"}</Badge>{!accepted && <span className="text-muted-foreground">{readiness.status}</span>}{variant.qc_reports.map((report) => <Badge key={report.id} variant="outline">{report.lane}: {report.status}</Badge>)}</div>;
})}</div>)}</div>; }

export function latestVariantFeedback(events: { id: string; subject_id: string; event_type: string; decision: string; undo_of_id: string; created_at?: string }[]): Map<string, string> {
  const undone = new Set(events.filter((event) => event.event_type === "undo" && event.undo_of_id).map((event) => event.undo_of_id));
  const latest = new Map<string, { decision: string; created_at: string; id: string }>();
  for (const event of events.filter((event) => event.event_type === "decision" && !undone.has(event.id))) {
    const current = latest.get(event.subject_id);
    const currentTime = Date.parse(current?.created_at ?? "") || 0;
    const eventTime = Date.parse(event.created_at ?? "") || 0;
    if (!current || eventTime > currentTime || (eventTime === currentTime && event.id.localeCompare(current.id) > 0)) latest.set(event.subject_id, { decision: event.decision, created_at: event.created_at ?? "", id: event.id });
  }
  return new Map([...latest.entries()].map(([variantId, event]) => [variantId, event.decision]));
}

export function creativeOrderAcceptanceStatus(order: CreativeOrder | undefined, variantDecisions: Map<string, string>): "待验收" | "已验收" {
  const variants = order?.items.flatMap((item) => item.variants) ?? [];
  return variants.length > 0 && variants.every((variant) => creativeVariantAcceptanceReadiness(variant).ready && variantDecisions.get(variant.id) === "accepted") ? "已验收" : "待验收";
}

function assetFeedbackReason(issueType: string): string {
  if (issueType === "theme_drift") return "theme_mismatch";
  if (issueType === "artifact") return "broken_image";
  if (issueType === "brand_prime") return "brand_or_prime";
  if (issueType === "copy_error") return "copy_error";
  return "other";
}

function CopyLibraries({ resources, onCreate, onArchive }: {
  resources: CreativeResource[];
  onCreate: () => void;
  onArchive: (id: string) => void;
}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState(resources[0]?.id ?? "");
  const [spreadsheet, setSpreadsheet] = useState<{ data: SpreadsheetData; filename: string } | null>(null);
  const [mapping, setMapping] = useState<Record<CopyField, string>>(emptyCopyMapping());
  const [editing, setEditing] = useState<CreativeCopyEntry | "new" | null>(null);
  const [query, setQuery] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const active = resources.find((resource) => resource.id === activeId) ?? resources[0];
  const entries = useQuery(creativeCopyEntriesOptions(wsId, active?.id ?? ""));
  const visibleEntries = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return entries.data?.entries ?? [];
    return (entries.data?.entries ?? []).filter((entry) => [
      entry.external_key,
      entry.headline,
      entry.subheadline,
      entry.benefit,
      entry.cta,
      entry.legal_text,
      entry.copy_role,
      entry.market,
      entry.locale,
      ...entry.tags,
    ].join(" ").toLocaleLowerCase().includes(needle));
  }, [entries.data?.entries, query]);

  const importRows = useMutation({
    mutationFn: async () => {
      if (!active || !spreadsheet) throw new Error("请选择文案库和文件");
      const parsed = spreadsheet.data.rows.map((row, index) => spreadsheetRowToCopy(row, spreadsheet.data.headers, mapping, index));
      return api.importCreativeCopyEntries(active.id, {
        mode: "upsert",
        source_filename: spreadsheet.filename,
        mapping,
        entries: parsed,
      });
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.copyEntries(wsId, active?.id ?? "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
      setSpreadsheet(null);
      toast.success(`导入完成：新增 ${result.created}，更新 ${result.updated}，跳过 ${result.skipped}`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法导入文案"),
  });

  const saveEntry = useMutation({
    mutationFn: async (input: CreativeCopyEntryInput) => {
      if (!active) throw new Error("请选择文案库");
      if (editing === "new") {
        await api.importCreativeCopyEntries(active.id, {
          mode: "upsert", source_filename: "在线编辑", mapping: {}, entries: [input],
        });
        return;
      }
      if (editing) await api.updateCreativeCopyEntry(editing.id, input);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.copyEntries(wsId, active?.id ?? "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
      setEditing(null);
      toast.success("文案已保存为草稿版本");
    },
    onError: () => toast.error("无法保存文案"),
  });

  const handleFile = async (file?: File) => {
    if (!file) return;
    try {
      const data = await readCopySpreadsheet(file);
      setMapping(inferCopyMapping(data.headers));
      setSpreadsheet({ data, filename: file.name });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法读取表格");
    }
  };

  return (
    <div className="grid h-full min-h-0 grid-cols-1 overflow-hidden border md:grid-cols-[260px_minmax(0,1fr)]">
      <ResourceList title="文案库" resources={resources} activeId={active?.id ?? ""} onSelect={setActiveId} onCreate={onCreate} />
      <div className="min-w-0 overflow-y-auto bg-background">
        {!active ? <EmptyResource title="还没有文案库" action="创建文案库" onAction={onCreate} /> : (
          <>
            <div className="sticky top-0 z-10 border-b bg-background px-4 py-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <ResourceTitle resource={active} />
                <div className="flex items-center gap-2">
                <input ref={fileRef} type="file" accept=".xlsx,.csv" className="hidden" onChange={(event) => handleFile(event.target.files?.[0])} />
                <Button size="sm" variant="outline" onClick={() => fileRef.current?.click()}><Upload className="h-4 w-4" />导入 Excel</Button>
                <Button size="sm" variant="outline" onClick={() => setEditing("new")}><Plus className="h-4 w-4" />添加文案</Button>
                <PublishButton resource={active} />
                <Button size="icon-sm" variant="ghost" title="归档文案库" aria-label="归档文案库" onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button>
                </div>
              </div>
              <div className="mt-3 flex items-center gap-3">
                <div className="relative max-w-xl flex-1"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索编号、标题、卖点、职责或标签" /></div>
                <span className="shrink-0 text-xs text-muted-foreground">{visibleEntries.length} / {entries.data?.entries.length ?? 0} 条</span>
              </div>
            </div>
            <div className="min-w-[920px]">
              <div className="grid grid-cols-[120px_minmax(180px,1fr)_minmax(160px,1fr)_140px_100px_72px_56px] gap-3 border-b bg-muted/30 px-4 py-2 text-xs font-medium text-muted-foreground">
                <span>编号</span><span>主标题</span><span>副标题 / 卖点</span><span>职责</span><span>市场</span><span>状态</span><span />
              </div>
              {visibleEntries.map((entry) => (
                <div key={entry.id} className="grid min-h-14 grid-cols-[120px_minmax(180px,1fr)_minmax(160px,1fr)_140px_100px_72px_56px] items-center gap-3 border-b px-4 py-2 text-sm hover:bg-muted/20">
                  <span className="truncate font-mono text-xs text-muted-foreground">{entry.external_key}</span>
                  <span className="truncate font-medium">{entry.headline || "未填写"}</span>
                  <span className="truncate text-muted-foreground">{entry.subheadline || entry.benefit || "-"}</span>
                  <span className="truncate">{entry.copy_role || "通用"}</span>
                  <span className="truncate">{entry.market || "-"}</span>
                  <Badge variant="outline" className="w-fit text-[10px]">{copyStatusLabel(entry.status)}</Badge>
                  <Button size="icon-sm" variant="ghost" title="编辑文案" aria-label="编辑文案" onClick={() => setEditing(entry)}><Settings2 className="h-4 w-4" /></Button>
                </div>
              ))}
              {!entries.isLoading && (entries.data?.entries.length ?? 0) === 0 && <div className="py-20 text-center text-sm text-muted-foreground">导入 Excel 或在线添加第一条文案。</div>}
              {!entries.isLoading && (entries.data?.entries.length ?? 0) > 0 && visibleEntries.length === 0 && <div className="py-20 text-center text-sm text-muted-foreground">没有匹配的文案。</div>}
            </div>
          </>
        )}
      </div>
      <SpreadsheetImportDialog
        value={spreadsheet}
        mapping={mapping}
        onMapping={setMapping}
        onClose={() => setSpreadsheet(null)}
        onImport={() => importRows.mutate()}
        busy={importRows.isPending}
      />
      <CopyEntryDialog
        entry={editing}
        onClose={() => setEditing(null)}
        onSave={(input) => saveEntry.mutate(input)}
        busy={saveEntry.isPending}
      />
    </div>
  );
}

function ResourceEditor({ resources, copyLibraries, onCreate, onArchive }: {
  resources: CreativeResource[];
  copyLibraries: CreativeResource[];
  onCreate: () => void;
  onArchive: (id: string) => void;
}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState(resources[0]?.id ?? "");
  const active = resources.find((resource) => resource.id === activeId) ?? resources[0];
  const [draft, setDraft] = useState<Record<string, unknown>>(active?.config ?? {});
  const draftKey = active ? `${active.id}:${active.version}` : "";
  useEffect(() => {
    setDraft(active?.config ?? {});
  }, [draftKey, active]);
  const save = useMutation({
    mutationFn: () => {
      if (!active) throw new Error("resource missing");
      return api.updateCreativeResource(active.id, { name: active.name, description: active.description, config: draft });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
      toast.success("配置已保存为新草稿版本");
    },
    onError: () => toast.error("无法保存配置"),
  });
  return (
    <div className="grid h-full min-h-0 grid-cols-1 overflow-hidden border md:grid-cols-[260px_minmax(0,1fr)]">
      <ResourceList title="市场资源包" resources={resources} activeId={active?.id ?? ""} onSelect={setActiveId} onCreate={onCreate} />
      <div className="min-w-0 overflow-y-auto bg-background">
        {!active ? <EmptyResource title="还没有市场资源包" action="创建资源包" onAction={onCreate} /> : (
          <>
            <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 border-b bg-background px-5 py-3">
              <ResourceTitle resource={active} />
              <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" onClick={() => save.mutate()} disabled={save.isPending}><Save className="h-4 w-4" />保存草稿</Button>
                <PublishButton resource={active} />
                <Button size="icon-sm" variant="ghost" title="归档资源" aria-label="归档资源" onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button>
              </div>
            </div>
            <MarketPackForm resource={active} value={draft} onChange={setDraft} copyLibraries={copyLibraries} />
          </>
        )}
      </div>
    </div>
  );
}

function MarketPackForm({ resource, value, onChange, copyLibraries }: {
  resource: CreativeResource;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  copyLibraries: CreativeResource[];
}) {
  const set = (key: string, next: unknown) => onChange({ ...value, [key]: next });
  const qrValidation = readQRValidation(resource.config.qr_validation);
  return (
    <div className="mx-auto max-w-5xl space-y-8 px-5 py-6">
      <FormSection title="市场与语言" description="运行时根据这里选择文案、合规和交付规则。">
        <Field label="品牌"><Input value={stringValue(value.brand)} onChange={(event) => set("brand", event.target.value)} /></Field>
        <Field label="市场"><Input value={stringValue(value.market)} placeholder="Indonesia" onChange={(event) => set("market", event.target.value)} /></Field>
        <Field label="语言"><Input value={stringValue(value.locale)} placeholder="id-ID" onChange={(event) => set("locale", event.target.value)} /></Field>
        <Field label="币种"><Input value={stringValue(value.currency)} placeholder="IDR" onChange={(event) => set("currency", event.target.value)} /></Field>
        <Field label="绑定文案库" wide>
          <NativeSelect value={stringValue(value.copy_library_id)} onChange={(event) => set("copy_library_id", event.target.value)}>
            <NativeSelectOption value="">选择已发布文案库</NativeSelectOption>
            {copyLibraries.map((library) => <NativeSelectOption key={library.id} value={library.id}>{library.name} · v{library.published_version || library.version}</NativeSelectOption>)}
          </NativeSelect>
        </Field>
      </FormSection>
      <FormSection title="创意理解" description="候选图按这里的市场词表识别主题与利益点；用户仍可逐图输入词表外的新值。">
        <Field label="利益点词表" wide><Textarea rows={4} value={stringListMultilineValue(value.benefit_taxonomy)} placeholder={"额度\n低利率\n费用减免\n快速放款"} onChange={(event) => set("benefit_taxonomy", splitList(event.target.value))} /></Field>
        <Field label="主题预设" wide><Textarea rows={4} value={stringListMultilineValue(value.theme_presets)} placeholder={"世界杯 / 足球赛事\n斋月\n开斋节\n发薪日"} onChange={(event) => set("theme_presets", splitList(event.target.value))} /></Field>
      </FormSection>
      <MarketResourceFiles resource={resource} />
      <FormSection title="二维码与规则" description="二维码必须经过业务批准，并与三个 Prime 模板的机器解码结果一致后才能发布。">
        <Field label="成品二维码目标地址" wide><Input value={stringValue(value.qr_payload)} placeholder="https://www.example.com/terms" onChange={(event) => set("qr_payload", event.target.value)} /></Field>
        <Field label="Prime 模板标准地址" wide><Input value={stringValue(value.qr_canonical_payload)} placeholder="从已批准 Prime 模板机器解码得到" onChange={(event) => set("qr_canonical_payload", event.target.value)} /></Field>
        <Field label="允许域名"><Input value={stringListValue(value.qr_allowed_domains)} placeholder="www.example.com" onChange={(event) => set("qr_allowed_domains", splitList(event.target.value))} /></Field>
        <Field label="业务批准状态"><NativeSelect value={stringValue(value.qr_approval_status) || "pending"} onChange={(event) => set("qr_approval_status", event.target.value)}><NativeSelectOption value="pending">待批准</NativeSelectOption><NativeSelectOption value="approved">已批准</NativeSelectOption></NativeSelect></Field>
        <Field label="批准说明" wide><Input value={stringValue(value.qr_approval_note)} placeholder="记录地址来源或批准依据" onChange={(event) => set("qr_approval_note", event.target.value)} /></Field>
        <Field label="合规规则" wide><Textarea rows={5} value={stringValue(value.compliance_rules)} onChange={(event) => set("compliance_rules", event.target.value)} /></Field>
        <Field label="文件命名" wide><Input value={stringValue(value.naming_rule)} placeholder="Month_P_Brand_Country_Date_Type_Designer_Size.png" onChange={(event) => set("naming_rule", event.target.value)} /></Field>
      </FormSection>
      <section>
        <div className="mb-3 flex items-center gap-2"><QrCode className="h-4 w-4" /><h3 className="text-sm font-semibold">发布校验</h3></div>
        {qrValidation ? <div className="border-y">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-emerald-50/50 px-4 py-3 dark:bg-emerald-950/15">
            <div className="flex min-w-0 items-center gap-2"><CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600" /><div className="min-w-0"><p className="text-sm font-medium">三个 Prime 模板校验通过</p><p className="truncate font-mono text-xs text-muted-foreground">{qrValidation.approved_payload}</p></div></div>
            <span className="text-xs text-muted-foreground">{formatValidationTime(qrValidation.validated_at)}</span>
          </div>
          <div className="divide-y">{qrValidation.templates.map((template) => <div key={template.role} className="grid gap-1 px-4 py-3 text-xs sm:grid-cols-[150px_minmax(0,1fr)_auto] sm:items-center"><span className="font-medium">{resourceSlotLabel(template.role)}</span><span className="truncate font-mono text-muted-foreground">{template.filename}</span><span className="text-emerald-700 dark:text-emerald-400">解码一致</span></div>)}</div>
        </div> : <div className="flex gap-3 border-y bg-amber-50/50 px-4 py-3 dark:bg-amber-950/15"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" /><div><p className="text-sm font-medium">当前草稿尚未通过二维码校验</p><p className="mt-1 text-xs text-muted-foreground">保存配置并点击发布。服务端会读取三个 Prime 模板，真实解码后再决定是否允许发布。</p></div></div>}
      </section>
    </div>
  );
}

function ResourceList({ title, resources, activeId, onSelect, onCreate }: {
  title: string;
  resources: CreativeResource[];
  activeId: string;
  onSelect: (id: string) => void;
  onCreate: () => void;
}) {
  return (
    <aside className="min-h-0 overflow-y-auto border-r bg-muted/15">
      <div className="sticky top-0 flex items-center justify-between border-b bg-background px-3 py-2.5">
        <span className="text-sm font-semibold">{title}</span>
        <Button size="icon-sm" variant="ghost" title={`创建${title}`} aria-label={`创建${title}`} onClick={onCreate}><Plus className="h-4 w-4" /></Button>
      </div>
      {resources.map((resource) => (
        <button key={resource.id} type="button" onClick={() => onSelect(resource.id)} className={cn("block w-full border-b px-3 py-3 text-left hover:bg-muted/40", resource.id === activeId && "bg-background shadow-[inset_2px_0_0_hsl(var(--primary))]") }>
          <div className="flex items-center justify-between gap-2"><span className="truncate text-sm font-medium">{resource.name}</span><span className="shrink-0 font-mono text-[10px] text-muted-foreground">v{resource.version}</span></div>
          <p className="mt-1 truncate text-xs text-muted-foreground">{resource.description || (resource.status === "published" ? "已发布" : "草稿")}</p>
        </button>
      ))}
    </aside>
  );
}

function ResourceTitle({ resource }: { resource: CreativeResource }) {
  return <div className="min-w-0"><div className="flex items-center gap-2"><h2 className="truncate text-sm font-semibold">{resource.name}</h2><Badge variant="outline">{resource.status === "published" ? `已发布 v${resource.published_version}` : `草稿 v${resource.version}`}</Badge></div><p className="mt-0.5 truncate text-xs text-muted-foreground">{resource.description}</p></div>;
}

function PublishButton({ resource }: { resource: CreativeResource }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const publish = useMutation({
    mutationFn: () => api.publishCreativeResource(resource.id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success("版本已发布"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法发布版本"),
  });
  return <Button size="sm" onClick={() => publish.mutate()} disabled={publish.isPending || (resource.status === "published" && resource.published_version === resource.version)}><Check className="h-4 w-4" />发布</Button>;
}

function CreateResourceDialog({ kind, onClose, onCreated }: { kind: CreativeResourceKind | null; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const create = useMutation({
    mutationFn: () => api.createCreativeResource({ kind: kind!, name, description, config: initialResourceConfig(kind!) }),
    onSuccess: () => { setName(""); setDescription(""); onCreated(); toast.success("资源已创建"); },
    onError: () => toast.error("无法创建资源"),
  });
  return (
    <Dialog open={kind !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent><DialogHeader><DialogTitle>创建{kindLabel(kind)}</DialogTitle><DialogDescription>先创建草稿，配置完成后再发布给 Issue 使用。</DialogDescription></DialogHeader>
        <div className="space-y-4"><Field label="名称" wide><Input value={name} onChange={(event) => setName(event.target.value)} /></Field><Field label="描述" wide><Textarea rows={3} value={description} onChange={(event) => setDescription(event.target.value)} /></Field></div>
        <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={!name.trim() || create.isPending} onClick={() => create.mutate()}>创建</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SpreadsheetImportDialog({ value, mapping, onMapping, onClose, onImport, busy }: {
  value: { data: SpreadsheetData; filename: string } | null;
  mapping: Record<CopyField, string>;
  onMapping: (value: Record<CopyField, string>) => void;
  onClose: () => void;
  onImport: () => void;
  busy: boolean;
}) {
  return <Dialog open={value !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-4xl"><DialogHeader><DialogTitle>映射 Excel 文案字段</DialogTitle><DialogDescription>{value ? `${value.filename} · ${value.data.sheetName} · ${value.data.rows.length} 行` : ""}</DialogDescription></DialogHeader>
    {value && <div className="grid max-h-[55vh] grid-cols-2 gap-4 overflow-y-auto pr-1 md:grid-cols-3">{COPY_FIELDS.map((field) => <Field key={field.key} label={`${field.label}${field.required ? " *" : ""}`} wide><NativeSelect value={mapping[field.key]} onChange={(event) => onMapping({ ...mapping, [field.key]: event.target.value })}><NativeSelectOption value="">不导入</NativeSelectOption>{value.data.headers.map((header) => <NativeSelectOption key={header} value={header}>{header}</NativeSelectOption>)}</NativeSelect></Field>)}</div>}
    <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button onClick={onImport} disabled={busy || !mapping.headline}>{busy ? "正在导入" : "确认导入"}</Button></DialogFooter></DialogContent></Dialog>;
}

function CopyEntryDialog({ entry, onClose, onSave, busy }: { entry: CreativeCopyEntry | "new" | null; onClose: () => void; onSave: (value: CreativeCopyEntryInput) => void; busy: boolean }) {
  const source = entry && entry !== "new" ? copyEntryToInput(entry) : EMPTY_COPY;
  const key = entry && entry !== "new" ? `${entry.id}:${entry.version}` : String(entry);
  const [loadedKey, setLoadedKey] = useState(key);
  const [draft, setDraft] = useState(source);
  if (key !== loadedKey) { setLoadedKey(key); setDraft(source); }
  const set = <K extends keyof CreativeCopyEntryInput>(field: K, value: CreativeCopyEntryInput[K]) => setDraft({ ...draft, [field]: value });
  return <Dialog open={entry !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-3xl"><DialogHeader><DialogTitle>{entry === "new" ? "添加文案" : "编辑文案"}</DialogTitle><DialogDescription>保存会创建可追溯的新版本，不覆盖已运行 Issue 的快照。</DialogDescription></DialogHeader>
    <div className="grid max-h-[60vh] grid-cols-1 gap-4 overflow-y-auto pr-1 sm:grid-cols-2">
      <Field label="文案编号"><Input value={draft.external_key} onChange={(event) => set("external_key", event.target.value)} /></Field><Field label="状态"><NativeSelect value={draft.status} onChange={(event) => set("status", event.target.value as CreativeCopyEntryInput["status"])}><NativeSelectOption value="draft">草稿</NativeSelectOption><NativeSelectOption value="approved">已审核</NativeSelectOption><NativeSelectOption value="disabled">停用</NativeSelectOption></NativeSelect></Field>
      <Field label="主标题" wide><Input value={draft.headline} onChange={(event) => set("headline", event.target.value)} /></Field><Field label="副标题" wide><Input value={draft.subheadline} onChange={(event) => set("subheadline", event.target.value)} /></Field><Field label="卖点" wide><Textarea rows={3} value={draft.benefit} onChange={(event) => set("benefit", event.target.value)} /></Field><Field label="CTA"><Input value={draft.cta} onChange={(event) => set("cta", event.target.value)} /></Field><Field label="文案职责"><Input value={draft.copy_role} onChange={(event) => set("copy_role", event.target.value)} /></Field><Field label="市场"><Input value={draft.market} onChange={(event) => set("market", event.target.value)} /></Field><Field label="语言"><Input value={draft.locale} onChange={(event) => set("locale", event.target.value)} /></Field><Field label="条款" wide><Textarea rows={3} value={draft.legal_text} onChange={(event) => set("legal_text", event.target.value)} /></Field><Field label="标签" wide><Input value={draft.tags.join(", ")} onChange={(event) => set("tags", event.target.value.split(",").map((item) => item.trim()).filter(Boolean))} /></Field>
    </div><DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={busy || !draft.headline.trim()} onClick={() => onSave(draft)}>保存</Button></DialogFooter></DialogContent></Dialog>;
}

function FormSection({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section><div className="mb-3"><h3 className="text-sm font-semibold">{title}</h3><p className="mt-1 text-xs text-muted-foreground">{description}</p></div><div className="grid grid-cols-1 gap-4 border-t pt-4 sm:grid-cols-2">{children}</div></section>;
}

function Field({ label, children, wide = false }: { label: string; children: React.ReactElement<{ id?: string }>; wide?: boolean }) {
  const generatedId = useId();
  const controlId = children.props.id || generatedId;
  return <div className={cn("space-y-1.5", wide && "sm:col-span-2")}><Label htmlFor={controlId} className="text-xs text-muted-foreground">{label}</Label>{cloneElement(children, { id: controlId })}</div>;
}

function EmptyResource({ title, action, onAction }: { title: string; action: string; onAction: () => void }) {
  return <div className="flex min-h-80 flex-col items-center justify-center gap-3 text-sm text-muted-foreground"><FileSpreadsheet className="h-6 w-6" /><span>{title}</span><Button size="sm" onClick={onAction}>{action}</Button></div>;
}

function initialResourceConfig(kind: CreativeResourceKind): Record<string, unknown> {
  if (kind === "copy_library") return { market: "", locale: "", source_filename: "", import_mapping: {} };
  return { brand: "", market: "", locale: "", currency: "", copy_library_id: "", benefit_taxonomy: [], theme_presets: [], qr_payload: "", qr_canonical_payload: "", qr_allowed_domains: [], qr_approval_status: "pending", qr_approval_note: "", compliance_rules: "", naming_rule: "" };
}

function kindLabel(kind: CreativeResourceKind | null): string {
  if (kind === "copy_library") return "文案库";
  return "市场资源包";
}

function stringValue(value: unknown): string { return typeof value === "string" ? value : ""; }
function stringListValue(value: unknown): string { return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string").join(", ") : ""; }
function stringListMultilineValue(value: unknown): string { return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string").join("\n") : ""; }
function splitList(value: string): string[] { return [...new Set(value.split(/[\n,，;；]+/).map((item) => item.trim()).filter(Boolean))]; }
function copyStatusLabel(status: string): string { return status === "approved" ? "已审核" : status === "disabled" ? "停用" : "草稿"; }

function copyEntryToInput(entry: CreativeCopyEntry): CreativeCopyEntryInput {
  return { external_key: entry.external_key, headline: entry.headline, subheadline: entry.subheadline, benefit: entry.benefit, cta: entry.cta, legal_text: entry.legal_text, copy_role: entry.copy_role, market: entry.market, locale: entry.locale, tags: entry.tags, status: entry.status, metadata: entry.metadata };
}

function emptyCopyMapping(): Record<CopyField, string> {
  return { external_key: "", headline: "", subheadline: "", benefit: "", cta: "", legal_text: "", copy_role: "", market: "", locale: "", tags: "", status: "" };
}

type QRValidation = {
  approved_payload: string;
  validated_at: string;
  templates: { role: string; filename: string; decoded_payload: string }[];
};

function readQRValidation(value: unknown): QRValidation | null {
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  if (record.status !== "passed" || typeof record.approved_payload !== "string" || !Array.isArray(record.templates)) return null;
  const templates = record.templates.flatMap((item) => {
    if (!item || typeof item !== "object") return [];
    const template = item as Record<string, unknown>;
    if (typeof template.role !== "string" || typeof template.filename !== "string" || typeof template.decoded_payload !== "string") return [];
    return [{ role: template.role, filename: template.filename, decoded_payload: template.decoded_payload }];
  });
  if (templates.length !== 3) return null;
  return { approved_payload: record.approved_payload, validated_at: stringValue(record.validated_at), templates };
}

function formatValidationTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "已由服务端验证" : `验证于 ${date.toLocaleString("zh-CN")}`;
}

function resourceSlotLabel(role: string): string {
  return PRIME_ROLE_LABELS[role] ?? role;
}

function inferCopyMapping(headers: string[]): Record<CopyField, string> {
  const synonyms: Record<CopyField, string[]> = {
    external_key: ["id", "code", "key", "编号", "文案编号"], headline: ["headline", "title", "judul", "主标题", "标题"],
    subheadline: ["subheadline", "subtitle", "副标题"], benefit: ["benefit", "selling point", "manfaat", "卖点", "利益点"],
    cta: ["cta", "button", "action", "按钮"], legal_text: ["legal", "terms", "syarat", "条款", "合规"],
    copy_role: ["role", "type", "职责", "文案类型"], market: ["market", "country", "市场", "国家"],
    locale: ["locale", "language", "语言"], tags: ["tags", "tag", "标签"], status: ["status", "状态"],
  };
  const mapping = emptyCopyMapping();
  for (const field of COPY_FIELDS) {
    const match = headers.find((header) => synonyms[field.key].some((synonym) => header.trim().toLowerCase() === synonym.toLowerCase()));
    mapping[field.key] = match ?? "";
  }
  if (!mapping.headline && headers[0]) mapping.headline = headers[0];
  return mapping;
}

function spreadsheetRowToCopy(row: string[], headers: string[], mapping: Record<CopyField, string>, index: number): CreativeCopyEntryInput {
  const read = (field: CopyField) => { const column = headers.indexOf(mapping[field]); return column >= 0 ? row[column] ?? "" : ""; };
  const status = read("status").toLowerCase();
  return {
    external_key: read("external_key") || `row-${index + 2}`,
    headline: read("headline"), subheadline: read("subheadline"), benefit: read("benefit"), cta: read("cta"), legal_text: read("legal_text"), copy_role: read("copy_role"), market: read("market"), locale: read("locale"),
    tags: read("tags").split(/[,;|]/).map((item) => item.trim()).filter(Boolean),
    status: /approved|active|已审核|确认/.test(status) ? "approved" : /disabled|停用/.test(status) ? "disabled" : "draft",
    metadata: { spreadsheet_row: index + 2 },
  };
}
