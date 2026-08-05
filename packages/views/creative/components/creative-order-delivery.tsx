"use client";

import { useId, useState } from "react";
import { Check, CheckCircle2, ChevronDown, Download, Image as ImageIcon, PackageCheck, RefreshCw } from "lucide-react";
import { strToU8, zipSync } from "fflate";
import type { Attachment, CreativeOrder, CreativeOrderAsset, CreativeOrderItem, CreativeOrderQCReport, CreativeOrderVariant } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
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
    case "action_required": return "需要处理";
    case "failed": return "失败";
    case "awaiting_adoption": return "待采用";
    case "completed": return "已完成";
    case "cancelled": return "已结束";
    default: return status ? "处理中" : "加载中";
  }
}

const CREATIVE_DELIVERY_SIZE_LABELS: Record<(typeof CREATIVE_DELIVERY_SIZES)[number], string> = {
  "1080x1080": "方形",
  "1200x628": "横版",
  "800x1000": "竖版",
};

type DeliveryAttachment = Pick<Attachment, "id" | "filename" | "url" | "download_url" | "markdown_url">;

export function adoptedCreativeOrderVariant(item: CreativeOrderItem): CreativeOrderVariant | undefined {
  const adoptedVariantId = item.adopted_variant_id;
  if (!adoptedVariantId) return undefined;
  return item.variants.find((variant) => variant.id === adoptedVariantId);
}

