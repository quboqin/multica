"use client";

import { useId, useState } from "react";
import { AlertTriangle, Check, CheckCircle2, ChevronDown, Download, ExternalLink, Eye, Image as ImageIcon, Info, PackageCheck, RefreshCw } from "lucide-react";
import { strToU8, zipSync } from "fflate";
import type { Attachment, CreativeOrder, CreativeOrderAsset, CreativeOrderDiagnosticAsset, CreativeOrderItem, CreativeOrderQCReport, CreativeOrderVariant, CreativeOrderWorkflowFailure } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";
import { creativeAttachmentBrowserURL } from "../lib/creative-attachment-url";

export const CREATIVE_DELIVERY_SIZES = ["1080x1080", "1200x628", "800x1000"] as const;

export type CreativeOrderStage = {
  key: "preparing" | "generating" | "review" | "attention" | "delivered" | "cancelled";
  label: string;
  detail: string;
  action: string;
  readyVariants: number;
  totalVariants: number;
  adoptedItems: number;
  totalItems: number;
};

export function creativeOrderStatusLabel(status: string | undefined): string {
  switch (status) {
    case "draft": return "草稿";
    case "queued": return "排队中";
    case "running": return "生成中";
    case "partial": return "生成中";
    case "action_required": return "待验收";
    case "failed": return "待验收";
    case "awaiting_adoption": return "待验收";
    case "completed": return "已采用";
    case "cancelled": return "已结束";
    default: return status ? "处理中" : "加载中";
  }
}

const CREATIVE_DELIVERY_SIZE_LABELS: Record<(typeof CREATIVE_DELIVERY_SIZES)[number], string> = {
  "1080x1080": "方形",
  "1200x628": "横版",
  "800x1000": "竖版",
};

type DeliveryAttachment = Pick<Attachment, "id" | "filename" | "url" | "download_url" | "markdown_url" | "content_type">;

export type CreativeDeliveryNamingContext = Pick<CreativeOrder, "created_at" | "input_snapshot">;

export type CreativeVariantAdoptionRisk = {
  acknowledged: true;
  reason: string;
};

export type CreativeVariantRetryAction = {
  kind: "workflow" | "qc";
  taskId: string;
  label: string;
};

export function adoptedCreativeOrderVariant(item: CreativeOrderItem): CreativeOrderVariant | undefined {
  const adoptedVariantId = item.adopted_variant_id;
  if (!adoptedVariantId) return undefined;
  return item.variants.find((variant) => variant.id === adoptedVariantId);
}

export function creativeOrderActionableWorkflowFailures(order: CreativeOrder | undefined): CreativeOrderWorkflowFailure[] {
  if (!order) return [];
  const variantsById = new Map(order.items.flatMap((item) => item.variants.map((variant) => [variant.id, variant] as const)));
  return (order.workflow_failures ?? []).filter((failure) => {
    const variant = variantsById.get(failure.subject_id);
    if (!variant) return true;
    if (!creativeVariantNeedsManualAction(variant)) return false;
    if (creativeVariantHasQCFailure(variant)) return creativeOrderWorkflowFailureIsQC(failure.workflow);
    return true;
  });
}

export function creativeOrderStage(order: CreativeOrder | undefined): CreativeOrderStage {
  const items = order?.items ?? [];
  const variants = items.flatMap((item) => item.variants);
  const readyVariants = variants.filter((variant) => creativeVariantAdoptionReadiness(variant).ready).length;
  const blockedVariants = variants.filter(creativeVariantNeedsManualAction).length;
  const productionStoppedVariants = variants.filter(creativeVariantHasProductionStop).length;
  const adoptedItems = items.filter((item) => Boolean(adoptedCreativeOrderVariant(item))).length;
  const actionableFailures = creativeOrderActionableWorkflowFailures(order);
  const base = { readyVariants, totalVariants: variants.length, adoptedItems, totalItems: items.length };
  const status = order?.derived_status || order?.status || "";

  if (status === "cancelled" || order?.status === "cancelled") {
    return { ...base, key: "cancelled", label: "已结束", detail: "订单已结束，结果与记录仍保留", action: "查看记录" };
  }
  if (status === "completed" || (items.length > 0 && adoptedItems === items.length)) {
    return { ...base, key: "delivered", label: "已采用", detail: items.length > 0 ? `${adoptedItems}/${items.length} 个素材已采用，可查看和下载` : "已采用最终方案", action: "查看并下载" };
  }
  if (readyVariants > adoptedItems) {
    return { ...base, key: "review", label: "待验收", detail: `${readyVariants} 个变体可以比较`, action: "选择最终方案" };
  }
  if (status === "awaiting_adoption") {
    return { ...base, key: "review", label: "待验收", detail: "已有可采用方案，等待选择", action: "选择最终方案" };
  }
  if (actionableFailures.length > 0 || status === "failed" || blockedVariants > 0 || (status === "action_required" && variants.length === 0)) {
    const failureCount = actionableFailures.length;
    const detail = [
      productionStoppedVariants > 0 ? `${productionStoppedVariants} 个变体生成失败` : "",
      blockedVariants - productionStoppedVariants > 0 ? `${blockedVariants - productionStoppedVariants} 个变体有系统提醒` : "",
      readyVariants > 0 ? `${readyVariants} 个变体可验收` : "",
      failureCount > 0 ? `${failureCount} 条系统提醒` : "",
    ].filter(Boolean).join("，");
    return { ...base, key: "attention", label: "待验收", detail: detail || "等待人工验收，系统提醒仅作为参考", action: "查看结果" };
  }
  if (variants.length > 0 || ["queued", "running", "partial"].includes(status)) {
    const completedSizes = variants.reduce((count, variant) => count + creativeVariantPreviewAssets(variant).length, 0);
    const expectedSizes = variants.length * CREATIVE_DELIVERY_SIZES.length;
    return { ...base, key: "generating", label: "生成中", detail: `${completedSizes}/${expectedSizes} 张预览已就绪`, action: "查看生成进度" };
  }
  return { ...base, key: "preparing", label: "准备中", detail: items.length > 0 ? `${items.length} 个素材等待生成` : "正在准备订单", action: "查看订单" };
}

export function creativeVariantDeliveryAssets(variant: CreativeOrderVariant): CreativeOrderAsset[] {
  const selected = new Map<string, CreativeOrderAsset>();
  for (const asset of variant.assets) {
    if (asset.revision !== variant.revision || asset.stage !== "delivered" || asset.status !== "completed" || !asset.attachment_id) continue;
    if (!CREATIVE_DELIVERY_SIZES.includes(asset.size_key as (typeof CREATIVE_DELIVERY_SIZES)[number])) continue;
    const current = selected.get(asset.size_key);
    if (!current || compareDeliveryAssets(asset, current) > 0) selected.set(asset.size_key, asset);
  }
  return CREATIVE_DELIVERY_SIZES.flatMap((size) => {
    const asset = selected.get(size);
    return asset ? [asset] : [];
  });
}

export function creativeVariantPreviewAssets(variant: CreativeOrderVariant): CreativeOrderAsset[] {
  const selected = new Map<string, CreativeOrderAsset>();
  for (const asset of variant.assets) {
    if (asset.revision !== variant.revision || asset.status !== "completed" || !asset.attachment_id) continue;
    if (!CREATIVE_DELIVERY_SIZES.includes(asset.size_key as (typeof CREATIVE_DELIVERY_SIZES)[number])) continue;
    const current = selected.get(asset.size_key);
    if (!current || comparePreviewAssets(asset, current) > 0) selected.set(asset.size_key, asset);
  }
  return CREATIVE_DELIVERY_SIZES.flatMap((size) => {
    const asset = selected.get(size);
    return asset ? [asset] : [];
  });
}

export function creativeVariantDiagnosticAssets(variant: CreativeOrderVariant): CreativeOrderDiagnosticAsset[] {
  const selected = new Map<string, CreativeOrderDiagnosticAsset[]>();
  for (const asset of variant.diagnostic_assets ?? []) {
    if (asset.revision !== variant.revision || !asset.url) continue;
    if (!CREATIVE_DELIVERY_SIZES.includes(asset.size_key as (typeof CREATIVE_DELIVERY_SIZES)[number])) continue;
    const bucket = selected.get(asset.size_key) ?? [];
    bucket.push(asset);
    selected.set(asset.size_key, bucket);
  }
  return CREATIVE_DELIVERY_SIZES.flatMap((size) => selected.get(size) ?? []);
}

function creativeVariantUsesDirectEditNoQC(variant: CreativeOrderVariant): boolean {
  const contract = variant.brief?.creative_direct_edit_delivery;
  return Boolean(contract && typeof contract === "object" && !Array.isArray(contract) && (contract as { skip_qc?: unknown }).skip_qc === true);
}

