"use client";

import { cloneElement, useEffect, useId, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  Archive,
  BarChart3,
  BookOpenText,
  Check,
  CircleStop,
  Globe2,
  Images,
  LayoutDashboard,
  Layers3,
  Plus,
  RefreshCw,
  Save,
  Settings2,
} from "lucide-react";
import { api } from "@multica/core/api";
import {
  creativeFeedbackOptions,
  creativeKeys,
  creativeOrderOptions,
  creativeOrdersOptions,
  creativeMaterialLibraryOptions,
  creativeResourcesOptions,
  creativeSourceAnalysesOptions,
  useAdoptCreativeOrderVariant,
  useCancelCreativeOrder,
} from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import {
  workspaceCapabilitiesOptions,
  workspaceCapabilityKeys,
} from "@multica/core/workspace/queries";
import { attachmentDownloadPath } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type {
  CreativeOrder,
  CreativeOrderAsset,
  CreativeOrderItem,
  CreativeOrderVariant,
  CreativeOrderWorkflowFailure,
  CreativeDeliverySize,
  CreativeMaterialCandidate,
  CreativeResource,
  CreativeResourceKind,
  CreateCreativeFeedbackResponse,
  IssueMetadata,
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
import { useT } from "../../i18n";
import { creativeAttachmentBrowserURL } from "../lib/creative-attachment-url";
import { creativeAdjustmentCanRetry, creativeAdjustmentProgress, creativeAdjustmentTarget, latestOrderAdjustmentFeedback } from "../lib/creative-adjustment-progress";
import { createDefaultPrimeTemplateSet, withDefaultPrimeTemplateSet } from "../lib/prime-template-set";
import { creativeTimeZoneLabel, formatCreativeDateTime } from "../lib/creative-time";
import { CreativeMaterialLibrary, creativeMaterialAnalysisReadiness, creativeMaterialProductionState, defaultPreAdaptationResources, latestCandidateFeedback, latestCompletedAnalyses, materialAnalysisState, type MaterialLibraryFilter } from "./creative-material-library";
import { CreativeCollectionPlans } from "./creative-collection-plans";
import { ComposableCopyLibraryEditor } from "./composable-copy-library-editor";
import { CreativeComparisonWorkspace, type CreativeAnnotationDraft } from "./creative-comparison-workspace";
import { adoptedCreativeOrderVariant, CREATIVE_DELIVERY_SIZES, CreativeOrderDeliveryCandidates, creativeOrderActionableWorkflowFailures, creativeOrderStage, creativeVariantIsInProgress, creativeVariantNeedsManualAction, type CreativeOrderStage } from "./creative-order-delivery";
import { CreativeGenerationInfoDialog } from "./creative-generation-info-dialog";
import { CreativeFeedbackDashboard } from "./creative-feedback-dashboard";
import { CreativeWorkbench } from "./creative-workbench";
import { MarketResourceFiles } from "./market-resource-files";

type CreativeStudioTab = "home" | "materials" | "orders" | "resources" | "feedback";

function creativeStudioTab(searchParams: URLSearchParams): CreativeStudioTab {
  if (searchParams.get("order")) return "orders";
  const tab = searchParams.get("tab");
  if (tab === "orders" || tab === "materials" || tab === "resources" || tab === "feedback") return tab;
  if (tab === "discovery") return "materials";
  if (tab === "copy" || tab === "market") return "resources";
  return "home";
}

function creativeMaterialLibraryFilter(searchParams: URLSearchParams): MaterialLibraryFilter {
  const filter = searchParams.get("materialFilter");
  return filter === "analyze" || filter === "generated" || filter === "rejected" || filter === "all"
    ? filter
    : "available";
}

function creativeResourceSection(searchParams: URLSearchParams): "market" | "copy" {
  return searchParams.get("resource") === "copy" || searchParams.get("tab") === "copy" ? "copy" : "market";
}

export function creativeStudioPath(
  pathname: string,
  searchParams: URLSearchParams,
  tab: CreativeStudioTab,
  orderId = "",
  materialFilter: MaterialLibraryFilter = "available",
  materialRunId = "",
  resourceSection: "market" | "copy" = "market",
): string {
  const next = new URLSearchParams(searchParams);
  next.delete("tab");
  next.delete("order");
  next.delete("materialFilter");
  next.delete("run");
  next.delete("resource");
  if (tab !== "home") next.set("tab", tab);
  if (tab === "orders" && orderId) next.set("order", orderId);
  if (tab === "materials" && materialFilter !== "available") next.set("materialFilter", materialFilter);
  if (tab === "materials" && materialRunId) next.set("run", materialRunId);
  if (tab === "resources" && resourceSection === "copy") next.set("resource", "copy");
  const query = next.toString();
  return query ? `${pathname}?${query}` : pathname;
}

export function CreativeStudioPage() {
  const wsId = useWorkspaceId();
  const { t } = useT("settings");
  const capabilities = useQuery(workspaceCapabilitiesOptions(wsId));
  const enabled = capabilities.data?.items.some(
    (item) => item.key === workspaceCapabilityKeys.creativeFactory && item.enabled,
  ) === true;

  if (capabilities.isPending) {
    return <div className="flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground" />;
  }
  if (!enabled) {
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PageHeader className="min-w-0 px-5">
          <div className="flex items-center gap-2">
            <Settings2 className="h-4 w-4 text-muted-foreground" />
            <h1 className="text-sm font-medium">{t(($) => $.features.creative_factory_title)}</h1>
          </div>
        </PageHeader>
        <div className="flex flex-1 items-center justify-center p-6">
          <div className="max-w-sm space-y-2 text-center">
            <h2 className="text-sm font-semibold">{t(($) => $.features.creative_factory_disabled_title)}</h2>
            <p className="text-sm text-muted-foreground">{t(($) => $.features.creative_factory_disabled_description)}</p>
          </div>
        </div>
      </div>
    );
  }
  return <CreativeStudioContent />;
}

