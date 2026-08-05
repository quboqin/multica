"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowUpRight, CheckCircle2, Layers3, RefreshCw } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeFeedbackOptions, creativeMaterialLibraryOptions, creativeOrderOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { CreateCreativeFeedbackResponse, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { Badge } from "@multica/ui/components/ui/badge";
import { adoptedCreativeOrderVariant, CREATIVE_DELIVERY_SIZES, creativeOrderStatusLabel, creativeVariantDeliveryAssets } from "../../creative/components/creative-order-delivery";
import { creativeAttachmentBrowserURL } from "../../creative/lib/creative-attachment-url";
import { creativeAdjustmentProgress, creativeAdjustmentTarget, creativeAdjustmentTimeline, latestOrderAdjustmentFeedback } from "../../creative/lib/creative-adjustment-progress";
import { AppLink } from "../../navigation";

export function creativeOrderSummarySelections(items: CreativeOrderItem[], directEdit = false): { item: CreativeOrderItem; variant: CreativeOrderVariant }[] {
  return items.flatMap((item) => {
    const variant = adoptedCreativeOrderVariant(item)
      ?? (directEdit && item.variants.length === 1 && item.variants[0]?.status === "completed" ? item.variants[0] : undefined);
    return variant ? [{ item, variant }] : [];
  });
}

export function CreativeOrderSummary({ orderId }: { orderId: string }) {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const order = useQuery(creativeOrderOptions(wsId, orderId));
  const feedback = useQuery(creativeFeedbackOptions(wsId, "asset"));
  const library = useQuery(creativeMaterialLibraryOptions(wsId));
  const data = order.data;
  const variants = data?.items.flatMap((item) => item.variants) ?? [];
  const latestAdjustment = latestOrderAdjustmentFeedback(feedback.data?.events ?? [], orderId);
  const adjustmentTarget = creativeAdjustmentTarget(data?.items ?? [], latestAdjustment);
  const adjustmentVariant = adjustmentTarget?.variant;
  const adjustmentItem = adjustmentTarget?.item;
  const directEdit = data?.trigger_evidence_kind === "creative_direct_edit";
  const selections = useMemo(() => creativeOrderSummarySelections(data?.items ?? [], directEdit), [data?.items, directEdit]);
  const deliveryAssets = useMemo(() => selections.flatMap(({ variant }) => creativeVariantDeliveryAssets(variant)), [selections]);
  const attachmentIds = useMemo(() => [...new Set(deliveryAssets.map((asset) => asset.attachment_id).filter(Boolean))], [deliveryAssets]);
  const attachments = useQuery({
    queryKey: ["creative", wsId, "order-summary-attachments", orderId, attachmentIds],
    queryFn: () => Promise.all(attachmentIds.map((id) => api.getAttachment(id))),
    enabled: attachmentIds.length > 0,
  });
  const attachmentById = new Map((attachments.data ?? []).map((attachment) => [attachment.id, attachment]));
  const allItemsAdopted = Boolean(data?.items.length) && selections.length === data?.items.length;
  const orderHref = data?.issue_id ? `${paths.creativeOrder(orderId)}&fromIssue=${encodeURIComponent(data.issue_id)}` : paths.creativeOrder(orderId);

  return <section className="my-5 border bg-background" data-testid="creative-order-summary">
    <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
      <div className="flex items-center gap-2"><Layers3 className="h-4 w-4 text-emerald-700" /><span className="text-sm font-semibold">创意订单</span><Badge variant="outline">{creativeOrderStatusLabel(data?.derived_status || data?.status)}</Badge>{!directEdit && <Badge variant={allItemsAdopted ? "default" : "secondary"}>{allItemsAdopted ? "已采用" : "待选择"}</Badge>}</div>
      <AppLink href={orderHref} className="inline-flex items-center gap-1 text-xs font-medium underline-offset-4 hover:underline">查看订单<ArrowUpRight className="h-3.5 w-3.5" /></AppLink>
    </div>
    <div className="grid gap-3 px-4 py-3 text-xs sm:grid-cols-3">
      <div><p className="text-muted-foreground">素材条目</p><p className="mt-1 font-semibold">{data?.items.length ?? 0}</p></div>
      <div><p className="text-muted-foreground">创意变体</p><p className="mt-1 font-semibold">{variants.length}</p></div>
      <div><p className="text-muted-foreground">{directEdit ? "交付包" : "已采用方案"}</p><p className="mt-1 font-semibold">{selections.length}/{data?.items.length ?? 0}</p></div>
    </div>
    {latestAdjustment && <CreativeAdjustmentTrace event={latestAdjustment} variant={adjustmentVariant} item={adjustmentItem} />}
    {selections.length === 0 && !order.isLoading && <div className="border-t px-4 py-6 text-center"><p className="text-sm font-medium">{directEdit ? "正在等待交付包" : "尚未选择最终方案"}</p><p className="mt-1 text-xs text-muted-foreground">{directEdit ? "直接改图完成后会在这里展示结果。" : "请在创意工厂比较三个候选变体，并为每个素材条目采用一个方案。"}</p></div>}
    {selections.map(({ item, variant }) => {
      const candidate = library.data?.candidates.find((entry) => entry.id === item.candidate_id);
      const sourceUrl = resolvePublicFileUrl(candidate?.archived_url || candidate?.preview_url);
      const delivered = creativeVariantDeliveryAssets(variant);
      return <div key={item.id} className="border-t" data-testid={`creative-summary-adopted-${variant.id}`}>
        <div className="flex flex-wrap items-center gap-2 px-4 py-3"><CheckCircle2 className="h-4 w-4 text-emerald-700" /><p className="text-sm font-medium">{directEdit ? "交付包" : "最终采用方案 · 交付包"}</p><Badge variant="outline">{variant.variant_key}</Badge><span className="min-w-0 truncate text-xs text-muted-foreground">{item.direction || candidate?.title || item.candidate_id.slice(0, 8)}</span></div>
        <div className="grid border-t sm:grid-cols-2 xl:grid-cols-4">
          <figure className="min-w-0 border-b bg-muted/10 sm:border-r xl:border-b-0">
            <figcaption className="border-b px-3 py-2 text-[11px] text-muted-foreground">原图 · {candidate?.id.slice(0, 8) || item.candidate_id.slice(0, 8)}</figcaption>
            <div className="flex min-h-48 items-center justify-center p-2">{sourceUrl ? <img src={sourceUrl} alt={`原图 ${candidate?.title || item.candidate_id}`} width={720} height={720} loading="lazy" className="max-h-72 w-full object-contain" /> : <span className="text-xs text-muted-foreground">原图不可用</span>}</div>
          </figure>
          {CREATIVE_DELIVERY_SIZES.map((size, index) => {
            const asset = delivered.find((entry) => entry.size_key === size);
            const finalUrl = creativeAttachmentBrowserURL(asset ? attachmentById.get(asset.attachment_id) : undefined);
            return <figure key={size} className={`min-w-0 border-b bg-muted/10 ${index < 2 ? "xl:border-r" : ""} ${index === 0 ? "sm:border-r xl:border-l-0" : ""} xl:border-b-0`}>
              <figcaption className="border-b px-3 py-2 text-[11px] text-muted-foreground">成图 · {size}</figcaption>
              <div className="flex min-h-48 items-center justify-center p-2">{finalUrl ? <img src={finalUrl} alt={`最终采用方案 ${variant.variant_key} ${size}`} width={720} height={720} loading="lazy" className="max-h-72 w-full object-contain" /> : <span className="text-xs text-muted-foreground">交付图不可用</span>}</div>
            </figure>;
          })}
        </div>
      </div>;
    })}
    {selections.some(({ variant }) => variant.qc_status === "warning" || variant.qc_status === "failed") && <p className="border-t px-4 py-2 text-xs text-amber-700">已采用方案包含 QC 提醒，请在创意工厂查看对应标注。</p>}
  </section>;
}