export function creativeVariantArchiveEntries(
  variant: CreativeOrderVariant,
  attachments: Map<string, DeliveryAttachment>,
  order?: CreativeDeliveryNamingContext,
  item?: Pick<CreativeOrderItem, "candidate_id" | "copy_snapshot">,
): { asset: CreativeOrderAsset; attachment: DeliveryAttachment; filename: string }[] {
  const usedNames = new Set<string>();
  return creativeVariantDeliveryAssets(variant).flatMap((asset) => {
    const attachment = attachments.get(asset.attachment_id);
    if (!attachment || !creativeAttachmentBrowserURL(attachment)) return [];
    return [{
      asset,
      attachment,
      filename: creativeDeliveryFilename({ order, item, asset, attachment, usedNames }),
    }];
  });
}

export async function downloadCreativeVariantArchive({
  orderId,
  item,
  variant,
  attachments,
  order,
}: {
  orderId: string;
  item: CreativeOrderItem;
  variant: CreativeOrderVariant;
  attachments: Map<string, DeliveryAttachment>;
  order?: CreativeDeliveryNamingContext;
}): Promise<void> {
  const entries = creativeVariantArchiveEntries(variant, attachments, order, item);
  if (entries.length !== CREATIVE_DELIVERY_SIZES.length) throw new Error("交付包的三张图片尚未齐备");
  const files: Record<string, Uint8Array> = {};
  const downloaded = await Promise.all(entries.map(async ({ attachment, filename }) => {
    const response = await fetch(creativeAttachmentBrowserURL(attachment), { credentials: "include" });
    if (!response.ok) throw new Error(`无法下载 ${filename}`);
    const bytes = new Uint8Array(await response.arrayBuffer());
    return { filename: filenameForResponse(filename, response.headers.get("content-type"), bytes), bytes };
  }));
  const usedNames = new Set<string>();
  for (const { filename, bytes } of downloaded) files[uniqueArchiveFilename(filename, usedNames)] = bytes;
  files["说明.txt"] = strToU8([
    "最终采用方案交付包",
    `订单：${orderId}`,
    `素材条目：${item.id}`,
    `变体：${variant.variant_key || variant.id}`,
    `采用时间：${item.adopted_at || "未记录"}`,
    "包含尺寸：1080x1080、1200x628、800x1000",
  ].join("\n"));
  const archive = zipSync(files, { level: 0 });
  const buffer = new ArrayBuffer(archive.byteLength);
  new Uint8Array(buffer).set(archive);
  triggerBrowserDownload(
    URL.createObjectURL(new Blob([buffer], { type: "application/zip" })),
    `${safeArchiveName(`creative-order-${orderId.slice(0, 8)}-${variant.variant_key || variant.id}`)}.zip`,
    true,
  );
}

export function CreativeOrderDeliveryCandidates({
  orderId,
  item,
  order,
  source,
  attachments,
  adoptingVariantId,
  onAdopt,
  onAssetSelect,
  onAssetInfo,
  disabled = false,
  showDirectionDetails = true,
  defaultOpen = true,
  adjustment,
  retryingVariantId = "",
  onRetryVariant,
}: {
  orderId: string;
  item: CreativeOrderItem;
  order?: CreativeDeliveryNamingContext;
  source: { label: string; url: string };
  attachments: Map<string, DeliveryAttachment>;
  adoptingVariantId: string;
  onAdopt: (variantId: string, risk?: CreativeVariantAdoptionRisk) => void;
  onAssetSelect: (assetId: string) => void;
  onAssetInfo?: (assetId: string) => void;
  disabled?: boolean;
  showDirectionDetails?: boolean;
  defaultOpen?: boolean;
  adjustment?: { variantId: string; sizeKey: string; status: string };
  retryingVariantId?: string;
  onRetryVariant?: (variant: CreativeOrderVariant, action: CreativeVariantRetryAction) => void;
}) {
  const adoptedVariant = adoptedCreativeOrderVariant(item);
  const otherVariants = adoptedVariant ? item.variants.filter((variant) => variant.id !== adoptedVariant.id) : [];
  const title = source.label || `素材 ${item.candidate_id.slice(0, 8)}`;
  const progress = creativeOrderItemProgress(item);
  const [packageOpen, setPackageOpen] = useState(defaultOpen);

  return <details className="group border bg-background" open={packageOpen} onToggle={(event) => setPackageOpen(event.currentTarget.open)} data-testid={`creative-order-item-${item.id}`}>
    <summary className="flex cursor-pointer list-none flex-wrap items-start justify-between gap-3 px-4 py-3 marker:content-none">
      <div className="flex min-w-0 flex-1 items-start gap-2">
        <ChevronDown className="mt-0.5 h-4 w-4 shrink-0 transition-transform group-open:rotate-180" />
        <div className="min-w-0">
          <p className="break-words text-sm font-semibold">{title}</p>
          <p className="mt-1 text-xs text-muted-foreground">{progress.detail}</p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={progress.tone}>{progress.label}</Badge>
        <Badge variant={adoptedVariant ? "default" : "secondary"}>{adoptedVariant ? `已采用 ${adoptedVariant.variant_key}` : "待选择"}</Badge>
      </div>
    </summary>
    <div className="border-t">
      {showDirectionDetails && item.direction.trim() && <details className="border-b px-4 py-3 text-xs text-muted-foreground">
          <summary className="w-fit cursor-pointer select-none">生成方向详情</summary>
          <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words border bg-muted/20 p-3 font-sans text-xs leading-5">{item.direction.trim()}</pre>
        </details>}
      <CreativeOrderSourceAndPrompt source={source} progress={progress} />

      {adoptedVariant ? <>
        <AdoptedVariantDelivery
          orderId={orderId}
          item={item}
          order={order}
          variant={adoptedVariant}
          attachments={attachments}
          onAssetSelect={onAssetSelect}
          onAssetInfo={onAssetInfo}
        />
        {otherVariants.length > 0 && <details className="group/other border-t bg-muted/10">
          <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-medium marker:content-none">
            <ChevronDown className="h-4 w-4 transition-transform group-open/other:rotate-180" />
            查看其他候选
            <Badge variant="outline">{otherVariants.length}</Badge>
          </summary>
          <div className="grid gap-3 border-t p-4 lg:grid-cols-3">
            {otherVariants.map((variant) => <VariantCandidate
              key={variant.id}
              variant={variant}
              attachments={attachments}
              adoptedVariantId={adoptedVariant.id}
              adoptingVariantId={adoptingVariantId}
              onAdopt={onAdopt}
              onAssetSelect={onAssetSelect}
              onAssetInfo={onAssetInfo}
              disabled={disabled}
              subdued
              adjustment={adjustment}
              retrying={retryingVariantId === variant.id}
              onRetry={onRetryVariant}
            />)}
          </div>
        </details>}
      </> : item.variants.length > 0 ? <div className="grid gap-3 p-4 lg:grid-cols-3">
          {item.variants.map((variant) => <VariantCandidate
            key={variant.id}
            variant={variant}
            attachments={attachments}
            adoptedVariantId=""
            adoptingVariantId={adoptingVariantId}
            onAdopt={onAdopt}
            onAssetSelect={onAssetSelect}
            onAssetInfo={onAssetInfo}
            disabled={disabled}
            adjustment={adjustment}
            retrying={retryingVariantId === variant.id}
            onRetry={onRetryVariant}
          />)}
        </div> : <div className="grid gap-3 p-4 lg:grid-cols-3"><CreativeOrderVariantPlaceholder /></div>}
    </div>
  </details>;
}

type CreativeOrderItemProgress = {
  label: string;
  detail: string;
  tone: "default" | "secondary" | "destructive" | "outline";
  variants: number;
  previewAssets: number;
  expectedAssets: number;
  runningVariants: number;
  productionStoppedVariants: number;
  blockedVariants: number;
  readyVariants: number;
};

