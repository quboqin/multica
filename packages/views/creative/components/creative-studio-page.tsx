"use client";

import { cloneElement, useEffect, useId, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowLeft,
  Archive,
  BarChart3,
  BookOpenText,
  Check,
	CheckCircle2,
  CircleStop,
  FileSpreadsheet,
  Globe2,
  Images,
  LayoutDashboard,
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
  useAdoptCreativeOrderVariant,
  useCancelCreativeOrder,
} from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { attachmentDownloadPath } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type {
  CreativeCopyEntry,
  CreativeCopyEntryInput,
  CreativeOrder,
  CreativeOrderAsset,
  CreativeOrderVariant,
  CreativeOrderWorkflowFailure,
  CreativeResource,
  CreativeResourceKind,
  CreateCreativeFeedbackResponse,
} from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
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
import { creativeAttachmentBrowserURL } from "../lib/creative-attachment-url";
import { creativeAdjustmentProgress, creativeAdjustmentTarget, latestOrderAdjustmentFeedback } from "../lib/creative-adjustment-progress";
import { dynamicQRPolicy, withDynamicQRPayload } from "../lib/market-pack-qr";
import { creativeTimeZoneLabel, formatCreativeDateTime } from "../lib/creative-time";
import { CreativeMaterialLibrary } from "./creative-material-library";
import { CreativeCollectionPlans } from "./creative-collection-plans";
import { ComposableCopyLibraryEditor } from "./composable-copy-library-editor";
import { CreativeComparisonWorkspace, type CreativeAnnotationDraft } from "./creative-comparison-workspace";
import { adoptedCreativeOrderVariant, CreativeOrderDeliveryCandidates, creativeOrderStage } from "./creative-order-delivery";
import { CreativeGenerationInfoDialog } from "./creative-generation-info-dialog";
import { CreativeFeedbackDashboard } from "./creative-feedback-dashboard";
import { CreativeWorkbench } from "./creative-workbench";
import { MarketResourceFiles } from "./market-resource-files";
import { createDefaultPrimeComposition, PrimeCompositionEditor, readPrimeComposition } from "./prime-composition-editor";

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
  prime_logo: "品牌 Logo",
  prime_store_badges: "应用商店标识",
  prime_qr: "静态二维码",
  prime_afpi: "AFPI 标识",
  prime_pindai_legal: "Pindai Legal",
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

type CreativeStudioTab = "home" | "materials" | "orders" | "resources" | "feedback";

function creativeStudioTab(searchParams: URLSearchParams): CreativeStudioTab {
  if (searchParams.get("order")) return "orders";
  const tab = searchParams.get("tab");
  if (tab === "orders" || tab === "materials" || tab === "resources" || tab === "feedback") return tab;
  if (tab === "discovery") return "materials";
  if (tab === "copy" || tab === "market") return "resources";
  return "home";
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
  if (tab !== "home") next.set("tab", tab);
  if (tab === "orders" && orderId) next.set("order", orderId);
  const query = next.toString();
  return query ? `${pathname}?${query}` : pathname;
}

export function CreativeStudioPage() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const navigation = useNavigation();
  const workspacePaths = useWorkspacePaths();
  const routeOrderId = navigation.searchParams.get("order") ?? "";
  const returnIssueId = navigation.searchParams.get("fromIssue") ?? "";
  const routeTab = creativeStudioTab(navigation.searchParams);
  const [tab, setTab] = useState<CreativeStudioTab>(routeTab);
  const [selectedOrderId, setSelectedOrderId] = useState(routeOrderId);
  const [createKind, setCreateKind] = useState<CreativeResourceKind | null>(null);
  const resources = useQuery(creativeResourcesOptions(wsId));
  const orders = useQuery(creativeOrdersOptions(wsId));
  const materials = useQuery(creativeMaterialLibraryOptions(wsId));
  const allResources = resources.data?.resources ?? [];
  const attentionCount = (orders.data?.orders ?? []).filter((order) => {
    const stage = creativeOrderStage(order);
    return stage.key === "review" || stage.key === "attention";
  }).length;

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
    const nextTab: CreativeStudioTab = value === "materials" || value === "orders" || value === "resources" || value === "feedback" ? value : "home";
    navigateStudio(nextTab, nextTab === "orders" ? selectedOrderId : "");
  };

  const openOrder = (orderId: string) => navigateStudio("orders", orderId, "push");
  const closeOrder = () => {
    if (returnIssueId) navigation.push(workspacePaths.issueDetail(returnIssueId));
    else navigateStudio("orders");
  };

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
          <span className="hidden text-xs text-muted-foreground md:inline">选素材、验收成图、维护市场规则</span>
        </div>
        <Badge variant={attentionCount > 0 ? "default" : "outline"}>{attentionCount > 0 ? `${attentionCount} 个待处理` : "暂无待处理"}</Badge>
      </PageHeader>

      <Tabs value={tab} onValueChange={changeTab} className="flex min-h-0 flex-1 flex-col">
        <div className="border-b px-5 py-2">
          <TabsList className="h-8 overflow-x-auto">
            <TabsTrigger value="home"><LayoutDashboard className="h-3.5 w-3.5" />工作台</TabsTrigger>
            <TabsTrigger value="materials"><Images className="h-3.5 w-3.5" />素材库</TabsTrigger>
            <TabsTrigger value="orders"><Layers3 className="h-3.5 w-3.5" />创意订单</TabsTrigger>
            <TabsTrigger value="resources"><Globe2 className="h-3.5 w-3.5" />品牌与市场规则</TabsTrigger>
            <TabsTrigger value="feedback"><BarChart3 className="h-3.5 w-3.5" />数据反馈</TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="home" className="min-h-0 flex-1 overflow-y-auto p-5">
          <CreativeWorkbench candidates={materials.data?.candidates ?? []} orders={orders.data?.orders ?? []} onOpenMaterialLibrary={() => navigateStudio("materials")} onOpenOrder={openOrder} />
        </TabsContent>
        <TabsContent value="materials" className="min-h-0 flex-1 overflow-y-auto p-5">
          <CreativeDiscoveryWorkspace onOrderCreated={openOrder} />
        </TabsContent>
        <TabsContent value="orders" className="min-h-0 flex-1 overflow-y-auto p-5"><CreativeOrdersWorkspace selectedOrderId={selectedOrderId} onSelectOrder={openOrder} onBack={closeOrder} backLabel={returnIssueId ? "返回 issue" : "返回订单列表"} /></TabsContent>
        <TabsContent value="resources" className="min-h-0 flex-1 overflow-hidden p-5">
          <CreativeResourcesWorkspace resources={allResources} onCreate={setCreateKind} onArchive={(id) => archiveResource.mutate(id)} />
        </TabsContent>
        <TabsContent value="feedback" className="min-h-0 flex-1 overflow-y-auto p-5">
          <CreativeFeedbackDashboard />
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
  return <div className="mx-auto max-w-[1440px] space-y-4"><div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3"><div><h2 className="text-base font-semibold">素材库</h2><p className="mt-1 text-sm text-muted-foreground">查看采集结果，筛选参考素材并发起创意订单</p></div><Badge variant="outline">稳定原图优先</Badge></div><CreativeCollectionPlans /><CreativeMaterialLibrary onOrderCreated={onOrderCreated} /></div>;
}