function CreativeStudioContent() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const navigation = useNavigation();
  const workspacePaths = useWorkspacePaths();
  const routeOrderId = navigation.searchParams.get("order") ?? "";
  const routeMaterialRunId = navigation.searchParams.get("run") ?? "";
  const returnIssueId = navigation.searchParams.get("fromIssue") ?? "";
  const routeTab = creativeStudioTab(navigation.searchParams);
  const resourceSection = creativeResourceSection(navigation.searchParams);
  const materialFilter = creativeMaterialLibraryFilter(navigation.searchParams);
  const [tab, setTab] = useState<CreativeStudioTab>(routeTab);
  const [selectedOrderId, setSelectedOrderId] = useState(routeOrderId);
  const [trackedCollectionAutopilotRunId, setTrackedCollectionAutopilotRunId] = useState("");
  const [createKind, setCreateKind] = useState<CreativeResourceKind | null>(null);
  const resources = useQuery(creativeResourcesOptions(wsId));
  const orders = useQuery(creativeOrdersOptions(wsId));
  const materials = useQuery(creativeMaterialLibraryOptions(wsId));
  const materialAnalyses = useQuery(creativeSourceAnalysesOptions(wsId));
  const candidateFeedback = useQuery(creativeFeedbackOptions(wsId, "candidate"));
  const allResources = useMemo(() => resources.data?.resources ?? [], [resources.data?.resources]);
  const { marketPack, copyLibrary } = useMemo(
    () => defaultPreAdaptationResources(allResources),
    [allResources],
  );
  const materialProductionStateById = useMemo(() => {
    const decisions = latestCandidateFeedback(candidateFeedback.data?.events ?? []);
    const generatedCandidateIds = new Set((orders.data?.orders ?? []).flatMap((order) => order.items.map((item) => item.candidate_id)));
    const completedAnalyses = latestCompletedAnalyses(materialAnalyses.data?.analyses ?? []);
    return new Map((materials.data?.candidates ?? []).map((candidate) => {
      const analysisState = materialAnalysisState(candidate, materialAnalyses.data?.analyses ?? []);
      const analysisReadiness = creativeMaterialAnalysisReadiness(completedAnalyses.get(candidate.id), analysisState, marketPack, copyLibrary);
      return [candidate.id, creativeMaterialProductionState(candidate, analysisState, decisions.get(candidate.id)?.decision, generatedCandidateIds.has(candidate.id), analysisReadiness)];
    }));
  }, [candidateFeedback.data?.events, copyLibrary, marketPack, materialAnalyses.data?.analyses, materials.data?.candidates, orders.data?.orders]);
  const workbenchExcludedCandidateIds = useMemo(() => {
    return (materials.data?.candidates ?? [])
      .filter((candidate) => materialProductionStateById.get(candidate.id)?.status !== "available")
      .map((candidate) => candidate.id);
  }, [materialProductionStateById, materials.data?.candidates]);

  useEffect(() => {
    setTab(routeTab);
    setSelectedOrderId(routeOrderId);
  }, [routeOrderId, routeTab]);

  const navigateStudio = (nextTab: CreativeStudioTab, orderId = "", mode: "push" | "replace" = "replace", nextMaterialFilter: MaterialLibraryFilter = "available", nextMaterialRunId = "", nextResourceSection: "market" | "copy" = "market") => {
    setTab(nextTab);
    setSelectedOrderId(nextTab === "orders" ? orderId : "");
    const path = creativeStudioPath(navigation.pathname, navigation.searchParams, nextTab, orderId, nextMaterialFilter, nextMaterialRunId, nextResourceSection);
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
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <PageHeader className="min-w-0 justify-between px-5">
        <div className="flex min-w-0 items-center gap-2">
          <Settings2 className="h-4 w-4 text-emerald-700" />
          <h1 className="text-sm font-medium">创意工厂</h1>
          <span className="hidden text-xs text-muted-foreground md:inline">选素材、验收成图、维护市场规则</span>
        </div>
      </PageHeader>

      <Tabs value={tab} onValueChange={changeTab} className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="min-w-0 border-b px-5 py-2">
          <TabsList className="h-auto min-h-8 w-full max-w-full min-w-0 flex-wrap justify-start gap-1 overflow-visible">
            <TabsTrigger className="flex-none" value="home"><LayoutDashboard className="h-3.5 w-3.5" />工作台</TabsTrigger>
            <TabsTrigger className="flex-none" value="materials"><Images className="h-3.5 w-3.5" />素材库</TabsTrigger>
            <TabsTrigger className="flex-none" value="orders"><Layers3 className="h-3.5 w-3.5" />创意订单</TabsTrigger>
            <TabsTrigger className="flex-none" value="resources"><Globe2 className="h-3.5 w-3.5" />品牌与市场规则</TabsTrigger>
            <TabsTrigger className="flex-none" value="feedback"><BarChart3 className="h-3.5 w-3.5" />数据反馈</TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="home" className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4">
          <div className="min-w-0 space-y-4">
            <CreativeWorkbench candidates={materials.data?.candidates ?? []} orders={orders.data?.orders ?? []} excludedCandidateIds={workbenchExcludedCandidateIds} onOpenMaterialLibrary={(filter = "available") => navigateStudio("materials", "", "push", filter)} onOpenOrder={openOrder} />
            <div className="mx-auto w-full min-w-0 max-w-[1440px]"><CreativeCollectionPlans
              trackedAutopilotRunId={trackedCollectionAutopilotRunId}
              onTrackedAutopilotRunIdChange={setTrackedCollectionAutopilotRunId}
              onOpenMaterialLibrary={(crawlRunId) => navigateStudio("materials", "", "push", "all", crawlRunId)}
            /></div>
          </div>
        </TabsContent>
        <TabsContent value="materials" className="min-h-0 min-w-0 flex-1 overflow-y-auto p-5">
          <CreativeDiscoveryWorkspace
            filter={materialFilter}
            runId={routeMaterialRunId}
            onFilterChange={(nextFilter) => navigateStudio("materials", "", "replace", nextFilter, routeMaterialRunId)}
            onRunChange={(nextRunId) => navigateStudio("materials", "", "replace", nextRunId ? "all" : "available", nextRunId)}
            onOpenCopyLibrary={() => navigateStudio("resources", "", "push", "available", "", "copy")}
            onOrderCreated={openOrder}
          />
        </TabsContent>
        <TabsContent value="orders" className="min-h-0 min-w-0 flex-1 overflow-y-auto p-5"><CreativeOrdersWorkspace selectedOrderId={selectedOrderId} onSelectOrder={openOrder} onBack={closeOrder} backLabel={returnIssueId ? "返回 issue" : "返回订单列表"} /></TabsContent>
        <TabsContent value="resources" className="min-h-0 min-w-0 flex-1 overflow-hidden p-5">
          <CreativeResourcesWorkspace initialSection={resourceSection} resources={allResources} onCreate={setCreateKind} onArchive={(id) => archiveResource.mutate(id)} />
        </TabsContent>
        <TabsContent value="feedback" className="min-h-0 min-w-0 flex-1 overflow-y-auto p-5">
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

function CreativeDiscoveryWorkspace({ filter, runId, onFilterChange, onRunChange, onOrderCreated, onOpenCopyLibrary }: { filter: MaterialLibraryFilter; runId: string; onFilterChange: (filter: MaterialLibraryFilter) => void; onRunChange: (runId: string) => void; onOrderCreated: (orderId: string) => void; onOpenCopyLibrary: () => void }) {
  return <div className="mx-auto w-full min-w-0 max-w-[1440px]"><CreativeMaterialLibrary filter={filter} runId={runId} onFilterChange={onFilterChange} onRunChange={onRunChange} onOpenCopyLibrary={onOpenCopyLibrary} onOrderCreated={onOrderCreated} /></div>;
}

function CreativeOrdersWorkspace({ selectedOrderId, onSelectOrder, onBack, backLabel }: { selectedOrderId: string; onSelectOrder: (orderId: string) => void; onBack: () => void; backLabel: string }) {
  const wsId = useWorkspaceId();
  const orders = useQuery(creativeOrdersOptions(wsId));
  const materials = useQuery(creativeMaterialLibraryOptions(wsId));
  const creativeOrders = orders.data?.orders ?? [];
  if (selectedOrderId) return <CreativeOrderDetail orderId={selectedOrderId} onBack={onBack} onBrowseOrders={() => onSelectOrder("")} backLabel={backLabel} />;
  const candidatesById = new Map((materials.data?.candidates ?? []).map((candidate) => [candidate.id, candidate]));
  return <div className="mx-auto w-full min-w-0 max-w-[1440px] space-y-4">
    <div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3"><div><h2 className="text-base font-semibold">创意订单</h2><p className="mt-1 text-sm text-muted-foreground">从生成到验收、采用和下载都在订单内完成</p></div><Badge variant="outline">{creativeOrders.length} 个订单</Badge></div>
    <div className="divide-y border-y">{creativeOrders.map((creativeOrder) => {
      const stage = creativeOrderStage(creativeOrder);
      const listState = creativeOrderListState(stage);
      const summary = creativeOrderListSummary(creativeOrder);
      const sources = creativeOrderSourceSummaries(creativeOrder, candidatesById);
      return <div key={creativeOrder.id} className="grid min-h-24 gap-3 bg-background px-4 py-3 transition-colors hover:bg-muted/20 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
        <button type="button" className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => onSelectOrder(creativeOrder.id)}>
          <CreativeOrderSourceThumbs sources={sources} />
          <span className="min-w-0">
            <span className="block truncate text-sm font-semibold">{creativeOrderSourceTitle(sources, creativeOrder)}</span>
            <span className="mt-1 block truncate text-xs text-muted-foreground">{summary} · 更新于 {formatCreativeDateTime(creativeOrder.updated_at)}（{creativeTimeZoneLabel()}）</span>
          </span>
        </button>
        <div className="flex flex-wrap items-center gap-2 lg:justify-end"><Badge variant={listState.badgeVariant}>{listState.label}</Badge><Button size="sm" onClick={() => onSelectOrder(creativeOrder.id)}>{listState.action}<ArrowRight className="h-4 w-4" /></Button></div>
      </div>;
    })}</div>
    {!orders.isLoading && creativeOrders.length === 0 && <div className="flex min-h-64 items-center justify-center border border-dashed text-sm text-muted-foreground">暂无创意订单。请先在素材库选择素材并开始生成。</div>}
  </div>;
}

type CreativeOrderListState = {
  label: "生成中" | "待验收" | "已采用" | "已结束";
  action: "查看进度" | "去验收" | "查看交付" | "查看记录";
  badgeVariant: "default" | "secondary" | "outline";
};

function creativeOrderListState(stage: CreativeOrderStage): CreativeOrderListState {
  if (stage.key === "cancelled") return { label: "已结束", action: "查看记录", badgeVariant: "outline" };
  if (stage.key === "delivered") return { label: "已采用", action: "查看交付", badgeVariant: "default" };
  if (stage.key === "generating" || stage.key === "preparing") return { label: "生成中", action: "查看进度", badgeVariant: "outline" };
  return { label: "待验收", action: "去验收", badgeVariant: "default" };
}

function creativeOrderListSummary(order: CreativeOrder): string {
  const variants = order.items.flatMap((item) => item.variants);
  const generated = variants.reduce((count, variant) => count + variant.assets.filter((asset) => asset.revision === variant.revision && asset.stage === "generated" && asset.status === "completed" && Boolean(asset.attachment_id)).length, 0);
  const expected = variants.length * CREATIVE_DELIVERY_SIZES.length;
  return `已选 ${order.items.length} 张素材 · 已生成 ${generated}/${expected} 张成图`;
}

type CreativeOrderSourceSummary = {
  id: string;
  label: string;
  url: string;
};

function creativeOrderSourceSummaries(order: CreativeOrder, candidatesById: Map<string, CreativeMaterialCandidate>): CreativeOrderSourceSummary[] {
  return order.items.map((item, index) => {
    const candidate = candidatesById.get(item.candidate_id);
    return {
      id: item.candidate_id || item.id || String(index),
      label: creativeOrderSourceLabel(candidate, item, index),
      url: resolvePublicFileUrl(candidate?.archived_url || candidate?.poster_url || candidate?.preview_url || "") ?? "",
    };
  });
}