export function creativeOrderStage(order: CreativeOrder | undefined): CreativeOrderStage {
  const items = order?.items ?? [];
  const variants = items.flatMap((item) => item.variants);
  const readyVariants = variants.filter((variant) => creativeVariantAdoptionReadiness(variant).ready).length;
  const adoptedItems = items.filter((item) => Boolean(adoptedCreativeOrderVariant(item))).length;
  const base = { readyVariants, totalVariants: variants.length, adoptedItems, totalItems: items.length };
  const status = order?.derived_status || order?.status || "";

  if (status === "cancelled" || order?.status === "cancelled") {
    return { ...base, key: "cancelled", label: "已结束", detail: "订单已结束，结果与记录仍保留", action: "查看记录" };
  }
  if (status === "completed" || (items.length > 0 && adoptedItems === items.length)) {
    return { ...base, key: "delivered", label: "已交付", detail: items.length > 0 ? `${adoptedItems}/${items.length} 个素材已采用` : "交付已完成", action: "查看并下载" };
  }
  if (readyVariants > adoptedItems) {
    return { ...base, key: "review", label: "待验收", detail: `${readyVariants} 个变体可以比较`, action: "选择最终方案" };
  }
  if (status === "awaiting_adoption") {
    return { ...base, key: "review", label: "待验收", detail: "已有可采用方案，等待选择", action: "选择最终方案" };
  }
  if ((order?.workflow_failures?.length ?? 0) > 0 || status === "action_required" || status === "failed") {
    const failureCount = order?.workflow_failures?.length ?? 0;
    return { ...base, key: "attention", label: "需要处理", detail: failureCount > 0 ? `${failureCount} 个步骤失败` : "自动流程已停止，等待人工决定", action: "处理订单" };
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

export function creativeVariantArchiveEntries(
  variant: CreativeOrderVariant,
  attachments: Map<string, DeliveryAttachment>,
): { asset: CreativeOrderAsset; attachment: DeliveryAttachment; filename: string }[] {
  return creativeVariantDeliveryAssets(variant).flatMap((asset) => {
    const attachment = attachments.get(asset.attachment_id);
    if (!attachment || !creativeAttachmentBrowserURL(attachment)) return [];
    const extension = fileExtension(attachment.filename) || ".png";
    return [{
      asset,
      attachment,
      filename: `${safeArchiveName(variant.variant_key || variant.id)}_${safeArchiveName(asset.size_key)}${extension}`,
    }];
  });
}

export async function downloadCreativeVariantArchive({
  orderId,
  item,
  variant,
  attachments,
}: {
  orderId: string;
  item: CreativeOrderItem;
  variant: CreativeOrderVariant;
  attachments: Map<string, DeliveryAttachment>;
}): Promise<void> {
  const entries = creativeVariantArchiveEntries(variant, attachments);
  if (entries.length !== CREATIVE_DELIVERY_SIZES.length) throw new Error("交付包的三张图片尚未齐备");
  const files: Record<string, Uint8Array> = {};
  const downloaded = await Promise.all(entries.map(async ({ attachment, filename }) => {
    const response = await fetch(creativeAttachmentBrowserURL(attachment), { credentials: "include" });
    if (!response.ok) throw new Error(`无法下载 ${filename}`);
    return { filename, bytes: new Uint8Array(await response.arrayBuffer()) };
  }));
  for (const { filename, bytes } of downloaded) files[filename] = bytes;
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
  source,
  attachments,
  adoptingVariantId,
  retryingQCVariantId = "",
  repairingPrimeVariantId = "",
  onAdopt,
  onRetryQC,
  onRepairPrime,
  onAssetSelect,
  disabled = false,
}: {
  orderId: string;
  item: CreativeOrderItem;
  source: { label: string; url: string };
  attachments: Map<string, DeliveryAttachment>;
	adoptingVariantId: string;
	retryingQCVariantId?: string;
	repairingPrimeVariantId?: string;
  onAdopt: (variantId: string) => void;
	onRetryQC?: (variantId: string) => void;
	onRepairPrime?: (variantId: string) => void;
  onAssetSelect: (assetId: string) => void;
  disabled?: boolean;
}) {
  const adoptedVariant = adoptedCreativeOrderVariant(item);
  const otherVariants = adoptedVariant ? item.variants.filter((variant) => variant.id !== adoptedVariant.id) : [];

  return <section className="border bg-background" data-testid={`creative-order-item-${item.id}`}>
    <div className="flex flex-wrap items-start justify-between gap-3 border-b px-4 py-3">
      <div className="min-w-0">
        <p className="break-words text-sm font-semibold">{item.direction || `素材 ${item.candidate_id.slice(0, 8)}`}</p>
        <p className="mt-1 text-xs text-muted-foreground">每个素材条目只能采用一个变体，重新采用会替换当前方案。</p>
      </div>
      <Badge variant={adoptedVariant ? "default" : "secondary"}>{adoptedVariant ? `已采用 ${adoptedVariant.variant_key}` : "待选择"}</Badge>
    </div>

    {adoptedVariant ? <>
      <AdoptedVariantDelivery
        orderId={orderId}
        item={item}
        variant={adoptedVariant}
        source={source}
        attachments={attachments}
        onAssetSelect={onAssetSelect}
      />
      {otherVariants.length > 0 && <details className="group border-t bg-muted/10">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-medium marker:content-none">
          <ChevronDown className="h-4 w-4 transition-transform group-open:rotate-180" />
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
            retryingQCVariantId={retryingQCVariantId}
            repairingPrimeVariantId={repairingPrimeVariantId}
            onAdopt={onAdopt}
            onRetryQC={onRetryQC}
            onRepairPrime={onRepairPrime}
            onAssetSelect={onAssetSelect}
            disabled={disabled}
            subdued
          />)}
        </div>
      </details>}
    </> : <div className="grid gap-3 p-4 lg:grid-cols-3">
      {item.variants.map((variant) => <VariantCandidate
        key={variant.id}
        variant={variant}
        attachments={attachments}
        adoptedVariantId=""
        adoptingVariantId={adoptingVariantId}
        retryingQCVariantId={retryingQCVariantId}
        repairingPrimeVariantId={repairingPrimeVariantId}
        onAdopt={onAdopt}
        onRetryQC={onRetryQC}
        onRepairPrime={onRepairPrime}
        onAssetSelect={onAssetSelect}
        disabled={disabled}
      />)}
    </div>}
  </section>;
}