function CreativeOrdersWorkspace({ selectedOrderId, onSelectOrder, onBack, backLabel }: { selectedOrderId: string; onSelectOrder: (orderId: string) => void; onBack: () => void; backLabel: string }) {
  const wsId = useWorkspaceId();
  const navigation = useNavigation();
  const workspacePaths = useWorkspacePaths();
  const orders = useQuery(creativeOrdersOptions(wsId));
  const creativeOrders = orders.data?.orders ?? [];
  if (selectedOrderId) return <CreativeOrderDetail orderId={selectedOrderId} onBack={onBack} onBrowseOrders={() => onSelectOrder("")} backLabel={backLabel} />;
  const stages = creativeOrders.map((order) => creativeOrderStage(order));
  const pendingCount = stages.filter((stage) => stage.key === "review" || stage.key === "attention").length;
  return <div className="mx-auto max-w-[1440px] space-y-4">
    <div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3"><div><h2 className="text-base font-semibold">创意订单</h2><p className="mt-1 text-sm text-muted-foreground">从生成到验收、采用和下载都在订单内完成</p></div><div className="flex gap-2"><Badge variant="outline">{creativeOrders.length} 个订单</Badge>{pendingCount > 0 && <Badge>{pendingCount} 个待处理</Badge>}</div></div>
    <div className="divide-y border-y">{creativeOrders.map((creativeOrder) => {
      const stage = creativeOrderStage(creativeOrder);
      return <div key={creativeOrder.id} className="grid min-h-20 gap-3 bg-background px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
        <button type="button" className="min-w-0 text-left" onClick={() => onSelectOrder(creativeOrder.id)}><span className="block truncate text-sm font-semibold">订单 {creativeOrder.id.slice(0, 8)}</span><span className="mt-1 block text-xs text-muted-foreground">{stage.detail} · 更新于 {formatCreativeDateTime(creativeOrder.updated_at)}（{creativeTimeZoneLabel()}）</span></button>
        <div className="flex flex-wrap items-center gap-2"><Badge variant={stage.key === "attention" ? "destructive" : stage.key === "review" ? "default" : "outline"}>{stage.label}</Badge>{creativeOrder.issue_id && <Button size="sm" variant="ghost" onClick={() => navigation.push(workspacePaths.issueDetail(creativeOrder.issue_id))}>协作记录</Button>}<Button size="sm" variant="outline" onClick={() => onSelectOrder(creativeOrder.id)}>{stage.action}</Button></div>
      </div>;
    })}</div>
    {!orders.isLoading && creativeOrders.length === 0 && <div className="flex min-h-64 items-center justify-center border border-dashed text-sm text-muted-foreground">暂无创意订单。请先在素材库选择素材并开始生成。</div>}
  </div>;
}