function creativeOrderItemProgress(item: CreativeOrderItem): CreativeOrderItemProgress {
  const variants = item.variants;
  const previewAssets = variants.reduce((count, variant) => count + creativeVariantPreviewAssets(variant).length, 0);
  const expectedAssets = variants.length * CREATIVE_DELIVERY_SIZES.length;
  const runningVariants = variants.filter(creativeVariantIsInProgress).length;
  const productionStoppedVariants = variants.filter(creativeVariantHasProductionStop).length;
  const blockedVariants = variants.filter(creativeVariantNeedsManualAction).length;
  const readyVariants = variants.filter((variant) => creativeVariantAdoptionReadiness(variant).ready).length;
  if (item.adopted_variant_id) return { label: "已采用", detail: "已选择最终方案，可查看交付包。", tone: "default", variants: variants.length, previewAssets, expectedAssets, runningVariants, productionStoppedVariants, blockedVariants, readyVariants };
  if (readyVariants > 0) return { label: "可验收", detail: `${readyVariants}/${variants.length} 个变体可验收，先比较成图再采用。`, tone: "default", variants: variants.length, previewAssets, expectedAssets, runningVariants, productionStoppedVariants, blockedVariants, readyVariants };
  if (productionStoppedVariants > 0) return { label: "生成失败", detail: `${productionStoppedVariants} 个变体生成失败，过程图片仅用于排查。`, tone: "secondary", variants: variants.length, previewAssets, expectedAssets, runningVariants, productionStoppedVariants, blockedVariants, readyVariants };
  if (blockedVariants > 0) return { label: "待验收", detail: `${blockedVariants} 个变体有系统提醒，先查看现有结果再决定。`, tone: "secondary", variants: variants.length, previewAssets, expectedAssets, runningVariants, productionStoppedVariants, blockedVariants, readyVariants };
  if (runningVariants > 0 || variants.length > 0) return { label: "生成中", detail: `${previewAssets}/${Math.max(expectedAssets, 1)} 张成图已就绪，页面会自动刷新。`, tone: "outline", variants: variants.length, previewAssets, expectedAssets, runningVariants, productionStoppedVariants, blockedVariants, readyVariants };
  return { label: "准备中", detail: "等待后台创建生成任务。", tone: "secondary", variants: 0, previewAssets: 0, expectedAssets: 0, runningVariants: 0, productionStoppedVariants: 0, blockedVariants: 0, readyVariants: 0 };
}

function CreativeOrderSourceAndPrompt({ source, progress }: { source: { label: string; url: string }; progress: CreativeOrderItemProgress }) {
  return <div className="grid border-b bg-muted/10 lg:grid-cols-[minmax(240px,0.72fr)_minmax(0,1.28fr)]">
    <figure className="min-w-0 border-b bg-background lg:border-b-0 lg:border-r">
      <figcaption className="border-b px-3 py-2 text-xs font-medium">原图 · {source.label}</figcaption>
      <div className="flex min-h-64 items-center justify-center p-3">
        {source.url ? <img src={source.url} alt={`原图 ${source.label}`} width={1200} height={1200} loading="lazy" className="max-h-[360px] w-full object-contain" /> : <EmptyImage label="原图不可用" />}
      </div>
    </figure>
    <div className="min-w-0 space-y-3 px-4 py-3">
      <div className="flex flex-wrap gap-2 text-xs">
        <Badge variant="outline">{progress.variants} 个变体</Badge>
        <Badge variant="outline">{progress.expectedAssets > 0 ? `${progress.previewAssets}/${progress.expectedAssets} 张成图` : "等待任务"}</Badge>
        {progress.runningVariants > 0 && <Badge variant="outline">{progress.runningVariants} 个生成中</Badge>}
        {progress.productionStoppedVariants > 0 && <Badge variant="secondary">{progress.productionStoppedVariants} 个生成失败</Badge>}
        {progress.blockedVariants - progress.productionStoppedVariants > 0 && <Badge variant="secondary">{progress.blockedVariants - progress.productionStoppedVariants} 条系统提醒</Badge>}
        {progress.readyVariants > 0 && <Badge>{progress.readyVariants} 个可验收</Badge>}
      </div>
      <p className="text-xs leading-5 text-muted-foreground">每张成图旁都可查看完整模型提示词、冻结文案、生成记录和版本溯源。</p>
    </div>
  </div>;
}

function CreativeOrderVariantPlaceholder() {
  return <div className="flex min-h-48 items-center justify-center border border-dashed bg-muted/10 px-6 text-center text-sm text-muted-foreground lg:col-span-3">
    等待后台创建生成任务，完成后这里会出现成图和验收入口。
  </div>;
}

function AdoptedVariantDelivery({
  orderId,
  item,
  order,
  variant,
  attachments,
  onAssetSelect,
  onAssetInfo,
}: {
  orderId: string;
  item: CreativeOrderItem;
  order?: CreativeDeliveryNamingContext;
  variant: CreativeOrderVariant;
  attachments: Map<string, DeliveryAttachment>;
  onAssetSelect: (assetId: string) => void;
  onAssetInfo?: (assetId: string) => void;
}) {
  const [archiveBusy, setArchiveBusy] = useState(false);
  const [archiveError, setArchiveError] = useState("");
  const delivered = creativeVariantDeliveryAssets(variant);
  const entries = creativeVariantArchiveEntries(variant, attachments, order, item);
  const downloadArchive = async () => {
    setArchiveBusy(true);
    setArchiveError("");
    try {
      await downloadCreativeVariantArchive({ orderId, item, variant, attachments, order });
    } catch (error) {
      setArchiveError(error instanceof Error ? error.message : "无法打包下载交付包");
    } finally {
      setArchiveBusy(false);
    }
  };

  return <div className="bg-emerald-50/40 dark:bg-emerald-950/10" data-testid="creative-adopted-variant">
    <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
      <div className="flex items-center gap-2"><PackageCheck className="h-4 w-4 text-emerald-700 dark:text-emerald-400" /><h3 className="text-sm font-semibold">最终采用方案 · 交付包</h3><Badge variant="outline">{variant.variant_key}</Badge></div>
      <Button size="sm" variant="outline" disabled={archiveBusy || entries.length !== CREATIVE_DELIVERY_SIZES.length} onClick={() => void downloadArchive()}><Download className="h-4 w-4" />{archiveBusy ? "正在打包" : "下载交付包"}</Button>
    </div>
    {archiveError && <p role="alert" className="border-y border-destructive/20 bg-destructive/5 px-4 py-2 text-xs text-destructive">{archiveError}</p>}
    <div className="grid min-w-0 border-t md:grid-cols-3">
      {CREATIVE_DELIVERY_SIZES.map((size, index) => {
        const asset = delivered.find((candidate) => candidate.size_key === size);
        const attachment = asset ? attachments.get(asset.attachment_id) : undefined;
        const entry = asset ? entries.find((candidate) => candidate.asset.id === asset.id) : undefined;
        return <DeliveryAssetPane key={size} asset={asset} attachment={attachment} size={size} downloadFilename={entry?.filename} onAssetSelect={onAssetSelect} onAssetInfo={onAssetInfo} divided={index > 0} />;
      })}
    </div>
  </div>;
}