function CreativeAdjustmentTrace({ event, variant, item }: { event: CreateCreativeFeedbackResponse; variant?: CreativeOrderVariant; item?: CreativeOrderItem }) {
  const steps = creativeAdjustmentTimeline(variant, event);
  const sourceRevision = typeof event.context_snapshot.revision === "number" ? event.context_snapshot.revision : 0;
  const size = typeof event.context_snapshot.size_key === "string" ? event.context_snapshot.size_key : "";
  const variantKey = variant?.variant_key || String(event.context_snapshot.variant_id || "").slice(0, 8);
  const complete = variant?.status === "completed" && variant.revision > sourceRevision;
  return <section className="border-t" aria-label="当前调整进度" data-testid="creative-adjustment-trace">
    <div className="flex flex-wrap items-start gap-3 px-4 py-3">
      <RefreshCw className={`mt-0.5 h-4 w-4 shrink-0 ${complete ? "text-emerald-700" : "text-amber-700"}`} />
      <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="text-sm font-semibold">{creativeAdjustmentProgress(variant, event)}</p><Badge variant="outline">{variantKey}{size ? ` · ${size}` : ""}</Badge><Badge variant="outline">r{sourceRevision} → r{variant?.revision ?? sourceRevision}</Badge></div><p className="mt-1 text-xs text-muted-foreground">{item ? `素材 ${item.candidate_id.slice(0, 8)}` : "目标素材"} · 用户调整要求</p><p className="mt-2 whitespace-pre-line break-words text-sm">{event.comment || "已提交调整要求"}</p></div>
    </div>
    <ol className="grid grid-cols-2 border-t sm:grid-cols-3 lg:grid-cols-6">{steps.map((step, index) => <li key={step.key} className="flex min-h-14 items-center gap-2 border-b px-3 py-2 last:border-r-0 sm:border-r lg:border-b-0"><span className={`flex h-5 min-w-5 items-center justify-center rounded-full text-[10px] font-semibold ${step.status === "done" ? "bg-emerald-600 text-white" : step.status === "failed" ? "bg-destructive text-destructive-foreground" : step.status === "current" ? "bg-amber-500 text-black" : "bg-muted text-muted-foreground"}`}>{step.status === "done" ? <CheckCircle2 className="h-3.5 w-3.5" /> : index + 1}</span><span className={`text-xs ${step.status === "current" || step.status === "failed" ? "font-semibold" : "text-muted-foreground"}`}>{step.label}</span></li>)}</ol>
  </section>;
}