function CreativeOrderDetail({ orderId, onBack, onBrowseOrders, backLabel }: { orderId: string; onBack: () => void; onBrowseOrders: () => void; backLabel: string }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const order = useQuery({
    ...creativeOrderOptions(wsId, orderId),
    refetchInterval: (query) => query.state.data?.items.some((item) => item.variants.some((variant) => ["queued", "running"].includes(variant.status))) ? 5000 : false,
  });
  const feedback = useQuery(creativeFeedbackOptions(wsId, "asset"));
  const library = useQuery(creativeMaterialLibraryOptions(wsId));
  const adoptVariant = useAdoptCreativeOrderVariant(wsId, orderId);
  const cancelOrder = useCancelCreativeOrder(wsId, orderId);
  const data = order.data;
  const assets = data?.items.flatMap((item) => item.variants.flatMap((variant) => variant.assets)) ?? [];
  const ids = [...new Set(assets.map((asset) => asset.attachment_id).filter(Boolean))];
  const attachments = useQuery({ queryKey: ["creative", wsId, "order-attachments", orderId, ids], queryFn: () => Promise.all(ids.map((id) => api.getAttachment(id))), enabled: ids.length > 0 });
  const byId = new Map((attachments.data ?? []).map((item) => [item.id, item]));
  const variantOrder = data?.items.flatMap((item) => item.variants.map((variant) => variant.id)) ?? [];
  const reviewAssets = selectCreativeReviewAssets(assets, variantOrder);
  const [activeAssetId, setActiveAssetId] = useState("");
  const [generationInfoAssetId, setGenerationInfoAssetId] = useState("");
  const [adjustOpen, setAdjustOpen] = useState(false);
  const [adjustment, setAdjustment] = useState("");
  const [adjustBusy, setAdjustBusy] = useState(false);
  const [retryingTaskId, setRetryingTaskId] = useState("");
	const [retryingQCVariantId, setRetryingQCVariantId] = useState("");
	const [repairingPrimeVariantId, setRepairingPrimeVariantId] = useState("");
  const [cancelOpen, setCancelOpen] = useState(false);
  const comparisonRef = useRef<HTMLDivElement>(null);
  const adoptedVariantIds = new Set(data?.items.flatMap((item) => {
    const variant = adoptedCreativeOrderVariant(item);
    return variant ? [variant.id] : [];
  }) ?? []);
  const defaultAsset = reviewAssets.find((asset) => adoptedVariantIds.has(asset.variant_id) && asset.size_key === "1080x1080")
    ?? reviewAssets.find((asset) => adoptedVariantIds.has(asset.variant_id))
    ?? reviewAssets.find((asset) => asset.status === "completed")
    ?? reviewAssets[0];
  const active = reviewAssets.find((asset) => asset.id === activeAssetId) ?? defaultAsset;
  const generationInfoAsset = assets.find((asset) => asset.id === generationInfoAssetId);
  const variantById = new Map(data?.items.flatMap((item) => item.variants.map((variant) => [variant.id, { variant, item }] as const)) ?? []);
	const qcRecoveryAvailableVariantIds = new Set(data?.items.flatMap((item) => item.variants
		.filter((variant) => variant.status === "action_required" && variant.qc_recovery_available === true && variant.qc_recovery_used !== true)
		.map((variant) => variant.id)) ?? []);
  const activeVariant = active ? variantById.get(active.variant_id) : undefined;
  const generationInfoVariant = generationInfoAsset ? variantById.get(generationInfoAsset.variant_id) : undefined;
  const latestAdjustment = latestOrderAdjustmentFeedback(feedback.data?.events ?? [], orderId);
  const adjustedVariant = creativeAdjustmentTarget(data?.items ?? [], latestAdjustment)?.variant;
  const isDirectEdit = data?.trigger_evidence_kind === "creative_direct_edit";
  const adoptionStatus = creativeOrderAdoptionStatus(data);
  const stage = creativeOrderStage(data);
  const isCancelled = stage.key === "cancelled";
  const source = library.data?.candidates.find((candidate) => candidate.id === activeVariant?.item.candidate_id);
  const generatedFor = (asset: CreativeOrderAsset) => assets.find((candidate) => candidate.status === "completed" && candidate.variant_id === asset.variant_id && candidate.size_key === asset.size_key && candidate.revision === asset.revision && candidate.stage === "generated");
  const attachmentURL = (asset?: CreativeOrderAsset) => {
    const attachment = asset ? byId.get(asset.attachment_id) : undefined;
    return creativeAttachmentBrowserURL(attachment);
  };
  const comparisonAssets = reviewAssets
    .filter((asset) => byId.has(asset.attachment_id) && (!activeVariant || variantById.get(asset.variant_id)?.item.id === activeVariant.item.id))
    .map((asset) => ({ id: asset.id, label: `${asset.stage} · ${asset.size_key}`, finalUrl: attachmentURL(asset), baseUrl: attachmentURL(generatedFor(asset)), thumbnailUrl: attachmentURL(asset), size: asset.size_key, variant: variantById.get(asset.variant_id)?.variant.variant_key || "" }));
  const selectReviewAsset = (assetId: string) => {
    setActiveAssetId(assetId);
    window.requestAnimationFrame(() => comparisonRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
  };

  const event = async (asset: CreativeOrderAsset, decision: "accepted" | "abandoned" | "downloaded") => {
    if (decision !== "downloaded") return;
    try {
      await api.createCreativeFeedback({ issue_id: data?.issue_id ?? "", subject_type: "asset", subject_id: asset.id, event_type: "viewed", decision: "", context_snapshot: { action: "download", order_id: orderId, variant_id: asset.variant_id, size_key: asset.size_key, revision: asset.revision } });
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法记录下载行为"); }
  };
  const annotation = async (asset: CreativeOrderAsset, drafts: CreativeAnnotationDraft[]) => {
    if (!data?.issue_id || !activeVariant || drafts.length === 0 || drafts.some((draft) => !draft.comment.trim())) return false;
    const summary = drafts.map((draft, index) => `标注 ${index + 1}（${draft.scope === "size" ? "当前尺寸" : draft.scope === "variant" ? "当前变体三个尺寸" : "整个订单"}）：${draft.comment.trim()}`).join("\n");
    const first = drafts[0]!;
    try {
      await api.createCreativeFeedback({ issue_id: data.issue_id, subject_type: "asset", subject_id: asset.id, event_type: "annotation", decision: "needs_revision", reason_codes: [...new Set(drafts.map((draft) => assetFeedbackReason(draft.issueType)))], comment: summary, annotation: { id: crypto.randomUUID(), asset_id: asset.id, kind: first.kind, issue_type: first.issueType, x: first.x, y: first.y, width: first.width, height: first.height, scope: first.scope, comment: first.comment }, context_snapshot: { order_id: orderId, variant_id: asset.variant_id, size_key: asset.size_key, revision: asset.revision, annotations: drafts } });
      await api.createComment(data.issue_id, creativeAdjustmentComment({ request: summary, orderId, itemId: activeVariant.item.id, variantId: asset.variant_id, variantKey: activeVariant.variant.variant_key, assetId: asset.id, attachmentId: asset.attachment_id, sizeKey: asset.size_key, revision: asset.revision }));
      await queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "asset", "") });
      toast.success("标注与调整请求已提交");
      return true;
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法提交标注调整"); return false; }
  };
  const submitAdjustment = async () => {
    if (!active || !activeVariant || !data?.issue_id || !adjustment.trim()) return;
    setAdjustBusy(true);
    try {
      await api.createCreativeFeedback({ issue_id: data.issue_id, subject_type: "asset", subject_id: active.id, event_type: "decision", decision: "needs_revision", reason_codes: ["other"], comment: adjustment.trim(), context_snapshot: { order_id: orderId, variant_id: active.variant_id, size_key: active.size_key, revision: active.revision } });
      await api.createComment(data.issue_id, creativeAdjustmentComment({
        request: adjustment,
        orderId,
        itemId: activeVariant.item.id,
        variantId: active.variant_id,
        variantKey: activeVariant.variant.variant_key,
        assetId: active.id,
        attachmentId: active.attachment_id,
        sizeKey: active.size_key,
        revision: active.revision,
      }));
      await queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "asset", "") });
      setAdjustment(""); setAdjustOpen(false); toast.success("调整请求已提交");
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法提交调整请求"); }
    finally { setAdjustBusy(false); }
  };

  const retryVariantQC = async (variantId: string) => {
    if (!variantId || retryingQCVariantId) return;
    setRetryingQCVariantId(variantId);
    try {
      await api.retryCreativeOrderVariantQC(orderId, variantId);
      await Promise.all([
        order.refetch(),
        queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) }),
      ]);
      toast.success("技术与视觉质检已重新排队，正在复用现有成图");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法重新执行双路质检");
    } finally {
      setRetryingQCVariantId("");
    }
  };

  const repairVariantPrimePackage = async (variantId: string) => {
    if (!variantId || repairingPrimeVariantId) return;
    setRepairingPrimeVariantId(variantId);
    try {
      await api.repairCreativeOrderVariantPrimePackage(orderId, variantId);
      await Promise.all([
        order.refetch(),
        queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) }),
      ]);
      toast.success("Prime 包修复已排队，完成后会重新进入双路质检");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法修复 Prime 包");
    } finally {
      setRepairingPrimeVariantId("");
    }
  };

  const retryFailure = async (failure: CreativeOrderWorkflowFailure) => {
    const retryMode = workflowFailureRetryMode(failure);
    if (!retryMode) return;
	if (retryMode === "qc_recovery") {
		await retryVariantQC(failure.subject_id);
		return;
	}
	setRetryingTaskId(failure.task_id);
    try {
      if (retryMode === "creative_recovery") {
        await api.retryCreativeOrderWorkflowFailure(orderId, failure.task_id);
      } else {
        await api.retryFailedAgentTasksBySource(
          failure.agent_id,
          failure.trigger_evidence_kind,
          failure.trigger_evidence_ref_id,
        );
      }
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

  return <div className="mx-auto max-w-[1440px] space-y-4">
    <div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
      <div><div className="flex items-center gap-1"><Button size="sm" variant="ghost" onClick={onBack}><ArrowLeft className="h-4 w-4" />{backLabel}</Button>{backLabel === "返回 issue" && <Button size="sm" variant="ghost" onClick={onBrowseOrders}>全部订单</Button>}</div><h2 className="mt-2 text-base font-semibold">订单 {orderId.slice(0, 8)}</h2><p className="mt-1 text-xs text-muted-foreground">{stage.detail} · 更新于 {formatCreativeDateTime(data?.updated_at || "")}（{creativeTimeZoneLabel()}）</p></div>
      <div className="flex items-center gap-2"><Badge variant={stage.key === "attention" ? "destructive" : stage.key === "review" || stage.key === "delivered" ? "default" : "outline"}>{stage.label}</Badge>{!isDirectEdit && <Badge variant={adoptionStatus === "已采用" ? "default" : "secondary"}>{adoptionStatus}</Badge>}{data && !["delivered", "cancelled"].includes(stage.key) && <Button size="sm" variant="outline" onClick={() => setCancelOpen(true)}><CircleStop className="h-4 w-4" />结束订单</Button>}</div>
    </div>
    <CreativeOrderJourney stageKey={stage.key} />
		{data && <CreativeOrderFailureNotice failures={data.workflow_failures ?? []} qcRecoveryAvailableVariantIds={qcRecoveryAvailableVariantIds} retryingTaskId={retryingTaskId} retryingQCVariantId={retryingQCVariantId} onRetry={(failure) => void retryFailure(failure)} compact={reviewAssets.length > 0} closed={isCancelled} />}
    {!isDirectEdit && data?.items.map((item) => {
      const itemSource = library.data?.candidates.find((candidate) => candidate.id === item.candidate_id);
      return <CreativeOrderDeliveryCandidates
        key={item.id}
        orderId={orderId}
        item={item}
        source={{ label: itemSource?.title || itemSource?.competitor || "原始素材", url: resolvePublicFileUrl(itemSource?.archived_url || itemSource?.preview_url) ?? "" }}
        attachments={byId}
        adoptingVariantId={adoptVariant.isPending ? adoptVariant.variables?.variantId ?? "" : ""}
		retryingQCVariantId={retryingQCVariantId}
		repairingPrimeVariantId={repairingPrimeVariantId}
        disabled={isCancelled}
        onAdopt={(variantId, risk) => {
          const selectedAsset = reviewAssets.find((asset) => asset.variant_id === variantId && asset.size_key === "1080x1080")
            ?? reviewAssets.find((asset) => asset.variant_id === variantId);
          adoptVariant.mutate({ itemId: item.id, variantId, qcRiskAcknowledged: risk?.acknowledged, qcRiskReason: risk?.reason }, {
            onSuccess: () => {
              if (selectedAsset) setActiveAssetId(selectedAsset.id);
              toast.success("最终采用方案已更新");
            },
            onError: (error) => toast.error(error instanceof Error ? error.message : "无法采用此变体"),
          });
        }}
		onRetryQC={(variantId) => void retryVariantQC(variantId)}
		onRepairPrime={(variantId) => void repairVariantPrimePackage(variantId)}
        onAssetSelect={selectReviewAsset}
        onAssetInfo={setGenerationInfoAssetId}
      />;
    })}
    {latestAdjustment && data && <CreativeAdjustmentStatus event={latestAdjustment} variant={adjustedVariant} issueId={data.issue_id} />}
    {active && attachmentURL(active) && <div ref={comparisonRef} className={cn("h-[min(78vh,860px)] min-h-[620px] scroll-mt-4 overflow-hidden border", isDirectEdit && "grid grid-rows-[auto_minmax(0,1fr)]")}>
      {isDirectEdit && <div className="flex items-center gap-2 border-b px-4 py-3"><span className="text-sm font-semibold">交付包</span><Badge variant="outline">直接改图</Badge></div>}
      <CreativeComparisonWorkspace
        source={{ label: source?.title || "原始素材", url: resolvePublicFileUrl(source?.archived_url || source?.preview_url) ?? "" }}
        result={{ id: active.id, label: `${activeVariant?.variant.variant_key || "结果"} · ${active.size_key}`, finalUrl: attachmentURL(active), baseUrl: attachmentURL(generatedFor(active)), size: active.size_key, variant: activeVariant?.variant.variant_key }}
        assets={comparisonAssets}
        onAssetChange={setActiveAssetId}
        onAdjust={isCancelled ? undefined : () => setAdjustOpen(true)}
        onViewInfo={() => setGenerationInfoAssetId(active.id)}
        onDecision={(decision) => void event(active, decision)}
        onAnnotations={isCancelled ? undefined : (drafts) => annotation(active, drafts)}
        showDecisionActions={false}
      />
    </div>}
    <CreativeGenerationInfoDialog
      open={Boolean(generationInfoAsset && generationInfoVariant)}
      onOpenChange={(open) => { if (!open) setGenerationInfoAssetId(""); }}
      item={generationInfoVariant?.item}
      variant={generationInfoVariant?.variant}
      asset={generationInfoAsset}
      imageUrl={attachmentURL(generationInfoAsset)}
    />
    {data && !order.isLoading && reviewAssets.length === 0 && <div className="flex min-h-72 items-center justify-center border border-dashed px-6 text-center text-sm text-muted-foreground">{creativeOrderWaitingMessage(data.workflow_failures ?? [], isCancelled)}</div>}
    <Dialog open={adjustOpen} onOpenChange={setAdjustOpen}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>调整当前成图</DialogTitle><DialogDescription>{activeVariant?.variant.variant_key || "当前变体"} · {active?.size_key}。修改会保留在订单记录中，并按所选范围重新处理。</DialogDescription></DialogHeader><Textarea rows={5} value={adjustment} onChange={(event) => setAdjustment(event.target.value)} placeholder="说明需要改什么、必须保留什么，以及只影响当前尺寸还是整个变体..." /><DialogFooter><Button variant="outline" onClick={() => setAdjustOpen(false)}>取消</Button><Button disabled={adjustBusy || !adjustment.trim()} onClick={() => void submitAdjustment()}>{adjustBusy ? "正在提交" : "提交调整"}</Button></DialogFooter></DialogContent></Dialog>
    <AlertDialog open={cancelOpen} onOpenChange={(open) => { if (!cancelOrder.isPending) setCancelOpen(open); }}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>结束这个创意订单？</AlertDialogTitle><AlertDialogDescription>仍在运行的生成任务会停止。已有成图、失败原因和协作记录会保留，但订单不再占用工作台待处理列表，也不能继续采用或调整。</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={cancelOrder.isPending}>继续保留</AlertDialogCancel><AlertDialogAction className="bg-destructive text-white hover:bg-destructive/90" disabled={cancelOrder.isPending} onClick={() => cancelOrder.mutate(undefined, { onSuccess: () => { setCancelOpen(false); toast.success("订单已结束"); }, onError: (error) => toast.error(error instanceof Error ? error.message : "无法结束订单") })}>{cancelOrder.isPending ? "正在结束" : "结束订单"}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}

function CreativeAdjustmentStatus({ event, variant, issueId }: { event: CreateCreativeFeedbackResponse; variant?: CreativeOrderVariant; issueId: string }) {
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const variantKey = variant?.variant_key || String(event.context_snapshot.variant_id || "").slice(0, 8);
  const sizeKey = typeof event.context_snapshot.size_key === "string" ? event.context_snapshot.size_key : "";
  return <section className="flex flex-wrap items-center gap-3 border-y bg-amber-50/60 px-4 py-3 dark:bg-amber-950/10" role="status" data-testid="creative-adjustment-status">
    <RefreshCw className={cn("h-4 w-4 text-amber-700", variant && !["completed", "action_required", "failed"].includes(variant.status) && "animate-spin")} />
    <div className="min-w-0 flex-1"><p className="text-sm font-medium">{creativeAdjustmentProgress(variant, event)}</p><p className="mt-0.5 truncate text-xs text-muted-foreground">{variantKey}{sizeKey ? ` · ${sizeKey}` : ""} · {event.comment || "已记录调整要求"}</p></div>
    <Button size="sm" variant="outline" onClick={() => navigation.push(paths.issueDetail(issueId))}>查看协作记录</Button>
  </section>;
}

function CreativeOrderJourney({ stageKey }: { stageKey: ReturnType<typeof creativeOrderStage>["key"] }) {
  if (stageKey === "cancelled") return <div className="flex min-h-12 items-center gap-2 border px-4 py-3 text-sm text-muted-foreground"><CircleStop className="h-4 w-4" /><span><strong className="font-medium text-foreground">订单已结束</strong> · 已有结果和过程记录仍可查看</span></div>;
  const steps = [
    { key: "preparing", label: "确认素材与方向" },
    { key: "generating", label: "生成三套方案" },
    { key: "review", label: "验收与调整" },
    { key: "delivered", label: "采用并下载" },
  ] as const;
  const currentIndex = stageKey === "delivered" ? 3 : stageKey === "review" ? 2 : stageKey === "generating" || stageKey === "attention" ? 1 : 0;
  return <ol className="grid grid-cols-2 border sm:grid-cols-4" aria-label="订单进度">
    {steps.map((step, index) => <li key={step.key} className={cn("flex min-h-12 items-center gap-2 px-3 py-2 text-xs", index > 0 && "border-l", index > 1 && "border-t sm:border-t-0", index <= currentIndex ? "text-foreground" : "text-muted-foreground")}>
      <span className={cn("flex h-5 w-5 shrink-0 items-center justify-center rounded-full border text-[10px] font-semibold", index < currentIndex && "border-foreground bg-foreground text-background", index === currentIndex && "border-foreground")}>{index < currentIndex ? <Check className="h-3 w-3" /> : index + 1}</span>
      <span className={cn(index === currentIndex && "font-semibold")}>{step.label}</span>
    </li>)}
  </ol>;
}

export function creativeAdjustmentComment(input: {
  request: string;
  orderId: string;
  itemId: string;
  variantId: string;
  variantKey: string;
  assetId: string;
  attachmentId?: string;
  sizeKey: string;
  revision: number;
}): string {
  const sizeLabel = input.sizeKey === "1080x1080" ? "方形" : input.sizeKey === "1200x628" ? "横版" : input.sizeKey === "800x1000" ? "竖版" : input.sizeKey;
  const target = `订单 \`${input.orderId.slice(0, 8)}\` · \`${input.variantKey}\` · ${sizeLabel} \`${input.sizeKey}\` · \`r${input.revision}\` · 成图 \`${input.assetId.slice(0, 8)}\``;
  const imageURL = input.attachmentId ? attachmentDownloadPath(input.attachmentId) : "";
  const preview = imageURL ? `\n\n[![目标成图：${input.variantKey} ${sizeLabel} r${input.revision}](${imageURL})](${imageURL})` : "";
  return `用户提出成图调整：\n\n**目标成图：** ${target}${preview}\n\n${input.request.trim()}\n\n<!-- creative-workflow-context\ncreative_order_id: ${input.orderId}\ncreative_order_item_id: ${input.itemId}\nvariant_id: ${input.variantId}\nvariant_key: ${input.variantKey}\nasset_id: ${input.assetId}\nsize_key: ${input.sizeKey}\nrevision: ${input.revision}\ninstruction: Process only the requested scope and preserve other approved assets.\n-->`;
}

export function CreativeOrderFailureNotice({ failures, qcRecoveryAvailableVariantIds = EMPTY_QC_RECOVERY_VARIANT_IDS, retryingTaskId, retryingQCVariantId = "", onRetry, compact = false, closed = false }: {
  failures: CreativeOrderWorkflowFailure[];
	qcRecoveryAvailableVariantIds?: ReadonlySet<string>;
  retryingTaskId: string;
	retryingQCVariantId?: string;
  onRetry: (failure: CreativeOrderWorkflowFailure) => void;
  compact?: boolean;
  closed?: boolean;
}) {
  if (failures.length === 0) return null;
	const coveredQCVariants = new Set<string>();
	const failuresList = <div className="divide-y divide-destructive/20">{failures.map((failure) => {
		const retryMode = workflowFailureRetryMode(failure);
		const qcRecoveryAvailable = retryMode !== "qc_recovery" || qcRecoveryAvailableVariantIds.has(failure.subject_id);
      const retryable = Boolean(retryMode) && qcRecoveryAvailable;
		const duplicateQCRecovery = retryMode === "qc_recovery" && (coveredQCVariants.has(failure.subject_id) || !failure.subject_id);
		if (retryMode === "qc_recovery" && failure.subject_id) coveredQCVariants.add(failure.subject_id);
      const busy = retryMode === "qc_recovery" ? retryingQCVariantId === failure.subject_id : retryingTaskId === failure.task_id;
      const message = workflowFailureMessage(failure);
      const technicalDetails = workflowFailureTechnicalDetails(failure, message);
      return <div key={failure.task_id || `${failure.workflow}:${failure.subject_id}:${failure.failed_at}`} className="px-4 py-3">
        <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="text-sm font-medium">失败步骤：{creativeWorkflowLabel(failure.workflow)}</span>{failure.item_key && <Badge variant="outline" title={failure.item_key}>对象 {failure.item_key.slice(0, 8)}</Badge>}</div><p className="mt-1 break-words text-sm text-destructive">{message}</p><p className="mt-1 break-words text-xs text-muted-foreground">范围：{failure.scope || "未提供"}{failure.failure_reason && failure.error !== failure.failure_reason ? ` · 分类：${failure.failure_reason}` : ""}</p>{technicalDetails && <details className="mt-2 text-xs text-muted-foreground"><summary className="w-fit cursor-pointer select-none">技术详情</summary><pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap border bg-background p-3 font-mono text-[11px] leading-5">{technicalDetails}</pre></details>}</div>
		<div className="mt-3">{closed ? <span className="text-xs text-muted-foreground">订单已结束，不再继续处理</span> : retryMode === "qc_recovery" && !qcRecoveryAvailable ? <span className="text-xs text-muted-foreground">双路 QC 恢复不可用，需修复 Prime 包或人工处理</span> : retryable && !duplicateQCRecovery ? <Button size="sm" variant="outline" disabled={busy || Boolean(retryingTaskId) || Boolean(retryingQCVariantId)} onClick={() => onRetry(failure)}><RefreshCw className={cn("h-4 w-4", busy && "animate-spin")} />{busy ? retryMode === "qc_recovery" ? "正在重新执行双路质检" : "正在重试失败步骤" : retryMode === "qc_recovery" ? "重新执行双路质检" : "重试失败步骤"}</Button> : duplicateQCRecovery ? <span className="text-xs text-muted-foreground">此变体的双路质检可由上方恢复操作统一重跑</span> : <span className="text-xs text-muted-foreground">该步骤已无重试次数，请结束订单后重新发起</span>}</div>
      </div>;
    })}</div>;
  if (compact) return <details className="border border-destructive/30 bg-destructive/5" role="alert">
    <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm marker:content-none"><AlertTriangle className="h-4 w-4 shrink-0 text-destructive" /><span className="font-medium">{failures.length} 个局部步骤需要处理</span><span className="text-xs text-muted-foreground">其他已完成成图不受影响</span></summary>
    <div className="border-t border-destructive/20">{failuresList}</div>
  </details>;
  return <section className="border border-destructive/40 bg-destructive/5" role="alert" aria-label="创意流程失败">
    <div className="flex gap-3 border-b border-destructive/30 px-4 py-3"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" /><div><h3 className="text-sm font-semibold">创意流程需要处理</h3><p className="mt-1 text-xs text-muted-foreground">失败步骤会单独显示，可重试步骤会从原失败位置继续。</p></div></div>
    {failuresList}
  </section>;
}

export function creativeOrderWaitingMessage(failures: CreativeOrderWorkflowFailure[], closed = false): string {
  if (closed) return "订单已结束，没有生成可验收的成图。失败记录仍保留在上方。";
  return failures.length > 0
    ? "失败步骤恢复后，首批成图会自动出现在这里。"
    : "正在等待首批成图。各变体完成后会立即出现在这里。";
}

export function workflowFailureRetryMode(failure: CreativeOrderWorkflowFailure): "creative_recovery" | "qc_recovery" | "by_source" | null {
	if (isCreativeQCWorkflow(failure.workflow)) return failure.subject_id ? "qc_recovery" : null;
  if (!failure.retryable) return null;
  if (failure.failure_reason === "agent_reported_action_required") {
    return failure.task_id ? "creative_recovery" : null;
  }
  return failure.agent_id && failure.trigger_evidence_kind && failure.trigger_evidence_ref_id ? "by_source" : null;
}

const EMPTY_QC_RECOVERY_VARIANT_IDS: ReadonlySet<string> = new Set();

function isCreativeQCWorkflow(workflow: string): boolean {
	return workflow === "creative_qc" || workflow === "creative_qc_technical" || workflow === "creative_qc_visual";
}

function workflowFailureMessage(failure: CreativeOrderWorkflowFailure): string {
  if (failure.failure_reason === "provider_rate_limited") return "生成服务暂时繁忙，当前步骤未完成。";
  if (failure.failure_reason === "codex_semantic_inactivity") {
    if (/unknown flag|exit code/i.test(failure.error)) return "智能体运行在生成开始前异常退出，当前步骤未完成。";
    return "智能体在等待时间内没有返回有效进展，当前步骤未完成。";
  }
  return failure.error || failure.failure_reason || "任务执行失败，未返回详细原因";
}

function workflowFailureTechnicalDetails(failure: CreativeOrderWorkflowFailure, message: string): string {
  const details = failure.error?.trim();
  return details && details !== message ? details : "";
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

export function selectCreativeReviewAssets(assets: CreativeOrderAsset[], variantOrder: string[] = []): CreativeOrderAsset[] {
  const selected = new Map<string, CreativeOrderAsset>();
  for (const asset of assets) {
    if (asset.status !== "completed" || !asset.attachment_id) continue;
    const key = `${asset.variant_id}:${asset.size_key}`;
    const current = selected.get(key);
    if (!current || compareCreativeAssets(asset, current) > 0) selected.set(key, asset);
  }
  const rank = new Map(variantOrder.map((variantId, index) => [variantId, index]));
  return [...selected.values()].sort((left, right) => {
    const leftRank = rank.get(left.variant_id);
    const rightRank = rank.get(right.variant_id);
    if (leftRank !== undefined || rightRank !== undefined) {
      const difference = (leftRank ?? Number.MAX_SAFE_INTEGER) - (rightRank ?? Number.MAX_SAFE_INTEGER);
      if (difference !== 0) return difference;
    }
    return left.variant_id.localeCompare(right.variant_id) || left.size_key.localeCompare(right.size_key);
  });
}

function compareCreativeAssets(left: CreativeOrderAsset, right: CreativeOrderAsset): number {
  if (left.revision !== right.revision) return left.revision - right.revision;
  const stageRank = (asset: CreativeOrderAsset) => asset.stage === "delivered" ? 3 : asset.stage === "primed" ? 2 : asset.stage === "generated" ? 1 : 0;
  const stageDifference = stageRank(left) - stageRank(right);
  if (stageDifference !== 0) return stageDifference;
  const updatedDifference = (Date.parse(left.updated_at) || 0) - (Date.parse(right.updated_at) || 0);
  return updatedDifference || left.id.localeCompare(right.id);
}

export function creativeOrderAdoptionStatus(order: CreativeOrder | undefined): "待选择" | "已采用" {
  if (!order?.items.length) return "待选择";
  return order.items.every((item) => Boolean(adoptedCreativeOrderVariant(item))) ? "已采用" : "待选择";
}

function assetFeedbackReason(issueType: string): string {
  if (issueType === "theme_drift") return "theme_mismatch";
  if (issueType === "artifact") return "broken_image";
  if (issueType === "brand_prime") return "brand_or_prime";
  if (issueType === "copy_error") return "copy_error";
  return "other";
}

function CreativeResourcesWorkspace({ resources, onCreate, onArchive }: {
  resources: CreativeResource[];
  onCreate: (kind: CreativeResourceKind) => void;
  onArchive: (id: string) => void;
}) {
  const [section, setSection] = useState<"market" | "copy">("market");
  const marketPacks = resources.filter((resource) => resource.kind === "market_pack");
  const copyLibraries = resources.filter((resource) => resource.kind === "copy_library");

  return <div className="mx-auto flex h-full max-w-[1440px] min-h-0 flex-col">
    <div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
      <div><h2 className="text-base font-semibold">品牌与市场规则</h2><p className="mt-1 text-sm text-muted-foreground">按市场维护品牌组件、合规内容和可用文案</p></div>
      <div className="inline-flex border" aria-label="资源类型">
        <button type="button" onClick={() => setSection("market")} className={cn("inline-flex h-8 items-center gap-1.5 px-3 text-xs", section === "market" && "bg-foreground text-background")}><Globe2 className="h-3.5 w-3.5" />市场规则</button>
        <button type="button" onClick={() => setSection("copy")} className={cn("inline-flex h-8 items-center gap-1.5 border-l px-3 text-xs", section === "copy" && "bg-foreground text-background")}><BookOpenText className="h-3.5 w-3.5" />文案库</button>
      </div>
    </div>
    <div className="mt-4 min-h-0 flex-1">
      {section === "market" ? <ResourceEditor resources={marketPacks} copyLibraries={copyLibraries} onCreate={() => onCreate("market_pack")} onArchive={onArchive} /> : <ComposableCopyLibraryEditor resources={copyLibraries} onCreate={() => onCreate("copy_library")} onArchive={onArchive} />}
    </div>
  </div>;
}

export function CopyLibraries({ resources, onCreate, onArchive }: {
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
  const primeComposition = readPrimeComposition(value.prime_composition);
  const displayedQRValidation = qrValidation && (!primeComposition || qrValidation.mode === primeComposition.qr_mode) ? qrValidation : null;
  return (
    <div className="mx-auto max-w-6xl px-5 py-6">
      <Tabs defaultValue="identity" className="space-y-7">
        <TabsList className="h-9 w-full justify-start overflow-x-auto border bg-muted/20 p-0.5">
          <TabsTrigger value="identity">基础信息</TabsTrigger>
          <TabsTrigger value="creative">创意规则</TabsTrigger>
          <TabsTrigger value="brand">品牌与 Prime</TabsTrigger>
          <TabsTrigger value="delivery">交付规则</TabsTrigger>
        </TabsList>
        <TabsContent value="identity" className="mt-0 space-y-8">
          <FormSection title="市场与语言" description="运行时根据这里选择文案、合规和交付规则。">
            <Field label="品牌"><Input value={stringValue(value.brand)} onChange={(event) => set("brand", event.target.value)} /></Field>
            <Field label="市场"><Input value={stringValue(value.market)} placeholder="Indonesia" onChange={(event) => set("market", event.target.value)} /></Field>
            <Field label="语言"><Input value={stringValue(value.locale)} placeholder="id-ID" onChange={(event) => set("locale", event.target.value)} /></Field>
            <Field label="币种"><Input value={stringValue(value.currency)} placeholder="IDR" onChange={(event) => set("currency", event.target.value)} /></Field>
            <Field label="绑定文案库" wide><NativeSelect value={stringValue(value.copy_library_id)} onChange={(event) => set("copy_library_id", event.target.value)}><NativeSelectOption value="">选择已发布文案库</NativeSelectOption>{copyLibraries.filter((library) => library.published_version > 0).map((library) => <NativeSelectOption key={library.id} value={library.id}>{library.name} · v{library.published_version}</NativeSelectOption>)}</NativeSelect></Field>
          </FormSection>
        </TabsContent>
        <TabsContent value="creative" className="mt-0 space-y-8">
          <FormSection title="创意理解" description="候选图按这里的市场词表识别主题与利益点；用户仍可逐图输入词表外的新值。">
            <Field label="利益点词表" wide><Textarea rows={6} value={stringListMultilineValue(value.benefit_taxonomy)} placeholder={"额度\n低利率\n费用减免\n快速放款"} onChange={(event) => set("benefit_taxonomy", splitList(event.target.value))} /></Field>
            <Field label="主题预设" wide><Textarea rows={6} value={stringListMultilineValue(value.theme_presets)} placeholder={"世界杯 / 足球赛事\n斋月\n开斋节\n发薪日"} onChange={(event) => set("theme_presets", splitList(event.target.value))} /></Field>
          </FormSection>
        </TabsContent>
        <TabsContent value="brand" className="mt-0 space-y-8">
          <MarketResourceFiles resource={resource} value={value} composition={primeComposition} onChange={onChange} />
          <PrimeCompositionEditor resource={resource} value={value.prime_composition ?? createDefaultPrimeComposition()} qrPayload={stringValue(value.qr_payload)} onQRPayloadChange={(payload) => onChange(withDynamicQRPayload(value, payload))} onChange={(composition) => set("prime_composition", composition)} />
        </TabsContent>
        <TabsContent value="delivery" className="mt-0 space-y-8">
          <FormSection title="交付规则" description="控制成图合规要求和交付文件名称。">
            <Field label="合规规则" wide><Textarea rows={7} value={stringValue(value.compliance_rules)} onChange={(event) => set("compliance_rules", event.target.value)} /></Field>
            <Field label="文件命名" wide><Input value={stringValue(value.naming_rule)} placeholder="Month_P_Brand_Country_Date_Type_Designer_Size.png" onChange={(event) => set("naming_rule", event.target.value)} /></Field>
          </FormSection>
          <QRCodeAudit value={value} mode={primeComposition?.qr_mode ?? "legacy"} />
          <section>
            <div className="mb-3 flex items-center gap-2"><QrCode className="h-4 w-4" /><h3 className="text-sm font-semibold">发布校验</h3></div>
            {displayedQRValidation ? <div className="border-y"><div className="flex flex-wrap items-center justify-between gap-3 border-b bg-emerald-50/50 px-4 py-3 dark:bg-emerald-950/15"><div className="flex min-w-0 items-center gap-2"><CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600" /><div className="min-w-0"><p className="text-sm font-medium">Prime 组件校验通过</p><p className="truncate text-xs text-muted-foreground">{displayedQRValidation.mode === "none" ? "不使用二维码" : displayedQRValidation.mode === "static" ? "二维码图片解码通过" : displayedQRValidation.approved_payload}</p></div></div><span className="text-xs text-muted-foreground">{formatValidationTime(displayedQRValidation.validated_at)}</span></div><div className="divide-y">{displayedQRValidation.templates.map((template) => <div key={template.role} className="grid gap-1 px-4 py-3 text-xs sm:grid-cols-[150px_minmax(0,1fr)_auto] sm:items-center"><span className="font-medium">{resourceSlotLabel(template.role)}</span><span className="truncate font-mono text-muted-foreground">{template.filename}</span><span className="text-emerald-700 dark:text-emerald-400">解码通过</span></div>)}</div></div> : <div className="flex gap-3 border-y bg-amber-50/50 px-4 py-3 dark:bg-amber-950/15"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" /><div><p className="text-sm font-medium">当前草稿尚未完成发布校验</p><p className="mt-1 text-xs text-muted-foreground">保存并发布时，服务端会统一检查组件来源、位置和二维码。</p></div></div>}
          </section>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function QRCodeAudit({ value, mode }: { value: Record<string, unknown>; mode: string }) {
  const policy = dynamicQRPolicy(stringValue(value.qr_payload));
  const label = mode === "none" ? "不使用二维码" : mode === "static" ? "使用二维码图片" : "生成新二维码";
  const description = mode === "none"
    ? "生成和 QC 都不会要求二维码。"
    : mode === "static"
      ? "发布时读取一次二维码图片并验证，三个尺寸共用。"
      : mode === "dynamic"
        ? policy.valid ? `${policy.hostname} · 发布时生成并验证` : "请在品牌与 Prime 中填写有效的 HTTPS 地址。"
        : "请完成二维码配置。";
  return <section>
    <div className="mb-3 flex items-center gap-2"><QrCode className="h-4 w-4" /><h3 className="text-sm font-semibold">二维码</h3></div>
    <div className="border-y">
      <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"><div><p className="text-sm font-medium">{label}</p><p className={cn("mt-1 text-xs", mode === "dynamic" && !policy.valid ? "text-amber-700" : "text-muted-foreground")}>{description}</p></div><Badge variant="outline">{mode === "dynamic" && !policy.valid ? "待完善" : "随发布校验"}</Badge></div>
      <details className="border-t bg-muted/10"><summary className="cursor-pointer px-4 py-3 text-xs font-medium">技术审计信息</summary><dl className="grid gap-3 border-t px-4 py-3 text-xs sm:grid-cols-2"><div><dt className="text-muted-foreground">工作模式</dt><dd className="mt-1 font-mono">{mode}</dd></div><div><dt className="text-muted-foreground">批准状态</dt><dd className="mt-1">{stringValue(value.qr_approval_status) || (mode === "none" || mode === "static" ? "无需人工批准" : "待发布")}</dd></div>{mode === "dynamic" && <><div className="sm:col-span-2"><dt className="text-muted-foreground">目标与标准地址</dt><dd className="mt-1 break-all font-mono">{policy.payload || "未填写"}</dd></div><div><dt className="text-muted-foreground">允许域名</dt><dd className="mt-1 font-mono">{policy.hostname || "未生成"}</dd></div><div><dt className="text-muted-foreground">批准记录</dt><dd className="mt-1">{stringValue(value.qr_approval_note) || "发布资源包时记录"}</dd></div></>}</dl></details>
    </div>
  </section>;
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
      <DialogContent><DialogHeader><DialogTitle>创建{kindLabel(kind)}</DialogTitle><DialogDescription>先创建草稿，配置完成后再发布给创意订单使用。</DialogDescription></DialogHeader>
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
  return <Dialog open={entry !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-3xl"><DialogHeader><DialogTitle>{entry === "new" ? "添加文案" : "编辑文案"}</DialogTitle><DialogDescription>保存会创建可追溯的新版本，不影响已经开始生成的订单。</DialogDescription></DialogHeader>
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
  if (kind === "copy_library") return {
    schema_version: 2,
    market: "Indonesia",
    locale: "id-ID",
    source: { name: "", url: "", sync_status: "pending", note: "" },
    fragments: [],
    recipes: [],
    product_facts: [],
    calculation_rules: [],
    recommendation_policy: { type_weight: 1000, tag_weight: 80, concise_weight: 1, default_creative_type: "num" },
  };
  return { brand: "", market: "", locale: "", currency: "", copy_library_id: "", benefit_taxonomy: [], theme_presets: [], prime_composition: null, qr_payload: "", qr_canonical_payload: "", qr_allowed_domains: [], qr_approval_status: "pending", qr_approval_note: "", compliance_rules: "", naming_rule: "" };
}

function kindLabel(kind: CreativeResourceKind | null): string {
  if (kind === "copy_library") return "文案库";
  return "市场资源包";
}

function stringValue(value: unknown): string { return typeof value === "string" ? value : ""; }
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
  mode: "none" | "static" | "dynamic" | "legacy";
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
  const mode = ["none", "static", "dynamic"].includes(String(record.mode)) ? record.mode as QRValidation["mode"] : null;
  if (!mode) return null;
  if (mode === "static" && templates.length !== 1) return null;
  if (mode !== "static" && templates.length !== 0) return null;
  return { mode, approved_payload: record.approved_payload, validated_at: stringValue(record.validated_at), templates };
}

function formatValidationTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "已由服务端验证" : `验证于 ${formatCreativeDateTime(value)}（${creativeTimeZoneLabel()}）`;
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