function VariantCandidate({
  variant,
  attachments,
  adoptedVariantId,
  adoptingVariantId,
  onAdopt,
  onAssetSelect,
  onAssetInfo,
  disabled = false,
  subdued = false,
  adjustment,
  retrying = false,
  onRetry,
}: {
  variant: CreativeOrderVariant;
  attachments: Map<string, DeliveryAttachment>;
  adoptedVariantId: string;
  adoptingVariantId: string;
  onAdopt: (variantId: string, risk?: CreativeVariantAdoptionRisk) => void;
  onAssetSelect: (assetId: string) => void;
  onAssetInfo?: (assetId: string) => void;
  disabled?: boolean;
  subdued?: boolean;
  adjustment?: { variantId: string; sizeKey: string; status: string };
  retrying?: boolean;
  onRetry?: (variant: CreativeOrderVariant, action: CreativeVariantRetryAction) => void;
}) {
  const readiness = creativeVariantAdoptionReadiness(variant);
  const riskAdoption = creativeVariantRiskAdoptionReadiness(variant);
  const qcDetails = creativeVariantQCDetails(variant);
  const descriptionId = useId();
  const previews = creativeVariantPreviewAssets(variant);
  const diagnostics = creativeVariantDiagnosticAssets(variant);
  const [processOpen, setProcessOpen] = useState(false);
  const cover = previews.find((asset) => asset.size_key === "1080x1080") ?? previews[0];
  const coverURL = cover ? creativeAttachmentBrowserURL(attachments.get(cover.attachment_id)) : "";
  const coverDiagnostic = coverURL ? undefined : diagnostics.find((asset) => asset.size_key === "1080x1080") ?? diagnostics[0];
  const adopted = adoptedVariantId === variant.id;
  const busy = adoptingVariantId === variant.id;
  const blocked = creativeVariantNeedsManualAction(variant);
  const productionStopped = creativeVariantHasProductionStop(variant);
  const backgroundRunning = creativeVariantHasBackgroundWorkInProgress(variant);
  const requiresRiskAcknowledgement = riskAdoption.allowed;
  const currentSizeAdjustment = adjustment?.variantId === variant.id ? adjustment : undefined;
  const retryAction = creativeVariantRetryAction(variant);
  const adjustmentBadge = currentSizeAdjustment?.status.includes("已完成")
    ? "已完成"
    : currentSizeAdjustment?.status.includes("未启动")
      ? "未启动"
      : currentSizeAdjustment?.status.includes("需要处理")
        ? "待处理"
        : "调整中";
  const adoptionDisabled = disabled || !readiness.ready || adopted || Boolean(adoptingVariantId);
  const statusTone = blocked
    ? "text-amber-700 dark:text-amber-300"
    : requiresRiskAcknowledgement
      ? "text-amber-700 dark:text-amber-300"
      : readiness.ready
        ? "text-emerald-700 dark:text-emerald-400"
        : "text-muted-foreground";
  const adoptVariant = () => {
    if (requiresRiskAcknowledgement) {
      onAdopt(variant.id, { acknowledged: true, reason: "用户确认忽略系统提醒并采用" });
      return;
    }
    onAdopt(variant.id);
  };
  return <article className={cn("flex min-w-0 flex-col border bg-background", subdued && "opacity-75 transition-opacity hover:opacity-100")}>
    <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
      <div className="flex min-w-0 items-center gap-2"><span className="text-sm font-semibold">{variant.variant_key || variant.id.slice(0, 8)}</span><Badge variant="outline">r{variant.revision}</Badge>{currentSizeAdjustment && <span className="truncate text-xs text-amber-800 dark:text-amber-200">{CREATIVE_DELIVERY_SIZE_LABELS[currentSizeAdjustment.sizeKey as (typeof CREATIVE_DELIVERY_SIZES)[number]] ?? currentSizeAdjustment.sizeKey} {currentSizeAdjustment.status}</span>}</div>
      {adopted ? <Badge variant="default"><CheckCircle2 className="h-3 w-3" />已采用</Badge> : currentSizeAdjustment ? <Badge variant="outline">{adjustmentBadge}</Badge> : blocked ? <Badge variant="secondary">{productionStopped ? "生成失败" : "有系统提醒"}</Badge> : requiresRiskAcknowledgement ? <Badge variant="secondary">有系统提醒</Badge> : readiness.ready ? <Badge variant="outline"><CheckCircle2 className="h-3 w-3" />待验收</Badge> : backgroundRunning ? <Badge variant="outline">处理中</Badge> : <Badge variant="secondary">尚未完成</Badge>}
    </div>
    <button type="button" disabled={(!cover || !coverURL) && !coverDiagnostic} onClick={() => { if (cover && coverURL) onAssetSelect(cover.id); else if (coverDiagnostic) openCreativeDiagnosticAsset(coverDiagnostic); }} className="group relative flex min-h-72 w-full items-center justify-center border-b bg-muted/10 p-3 disabled:cursor-default">
      {coverURL ? <img src={coverURL} alt={`${variant.variant_key} 方形主预览`} width={720} height={720} loading="lazy" className="max-h-[420px] w-full object-contain transition-transform group-hover:scale-[1.01]" /> : coverDiagnostic ? <>
        <img src={coverDiagnostic.url} alt={`${variant.variant_key} 过程图片 ${coverDiagnostic.label}`} width={720} height={720} loading="lazy" className="max-h-[420px] w-full object-contain opacity-90 transition-transform group-hover:scale-[1.01]" />
        <span className="absolute left-3 top-3 rounded-full border border-amber-300 bg-amber-50 px-2 py-1 text-[11px] font-medium text-amber-900 shadow-sm dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">过程图片</span>
      </> : <EmptyImage label="待成图" />}
    </button>
    <div className="mt-auto space-y-2 p-3">
      <div className="grid grid-cols-3 divide-x border text-center text-[11px] text-muted-foreground">{CREATIVE_DELIVERY_SIZES.map((size) => {
        const asset = previews.find((candidate) => candidate.size_key === size);
        return <div key={size} className="relative min-w-0">
          <button type="button" disabled={!asset} onClick={() => { if (asset) onAssetSelect(asset.id); }} className="w-full px-2 py-2 pr-7 disabled:opacity-50"><span className="block font-medium text-foreground">{CREATIVE_DELIVERY_SIZE_LABELS[size]}</span><span>{asset ? "可查看" : "待成图"}</span></button>
          {asset && onAssetInfo && <Button size="icon-sm" variant="ghost" className="absolute right-0.5 top-1/2 -translate-y-1/2" title={`查看${CREATIVE_DELIVERY_SIZE_LABELS[size]}成图详情`} aria-label={`查看${CREATIVE_DELIVERY_SIZE_LABELS[size]}成图详情`} onClick={() => onAssetInfo(asset.id)}><Info className="h-3.5 w-3.5" /></Button>}
        </div>;
      })}</div>
      {diagnostics.length > 0 && <Button className="w-full" size="sm" variant="outline" onClick={() => setProcessOpen(true)}>
        <ImageIcon className="h-4 w-4" />
        查看过程图片
        <Badge variant="secondary">{diagnostics.length}</Badge>
      </Button>}
      <p id={descriptionId} className={cn("min-h-8 text-xs", statusTone)}>{readiness.status}</p>
      <VariantActionRequiredNotice variant={variant} disabled={disabled} />
      <VariantQCDetails details={qcDetails} />
      {retryAction && onRetry && <Button className="w-full" size="sm" variant="outline" disabled={disabled || retrying} onClick={() => onRetry(variant, retryAction)}>
        <RefreshCw className={cn("h-4 w-4", retrying && "animate-spin")} />
        {retrying ? "正在重试" : retryAction.label}
      </Button>}
      <Button className="w-full" size="sm" variant="outline" disabled={!cover || !coverURL} onClick={() => { if (cover && coverURL) onAssetSelect(cover.id); }}>
        <Eye className="h-4 w-4" />
        {cover && coverURL ? "查看并标注" : "暂无可标注成图"}
      </Button>
      <Button className={cn("w-full", requiresRiskAcknowledgement && !adopted && "border-amber-300 text-amber-800 hover:bg-amber-50 dark:border-amber-800 dark:text-amber-200 dark:hover:bg-amber-950/40")} size="sm" variant={adopted ? "secondary" : requiresRiskAcknowledgement ? "outline" : "default"} disabled={adoptionDisabled} aria-describedby={descriptionId} onClick={adoptVariant}>
        {adopted ? <CheckCircle2 className="h-4 w-4" /> : <Check className="h-4 w-4" />}
        {disabled ? "订单已结束" : busy ? "正在采用" : adopted ? "当前采用" : requiresRiskAcknowledgement ? "忽略提醒并采用" : readiness.ready ? "采用此变体" : "尚不可采用"}
      </Button>
    </div>
    <CreativeProcessImageDialog open={processOpen} onOpenChange={setProcessOpen} variant={variant} assets={diagnostics} />
  </article>;
}

function CreativeProcessImageDialog({
  open,
  onOpenChange,
  variant,
  assets,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variant: CreativeOrderVariant;
  assets: CreativeOrderDiagnosticAsset[];
}) {
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="grid max-h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(96vw,1280px)]">
      <DialogHeader className="border-b px-5 py-4 pr-14">
        <DialogTitle className="text-base">{variant.variant_key || "当前方案"} · 过程图片</DialogTitle>
        <DialogDescription>展示本次自动流程留下的中间图片；只能查看，不能作为最终采用图。</DialogDescription>
      </DialogHeader>
      <div className="min-h-0 overflow-y-auto p-4">
        {assets.length > 0 ? <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {assets.map((asset) => <figure key={asset.id} className="min-w-0 overflow-hidden border bg-background">
            <figcaption className="flex items-center justify-between gap-2 border-b px-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-xs font-medium">{CREATIVE_DELIVERY_SIZE_LABELS[asset.size_key as (typeof CREATIVE_DELIVERY_SIZES)[number]] ?? asset.size_key} · {creativeDiagnosticAssetLabel(asset)}</p>
                <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{asset.filename}</p>
              </div>
              <Button size="icon-sm" variant="ghost" title="打开原图" aria-label={`打开过程图片 ${asset.filename}`} onClick={() => openCreativeDiagnosticAsset(asset)}><ExternalLink className="h-4 w-4" /></Button>
            </figcaption>
            <button type="button" className="flex min-h-72 w-full items-center justify-center bg-muted/10 p-3" onClick={() => openCreativeDiagnosticAsset(asset)}>
              <img src={asset.url} alt={`${variant.variant_key} ${asset.label} ${asset.size_key}`} width={1200} height={1200} loading="lazy" className="max-h-[520px] w-full object-contain" />
            </button>
          </figure>)}
        </div> : <EmptyImage label="暂无过程图片" />}
      </div>
    </DialogContent>
  </Dialog>;
}

function VariantActionRequiredNotice({ variant, disabled = false }: { variant: CreativeOrderVariant; disabled?: boolean }) {
  if (!creativeVariantNeedsManualAction(variant)) return null;
  if (creativeVariantHasQCFailure(variant)) return null;
  const blocker = variant.action_required;
  const detail = blocker?.detail ? businessActionRequiredDetail(blocker.workflow, blocker.detail) : "该变体需要人工确认，但任务未返回可读原因。";
  return <div role="status" className="space-y-1 border border-amber-300 bg-amber-50 px-2 py-2 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200" data-testid="creative-variant-action-required">
    <p className="flex items-center gap-1.5 font-medium"><AlertTriangle className="h-3.5 w-3.5" />{creativeVariantBlockerTitle(blocker?.workflow, creativeVariantHasProductionStop(variant))}</p>
    <p className="break-words">{detail}</p>
    <p className="text-amber-800/80 dark:text-amber-200/80">{disabled ? "订单已结束，过程记录保留。" : blocker?.retryable ? "可以直接重试这个方案；也可以查看其他候选或标注调整。" : "后台已记录该步骤未补齐；可查看其他候选、标注调整或重新发起。"}</p>
  </div>;
}