function creativeOrderSourceLabel(candidate: CreativeMaterialCandidate | undefined, item: CreativeOrderItem, index: number): string {
  return candidate?.title || candidate?.competitor || item.candidate_id?.slice(0, 8) || `素材 ${index + 1}`;
}

function creativeOrderSourceTitle(sources: CreativeOrderSourceSummary[], order: CreativeOrder): string {
  if (sources.length === 0) return `订单 ${order.id.slice(0, 8)}`;
  const labels = [...new Set(sources.map((source) => source.label).filter(Boolean))];
  const visible = labels.slice(0, 3).join(" / ");
  if (labels.length <= 3) return visible;
  return `${visible} 等 ${labels.length} 张素材`;
}

function CreativeOrderSourceThumbs({ sources }: { sources: CreativeOrderSourceSummary[] }) {
  const visible = sources.slice(0, 3);
  if (visible.length === 0) {
    return <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-md border bg-muted/30 text-muted-foreground"><Images className="h-4 w-4" /></span>;
  }
  return <span className="flex shrink-0 items-center gap-1">
    {visible.map((source) => <span key={source.id} className="flex h-12 w-12 overflow-hidden rounded-md border bg-muted/20">
      {source.url ? <img src={source.url} alt={`来源素材 ${source.label}`} width={96} height={96} loading="lazy" className="h-full w-full object-cover" /> : <span className="flex h-full w-full items-center justify-center text-muted-foreground"><Images className="h-4 w-4" /></span>}
    </span>)}
    {sources.length > visible.length && <span className="flex h-12 min-w-12 items-center justify-center rounded-md border bg-muted/30 px-2 text-xs font-medium text-muted-foreground">+{sources.length - visible.length}</span>}
  </span>;
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
  const [cancelOpen, setCancelOpen] = useState(false);
  const comparisonRef = useRef<HTMLDivElement>(null);
  const confirmedContentRef = useRef<HTMLDivElement>(null);
  const adoptedVariantIds = new Set(data?.items.flatMap((item) => {
    const variant = adoptedCreativeOrderVariant(item);
    return variant ? [variant.id] : [];
  }) ?? []);
  const latestAdjustment = latestOrderAdjustmentFeedback(feedback.data?.events ?? [], orderId);
  const adjustedVariant = creativeAdjustmentTarget(data?.items ?? [], latestAdjustment)?.variant;
  const adjustmentVariantId = typeof latestAdjustment?.context_snapshot.variant_id === "string" ? latestAdjustment.context_snapshot.variant_id : "";
  const adjustmentSizeKey = typeof latestAdjustment?.context_snapshot.size_key === "string" ? latestAdjustment.context_snapshot.size_key : "";
  const adjustmentState = latestAdjustment ? creativeAdjustmentProgress(adjustedVariant, latestAdjustment).replace(/ · r\d+$/, "") : "";
  const defaultAsset = reviewAssets.find((asset) => adoptedVariantIds.has(asset.variant_id) && asset.size_key === "1080x1080")
    ?? reviewAssets.find((asset) => adoptedVariantIds.has(asset.variant_id))
    ?? reviewAssets.find((asset) => asset.variant_id === adjustmentVariantId && asset.size_key === adjustmentSizeKey)
    ?? reviewAssets.find((asset) => asset.status === "completed")
    ?? reviewAssets[0];
  const active = reviewAssets.find((asset) => asset.id === activeAssetId) ?? defaultAsset;
  const generationInfoAsset = assets.find((asset) => asset.id === generationInfoAssetId);
  const variantById = new Map(data?.items.flatMap((item) => item.variants.map((variant) => [variant.id, { variant, item }] as const)) ?? []);
  const actionableFailures = creativeOrderActionableWorkflowFailures(data);
  const activeVariant = active ? variantById.get(active.variant_id) : undefined;
  const generationInfoVariant = generationInfoAsset ? variantById.get(generationInfoAsset.variant_id) : undefined;
  const isDirectEdit = data?.trigger_evidence_kind === "creative_direct_edit";
  const adoptionStatus = creativeOrderAdoptionStatus(data);
  const stage = creativeOrderStage(data);
  const isCancelled = stage.key === "cancelled";
  const source = library.data?.candidates.find((candidate) => candidate.id === activeVariant?.item.candidate_id);
  const candidateLabels = new Map((library.data?.candidates ?? []).map((candidate) => [candidate.id, candidate.title || candidate.competitor || candidate.id.slice(0, 8)]));
  const generatedFor = (asset: CreativeOrderAsset) => assets.find((candidate) => candidate.status === "completed" && candidate.variant_id === asset.variant_id && candidate.size_key === asset.size_key && candidate.revision === asset.revision && candidate.stage === "generated");
  const attachmentURL = (asset?: CreativeOrderAsset) => {
    const attachment = asset ? byId.get(asset.attachment_id) : undefined;
    return creativeAttachmentBrowserURL(attachment);
  };
  const adjustmentBefore = active && active.revision > 1
    ? assets.find((candidate) => candidate.variant_id === active.variant_id && candidate.size_key === active.size_key && candidate.revision === active.revision - 1 && candidate.stage === "delivered" && candidate.status === "completed" && Boolean(candidate.attachment_id))
      ?? assets.find((candidate) => candidate.variant_id === active.variant_id && candidate.size_key === active.size_key && candidate.revision === active.revision - 1 && candidate.stage === "primed" && candidate.status === "completed" && Boolean(candidate.attachment_id))
      ?? assets.find((candidate) => candidate.variant_id === active.variant_id && candidate.size_key === active.size_key && candidate.revision === active.revision - 1 && candidate.stage === "generated" && candidate.status === "completed" && Boolean(candidate.attachment_id))
    : undefined;
  const adjustmentBeforeURL = attachmentURL(adjustmentBefore);
  const comparisonSource = adjustmentBefore && adjustmentBeforeURL
    ? { label: `调整前 · r${adjustmentBefore.revision}`, url: adjustmentBeforeURL }
    : { label: source?.title || "原始素材", url: resolvePublicFileUrl(source?.archived_url || source?.preview_url) ?? "" };
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
  const createOrderAdjustmentIssue = async (asset: CreativeOrderAsset, request: string) => {
    if (!data?.issue_id) throw new Error("该订单缺少协作记录，无法创建精准调整");
    const target = variantById.get(asset.variant_id);
    if (!target) throw new Error("当前成图缺少变体上下文");
    const squadId = creativeOrderSquadId(data);
    if (!squadId) throw new Error("该订单缺少素材小队，无法创建精准调整");
    const sizeKey = creativeOrderAdjustmentSize(asset);
    if (!sizeKey) throw new Error("当前成图尺寸不支持精准调整");
    const issueInput = {
      orderId,
      itemId: target.item.id,
      variantId: asset.variant_id,
      variantKey: target.variant.variant_key,
      assetId: asset.id,
      attachmentId: asset.attachment_id,
      sizeKey,
      sourceRevision: asset.revision,
      request,
    };
    const issue = await api.createIssue({
      title: creativeOrderAdjustmentIssueTitle(issueInput),
      description: creativeOrderAdjustmentIssueDescription(issueInput),
      parent_issue_id: data.issue_id,
      assignee_type: "squad",
      assignee_id: squadId,
      // The order endpoint owns production routing. Keeping the record in the
      // backlog prevents the generic issue path from inventing a direct-edit task.
      status: "backlog",
      metadata: creativeOrderAdjustmentIssueMetadata(issueInput),
    });
    if (!issue.id) throw new Error("调整协作记录创建失败，请重试");
    return { issue, sizeKey };
  };
  const annotation = async (asset: CreativeOrderAsset, drafts: CreativeAnnotationDraft[]) => {
    if (!data?.issue_id || drafts.length === 0 || drafts.some((draft) => !draft.comment.trim())) return false;
    const summary = creativeAnnotationAdjustmentSummary(drafts);
    const first = drafts[0]!;
    try {
      const adjustment = await createOrderAdjustmentIssue(asset, summary);
      const sourceBase = generatedFor(asset);
      const sourceBaseURL = attachmentURL(sourceBase);
      if (!sourceBaseURL) throw new Error("当前调整缺少贴片前的无品牌底图");
      const annotationGuide = await createAnnotationGuideAttachment(sourceBaseURL, adjustment.issue.id, asset.size_key as CreativeDeliverySize, asset.revision, drafts);
      const annotations = drafts.map((draft) => ({ ...draft, scope: "size" }));
      await api.queueCreativeOrderAdjustment(orderId, { adjustment_issue_id: adjustment.issue.id, asset_id: asset.id, size_key: adjustment.sizeKey, source_revision: asset.revision, annotation_guide_attachment_id: annotationGuide.id, comment: summary, event_type: "annotation", reason_codes: [...new Set(drafts.map((draft) => assetFeedbackReason(draft.issueType)))], annotation: { id: crypto.randomUUID(), asset_id: asset.id, kind: first.kind, issue_type: first.issueType, x: first.x, y: first.y, width: first.width, height: first.height, scope: "size", comment: first.comment }, context_snapshot: { annotations, annotation_guide_attachment_id: annotationGuide.id } });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "asset", "") });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      toast.success("精准调整请求已创建");
      return true;
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法提交标注调整"); return false; }
  };
  const submitAdjustment = async () => {
    if (!active || !data?.issue_id || !adjustment.trim()) return;
    setAdjustBusy(true);
    try {
      const request = adjustment.trim();
      const created = await createOrderAdjustmentIssue(active, request);
      await api.queueCreativeOrderAdjustment(orderId, { adjustment_issue_id: created.issue.id, asset_id: active.id, size_key: created.sizeKey, source_revision: active.revision, comment: request, event_type: "decision", reason_codes: ["other"] });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "asset", "") });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      setAdjustment(""); setAdjustOpen(false); toast.success("精准调整请求已创建");
    } catch (error) { toast.error(error instanceof Error ? error.message : "无法提交调整请求"); }
    finally { setAdjustBusy(false); }
  };

  return <div className="mx-auto max-w-[1440px] space-y-4">
    <div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
      <div><div className="flex items-center gap-1"><Button size="sm" variant="ghost" onClick={onBack}><ArrowLeft className="h-4 w-4" />{backLabel}</Button>{backLabel === "返回 issue" && <Button size="sm" variant="ghost" onClick={onBrowseOrders}>全部订单</Button>}</div><h2 className="mt-2 text-base font-semibold">订单 {orderId.slice(0, 8)}</h2><p className="mt-1 text-xs text-muted-foreground">{stage.detail} · 更新于 {formatCreativeDateTime(data?.updated_at || "")}（{creativeTimeZoneLabel()}）</p></div>
      <div className="flex items-center gap-2"><Badge variant={stage.key === "review" || stage.key === "attention" || stage.key === "delivered" ? "default" : "outline"}>{stage.label}</Badge>{!isDirectEdit && <Badge variant={adoptionStatus === "已采用" ? "default" : "secondary"}>{adoptionStatus}</Badge>}{data && !["delivered", "cancelled"].includes(stage.key) && <Button size="sm" variant="outline" onClick={() => setCancelOpen(true)}><CircleStop className="h-4 w-4" />结束订单</Button>}</div>
    </div>
    {data && <CreativeOrderStatusPanel order={data} stage={stage} />}
    <CreativeOrderJourney stageKey={stage.key} />
    <div className="space-y-4">
      {!isDirectEdit && data?.items.map((item, index) => {
        const itemSource = library.data?.candidates.find((candidate) => candidate.id === item.candidate_id);
        return <CreativeOrderDeliveryCandidates
          key={item.id}
          orderId={orderId}
          item={item}
          order={data}
          source={{ label: itemSource?.title || itemSource?.competitor || "原始素材", url: resolvePublicFileUrl(itemSource?.archived_url || itemSource?.preview_url) ?? "" }}
          attachments={byId}
          adoptingVariantId={adoptVariant.isPending ? adoptVariant.variables?.variantId ?? "" : ""}
          disabled={isCancelled}
          defaultOpen={index === 0}
          showDirectionDetails={false}
          adjustment={adjustmentVariantId && adjustmentSizeKey ? { variantId: adjustmentVariantId, sizeKey: adjustmentSizeKey, status: adjustmentState } : undefined}
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
          onAssetSelect={selectReviewAsset}
          onAssetInfo={setGenerationInfoAssetId}
        />;
      })}
    </div>
    {latestAdjustment && data && <CreativeAdjustmentStatus event={latestAdjustment} variant={adjustedVariant} issueId={data.issue_id} retrying={adjustBusy} onRetry={async () => {
      const asset = assets.find((candidate) => candidate.id === latestAdjustment.subject_id);
      const adjustmentIssueId = typeof latestAdjustment.context_snapshot.adjustment_issue_id === "string" ? latestAdjustment.context_snapshot.adjustment_issue_id : "";
      const sizeKey = typeof latestAdjustment.context_snapshot.size_key === "string" ? latestAdjustment.context_snapshot.size_key : "";
      const sourceRevision = typeof latestAdjustment.context_snapshot.revision === "number" ? latestAdjustment.context_snapshot.revision : 0;
      const targetSize = creativeOrderAdjustmentSize({ size_key: sizeKey });
      const annotationGuideAttachmentID = typeof latestAdjustment.context_snapshot.annotation_guide_attachment_id === "string" ? latestAdjustment.context_snapshot.annotation_guide_attachment_id : "";
      if (!asset || !adjustmentIssueId || !targetSize || sourceRevision < 1) {
        toast.error("当前调整记录缺少可重启的目标信息");
        return;
      }
      setAdjustBusy(true);
      try {
        await api.queueCreativeOrderAdjustment(orderId, { adjustment_issue_id: adjustmentIssueId, asset_id: asset.id, size_key: targetSize, source_revision: sourceRevision, annotation_guide_attachment_id: annotationGuideAttachmentID || undefined, comment: latestAdjustment.comment, event_type: latestAdjustment.event_type === "annotation" ? "annotation" : "decision", reason_codes: latestAdjustment.reason_codes, annotation: latestAdjustment.annotation, context_snapshot: latestAdjustment.context_snapshot });
        await queryClient.invalidateQueries({ queryKey: creativeKeys.feedback(wsId, "asset", "") });
        await queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
        await queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
        toast.success("已重新启动当前尺寸调整");
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "无法重新启动调整");
      } finally {
        setAdjustBusy(false);
      }
    }} />}
    {active && attachmentURL(active) && <div ref={comparisonRef} className={cn("h-[min(78vh,860px)] min-h-[620px] scroll-mt-4 overflow-hidden border", isDirectEdit && "grid grid-rows-[auto_minmax(0,1fr)]")}>
      {isDirectEdit && <div className="flex items-center gap-2 border-b px-4 py-3"><span className="text-sm font-semibold">交付包</span><Badge variant="outline">直接改图</Badge></div>}
      <CreativeComparisonWorkspace
        source={comparisonSource}
        result={{ id: active.id, label: `${activeVariant?.variant.variant_key || "结果"} · ${active.size_key}`, finalUrl: attachmentURL(active), baseUrl: attachmentURL(generatedFor(active)), size: active.size_key, variant: activeVariant?.variant.variant_key }}
        assets={comparisonAssets}
        onAssetChange={setActiveAssetId}
        onAdjust={isCancelled ? undefined : () => setAdjustOpen(true)}
        onViewInfo={() => setGenerationInfoAssetId(active.id)}
        onDecision={(decision) => void event(active, decision)}
        onAnnotations={isCancelled ? undefined : (drafts) => annotation(active, drafts)}
        annotationScopes={["size"]}
        showDecisionActions={false}
        comparisonMode={adjustmentBefore && adjustmentBeforeURL ? "adjustment" : "source"}
      />
    </div>}
    <div ref={confirmedContentRef} className="scroll-mt-4">
      {data && !isDirectEdit && <CreativeOrderConfirmedContent order={data} candidateLabels={candidateLabels} />}
    </div>
    <CreativeGenerationInfoDialog
      open={Boolean(generationInfoAsset && generationInfoVariant)}
      onOpenChange={(open) => { if (!open) setGenerationInfoAssetId(""); }}
      item={generationInfoVariant?.item}
      variant={generationInfoVariant?.variant}
      asset={generationInfoAsset}
      imageUrl={attachmentURL(generationInfoAsset)}
    />
    {data && !order.isLoading && reviewAssets.length === 0 && <div className="flex min-h-72 items-center justify-center border border-dashed px-6 text-center text-sm text-muted-foreground">{creativeOrderWaitingMessage(actionableFailures, isCancelled)}</div>}
    <Dialog open={adjustOpen} onOpenChange={setAdjustOpen}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>精准调整当前成图</DialogTitle><DialogDescription>{activeVariant?.variant.variant_key || "当前变体"} · {active?.size_key}。本次只处理当前尺寸，协作记录会单独保存。</DialogDescription></DialogHeader><Textarea rows={5} value={adjustment} onChange={(event) => setAdjustment(event.target.value)} placeholder="说明当前成图需要改什么，以及必须保留的视觉风格、文案和布局..." /><DialogFooter><Button variant="outline" onClick={() => setAdjustOpen(false)}>取消</Button><Button disabled={adjustBusy || !adjustment.trim()} onClick={() => void submitAdjustment()}>{adjustBusy ? "正在提交" : "提交精准调整"}</Button></DialogFooter></DialogContent></Dialog>
    <AlertDialog open={cancelOpen} onOpenChange={(open) => { if (!cancelOrder.isPending) setCancelOpen(open); }}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>结束这个创意订单？</AlertDialogTitle><AlertDialogDescription>仍在运行的生成任务会停止。已有成图、失败原因和协作记录会保留，但订单不再占用工作台待验收列表，也不能继续采用或调整。</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={cancelOrder.isPending}>继续保留</AlertDialogCancel><AlertDialogAction className="bg-destructive text-white hover:bg-destructive/90" disabled={cancelOrder.isPending} onClick={() => cancelOrder.mutate(undefined, { onSuccess: () => { setCancelOpen(false); toast.success("订单已结束"); }, onError: (error) => toast.error(error instanceof Error ? error.message : "无法结束订单") })}>{cancelOrder.isPending ? "正在结束" : "结束订单"}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}