function AdoptedVariantDelivery({
  orderId,
  item,
  variant,
  source,
  attachments,
  onAssetSelect,
}: {
  orderId: string;
  item: CreativeOrderItem;
  variant: CreativeOrderVariant;
  source: { label: string; url: string };
  attachments: Map<string, DeliveryAttachment>;
  onAssetSelect: (assetId: string) => void;
}) {
  const [archiveBusy, setArchiveBusy] = useState(false);
  const [archiveError, setArchiveError] = useState("");
  const delivered = creativeVariantDeliveryAssets(variant);
  const entries = creativeVariantArchiveEntries(variant, attachments);
  const downloadArchive = async () => {
    setArchiveBusy(true);
    setArchiveError("");
    try {
      await downloadCreativeVariantArchive({ orderId, item, variant, attachments });
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
    <div className="grid border-t xl:grid-cols-[minmax(260px,0.8fr)_minmax(0,2.2fr)]">
      <figure className="min-w-0 border-b bg-background xl:border-b-0 xl:border-r">
        <figcaption className="border-b px-3 py-2 text-xs font-medium">原图 · {source.label}</figcaption>
        <div className="flex min-h-72 items-center justify-center p-3">
          {source.url ? <img src={source.url} alt={`原图 ${source.label}`} width={1200} height={1200} loading="lazy" className="max-h-[480px] w-full object-contain" /> : <EmptyImage label="原图不可用" />}
        </div>
      </figure>
      <div className="grid min-w-0 md:grid-cols-3">
        {CREATIVE_DELIVERY_SIZES.map((size, index) => {
          const asset = delivered.find((candidate) => candidate.size_key === size);
          const attachment = asset ? attachments.get(asset.attachment_id) : undefined;
          return <DeliveryAssetPane key={size} asset={asset} attachment={attachment} size={size} onAssetSelect={onAssetSelect} divided={index > 0} />;
        })}
      </div>
    </div>
  </div>;
}

function VariantCandidate({
  variant,
  attachments,
  adoptedVariantId,
  adoptingVariantId,
  retryingQCVariantId,
  repairingPrimeVariantId,
  onAdopt,
  onRetryQC,
  onRepairPrime,
  onAssetSelect,
  disabled = false,
  subdued = false,
}: {
  variant: CreativeOrderVariant;
  attachments: Map<string, DeliveryAttachment>;
  adoptedVariantId: string;
	adoptingVariantId: string;
	retryingQCVariantId: string;
	repairingPrimeVariantId: string;
  onAdopt: (variantId: string) => void;
	onRetryQC?: (variantId: string) => void;
	onRepairPrime?: (variantId: string) => void;
  onAssetSelect: (assetId: string) => void;
  disabled?: boolean;
  subdued?: boolean;
}) {
  const readiness = creativeVariantAdoptionReadiness(variant);
  const qcDetails = creativeVariantQCDetails(variant);
  const descriptionId = useId();
  const previews = creativeVariantPreviewAssets(variant);
  const cover = previews.find((asset) => asset.size_key === "1080x1080") ?? previews[0];
  const coverURL = cover ? creativeAttachmentBrowserURL(attachments.get(cover.attachment_id)) : "";
  const adopted = adoptedVariantId === variant.id;
  const busy = adoptingVariantId === variant.id;
  const canRepairPrime = Boolean(onRepairPrime) && !disabled && creativeVariantCanRepairPrime(variant);
  const canRetryQC = Boolean(onRetryQC) && !disabled && !canRepairPrime && creativeVariantCanRetryQC(variant);
  const qcRecoveryExhausted = !canRepairPrime && !disabled && variant.status === "action_required" && variant.qc_recovery_used === true;
  const qcRetryBusy = retryingQCVariantId === variant.id;
  const primeRepairExhausted = !disabled && variant.status === "action_required" && variant.prime_repair_used === true;
  const primeRepairBusy = repairingPrimeVariantId === variant.id;
  return <article className={cn("flex min-w-0 flex-col border bg-background", subdued && "opacity-75 transition-opacity hover:opacity-100")}>
    <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
      <div className="flex items-center gap-2"><span className="text-sm font-semibold">{variant.variant_key || variant.id.slice(0, 8)}</span><Badge variant="outline">r{variant.revision}</Badge></div>
      {readiness.ready ? <Badge variant="outline"><CheckCircle2 className="h-3 w-3" />QC 通过</Badge> : <Badge variant="secondary">尚未完成</Badge>}
    </div>
    <button type="button" disabled={!cover || !coverURL} onClick={() => cover && onAssetSelect(cover.id)} className="group flex min-h-72 w-full items-center justify-center border-b bg-muted/10 p-3 disabled:cursor-default">
      {coverURL ? <img src={coverURL} alt={`${variant.variant_key} 方形主预览`} width={720} height={720} loading="lazy" className="max-h-[420px] w-full object-contain transition-transform group-hover:scale-[1.01]" /> : <EmptyImage label="待成图" />}
    </button>
    <div className="mt-auto space-y-2 p-3">
      <div className="grid grid-cols-3 divide-x border text-center text-[11px] text-muted-foreground">{CREATIVE_DELIVERY_SIZES.map((size) => <button key={size} type="button" disabled={!previews.some((asset) => asset.size_key === size)} onClick={() => { const asset = previews.find((candidate) => candidate.size_key === size); if (asset) onAssetSelect(asset.id); }} className="px-2 py-2 disabled:opacity-50"><span className="block font-medium text-foreground">{CREATIVE_DELIVERY_SIZE_LABELS[size]}</span><span>{previews.some((asset) => asset.size_key === size) ? "可查看" : "待生成"}</span></button>)}</div>
      <p id={descriptionId} className={cn("min-h-8 text-xs", readiness.ready ? "text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>{readiness.status}</p>
      <VariantQCDetails revision={variant.revision} details={qcDetails} />
      {primeRepairExhausted && <p role="status" className="border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200">Prime 包已修复一次，需人工处理</p>}
      {canRepairPrime && <Button className="w-full" size="sm" variant="outline" disabled={primeRepairBusy || Boolean(repairingPrimeVariantId) || Boolean(retryingQCVariantId)} onClick={() => onRepairPrime?.(variant.id)}>
        <RefreshCw className={cn("h-4 w-4", primeRepairBusy && "animate-spin")} />
        {primeRepairBusy ? "正在修复 Prime 包" : "修复 Prime 包并重新质检"}
      </Button>}
      {qcRecoveryExhausted && <p role="status" className="border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200">双路 QC 恢复不可用，需修复 Prime 包或人工处理</p>}
      {canRetryQC && <Button className="w-full" size="sm" variant="outline" disabled={qcRetryBusy || Boolean(retryingQCVariantId)} onClick={() => onRetryQC?.(variant.id)}>
        <RefreshCw className={cn("h-4 w-4", qcRetryBusy && "animate-spin")} />
        {qcRetryBusy ? "正在重新执行双路质检" : "重新执行双路质检"}
      </Button>}
      <Button className="w-full" size="sm" variant={adopted ? "secondary" : "default"} disabled={disabled || !readiness.ready || adopted || Boolean(adoptingVariantId)} aria-describedby={descriptionId} onClick={() => onAdopt(variant.id)}>
        {adopted ? <CheckCircle2 className="h-4 w-4" /> : <Check className="h-4 w-4" />}
        {disabled ? "订单已结束" : busy ? "正在采用" : adopted ? "当前采用" : "采用此变体"}
      </Button>
    </div>
  </article>;
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
    const blockingFailures = qcFindingMessages(report.findings?.blocking_failures);
    return [{
      lane,
      label: lane === "technical" ? "技术质检" : "视觉质检",
      status: blockingFailures.length > 0 ? "failed" : report.status,
      blockingFailures,
      qualityWarnings: qcFindingMessages(report.findings?.quality_warnings),
    }];
  });
}

function VariantQCDetails({ revision, details }: { revision: number; details: CreativeVariantQCDetail[] }) {
  const visible = details.filter((detail) =>
    detail.status === "failed" || detail.blockingFailures.length > 0 || detail.qualityWarnings.length > 0,
  );
  if (visible.length === 0) return null;
  const hasFailure = visible.some((detail) => detail.status === "failed");

  return <div
    className={cn("space-y-3 border-l-2 pl-3 text-xs", hasFailure ? "border-destructive" : "border-border")}
    role={hasFailure ? "alert" : "status"}
    data-testid="creative-variant-qc-details"
  >
    {visible.map((detail) => <div key={detail.lane} className="space-y-1.5">
      <p className={cn("font-medium", detail.status === "failed" && "text-destructive")}>
        {detail.status === "failed" ? `${detail.label}未通过 · r${revision}` : `${detail.label}提醒 · r${revision}`}
      </p>
      {detail.blockingFailures.length > 0 ? <QCMessageList label="阻断原因" messages={detail.blockingFailures} /> : detail.status === "failed" ? <p className="text-muted-foreground">报告未提供具体失败原因</p> : null}
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
  onAssetSelect,
  divided,
}: {
  asset?: CreativeOrderAsset;
  attachment?: DeliveryAttachment;
  size: (typeof CREATIVE_DELIVERY_SIZES)[number];
  onAssetSelect: (assetId: string) => void;
  divided: boolean;
}) {
  const url = creativeAttachmentBrowserURL(attachment);
  return <figure className={cn("min-w-0 bg-background", divided && "border-t md:border-l md:border-t-0")}>
    <figcaption className="flex items-center justify-between gap-2 border-b px-3 py-2">
      <span className="text-xs font-medium">{CREATIVE_DELIVERY_SIZE_LABELS[size]} · {size}</span>
      {url && attachment && <Button size="icon-sm" variant="ghost" title={`下载 ${size}`} aria-label={`下载 ${size}`} onClick={() => void downloadCreativeAttachment(attachment, `${size}${fileExtension(attachment.filename) || ".png"}`).catch((error: unknown) => toast.error(error instanceof Error ? error.message : "无法下载交付图"))}><Download className="h-4 w-4" /></Button>}
    </figcaption>
    <button type="button" disabled={!asset || !url} onClick={() => asset && onAssetSelect(asset.id)} className="flex min-h-72 w-full items-center justify-center p-3 disabled:cursor-default">
      {url ? <img src={url} alt={`最终采用方案 ${size}`} width={1200} height={1200} loading="lazy" className="max-h-[480px] w-full object-contain" /> : <EmptyImage label={`${size} 尚未交付`} />}
    </button>
  </figure>;
}

function EmptyImage({ label }: { label: string }) {
  return <span className="flex min-h-40 w-full flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />{label}</span>;
}

export function creativeVariantAdoptionReadiness(variant: CreativeOrderVariant): { ready: boolean; status: string } {
  const delivered = creativeVariantDeliveryAssets(variant);
  const primedSizes = new Set(variant.assets
    .filter((asset) => asset.revision === variant.revision && asset.stage === "primed" && asset.status === "completed" && asset.attachment_id)
    .map((asset) => asset.size_key)
    .filter((size) => CREATIVE_DELIVERY_SIZES.includes(size as (typeof CREATIVE_DELIVERY_SIZES)[number])));
  const reportByLane = new Map(creativeVariantQCDetails(variant).map((detail) => [detail.lane, detail.status]));
  const technical = reportByLane.get("technical") ?? "pending";
  const visual = reportByLane.get("visual") ?? "pending";
  if (technical === "failed" || visual === "failed") return { ready: false, status: `QC 未通过：technical ${qcStatusLabel(technical)}，visual ${qcStatusLabel(visual)}` };
  if (!qcStatusAllowsAdoption(technical) || !qcStatusAllowsAdoption(visual)) return { ready: false, status: `等待双路 QC：technical ${qcStatusLabel(technical)}，visual ${qcStatusLabel(visual)}` };
  if (primedSizes.size !== CREATIVE_DELIVERY_SIZES.length) return { ready: false, status: `等待 Prime：已完成 ${primedSizes.size}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  if (delivered.length !== CREATIVE_DELIVERY_SIZES.length) return { ready: false, status: `等待正式交付：已完成 ${delivered.length}/${CREATIVE_DELIVERY_SIZES.length} 个尺寸` };
  if (variant.status !== "completed") return { ready: false, status: `等待变体完成：当前状态 ${variant.status}` };
  return { ready: true, status: "三尺寸、Prime 与双路 QC 均已完成，可以采用" };
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

export function creativeVariantCanRepairPrime(variant: CreativeOrderVariant): boolean {
  if (variant.prime_repair_available !== true || variant.prime_repair_used === true || variant.status !== "action_required") return false;
  const generatedSizes = new Set(variant.assets
    .filter((asset) => asset.revision === variant.revision && asset.stage === "generated" && asset.status === "completed")
    .map((asset) => asset.size_key)
    .filter((size) => CREATIVE_DELIVERY_SIZES.includes(size as (typeof CREATIVE_DELIVERY_SIZES)[number])));
  return generatedSizes.size === CREATIVE_DELIVERY_SIZES.length;
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

function qcFindingMessages(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.map(qcFindingMessage).filter((message): message is string => Boolean(message)))];
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
  triggerBrowserDownload(URL.createObjectURL(await response.blob()), filename, true);
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

function safeArchiveName(value: string): string {
  const printable = [...value].map((character) => character.charCodeAt(0) < 32 ? "_" : character).join("");
  return printable.replace(/[<>:"/\\|?*]/g, "_").replace(/\s+/g, " ").trim() || "creative-assets";
}