function creativeVariantBlockerTitle(workflow: string | undefined, productionStopped = false): string {
  if (productionStopped) return "成图生成失败";
  const label = creativeVariantWorkflowLabel(workflow || "");
  return label ? `${label}提醒` : "系统提醒";
}

function creativeVariantWorkflowLabel(workflow: string): string {
  return ({
    creative_plan: "创意方案",
    creative_production: "成图生成",
    brand_components: "品牌组件合成",
    creative_qc_technical: "技术质检",
    creative_qc_visual: "视觉质检",
    creative_direct_edit: "图片调整",
  } as Record<string, string>)[workflow] || "";
}

function creativeDiagnosticAssetLabel(asset: CreativeOrderDiagnosticAsset): string {
  const adjustment = isRecord(asset.metadata?.order_adjustment) ? asset.metadata.order_adjustment : undefined;
  const sourceRevision = adjustment && typeof adjustment.source_revision === "number" ? adjustment.source_revision : undefined;
  return sourceRevision ? `${asset.label} · 沿用 r${sourceRevision}` : asset.label;
}

export type CreativeVariantQCDetail = {
  lane: "technical" | "visual";
  label: string;
  status: string;
  blockingFailures: string[];
  qualityWarnings: string[];
};

export function creativeVariantQCDetails(variant: CreativeOrderVariant): CreativeVariantQCDetail[] {
  const reports = currentCreativeVariantQCReports(variant);
  return (["technical", "visual"] as const).flatMap((lane) => {
    const report = reports.get(lane);
    if (!report) return [];
    const blockingFailures = creativeVariantQCBlockingFailures(variant, lane, report);
    return [{
      lane,
      label: lane === "technical" ? "技术质检" : "视觉质检",
      status: blockingFailures.length > 0 ? "failed" : report.status,
      blockingFailures,
      qualityWarnings: qcFindingMessages(report.findings?.quality_warnings),
    }];
  });
}

function VariantQCDetails({ details }: { details: CreativeVariantQCDetail[] }) {
  const visible = details.filter((detail) =>
    detail.status === "failed" || detail.blockingFailures.length > 0 || detail.qualityWarnings.length > 0,
  );
  if (visible.length === 0) return null;
  const hasFailure = visible.some((detail) => detail.status === "failed");

  return <div
    className={cn("space-y-3 border-l-2 pl-3 text-xs", hasFailure ? "border-amber-300 bg-amber-50/50 py-2 pr-2 dark:border-amber-800 dark:bg-amber-950/20" : "border-border")}
    role="status"
    data-testid="creative-variant-qc-details"
  >
    {visible.map((detail) => <div key={detail.lane} className="space-y-1.5">
      <p className={cn("font-medium", detail.status === "failed" && "text-amber-800 dark:text-amber-200")}>
        {detail.status === "failed" ? `${detail.label}未通过` : `${detail.label}提醒`}
      </p>
      {detail.blockingFailures.length > 0 ? <QCMessageList label="阻断原因" messages={detail.blockingFailures} /> : null}
      {detail.qualityWarnings.length > 0 && <QCMessageList label="质量提醒" messages={detail.qualityWarnings} />}
    </div>)}
  </div>;
}

function QCMessageList({ label, messages }: { label: string; messages: string[] }) {
  return <div>
    <p className="text-muted-foreground">{label}</p>
    <ul className="mt-1 list-disc space-y-1 pl-4 text-foreground">
      {messages.map((message, index) => <li key={`${index}-${message}`}>{message}</li>)}
    </ul>
  </div>;
}

function DeliveryAssetPane({
  asset,
  attachment,
  size,
  downloadFilename,
  onAssetSelect,
  onAssetInfo,
  divided,
}: {
  asset?: CreativeOrderAsset;
  attachment?: DeliveryAttachment;
  size: (typeof CREATIVE_DELIVERY_SIZES)[number];
  downloadFilename?: string;
  onAssetSelect: (assetId: string) => void;
  onAssetInfo?: (assetId: string) => void;
  divided: boolean;
}) {
  const url = creativeAttachmentBrowserURL(attachment);
  return <figure className={cn("min-w-0 bg-background", divided && "border-t md:border-l md:border-t-0")}>
    <figcaption className="flex items-center justify-between gap-2 border-b px-3 py-2">
      <span className="text-xs font-medium">{CREATIVE_DELIVERY_SIZE_LABELS[size]} · {size}</span>
      <span className="flex items-center gap-1">
        {asset && onAssetInfo && <Button size="icon-sm" variant="ghost" title={`查看${CREATIVE_DELIVERY_SIZE_LABELS[size]}成图详情`} aria-label={`查看${CREATIVE_DELIVERY_SIZE_LABELS[size]}成图详情`} onClick={() => onAssetInfo(asset.id)}><Info className="h-4 w-4" /></Button>}
        {url && attachment && <Button size="icon-sm" variant="ghost" title={`下载 ${size}`} aria-label={`下载 ${size}`} onClick={() => void downloadCreativeAttachment(attachment, downloadFilename || `${size}${attachmentExtension(attachment)}`).catch((error: unknown) => toast.error(error instanceof Error ? error.message : "无法下载交付图"))}><Download className="h-4 w-4" /></Button>}
      </span>
    </figcaption>
    <button type="button" disabled={!asset || !url} onClick={() => asset && onAssetSelect(asset.id)} className="flex min-h-72 w-full items-center justify-center p-3 disabled:cursor-default">
      {url ? <img src={url} alt={`最终采用方案 ${size}`} width={1200} height={1200} loading="lazy" className="max-h-[480px] w-full object-contain" /> : <EmptyImage label={`${size} 尚未交付`} />}
    </button>
  </figure>;
}

function EmptyImage({ label }: { label: string }) {
  return <span className="flex min-h-40 w-full flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />{label}</span>;
}

function openCreativeDiagnosticAsset(asset: CreativeOrderDiagnosticAsset) {
  window.open(asset.url, "_blank", "noopener,noreferrer");
}

