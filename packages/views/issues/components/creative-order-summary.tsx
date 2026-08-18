"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowUpRight, Bot, CheckCircle2, CircleDotDashed, Layers3, MessageSquareText, RefreshCw, UserRound } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeFeedbackOptions, creativeMaterialLibraryOptions, creativeOrderOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { CreateCreativeFeedbackResponse, CreativeOrder, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { Badge } from "@multica/ui/components/ui/badge";
import { adoptedCreativeOrderVariant, CREATIVE_DELIVERY_SIZES, creativeOrderStage, creativeOrderStatusLabel, creativeVariantDeliveryAssets } from "../../creative/components/creative-order-delivery";
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
  const stage = creativeOrderStage(data);
  const collaboration = data ? creativeOrderCollaboration(data, latestAdjustment) : [];
  const orderHref = data?.issue_id ? `${paths.creativeOrder(orderId)}&fromIssue=${encodeURIComponent(data.issue_id)}` : paths.creativeOrder(orderId);

  return <section className="my-5 border bg-background" data-testid="creative-order-summary">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
      <div className="flex items-center gap-2"><Layers3 className="h-4 w-4 text-emerald-700" /><span className="text-sm font-semibold">创意订单</span><Badge variant="outline">{creativeOrderStatusLabel(data?.derived_status || data?.status)}</Badge>{!directEdit && <Badge variant={allItemsAdopted ? "default" : "secondary"}>{allItemsAdopted ? "已采用" : "待选择"}</Badge>}</div>
      <AppLink href={orderHref} className="inline-flex items-center gap-1 text-xs font-medium underline-offset-4 hover:underline">查看订单<ArrowUpRight className="h-3.5 w-3.5" /></AppLink>
    </div>
    {data && <CreativeOrderCollaborationBoard order={data} stage={stage} entries={collaboration} orderHref={orderHref} />}
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

type CollaborationEntry = {
  key: string;
  actor: "user" | "ai" | "system";
  title: string;
  detail: string;
  tone?: "default" | "warning" | "danger";
};