function CreativeAdjustmentStatus({ event, variant, issueId, onRetry, retrying = false }: { event: CreateCreativeFeedbackResponse; variant?: CreativeOrderVariant; issueId: string; onRetry: () => Promise<void>; retrying?: boolean }) {
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const variantKey = variant?.variant_key || String(event.context_snapshot.variant_id || "").slice(0, 8);
  const sizeKey = typeof event.context_snapshot.size_key === "string" ? event.context_snapshot.size_key : "";
  const adjustmentIssueId = typeof event.context_snapshot.adjustment_issue_id === "string" ? event.context_snapshot.adjustment_issue_id : "";
  const collaborationIssueId = adjustmentIssueId || issueId;
  const canRetry = creativeAdjustmentCanRetry(variant, event);
  return <section className="flex flex-wrap items-center gap-3 border-y bg-amber-50/60 px-4 py-3 dark:bg-amber-950/10" role="status" data-testid="creative-adjustment-status">
    <RefreshCw className={cn("h-4 w-4 text-amber-700", variant && !["completed", "action_required", "failed"].includes(variant.status) && "animate-spin")} />
    <div className="min-w-0 flex-1"><p className="text-sm font-medium">{creativeAdjustmentProgress(variant, event)}</p><p className="mt-0.5 truncate text-xs text-muted-foreground">{variantKey}{sizeKey ? ` · ${sizeKey}` : ""} · {event.comment || "已记录调整要求"}</p></div>
    {canRetry && <Button size="sm" disabled={retrying} onClick={() => void onRetry()}>{retrying ? "正在启动" : "重新启动调整"}</Button>}
    <Button size="sm" variant="outline" disabled={!collaborationIssueId} onClick={() => navigation.push(paths.issueDetail(collaborationIssueId))}>查看协作记录</Button>
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

function CreativeOrderStatusPanel({
  order,
  stage,
}: {
  order: CreativeOrder;
  stage: CreativeOrderStage;
}) {
  const stats = creativeOrderRuntimeStats(order);
  const actionableFailures = creativeOrderActionableWorkflowFailures(order);
  const hasFailures = actionableFailures.length > 0;
  const title = creativeOrderStatusTitle(stage, stats, hasFailures);
  const detail = creativeOrderStatusDetail(stage, stats, hasFailures);

  return <section className="border bg-background" aria-label="订单现状">
    <div className="grid gap-0 md:grid-cols-[minmax(0,1fr)_auto]">
      <div className="min-w-0 px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={stage.key === "review" || stage.key === "attention" || stage.key === "delivered" ? "default" : "outline"}>{stage.label}</Badge>
          <h3 className="min-w-0 truncate text-sm font-semibold">{title}</h3>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      </div>
      <div className="grid grid-cols-2 border-t text-xs md:w-[420px] md:grid-cols-4 md:border-l md:border-t-0">
        <OrderStat label="素材" value={`${stage.totalItems}`} />
        <OrderStat label="可验收" value={`${stage.readyVariants}`} />
        <OrderStat label="运行中" value={`${stats.runningVariants}`} />
        <OrderStat label="提醒" value={`${stats.blockedVariants + actionableFailures.length}`} />
      </div>
    </div>
  </section>;
}

function OrderStat({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0 border-l px-3 py-3 first:border-l-0 md:first:border-l">
    <span className="block text-base font-semibold tabular-nums">{value}</span>
    <span className="mt-0.5 block text-muted-foreground">{label}</span>
  </div>;
}

function creativeOrderRuntimeStats(order: CreativeOrder): { runningVariants: number; blockedVariants: number; previewAssets: number; expectedPreviewAssets: number } {
  const variants = order.items.flatMap((item) => item.variants);
  return {
    runningVariants: variants.filter(creativeVariantIsInProgress).length,
    blockedVariants: variants.filter(creativeVariantNeedsManualAction).length,
    previewAssets: variants.reduce((count, variant) => count + variant.assets.filter((asset) => asset.status === "completed" && asset.attachment_id).length, 0),
    expectedPreviewAssets: variants.length * CREATIVE_DELIVERY_SIZES.length,
  };
}

function creativeOrderStatusTitle(stage: CreativeOrderStage, stats: ReturnType<typeof creativeOrderRuntimeStats>, hasFailures: boolean): string {
  if (stage.key === "delivered") return "交付完成，可以下载最终采用方案";
  if (stage.key === "cancelled") return "订单已结束，过程记录仍保留";
  if (stage.key === "review" && hasFailures) return "已有可验收方案，另有系统提醒";
  if (stage.key === "review") return "已有方案可验收";
  if (stage.key === "attention") return "等待人工验收，系统提醒仅作参考";
  if (stage.key === "generating") return "后台正在生成方案";
  if (stats.expectedPreviewAssets > 0) return "订单已创建，等待首批成图";
  return "订单正在准备";
}

function creativeOrderStatusDetail(stage: CreativeOrderStage, stats: ReturnType<typeof creativeOrderRuntimeStats>, hasFailures: boolean): string {
  if (stage.key === "review" && hasFailures) return "可以先验收已完成方案；系统提醒只说明后台还有步骤未补齐。";
  if (stage.key === "review") return "先比较可用方案，采用后会生成交付包。";
  if (stage.key === "attention") return "不用手动重试流程；可查看可用图、标注调整或结束订单。";
  if (stage.key === "generating") return `${stats.previewAssets}/${Math.max(stats.expectedPreviewAssets, 1)} 张过程图已就绪，页面会自动刷新。`;
  return stage.detail;
}

type ConfirmedReplacement = {
  id: string;
  location: string;
  role: string;
  sourceText: string;
  finalText: string;
  sourceKind: string;
  status: string;
  note: string;
};

type ConfirmedNumericLayout = {
  id: string;
  location: string;
  instruction: string;
};

type ConfirmedOrderSummary = {
  headline: string;
  rate: string;
  amount: string;
  cta: string;
  numeric: string;
};

type ConfirmedOrderItem = {
  id: string;
  label: string;
  replacements: ConfirmedReplacement[];
  numericLayouts: ConfirmedNumericLayout[];
  repaymentPlans: { label: string; value: string }[];
  summary: ConfirmedOrderSummary;
  prompt: string;
};

function CreativeOrderConfirmedContent({ order, candidateLabels }: { order: CreativeOrder; candidateLabels: Map<string, string> }) {
  const items = order.items.map((item, index) => confirmedOrderItem(item, index, candidateLabels.get(item.candidate_id)));
  if (items.length === 0) return null;
  const replacementCount = items.reduce((count, item) => count + item.replacements.length, 0);
  const numericCount = items.reduce((count, item) => count + item.numericLayouts.length + item.repaymentPlans.length, 0);
  const manualCount = items.reduce((count, item) => count + item.replacements.filter((entry) => entry.sourceKind === "manual").length, 0);
  const calculationCount = items.reduce((count, item) => count + item.replacements.filter((entry) => entry.sourceKind === "calculation").length, 0);

  return <details className="group border bg-background" aria-label="冻结明细">
    <summary className="flex cursor-pointer list-none flex-wrap items-start justify-between gap-3 px-4 py-3 marker:content-none">
      <div className="min-w-0">
        <div className="flex items-center gap-2"><BookOpenText className="h-4 w-4 text-emerald-700" /><h3 className="text-sm font-semibold">冻结明细</h3></div>
        <p className="mt-1 text-xs text-muted-foreground">逐区文案、数值口径和完整生成指令；排查问题时展开查看。</p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Badge variant="outline">{items.length} 张素材</Badge>
        <Badge variant="outline">{replacementCount} 个文案</Badge>
        {numericCount > 0 && <Badge variant="outline">{numericCount} 个数值</Badge>}
        {manualCount > 0 && <Badge variant="secondary">{manualCount} 个人工改写</Badge>}
        {calculationCount > 0 && <Badge variant="secondary">{calculationCount} 个公式计算</Badge>}
      </div>
    </summary>
    <div className="grid gap-3 border-t p-4">
      {items.map((item, index) => <section key={item.id} className="border bg-muted/10">
        <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-3">
          <h4 className="min-w-0 truncate text-sm font-semibold">素材 {index + 1} · {item.label}</h4>
          <div className="flex flex-wrap gap-1.5 text-xs">
            <Badge variant="outline">{item.replacements.length} 个文案</Badge>
            {item.numericLayouts.length + item.repaymentPlans.length > 0 && <Badge variant="outline">{item.numericLayouts.length + item.repaymentPlans.length} 个数值</Badge>}
          </div>
        </div>
        <div className="grid gap-3 border-t bg-background px-3 py-3 md:grid-cols-2 xl:grid-cols-4">
          <ConfirmedSummaryValue label="主标题" value={item.summary.headline} />
          <ConfirmedSummaryValue label="利率/金额" value={[item.summary.rate, item.summary.amount].filter(Boolean).join("；")} />
          <ConfirmedSummaryValue label="CTA" value={item.summary.cta} />
          <ConfirmedSummaryValue label="数值口径" value={item.summary.numeric} />
        </div>
        <details className="group border-t bg-background text-sm">
          <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-3 py-2.5 text-xs font-medium text-muted-foreground marker:content-none">
            <span>查看逐区明细和生成指令</span>
            <span>{item.replacements.length} 个文案，{item.numericLayouts.length + item.repaymentPlans.length} 个数值</span>
          </summary>
          {item.replacements.length > 0 ? <div className="divide-y border-t">
            {item.replacements.map((entry, entryIndex) => <div key={entry.id || `${item.id}-${entryIndex}`} className="grid gap-3 px-3 py-3 lg:grid-cols-[minmax(150px,0.72fr)_minmax(180px,0.9fr)_minmax(220px,1.35fr)_auto] lg:items-start">
              <div className="min-w-0"><p className="truncate text-xs text-muted-foreground">区域</p><p className="mt-1 break-words font-medium">{friendlyCopyLocation(entry.location, entryIndex)}</p></div>
              <div className="min-w-0"><p className="text-xs text-muted-foreground">原图文字</p><p className="mt-1 whitespace-pre-wrap break-words">{entry.sourceText || "未识别清晰文字"}</p></div>
              <div className="min-w-0"><p className="text-xs text-muted-foreground">最终替换文字</p><p className="mt-1 whitespace-pre-wrap break-words font-medium">{entry.finalText || "留空，移除原文"}</p>{entry.note && <p className="mt-1 whitespace-pre-wrap break-words text-xs text-muted-foreground">{entry.note}</p>}</div>
              <ConfirmedSourceBadge entry={entry} />
            </div>)}
          </div> : <p className="border-t px-3 py-4 text-sm text-muted-foreground">这张素材没有记录逐区替换文案。</p>}
          {(item.numericLayouts.length > 0 || item.repaymentPlans.length > 0) && <div className="border-t bg-muted/10 px-3 py-3">
            <p className="text-xs font-medium text-muted-foreground">数值版式</p>
            <div className="mt-2 space-y-2 text-sm">
              {item.numericLayouts.map((layout) => <div key={layout.id} className="grid gap-1 sm:grid-cols-[180px_minmax(0,1fr)]"><span className="text-muted-foreground">{friendlyCopyLocation(layout.location, 0)}</span><span className="whitespace-pre-wrap break-words">{layout.instruction || "沿用冻结版式"}</span></div>)}
              {item.repaymentPlans.map((plan, planIndex) => <div key={`${item.id}-repayment-${planIndex}-${plan.label}`} className="grid gap-1 sm:grid-cols-[180px_minmax(0,1fr)]"><span className="text-muted-foreground">{plan.label}</span><span className="whitespace-pre-wrap break-words">{plan.value}</span></div>)}
            </div>
          </div>}
          {item.prompt && <details className="border-t px-3 py-2.5 text-xs text-muted-foreground">
            <summary className="w-fit cursor-pointer select-none">生成方向详情</summary>
            <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-words border bg-muted/20 p-3 font-sans text-xs leading-5">{item.prompt}</pre>
          </details>}
        </details>
      </section>)}
    </div>
  </details>;
}

function ConfirmedSummaryValue({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0">
    <p className="text-xs text-muted-foreground">{label}</p>
    <p className={cn("mt-1 line-clamp-2 break-words text-sm", value ? "font-medium" : "text-muted-foreground")}>{value || "未单独记录"}</p>
  </div>;
}

function ConfirmedSourceBadge({ entry }: { entry: ConfirmedReplacement }) {
  const empty = entry.status === "missing" || !entry.finalText;
  return <div className="flex lg:justify-end">
    <Badge variant={empty ? "outline" : entry.sourceKind === "manual" || entry.sourceKind === "calculation" ? "secondary" : "outline"}>
      {empty ? "留空" : confirmedSourceKindLabel(entry.sourceKind)}
    </Badge>
  </div>;
}

function confirmedOrderItem(item: CreativeOrderItem, index: number, label?: string): ConfirmedOrderItem {
  const snapshot = recordValue(item.copy_snapshot);
  const preAdaptation = recordValue(snapshot.pre_adaptation);
  const visualDirection = recordValue(snapshot.visual_direction);
  const replacements = arrayValue(preAdaptation.text_replacements).map(confirmedReplacement).filter((entry): entry is ConfirmedReplacement => entry !== null);
  const numericLayouts = arrayValue(preAdaptation.numeric_layouts).map(confirmedNumericLayout).filter((entry): entry is ConfirmedNumericLayout => entry !== null);
  const repaymentPlans = confirmedRepaymentPlans(preAdaptation.repayment_plan_selections);
  return {
    id: item.id || item.candidate_id || String(index),
    label: label || item.candidate_id.slice(0, 8) || `素材 ${index + 1}`,
    replacements: replacements.length > 0 ? replacements : fallbackSnapshotReplacements(snapshot),
    numericLayouts,
    repaymentPlans,
    summary: confirmedOrderSummary(replacements.length > 0 ? replacements : fallbackSnapshotReplacements(snapshot), numericLayouts, repaymentPlans),
    prompt: [
      trimmedStringValue(visualDirection.theme) ? `主题：${trimmedStringValue(visualDirection.theme)}` : "",
      stringArrayValue(visualDirection.style_tags).length > 0 ? `风格：${stringArrayValue(visualDirection.style_tags).join("、")}` : "",
      stringArrayValue(visualDirection.must_preserve).length > 0 ? `保留：${stringArrayValue(visualDirection.must_preserve).join("、")}` : "",
      stringArrayValue(visualDirection.avoid).length > 0 ? `避免：${stringArrayValue(visualDirection.avoid).join("、")}` : "",
    ].filter(Boolean).join("\n") || item.direction.trim(),
  };
}

function confirmedReplacement(value: unknown): ConfirmedReplacement | null {
  const entry = recordValue(value);
  const id = trimmedStringValue(entry.block_id);
  const location = trimmedStringValue(entry.location);
  const role = trimmedStringValue(entry.role);
  const sourceText = trimmedStringValue(entry.source_text);
  const finalText = trimmedStringValue(entry.replacement_text);
  if (!id && !location && !sourceText && !finalText) return null;
  return {
    id,
    location,
    role,
    sourceText,
    finalText,
    sourceKind: trimmedStringValue(entry.source_kind),
    status: trimmedStringValue(entry.status),
    note: trimmedStringValue(entry.note),
  };
}

function confirmedNumericLayout(value: unknown): ConfirmedNumericLayout | null {
  const entry = recordValue(value);
  const id = trimmedStringValue(entry.id);
  const location = trimmedStringValue(entry.location);
  const instruction = trimmedStringValue(entry.render_instruction);
  if (!id && !location && !instruction) return null;
  return { id, location, instruction };
}

function confirmedRepaymentPlans(value: unknown): { label: string; value: string }[] {
  return arrayValue(value).flatMap((entry) => {
    const selection = recordValue(entry);
    const values = recordValue(selection.values);
    const principal = trimmedStringValue(values.principal);
    const tenor = trimmedStringValue(values.tenor);
    const monthlyInstallment = trimmedStringValue(values.monthly_installment);
    const totalInterest = trimmedStringValue(values.total_interest);
    const totalRepayment = trimmedStringValue(values.total_repayment);
    const label = [principal, tenor].filter(Boolean).join(" / ");
    const facts = [
      monthlyInstallment ? `月还 ${monthlyInstallment}` : "",
      totalInterest ? `总利息 ${totalInterest}` : "",
      totalRepayment ? `总还款 ${totalRepayment}` : "",
    ].filter(Boolean).join("；");
    return label && facts ? [{ label, value: facts }] : [];
  });
}

function confirmedOrderSummary(
  replacements: ConfirmedReplacement[],
  numericLayouts: ConfirmedNumericLayout[],
  repaymentPlans: { label: string; value: string }[],
): ConfirmedOrderSummary {
  const headlineEntry = replacements.find((entry) => entry.finalText && /(headline|title|top center|main|主标题|标题)/i.test(`${entry.role} ${entry.location}`));
  const headline = headlineEntry?.finalText ?? "";
  const rate = firstReplacementText(replacements, (entry) => entry !== headlineEntry && entry.finalText.includes("%"))
    || firstReplacementText(replacements, (entry) => entry !== headlineEntry && /rate|interest|利率/.test(`${entry.location} ${entry.sourceText} ${entry.finalText}`.toLowerCase()));
  const amount = firstReplacementText(replacements, (entry) => entry !== headlineEntry && /rp|amount|principal|repayment|pinjaman|pembayaran|金额|还款/.test(`${entry.location} ${entry.sourceText} ${entry.finalText}`.toLowerCase()))
    || confirmedAmountFromNumeric(numericLayouts, repaymentPlans);
  const cta = firstReplacementText(replacements, (entry) => /(cta|button|action|按钮|行动)/i.test(`${entry.role} ${entry.location}`));
  const numeric = [
    numericLayouts.length > 0 ? `${numericLayouts.length} 个数值区域已冻结` : "",
    repaymentPlans.length > 0 ? `${repaymentPlans.length} 条还款计划已冻结` : "",
  ].filter(Boolean).join("；");
  return { headline, rate, amount, cta, numeric };
}

function firstReplacementText(replacements: ConfirmedReplacement[], predicate: (entry: ConfirmedReplacement) => boolean): string {
  return replacements.find((entry) => entry.finalText && predicate(entry))?.finalText ?? "";
}

function confirmedAmountFromNumeric(numericLayouts: ConfirmedNumericLayout[], repaymentPlans: { label: string; value: string }[]): string {
  const instructions = numericLayouts.map((layout) => layout.instruction).join(" ");
  const totalRepayment = instructions.match(/(?:total[-\s]?repayment|total pembayaran)[^R]*(Rp[\d.]+)/i)?.[1];
  if (totalRepayment) return `总还款 ${totalRepayment}`;
  const firstRepaymentTotal = repaymentPlans.map((plan) => plan.value.match(/总还款\s+(Rp[\d.]+)/)?.[1]).find(Boolean);
  if (firstRepaymentTotal) return `总还款 ${firstRepaymentTotal}`;
  const firstAmount = instructions.match(/Rp[\d.]+/)?.[0];
  return firstAmount ?? "";
}

function fallbackSnapshotReplacements(snapshot: Record<string, unknown>): ConfirmedReplacement[] {
  const fields = [
    ["headline", "主标题"],
    ["subheadline", "副标题"],
    ["benefit", "利益点"],
    ["supporting", "补充信息"],
    ["cta", "行动文案"],
    ["legal_text", "合规文案"],
  ] as const;
  return fields.flatMap(([key, label]) => {
    const value = trimmedStringValue(snapshot[key]);
    return value ? [{ id: key, location: label, role: key, sourceText: "", finalText: value, sourceKind: "snapshot", status: "ready", note: "" }] : [];
  });
}

function friendlyCopyLocation(location: string, index: number): string {
  const lower = location.toLowerCase();
  if (/top center|headline|主标题/.test(lower)) return "顶部主标题";
  if (/upper-right.*rate.*below|rate card below|利率.*数值/.test(lower)) return "右上利率卡数值";
  if (/upper-right.*rate|rate card|利率卡/.test(lower)) return "右上利率卡";
  if (/bottom.*benefit.*left/.test(lower)) return "底部利益点左侧";
  if (/bottom.*benefit.*center/.test(lower)) return "底部利益点中部";
  if (/bottom.*benefit.*right/.test(lower)) return "底部利益点右侧";
  if (/bottom.*button|green button|cta|行动/.test(lower)) return "底部 CTA 按钮";
  if (/donut|total|pembayaran|总还款/.test(lower)) return "中部总还款卡";
  if (/repayment summary|right-side|还款摘要/.test(lower)) return "右侧还款摘要";
  if (/table|four-column|还款表/.test(lower)) return "下方还款表";
  if (/principal|pinjaman|本金/.test(lower)) return "借款金额";
  if (/tenor|periode|期限/.test(lower)) return "期限";
  return location || `文案区域 ${index + 1}`;
}

function confirmedSourceKindLabel(kind: string): string {
  if (kind === "manual") return "人工改写";
  if (kind === "recommendation") return "本次推荐";
  if (kind === "calculation") return "公式计算";
  if (kind === "snapshot") return "已冻结";
  return "已审核文案";
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function stringArrayValue(value: unknown): string[] {
  return arrayValue(value).filter((entry): entry is string => typeof entry === "string" && Boolean(entry.trim())).map((entry) => entry.trim());
}

function trimmedStringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

async function createAnnotationGuideAttachment(
  sourceURL: string,
  issueId: string,
  sizeKey: CreativeDeliverySize,
  sourceRevision: number,
  drafts: CreativeAnnotationDraft[],
): Promise<{ id: string }> {
  const response = await fetch(sourceURL, { credentials: "include" });
  if (!response.ok) throw new Error(`无法读取贴片前底图：${response.status}`);
  const sourceBlob = await response.blob();
  const objectURL = URL.createObjectURL(sourceBlob);
  try {
    const image = await loadGuideImage(objectURL);
    const canvas = document.createElement("canvas");
    canvas.width = image.naturalWidth;
    canvas.height = image.naturalHeight;
    const context = canvas.getContext("2d");
    if (!context || canvas.width < 1 || canvas.height < 1) throw new Error("无法创建标注引导图画布");
    context.drawImage(image, 0, 0);
    const lineWidth = Math.max(4, Math.round(Math.min(canvas.width, canvas.height) * 0.004));
    const labelRadius = Math.max(14, Math.round(Math.min(canvas.width, canvas.height) * 0.018));
    context.lineWidth = lineWidth;
    context.font = `700 ${Math.max(18, labelRadius)}px sans-serif`;
    context.textAlign = "center";
    context.textBaseline = "middle";
    drafts.forEach((draft, index) => {
      const x = clampNormalized(draft.x) * canvas.width;
      const y = clampNormalized(draft.y) * canvas.height;
      const width = clampNormalized(draft.width) * canvas.width;
      const height = clampNormalized(draft.height) * canvas.height;
      context.strokeStyle = "#e11d48";
      context.fillStyle = "#e11d48";
      if (draft.kind === "rect" && width > 0 && height > 0) {
        context.strokeRect(x, y, width, height);
      } else {
        context.beginPath();
        context.arc(x, y, labelRadius * 0.8, 0, Math.PI * 2);
        context.stroke();
      }
      const labelX = Math.min(Math.max(labelRadius, x), canvas.width - labelRadius);
      const labelY = Math.min(Math.max(labelRadius, y), canvas.height - labelRadius);
      context.beginPath();
      context.arc(labelX, labelY, labelRadius, 0, Math.PI * 2);
      context.fill();
      context.fillStyle = "#ffffff";
      context.fillText(String(index + 1), labelX, labelY + 1);
    });
    const guideBlob = await canvasBlob(canvas);
    const guide = await api.uploadFile(new File([guideBlob], `annotation-guide-${sizeKey}-r${sourceRevision}.png`, { type: "image/png" }), { issueId });
    if (!guide.id) throw new Error("标注引导图上传没有返回附件");
    return guide;
  } finally {
    URL.revokeObjectURL(objectURL);
  }
}

function loadGuideImage(sourceURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("无法读取贴片前底图像素"));
    image.src = sourceURL;
  });
}

function canvasBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error("无法生成标注引导图")), "image/png");
  });
}