export function creativeVariantAdoptionReadiness(variant: CreativeOrderVariant): { ready: boolean; status: string } {
  const delivered = creativeVariantDeliveryAssets(variant);
  const primedSizes = currentCreativeVariantSizeSet(variant, "primed");
  const reportByLane = new Map(creativeVariantQCDetails(variant).map((detail) => [detail.lane, detail.status]));
  const technical = reportByLane.get("technical") ?? "pending";
  const visual = reportByLane.get("visual") ?? "pending";
  const failedQC = creativeVariantFailedQCLabels(variant);
  if (creativeVariantUsesDirectEditNoQC(variant) && variant.status === "completed" && delivered.length === CREATIVE_DELIVERY_SIZES.length) {
    return { ready: true, status: "精准调整已完成，品牌贴片已重新合成，可以采用" };
  }
  if (creativeVariantHasProductionContinuation(variant)) {
    const generatedSizes = currentCreativeVariantSizeSet(variant, "generated");
    return { ready: false, status: `成图生成中：已完成 ${generatedSizes.size}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  }
  if (failedQC.length > 0) {
    const risk = creativeVariantRiskAdoptionReadiness(variant);
    if (risk.allowed) return { ready: true, status: `系统提醒：${failedQC.join("、")}未通过，仍可查看、标注或忽略提醒采用` };
    return { ready: false, status: risk.status };
  }
  if (delivered.length === CREATIVE_DELIVERY_SIZES.length && primedSizes.size === CREATIVE_DELIVERY_SIZES.length && qcStatusAllowsAdoption(technical) && qcStatusAllowsAdoption(visual)) {
    return { ready: true, status: "三尺寸、品牌组件与质检均已完成，可以采用" };
  }
  const productionStopDetail = creativeVariantProductionStopDetail(variant);
  if (productionStopDetail) return { ready: false, status: `生成失败：${productionStopDetail}` };
  if (creativeVariantHasBackgroundWorkInProgress(variant)) {
    if (primedSizes.size !== CREATIVE_DELIVERY_SIZES.length) return { ready: false, status: `成图已完成，正在合成品牌组件：已完成 ${primedSizes.size}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
    if (!qcStatusAllowsAdoption(technical) || !qcStatusAllowsAdoption(visual)) return { ready: false, status: `品牌组件已完成，等待质检：${creativeVariantPendingQCLabels(technical, visual).join("、")}` };
  }
  if (!qcStatusAllowsAdoption(technical) || !qcStatusAllowsAdoption(visual)) return { ready: false, status: `等待质检：${creativeVariantPendingQCLabels(technical, visual).join("、")}` };
  if (primedSizes.size !== CREATIVE_DELIVERY_SIZES.length) return { ready: false, status: `等待品牌组件合成：已完成 ${primedSizes.size}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  if (delivered.length !== CREATIVE_DELIVERY_SIZES.length) return { ready: false, status: `等待正式交付：已完成 ${delivered.length}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  if (variant.action_required?.detail) return { ready: false, status: `系统提醒：${variant.action_required.detail}` };
  return { ready: false, status: `等待变体完成：当前状态 ${variant.status}` };
}

export function creativeVariantIsInProgress(variant: CreativeOrderVariant): boolean {
  return variant.status === "queued" || variant.status === "running" || variant.status === "partial" || creativeVariantHasBackgroundWorkInProgress(variant);
}

export function creativeVariantNeedsManualAction(variant: CreativeOrderVariant): boolean {
  if (creativeVariantHasCompletePassingDelivery(variant)) return false;
  if (creativeVariantHasProductionContinuation(variant)) return false;
  if (creativeVariantHasQCFailure(variant)) return !creativeVariantRiskAdoptionReadiness(variant).allowed;
  if (creativeVariantHasBackgroundWorkInProgress(variant)) return false;
  return variant.status === "action_required" || variant.status === "failed" || Boolean(variant.action_required);
}

function creativeVariantHasProductionStop(variant: CreativeOrderVariant): boolean {
  return Boolean(creativeVariantProductionStopDetail(variant));
}

function creativeVariantProductionStopDetail(variant: CreativeOrderVariant): string {
  const blocker = variant.action_required;
  if (!blocker || blocker.workflow !== "creative_production") return "";
  if (variant.status !== "action_required" && variant.status !== "failed") return "";
  return businessActionRequiredDetail(blocker.workflow, blocker.detail);
}

function creativeVariantHasProductionContinuation(variant: CreativeOrderVariant): boolean {
  const blocker = variant.action_required;
  if (!blocker || blocker.workflow !== "creative_production") return false;
  return variant.status === "queued" || variant.status === "running" || variant.status === "partial";
}

function creativeOrderWorkflowFailureIsQC(workflow: string): boolean {
  return workflow === "creative_qc" || workflow === "creative_qc_technical" || workflow === "creative_qc_visual";
}

function creativeVariantHasBackgroundWorkInProgress(variant: CreativeOrderVariant): boolean {
  if (creativeVariantHasQCFailure(variant)) return false;
  if (creativeVariantHasCompletePassingDelivery(variant)) return false;
  const pendingQCFinalize = creativeVariantHasPendingQCFinalize(variant);
  if (variant.status !== "queued" && variant.status !== "running" && variant.status !== "partial" && !pendingQCFinalize) return false;
  const generatedSizes = currentCreativeVariantSizeSet(variant, "generated");
  const primedSizes = currentCreativeVariantSizeSet(variant, "primed");
  if (generatedSizes.size === CREATIVE_DELIVERY_SIZES.length && primedSizes.size !== CREATIVE_DELIVERY_SIZES.length) return true;
  const reportByLane = new Map(creativeVariantQCDetails(variant).map((detail) => [detail.lane, detail.status]));
  const technical = reportByLane.get("technical") ?? "pending";
  const visual = reportByLane.get("visual") ?? "pending";
  if (primedSizes.size === CREATIVE_DELIVERY_SIZES.length && (!qcStatusAllowsAdoption(technical) || !qcStatusAllowsAdoption(visual))) return true;
  if (!pendingQCFinalize) return false;
  const deliveredSizes = new Set(creativeVariantDeliveryAssets(variant).map((asset) => asset.size_key));
  if (deliveredSizes.size === CREATIVE_DELIVERY_SIZES.length) return false;
  return true;
}

function creativeVariantHasCompletePassingDelivery(variant: CreativeOrderVariant): boolean {
  const delivered = creativeVariantDeliveryAssets(variant);
  const primedSizes = currentCreativeVariantSizeSet(variant, "primed");
  const reportByLane = new Map(creativeVariantQCDetails(variant).map((detail) => [detail.lane, detail.status]));
  const technical = reportByLane.get("technical") ?? "pending";
  const visual = reportByLane.get("visual") ?? "pending";
  if (creativeVariantUsesDirectEditNoQC(variant)) {
    return delivered.length === CREATIVE_DELIVERY_SIZES.length && variant.status === "completed";
  }
  return delivered.length === CREATIVE_DELIVERY_SIZES.length
    && primedSizes.size === CREATIVE_DELIVERY_SIZES.length
    && qcStatusAllowsAdoption(technical)
    && qcStatusAllowsAdoption(visual);
}

function creativeVariantHasPendingQCFinalize(variant: CreativeOrderVariant): boolean {
  const blocker = variant.action_required;
  if (!blocker || !creativeOrderWorkflowFailureIsQC(blocker.workflow)) return false;
  const detail = blocker.detail.toLowerCase();
  if (detail.includes("质检未通过") || detail.includes("blocking") || detail.includes("failed")) return false;
  return detail.includes("pending")
    || detail.includes("等待")
    || detail.includes("尚未完成")
    || detail.includes("未完成")
    || detail.includes("not complete")
    || detail.includes("not completed");
}

function creativeVariantHasQCFailure(variant: CreativeOrderVariant): boolean {
  return creativeVariantFailedQCLabels(variant).length > 0;
}

function creativeVariantFailedQCLabels(variant: CreativeOrderVariant): string[] {
  return creativeVariantQCDetails(variant)
    .filter((detail) => detail.status === "failed")
    .map((detail) => detail.label);
}

function creativeVariantPendingQCLabels(technical: string, visual: string): string[] {
  const labels: string[] = [];
  if (!qcStatusAllowsAdoption(technical)) labels.push(`技术质检${qcStatusLabel(technical)}`);
  if (!qcStatusAllowsAdoption(visual)) labels.push(`视觉质检${qcStatusLabel(visual)}`);
  return labels.length > 0 ? labels : ["质检结果同步中"];
}

function currentCreativeVariantSizeSet(variant: CreativeOrderVariant, stage: string): Set<string> {
  return new Set(variant.assets
    .filter((asset) => asset.revision === variant.revision && asset.stage === stage && asset.status === "completed" && asset.attachment_id)
    .map((asset) => asset.size_key)
    .filter((size) => CREATIVE_DELIVERY_SIZES.includes(size as (typeof CREATIVE_DELIVERY_SIZES)[number])));
}

export function creativeVariantRiskAdoptionReadiness(variant: CreativeOrderVariant): { allowed: boolean; status: string } {
  const failed = creativeVariantQCDetails(variant).filter((detail) => detail.status === "failed");
  if (failed.length === 0) return { allowed: false, status: "当前版本没有需要忽略的系统提醒" };
  const primedSizes = new Set(variant.assets
    .filter((asset) => asset.revision === variant.revision && asset.stage === "primed" && asset.status === "completed" && asset.attachment_id)
    .map((asset) => asset.size_key)
    .filter((size) => CREATIVE_DELIVERY_SIZES.includes(size as (typeof CREATIVE_DELIVERY_SIZES)[number])));
  if (primedSizes.size !== CREATIVE_DELIVERY_SIZES.length) {
    return { allowed: false, status: `系统提醒：品牌组件仅完成 ${primedSizes.size}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸，暂不可采用` };
  }
  return { allowed: true, status: `系统提醒：${failed.map((detail) => detail.label).join("、")}未通过` };
}

function qcStatusAllowsAdoption(status: string): boolean {
  return status === "passed" || status === "warning";
}

export function creativeVariantCanRetryQC(variant: CreativeOrderVariant): boolean {
  if (variant.qc_recovery_available !== true || variant.qc_recovery_used === true || variant.status !== "action_required") return false;
  const primedSizes = new Set(variant.assets
    .filter((asset) => asset.revision === variant.revision && asset.stage === "primed" && asset.status === "completed" && asset.attachment_id)
    .map((asset) => asset.size_key)
    .filter((size) => CREATIVE_DELIVERY_SIZES.includes(size as (typeof CREATIVE_DELIVERY_SIZES)[number])));
  if (primedSizes.size !== CREATIVE_DELIVERY_SIZES.length) return false;
  return creativeVariantQCDetails(variant).some((detail) => detail.status === "failed");
}

export function creativeVariantRetryAction(variant: CreativeOrderVariant): CreativeVariantRetryAction | null {
  if (creativeVariantCanRetryQC(variant)) {
    return { kind: "qc", taskId: "", label: "重新质检" };
  }
  const blocker = variant.action_required;
  if (!blocker?.retryable || !blocker.task_id || creativeOrderWorkflowFailureIsQC(blocker.workflow)) return null;
  if (variant.status !== "action_required" && variant.status !== "failed") return null;
  return { kind: "workflow", taskId: blocker.task_id, label: "重试此方案" };
}

function currentCreativeVariantQCReports(variant: CreativeOrderVariant): Map<"technical" | "visual", CreativeOrderQCReport> {
  const selected = new Map<"technical" | "visual", CreativeOrderQCReport>();
  for (const report of variant.qc_reports) {
    if (report.revision !== variant.revision || (report.lane !== "technical" && report.lane !== "visual")) continue;
    const current = selected.get(report.lane);
    if (!current || compareQCReports(report, current) > 0) selected.set(report.lane, report);
  }
  return selected;
}

function compareQCReports(left: CreativeOrderQCReport, right: CreativeOrderQCReport): number {
  return timestamp(left.updated_at) - timestamp(right.updated_at)
    || timestamp(left.created_at) - timestamp(right.created_at)
    || left.id.localeCompare(right.id);
}

function creativeVariantQCBlockingFailures(variant: CreativeOrderVariant, lane: "technical" | "visual", report: CreativeOrderQCReport): string[] {
  const blockingFailures = qcFindingMessages(report.findings?.blocking_failures);
  if (blockingFailures.length > 0 || report.status !== "failed") return blockingFailures;

  const findingFields = ["failures", "failure", "errors", "error", "issues", "issue", "reason", "message", "summary", "detail", "description"];
  const fromFindings = uniqueMessages(findingFields.flatMap((key) => qcFindingMessages(report.findings?.[key])))
    .filter((message) => !isPendingQCSyncMessage(message));
  if (fromFindings.length > 0) return fromFindings;

  const blocker = variant.action_required;
  if (blocker && (blocker.workflow === "creative_qc" || blocker.workflow === `creative_qc_${lane}`)) {
    const blockerDetail = businessQCMessage(blocker.detail);
    if (blockerDetail && !isPendingQCSyncMessage(blockerDetail)) return [blockerDetail];
  }

  return [lane === "technical"
    ? "技术质检没有同步具体失败明细；请人工复核文件完整性、品牌组件和渠道要求。"
    : "视觉质检没有同步具体失败明细；请人工复核文字可读性、遮挡、数值一致性和整体画面质量。"];
}

function qcFindingMessages(value: unknown): string[] {
  if (value === undefined || value === null) return [];
  const values = Array.isArray(value) ? value : [value];
  return uniqueMessages(values.map(qcFindingMessage).map(businessQCMessage).filter((message): message is string => Boolean(message)));
}

function qcFindingMessage(value: unknown): string {
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (!isRecord(value)) return "";

  const scope = firstString(value, ["size_key", "size", "region_id", "region"]);
  const message = firstString(value, ["summary", "message", "reason", "description", "detail", "issue", "code"]);
  if (scope && message) return `${scope}：${message}`;
  if (message) return message;
  if (scope) return scope;
  return "未提供可读详情";
}

function firstString(value: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const candidate = value[key];
    if (typeof candidate === "string" && candidate.trim()) return candidate.trim();
  }
  return "";
}

function uniqueMessages(messages: string[]): string[] {
  return [...new Set(messages.map((message) => message.trim()).filter(Boolean))];
}

function businessQCMessage(message: string): string {
  const text = message.trim();
  if (!text) return "";
  const mappedCode = businessQCCodeMessage(text);
  if (mappedCode) return mappedCode;
  if (/qc-finalize|technical lane|visual lane/i.test(text) && /pending|尚未完成|未完成|not complete|not completed/i.test(text) && !/failed|失败|质检未通过/i.test(text)) return "质检仍在同步中，等待另一条质检完成";
  if (/qc-finalize|technical lane|visual lane|action_required|failed/i.test(text)) return "质检未通过，需要人工确认当前成图是否可用";
  return text;
}

function businessQCCodeMessage(text: string): string {
  const normalized = text.toLowerCase();
  if (normalized.includes("attachment_download_primed_asset_failed")) return "品牌组件合成后的成图文件无法读取或下载，系统恢复后仍需人工确认可用性。";
  if (normalized.includes("manifest_layout_contract_missing")) return "成图缺少布局清单，无法确认三尺寸交付是否完整。";
  if (normalized.includes("visual_quality_failure")) return "视觉质检判断画面质量未达标，需要人工复核。";
  if (normalized.includes("corner_overlap")) return "四角品牌或商店区域疑似被画面内容遮挡。";
  if (normalized.includes("delegation_contract_missing_issue_id")) return "质检任务缺少订单关联信息，系统恢复后仍未同步完整结果。";
  return "";
}

function isPendingQCSyncMessage(message: string): boolean {
  return message === "质检仍在同步中，等待另一条质检完成";
}

function businessActionRequiredDetail(workflow: string | undefined, detail: string): string {
  if (workflow && creativeOrderWorkflowFailureIsQC(workflow)) return businessQCMessage(detail);
  return detail;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function timestamp(value: string | undefined): number {
  const parsed = Date.parse(value ?? "");
  return Number.isNaN(parsed) ? 0 : parsed;
}

function qcStatusLabel(status: string): string {
  if (status === "passed") return "通过";
  if (status === "warning") return "通过（有提醒）";
  if (status === "failed") return "失败";
  return "待完成";
}

function compareDeliveryAssets(left: CreativeOrderAsset, right: CreativeOrderAsset): number {
  const stageRank = (asset: CreativeOrderAsset) => asset.stage === "delivered" ? 2 : asset.stage === "primed" ? 1 : 0;
  return stageRank(left) - stageRank(right) || (Date.parse(left.updated_at) || 0) - (Date.parse(right.updated_at) || 0) || left.id.localeCompare(right.id);
}

function comparePreviewAssets(left: CreativeOrderAsset, right: CreativeOrderAsset): number {
  const stageRank = (asset: CreativeOrderAsset) => asset.stage === "delivered" ? 3 : asset.stage === "primed" ? 2 : asset.stage === "generated" ? 1 : 0;
  return stageRank(left) - stageRank(right) || compareDeliveryAssets(left, right);
}

async function downloadCreativeAttachment(attachment: DeliveryAttachment, filename: string): Promise<void> {
  const response = await fetch(creativeAttachmentBrowserURL(attachment), { credentials: "include" });
  if (!response.ok) throw new Error(`无法下载 ${filename}`);
  const bytes = new Uint8Array(await response.arrayBuffer());
  triggerBrowserDownload(URL.createObjectURL(new Blob([bytes], { type: response.headers.get("content-type") || attachment.content_type || "application/octet-stream" })), filenameForResponse(filename, response.headers.get("content-type"), bytes), true);
}

function triggerBrowserDownload(href: string, filename: string, revoke: boolean): void {
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  if (revoke) window.setTimeout(() => URL.revokeObjectURL(href), 1000);
}

function fileExtension(filename: string): string {
  const match = filename.match(/(\.[a-z0-9]{1,8})$/i);
  return match?.[1]?.toLowerCase() ?? "";
}

const CREATIVE_NAMING_TOKENS = new Set(["month", "kind", "brand", "market", "date", "type", "theme", "device", "designer", "size", "duration"]);
const ADAKAMI_IMAGE_NAMING_RULE = "{month}_P_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}";
const ADAKAMI_VIDEO_NAMING_RULE = "{month}_V_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}_{duration}";
const CREATIVE_SIZE_ABBREVIATIONS: Record<string, string> = {
  "1080x1080": "11",
  "1920x1080": "169",
  "1200x628": "191",
  "1080x1920": "916",
  "800x1000": "45",
};

function creativeDeliveryFilename({
  order,
  item,
  asset,
  attachment,
  usedNames,
}: {
  order?: CreativeDeliveryNamingContext;
  item?: Pick<CreativeOrderItem, "candidate_id" | "copy_snapshot">;
  asset: CreativeOrderAsset;
  attachment: DeliveryAttachment;
  usedNames: Set<string>;
}): string {
  const config = creativeOrderNamingConfig(order);
  const defaults = isRecord(config.naming_defaults) ? config.naming_defaults : {};
  const copySnapshot = isRecord(item?.copy_snapshot) ? item.copy_snapshot : {};
  const visualDirection = isRecord(copySnapshot.visual_direction) ? copySnapshot.visual_direction : {};
  const video = creativeAttachmentIsVideo(attachment);
  const generatedAt = asset.created_at || asset.updated_at || order?.created_at;
  const generatedDate = creativeNamingDate(generatedAt);
  const values: Record<string, string> = {
    month: generatedDate.month,
    kind: video ? "V" : "P",
    brand: creativeNamingAbbreviation(
      firstString(config, ["brand_abbreviation", "brand_code"]) || firstString(defaults, ["brand_abbreviation", "brand"]),
      firstString(config, ["brand"]) || "AdaKami",
      "AK",
    ),
    market: creativeNamingAbbreviation(
      firstString(config, ["market_abbreviation", "market_code"]) || firstString(defaults, ["market_abbreviation", "market"]),
      firstString(config, ["market"]) || "",
      "MY",
    ),
    date: generatedDate.date,
    type: creativeNamingType(copySnapshot),
    theme: firstString(visualDirection, ["theme", "campaign", "activity"]) || firstString(copySnapshot, ["theme", "campaign", "activity"]),
    device: firstString(config, ["naming_device", "device", "model", "machine"])
      || firstString(defaults, ["device", "model", "machine"])
      || "SX",
    designer: firstString(config, ["naming_designer", "designer", "creator"])
      || firstString(defaults, ["designer", "creator"])
      || "AI",
    size: creativeNamingSize(asset.size_key, config),
    duration: creativeNamingDuration(asset, video),
  };
  const rule = creativeOrderNamingRule(order, video);
  const rendered = renderCreativeNamingRule(rule, values);
  const filename = safeArchiveName(rendered) + attachmentExtension(attachment);
  return uniqueArchiveFilename(filename, usedNames, asset);
}

function creativeOrderNamingConfig(order?: CreativeDeliveryNamingContext): Record<string, unknown> {
  const snapshot = order?.input_snapshot;
  if (!isRecord(snapshot) || !isRecord(snapshot.market_pack) || !isRecord(snapshot.market_pack.config)) return {};
  return snapshot.market_pack.config;
}

function creativeOrderNamingRule(order: CreativeDeliveryNamingContext | undefined, video: boolean): string {
  const config = creativeOrderNamingConfig(order);
  const configured = video
    ? firstString(config, ["video_naming_rule", "naming_video_rule"])
    : firstString(config, ["image_naming_rule", "naming_rule"]);
  if (creativeNamingRuleIsValid(configured, video)) return configured;
  return video ? ADAKAMI_VIDEO_NAMING_RULE : ADAKAMI_IMAGE_NAMING_RULE;
}

function creativeNamingRuleIsValid(rule: string, video: boolean): boolean {
  const tokens = Array.from(rule.matchAll(/\{([^{}]+)\}/g), (match) => match[1]).filter((token): token is string => Boolean(token));
  const requiredTokens = video ? ["date", "type", "size", "duration"] : ["date", "type", "size"];
  return Boolean(rule && tokens.length > 0 && requiredTokens.every((token) => tokens.includes(token)) && tokens.every((token) => CREATIVE_NAMING_TOKENS.has(token)));
}

function creativeNamingDate(createdAt: string | undefined): { month: string; date: string } {
  const date = new Date(createdAt || "");
  if (Number.isNaN(date.getTime())) return { month: "", date: "" };
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCFullYear()) + month + String(date.getUTCDate()).padStart(2, "0");
  return { month, date: day };
}

function creativeNamingType(copySnapshot: Record<string, unknown>): string {
  const raw = firstString(copySnapshot, ["naming_type", "type", "creative_type"]);
  if (!raw) return "";
  const normalized = raw.replace(/[\s-]+/g, "_").toUpperCase();
  return normalized === "REPAYMENT_PLAN" ? "NUM" : normalized;
}

function creativeNamingSize(size: string, config: Record<string, unknown>): string {
  const configured = [config.naming_size_abbreviations, config.size_abbreviations, config.size_aliases]
    .map((value) => isRecord(value) ? value[size] : undefined)
    .find((value): value is string => typeof value === "string" && Boolean(value.trim()));
  return configured?.trim() || CREATIVE_SIZE_ABBREVIATIONS[size] || size;
}

function creativeNamingDuration(asset: CreativeOrderAsset, video: boolean): string {
  if (!video) return "";
  const sources = [asset.metadata, asset.evidence].filter(isRecord);
  for (const source of sources) {
    const value = firstNamingScalar(source, ["duration_seconds", "video_duration_seconds", "duration"]);
    if (value) return value;
    const nested = ["video", "media", "model_result"].map((key) => source[key]).filter(isRecord);
    for (const record of nested) {
      const nestedValue = firstNamingScalar(record, ["duration_seconds", "video_duration_seconds", "duration"]);
      if (nestedValue) return nestedValue;
    }
  }
  return "";
}

function firstNamingScalar(record: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "number" && Number.isFinite(value)) return String(Number(value.toFixed(3)));
    if (typeof value === "string" && value.trim()) {
      const match = value.trim().match(/^(?:(\d+):)?(\d+(?:\.\d+)?)$/);
      if (match) return match[1] ? String(Number(match[1]) * 60 + Number(match[2])) : match[2]!;
      return value.trim();
    }
  }
  return "";
}