function CreativeOrderCollaborationBoard({
  order,
  stage,
  entries,
  orderHref,
}: {
  order: CreativeOrder;
  stage: ReturnType<typeof creativeOrderStage>;
  entries: CollaborationEntry[];
  orderHref: string;
}) {
  const nextStep = collaborationNextStep(order, stage);
  return <section className="border-b bg-muted/15" aria-label="订单协作工作台" data-testid="creative-order-collaboration-board">
    <div className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
      <div className="flex min-w-0 gap-2.5"><MessageSquareText className="mt-0.5 h-4 w-4 shrink-0 text-emerald-700" /><div><h3 className="text-sm font-semibold">协作工作台</h3><p className="mt-0.5 text-xs text-muted-foreground">在这里确认业务决策、查看 AI 执行状态；下方动态只保留讨论、调整和真实异常。</p></div></div>
      <AppLink href={orderHref} className="inline-flex shrink-0 items-center gap-1 text-xs font-medium underline-offset-4 hover:underline">进入成图工作台<ArrowUpRight className="h-3.5 w-3.5" /></AppLink>
    </div>
    <div className="grid border-t text-sm md:grid-cols-3">
      <div className="min-w-0 border-b px-4 py-3 md:border-b-0 md:border-r"><p className="text-xs text-muted-foreground">当前 AI 状态</p><div className="mt-1 flex items-center gap-2"><CircleDotDashed className="h-4 w-4 text-emerald-700" /><span className="font-medium">{stage.label}</span></div><p className="mt-1 text-xs text-muted-foreground">{stage.detail}</p></div>
      <div className="min-w-0 border-b px-4 py-3 md:border-b-0 md:border-r"><p className="text-xs text-muted-foreground">下一步</p><p className="mt-1 font-medium">{nextStep.title}</p><p className="mt-1 text-xs text-muted-foreground">{nextStep.detail}</p></div>
      <div className="min-w-0 px-4 py-3"><p className="text-xs text-muted-foreground">本次范围</p><p className="mt-1 font-medium">{order.items.length} 张素材 · {order.items.reduce((count, item) => count + item.variants.length, 0)} 个方案</p><p className="mt-1 text-xs text-muted-foreground">{stage.readyVariants} 个方案可验收 · {stage.adoptedItems}/{stage.totalItems} 已采用</p></div>
    </div>
    <ol className="divide-y border-t">
      {entries.slice(0, 6).map((entry) => <li key={entry.key} className={`flex gap-2.5 px-4 py-3 ${entry.tone === "danger" ? "bg-destructive/5" : entry.tone === "warning" ? "bg-amber-50/70 dark:bg-amber-950/10" : ""}`}>
        {entry.actor === "user" ? <UserRound className="mt-0.5 h-4 w-4 shrink-0 text-sky-700" /> : entry.actor === "ai" ? <Bot className="mt-0.5 h-4 w-4 shrink-0 text-emerald-700" /> : <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" />}
        <div className="min-w-0"><p className="text-sm font-medium">{entry.title}</p><p className="mt-0.5 break-words text-xs text-muted-foreground">{entry.detail}</p></div>
      </li>)}
    </ol>
  </section>;
}

function collaborationNextStep(order: CreativeOrder, stage: ReturnType<typeof creativeOrderStage>): { title: string; detail: string } {
  if (stage.key === "attention") return { title: "请处理异常", detail: "打开成图工作台查看失败步骤、影响范围与可重试操作。" };
  if (stage.key === "review") return { title: "请选择最终方案", detail: "每张素材可采用一个变体；即使有 QC 提醒，也可在确认风险后采用。" };
  if (stage.key === "delivered") return { title: "可下载交付包", detail: "已采用方案和三个尺寸均保留在创意订单中。" };
  if (stage.key === "cancelled") return { title: "订单已结束", detail: "已有结果和协作记录可以继续查看，但不会再自动续跑。" };
  return { title: "AI 正在处理", detail: order.workflow_failures.length > 0 ? "系统正在等待异常处理。" : "无需重复提交；完成后会自动进入下一阶段。" };
}

function creativeOrderCollaboration(order: CreativeOrder, adjustment?: CreateCreativeFeedbackResponse): CollaborationEntry[] {
  const entries: CollaborationEntry[] = [];
  const directions = order.items.map((item) => item.direction.trim()).filter(Boolean);
  entries.push({
    key: "direction",
    actor: "user",
    title: `已确认 ${order.items.length} 张素材的生成方向`,
    detail: directions.length > 0 ? directions.slice(0, 2).join("；") : "使用已确认的素材分析与文案快照生成。",
  });
  for (const variant of order.items.flatMap((item) => item.variants).sort((a, b) => a.variant_key.localeCompare(b.variant_key))) {
    entries.push({
      key: `variant:${variant.id}`,
      actor: "ai",
      title: `${variant.variant_key || "方案"} · ${variantCollaborationTitle(variant)}`,
      detail: variantCollaborationDetail(variant),
      tone: variant.status === "action_required" || variant.status === "failed" || variant.action_required ? "danger" : variant.qc_status === "failed" ? "warning" : "default",
    });
  }
  if (adjustment) {
    entries.unshift({
      key: `adjustment:${adjustment.id}`,
      actor: "user",
      title: "已提交成图调整",
      detail: adjustment.comment || "已记录调整要求，AI 将按标注范围处理。",
      tone: "warning",
    });
  }
  for (const failure of order.workflow_failures) {
    entries.unshift({
      key: `failure:${failure.task_id}`,
      actor: "system",
      title: `${workflowLabel(failure.workflow)}需要处理`,
      detail: failure.error || failure.failure_reason || "该步骤未完成，打开成图工作台可查看并重试。",
      tone: "danger",
    });
  }
  return entries;
}

function variantCollaborationTitle(variant: CreativeOrderVariant): string {
  const delivered = creativeVariantDeliveryAssets(variant).length;
  if (variant.status === "action_required" || variant.status === "failed" || variant.action_required) return `${workflowLabel(variant.action_required?.workflow || "")}需要处理`;
  if (variant.qc_status === "failed") return "质检发现问题";
  if (variant.qc_status === "warning") return "可采用，存在质检提醒";
  if (delivered === CREATIVE_DELIVERY_SIZES.length) return "三尺寸已交付";
  if (variant.assets.some((asset) => asset.stage === "primed" && asset.status === "completed")) return "正在质检";
  if (variant.assets.some((asset) => asset.status === "completed")) return "正在合成品牌组件";
  return variant.status === "completed" ? "等待交付收口" : "正在生成";
}

function variantCollaborationDetail(variant: CreativeOrderVariant): string {
  const completedAssets = variant.assets.filter((asset) => asset.revision === variant.revision && asset.status === "completed").length;
  const delivered = creativeVariantDeliveryAssets(variant).length;
  if (variant.status === "action_required" || variant.status === "failed" || variant.action_required) return variant.action_required?.detail || `r${variant.revision} 已停止自动流程，打开成图工作台查看处理入口。`;
  if (variant.qc_status === "failed") return "已保留具体质检结果；可在成图工作台查看原因、调整或按风险采用。";
  if (delivered === CREATIVE_DELIVERY_SIZES.length) return `r${variant.revision} 的方形、横版和竖版均已就绪。`;
  return `r${variant.revision} · ${completedAssets} 个过程产物已完成，${delivered}/3 个交付尺寸可用。`;
}

function workflowLabel(workflow: string): string {
  return ({ creative_plan: "创意方案", creative_production: "成图生成", brand_components: "品牌组件合成", creative_qc_technical: "技术质检", creative_qc_visual: "视觉质检", creative_direct_edit: "图片调整" } as Record<string, string>)[workflow] || "自动流程";
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