function clampNormalized(value: number): number {
  return Math.min(1, Math.max(0, Number.isFinite(value) ? value : 0));
}

type CreativeOrderAdjustmentIssueInput = {
  orderId: string;
  itemId: string;
  variantId: string;
  variantKey: string;
  assetId: string;
  attachmentId: string;
  sizeKey: CreativeDeliverySize;
  sourceRevision: number;
  request: string;
};

export function creativeOrderSquadId(order: Pick<CreativeOrder, "input_snapshot">): string {
  const squadSnapshot = recordValue(recordValue(order.input_snapshot).squad_snapshot);
  return trimmedStringValue(squadSnapshot.squad_id);
}

export function creativeOrderAdjustmentSize(asset: Pick<CreativeOrderAsset, "size_key">): CreativeDeliverySize | "" {
  return CREATIVE_DELIVERY_SIZES.includes(asset.size_key as CreativeDeliverySize) ? asset.size_key as CreativeDeliverySize : "";
}

export function creativeAnnotationAdjustmentSummary(drafts: CreativeAnnotationDraft[]): string {
  return drafts.map((draft, index) => `标注 ${index + 1}（当前尺寸）：${draft.comment.trim()}`).join("\n");
}

export function creativeOrderAdjustmentIssueTitle(input: Pick<CreativeOrderAdjustmentIssueInput, "variantKey" | "sizeKey" | "sourceRevision">): string {
  return `${input.variantKey || "当前方案"} / ${creativeOrderSizeLabel(input.sizeKey)} 精准调整 · R${input.sourceRevision + 1}`;
}