function creativeNamingAbbreviation(explicit: string, source: string, fallback: string): string {
  const value = explicit.trim() || source.trim();
  if (!value) return fallback;
  const normalized = value.toUpperCase().replace(/[^A-Z0-9]+/g, " ").trim();
  if (normalized === "ADAKAMI") return "AK";
  if (normalized === "MALAYSIA") return "MY";
  if (/^[A-Z0-9]{2,4}$/.test(normalized)) return normalized;
  const initials = normalized.split(/\s+/).map((part) => part[0]).join("");
  return initials.slice(0, 4) || fallback;
}

function renderCreativeNamingRule(rule: string, values: Record<string, string>): string {
  const baseRule = rule.replace(/\.[a-z0-9]{1,8}$/i, "");
  return baseRule
    .replace(/\{([^{}]+)\}/g, (_, token: string) => values[token] || "")
    .split("_")
    .map((part) => part.trim())
    .filter(Boolean)
    .join("_");
}

function contentTypeExtension(contentType: string | undefined): string {
  const normalized = ((contentType || "").split(";", 1)[0] || "").trim().toLowerCase();
  return ({
    "image/png": ".png",
    "image/jpeg": ".jpg",
    "image/jpg": ".jpg",
    "image/webp": ".webp",
    "image/gif": ".gif",
    "image/avif": ".avif",
    "image/svg+xml": ".svg",
    "video/mp4": ".mp4",
    "application/mp4": ".mp4",
    "video/quicktime": ".mov",
    "video/webm": ".webm",
  } as Record<string, string>)[normalized] || "";
}

function attachmentExtension(attachment: DeliveryAttachment): string {
  return contentTypeExtension(attachment.content_type) || fileExtension(attachment.filename) || ".bin";
}

function creativeAttachmentIsVideo(attachment: DeliveryAttachment): boolean {
  return (attachment.content_type || "").trim().toLowerCase().startsWith("video/")
    || [".mp4", ".mov", ".webm", ".avi"].includes(fileExtension(attachment.filename));
}

function binaryExtension(bytes: Uint8Array): string {
  if (bytes.length >= 8 && bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47 && bytes[4] === 0x0d && bytes[5] === 0x0a && bytes[6] === 0x1a && bytes[7] === 0x0a) return ".png";
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) return ".jpg";
  if (bytes.length >= 6 && String.fromCharCode(...bytes.slice(0, 6)) === "GIF89a") return ".gif";
  if (bytes.length >= 12 && String.fromCharCode(...bytes.slice(0, 4)) === "RIFF" && String.fromCharCode(...bytes.slice(8, 12)) === "WEBP") return ".webp";
  if (bytes.length >= 12 && String.fromCharCode(...bytes.slice(4, 8)) === "ftyp") return ".mp4";
  return "";
}

function replaceFileExtension(filename: string, extension: string): string {
  return fileExtension(filename) ? filename.slice(0, -fileExtension(filename).length) + extension : `${filename}${extension}`;
}

function filenameForResponse(filename: string, contentType: string | null, bytes: Uint8Array): string {
  return replaceFileExtension(filename, binaryExtension(bytes) || contentTypeExtension(contentType || undefined) || fileExtension(filename) || ".bin");
}

function uniqueArchiveFilename(filename: string, usedNames: Set<string>, asset?: CreativeOrderAsset): string {
  if (!usedNames.has(filename)) {
    usedNames.add(filename);
    return filename;
  }
  const extension = fileExtension(filename);
  const stem = extension ? filename.slice(0, -extension.length) : filename;
  const suffix = safeArchiveName(asset?.size_key || "asset");
  let candidate = `${stem}_${suffix}${extension}`;
  if (usedNames.has(candidate)) candidate = `${stem}_${safeArchiveName(asset?.id.slice(0, 8) || "asset")}${extension}`;
  usedNames.add(candidate);
  return candidate;
}

function safeArchiveName(value: string): string {
  const printable = [...value].map((character) => character.charCodeAt(0) < 32 ? "_" : character).join("");
  return printable.replace(/[<>:"/\\|?*]/g, "_").replace(/\s+/g, " ").trim() || "creative-assets";
}