export function creativeOrderAdjustmentIssueMetadata(input: CreativeOrderAdjustmentIssueInput): IssueMetadata {
  return {
    workflow: "creative_adjustment",
    creative_adjustment_source: "creative_order",
    creative_order_id: input.orderId,
    creative_order_item_id: input.itemId,
    creative_variant_id: input.variantId,
    creative_variant_key: input.variantKey,
    creative_asset_id: input.assetId,
    creative_attachment_id: input.attachmentId,
    creative_scope: "size",
    creative_size: input.sizeKey,
    creative_source_revision: input.sourceRevision,
    creative_revision: input.sourceRevision + 1,
  };
}

export function creativeOrderAdjustmentIssueDescription(input: CreativeOrderAdjustmentIssueInput): string {
  const sizeLabel = creativeOrderSizeLabel(input.sizeKey);
  const target = `订单 \`${input.orderId.slice(0, 8)}\` · \`${input.variantKey || "当前方案"}\` · ${sizeLabel} \`${input.sizeKey}\` · \`r${input.sourceRevision}\` · 成图 \`${input.assetId.slice(0, 8)}\``;
  const imageURL = input.attachmentId ? attachmentDownloadPath(input.attachmentId) : "";
  const preview = imageURL ? `\n\n[![目标成图：${input.variantKey} ${sizeLabel} r${input.sourceRevision}](${imageURL})](${imageURL})` : "";
  return `订单画布提交精准调整。\n\n**目标成图：** ${target}${preview}\n\n${input.request.trim()}\n\n<!-- creative-workflow-context\ncreative_order_id: ${input.orderId}\ncreative_order_item_id: ${input.itemId}\nvariant_id: ${input.variantId}\nvariant_key: ${input.variantKey}\nasset_id: ${input.assetId}\nattachment_id: ${input.attachmentId}\nsize_key: ${input.sizeKey}\nsource_revision: ${input.sourceRevision}\ntarget_revision: ${input.sourceRevision + 1}\ninstruction: Process only this size from the current approved image context. Preserve the approved style, layout family, business facts, and other assets unless the user explicitly marked them in this adjustment issue.\n-->`;
}

function creativeOrderSizeLabel(sizeKey: string): string {
  return sizeKey === "1080x1080" ? "方形" : sizeKey === "1200x628" ? "横版" : sizeKey === "800x1000" ? "竖版" : sizeKey;
}

export function creativeOrderWaitingMessage(failures: CreativeOrderWorkflowFailure[], closed = false): string {
  if (closed) return "订单已结束，没有生成可验收的成图。过程记录仍保留在上方。";
  return failures.length > 0
    ? "目前没有可验收成图；可结束订单后重新发起。"
    : "正在等待首批成图。各变体完成后会立即出现在这里。";
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

function CreativeResourcesWorkspace({ resources, onCreate, onArchive, initialSection }: {
  resources: CreativeResource[];
  onCreate: (kind: CreativeResourceKind) => void;
  onArchive: (id: string) => void;
  initialSection: "market" | "copy";
}) {
  const [section, setSection] = useState<"market" | "copy">(initialSection);
  useEffect(() => { setSection(initialSection); }, [initialSection]);
  const marketPacks = resources.filter((resource) => resource.kind === "market_pack");
  const copyLibraries = resources.filter((resource) => resource.kind === "copy_library");

  return <div className="mx-auto flex h-full w-full max-w-[1440px] min-h-0 min-w-0 flex-col">
    <div className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
      <div><h2 className="text-base font-semibold">品牌与市场规则</h2><p className="mt-1 text-sm text-muted-foreground">按市场维护品牌组件、合规内容和可用文案</p></div>
      <div className="inline-flex border" aria-label="资源类型">
        <button type="button" onClick={() => setSection("market")} className={cn("inline-flex h-8 items-center gap-1.5 px-3 text-xs", section === "market" && "bg-foreground text-background")}><Globe2 className="h-3.5 w-3.5" />市场规则</button>
        <button type="button" onClick={() => setSection("copy")} className={cn("inline-flex h-8 items-center gap-1.5 border-l px-3 text-xs", section === "copy" && "bg-foreground text-background")}><BookOpenText className="h-3.5 w-3.5" />文案库</button>
      </div>
    </div>
    <div className="mt-4 min-h-0 min-w-0 flex-1">
      {section === "market" ? <ResourceEditor resources={marketPacks} copyLibraries={copyLibraries} onCreate={() => onCreate("market_pack")} onArchive={onArchive} /> : <ComposableCopyLibraryEditor resources={copyLibraries} onCreate={() => onCreate("copy_library")} onArchive={onArchive} />}
    </div>
  </div>;
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
  const [draft, setDraft] = useState<Record<string, unknown>>(() => withDefaultPrimeTemplateSet(active?.config ?? {}));
  const draftKey = active ? `${active.id}:${active.version}` : "";
  useEffect(() => {
    setDraft(withDefaultPrimeTemplateSet(active?.config ?? {}));
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
      <ResourceList title="市场配置" resources={resources} activeId={active?.id ?? ""} onSelect={setActiveId} onCreate={resources.length === 0 ? onCreate : undefined} />
      <div className="min-w-0 overflow-y-auto bg-background">
        {!active ? <EmptyResource title="还没有市场配置" action="创建市场配置" onAction={onCreate} /> : (
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
  const templateValidation = readPrimeTemplateValidation(resource.config.prime_template_set_validation);
  return (
    <div className="mx-auto max-w-6xl px-5 py-6">
      <Tabs defaultValue="identity" className="space-y-7">
        <TabsList className="h-9 w-full justify-start overflow-x-auto border bg-muted/20 p-0.5">
          <TabsTrigger value="identity">基础信息</TabsTrigger>
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
        <TabsContent value="brand" className="mt-0 space-y-8">
          <MarketResourceFiles resource={resource} value={value} />
        </TabsContent>
        <TabsContent value="delivery" className="mt-0 space-y-8">
          <FormSection title="交付规则" description="控制成图合规要求和交付文件名称。">
            <Field label="合规规则" wide><Textarea rows={7} value={stringValue(value.compliance_rules)} onChange={(event) => set("compliance_rules", event.target.value)} /></Field>
            <Field label="文件命名" wide><Input value={stringValue(value.naming_rule)} placeholder="Month_P_Brand_Country_Date_Type_Designer_Size.png" onChange={(event) => set("naming_rule", event.target.value)} /></Field>
          </FormSection>
          <section>
            <div className="mb-3"><h3 className="text-sm font-semibold">发布校验</h3></div>
            {templateValidation ? <div className="border-y">{Object.entries(templateValidation.templates).flatMap(([size, templates]) => templates.map((template) => <div key={`${size}-${template.source_role}`} className="grid gap-1 border-b px-4 py-3 text-xs last:border-b-0 sm:grid-cols-[116px_minmax(0,1fr)_auto] sm:items-center"><span className="font-medium">{size}</span><span className="truncate text-muted-foreground">{template.filename}</span><span className="text-emerald-700 dark:text-emerald-400">模板已验证</span></div>))}</div> : <div className="border-y px-4 py-3 text-sm text-muted-foreground">保存并发布后，系统会验证每张完整模板的文件、尺寸和布局映射。</div>}
          </section>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function ResourceList({ title, resources, activeId, onSelect, onCreate }: {
  title: string;
  resources: CreativeResource[];
  activeId: string;
  onSelect: (id: string) => void;
  onCreate?: () => void;
}) {
  return (
    <aside className="min-h-0 overflow-y-auto border-r bg-muted/15">
      <div className="sticky top-0 flex items-center justify-between border-b bg-background px-3 py-2.5">
        <span className="text-sm font-semibold">{title}</span>
        {onCreate && <Button size="icon-sm" variant="ghost" title={`创建${title}`} aria-label={`创建${title}`} onClick={onCreate}><Plus className="h-4 w-4" /></Button>}
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

function FormSection({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section><div className="mb-3"><h3 className="text-sm font-semibold">{title}</h3><p className="mt-1 text-xs text-muted-foreground">{description}</p></div><div className="grid grid-cols-1 gap-4 border-t pt-4 sm:grid-cols-2">{children}</div></section>;
}

function Field({ label, children, wide = false }: { label: string; children: React.ReactElement<{ id?: string }>; wide?: boolean }) {
  const generatedId = useId();
  const controlId = children.props.id || generatedId;
  return <div className={cn("space-y-1.5", wide && "sm:col-span-2")}><Label htmlFor={controlId} className="text-xs text-muted-foreground">{label}</Label>{cloneElement(children, { id: controlId })}</div>;
}

function EmptyResource({ title, action, onAction }: { title: string; action: string; onAction: () => void }) {
  return <div className="flex min-h-80 flex-col items-center justify-center gap-3 text-sm text-muted-foreground"><span>{title}</span><Button size="sm" onClick={onAction}>{action}</Button></div>;
}

function initialResourceConfig(kind: CreativeResourceKind): Record<string, unknown> {
  if (kind === "copy_library") return {
    schema_version: 4,
    market: "Indonesia",
    locale: "id-ID",
    source: { name: "", url: "", sync_status: "pending", note: "" },
    fragments: [],
    recipes: [],
    repayment_plan: {
      labels: {
        principal: "Jumlah Pinjaman",
        tenor: "Periode Cicilan",
        monthly_installment: "Cicilan per Bulan",
        total_interest: "Total Bunga",
        total_repayment: "Total Pembayaran",
      },
      entries: [],
    },
  };
  return { brand: "", market: "", locale: "", currency: "", copy_library_id: "", pre_adaptation_default: true, prime_template_set: createDefaultPrimeTemplateSet(), compliance_rules: "", naming_rule: "" };
}

function kindLabel(kind: CreativeResourceKind | null): string {
  if (kind === "copy_library") return "文案库";
  return "市场配置";
}

function stringValue(value: unknown): string { return typeof value === "string" ? value : ""; }

type PrimeTemplateValidation = { filename: string; source_role: string };

function readPrimeTemplateValidation(value: unknown): { templates: Record<string, PrimeTemplateValidation[]> } | null {
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  if (record.status !== "passed" || !record.templates || typeof record.templates !== "object") return null;
  const templates: Record<string, PrimeTemplateValidation[]> = {};
  for (const [size, items] of Object.entries(record.templates as Record<string, unknown>)) {
    if (!Array.isArray(items)) continue;
    const parsed: PrimeTemplateValidation[] = items.flatMap((item) => {
      if (!item || typeof item !== "object") return [];
      const template = item as Record<string, unknown>;
      return typeof template.filename === "string" && typeof template.source_role === "string" ? [{ filename: template.filename, source_role: template.source_role }] : [];
    });
    if (parsed.length) templates[size] = parsed;
  }
  return Object.keys(templates).length ? { templates } : null;
}
