import type { CreateCreativeFeedbackResponse, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";

const CREATIVE_ADJUSTMENT_ALL_SIZES = ["1080x1080", "1200x628", "800x1000"];

export type CreativeAdjustmentStep = {
  key: "submitted" | "planned" | "generated" | "primed" | "qc" | "completed";
  label: string;
  status: "done" | "current" | "pending" | "failed";
};

export function latestOrderAdjustmentFeedback(events: CreateCreativeFeedbackResponse[], orderId: string): CreateCreativeFeedbackResponse | undefined {
  return events
    .filter((event) => event.decision === "needs_revision" && event.context_snapshot.order_id === orderId)
    .sort((left, right) => right.created_at.localeCompare(left.created_at))[0];
}

export function creativeAdjustmentTarget(
  items: CreativeOrderItem[],
  event: CreateCreativeFeedbackResponse | undefined,
): { item: CreativeOrderItem; variant: CreativeOrderVariant } | undefined {
  const sourceVariantId = typeof event?.context_snapshot.variant_id === "string" ? event.context_snapshot.variant_id : "";
  if (!sourceVariantId) return undefined;
  const item = items.find((candidate) => candidate.variants.some((variant) => variant.id === sourceVariantId));
  const source = item?.variants.find((variant) => variant.id === sourceVariantId);
  if (!item || !source) return undefined;
  const variant = item.variants
    .filter((candidate) => candidate.variant_key === source.variant_key)
    .sort((left, right) => right.revision - left.revision)[0] ?? source;
  return { item, variant };
}

export function creativeAdjustmentProgress(variant: CreativeOrderVariant | undefined, event: CreateCreativeFeedbackResponse): string {
  const sourceRevision = adjustmentSourceRevision(event);
  if (!variant || variant.revision <= sourceRevision) return "调整未启动";
  if (variant.status === "completed") return `调整已完成 · r${variant.revision}`;
  if (variant.status === "action_required" || variant.status === "failed") return `调整需要处理 · r${variant.revision}`;
  return `当前尺寸调整中 · r${variant.revision}`;
}

export function creativeAdjustmentCanRetry(variant: CreativeOrderVariant | undefined, event: CreateCreativeFeedbackResponse): boolean {
  const sourceRevision = adjustmentSourceRevision(event);
  return Boolean(variant && (
    variant.revision <= sourceRevision ||
    (variant.revision === sourceRevision + 1 && ["running", "action_required", "failed"].includes(variant.status))
  ));
}

export function creativeAdjustmentTimeline(variant: CreativeOrderVariant | undefined, event: CreateCreativeFeedbackResponse): CreativeAdjustmentStep[] {
  const sourceRevision = adjustmentSourceRevision(event);
  const revisionStarted = Boolean(variant && variant.revision > sourceRevision);
  const failed = Boolean(variant && revisionStarted && (variant.status === "action_required" || variant.status === "failed"));
  const currentRevision = variant?.revision ?? sourceRevision;
  const expectedSizes = adjustmentExpectedSizes(event);
  const generated = new Set(variant?.assets.filter((asset) => asset.revision === currentRevision && asset.stage === "generated" && asset.status === "completed" && expectedSizes.includes(asset.size_key)).map((asset) => asset.size_key) ?? []).size;
  const primed = new Set(variant?.assets.filter((asset) => asset.revision === currentRevision && ["primed", "delivered"].includes(asset.stage) && asset.status === "completed" && expectedSizes.includes(asset.size_key)).map((asset) => asset.size_key) ?? []).size;
  const qcReports = variant?.qc_reports.filter((report) => report.revision === currentRevision && ["technical", "visual"].includes(report.lane)) ?? [];
  const directEditNoQC = Boolean(variant?.brief?.creative_direct_edit_delivery && typeof variant.brief.creative_direct_edit_delivery === "object" && !Array.isArray(variant.brief.creative_direct_edit_delivery) && (variant.brief.creative_direct_edit_delivery as { skip_qc?: unknown }).skip_qc === true);
  const qcDone = directEditNoQC || ["technical", "visual"].every((lane) => qcReports.some((report) => report.lane === lane && ["passed", "warning"].includes(report.status)));
  const completed = variant?.status === "completed";
  const done = { submitted: true, planned: revisionStarted, generated: generated >= expectedSizes.length, primed: primed >= expectedSizes.length, qc: qcDone, completed };
  const order: CreativeAdjustmentStep["key"][] = ["submitted", "planned", "generated", "primed", "qc", "completed"];
  const labels: Record<CreativeAdjustmentStep["key"], string> = { submitted: "已提交", planned: "精准调整", generated: expectedSizes.length === 1 ? "当前尺寸生成" : "三尺寸生成", primed: "品牌组件合成", qc: directEditNoQC ? "跳过质检" : "双路 QC", completed: "完成" };
  const firstPending = order.find((key) => !done[key]);
  return order.map((key) => ({
    key,
    label: labels[key],
    status: done[key] ? "done" : key === firstPending ? failed ? "failed" : "current" : "pending",
  }));
}

function adjustmentSourceRevision(event: CreateCreativeFeedbackResponse): number {
  return typeof event.context_snapshot.revision === "number" ? event.context_snapshot.revision : 0;
}

function adjustmentExpectedSizes(event: CreateCreativeFeedbackResponse): string[] {
  const scope = typeof event.context_snapshot.scope === "string" ? event.context_snapshot.scope : "";
  const size = typeof event.context_snapshot.size_key === "string" ? event.context_snapshot.size_key : "";
  return scope === "size" && size ? [size] : CREATIVE_ADJUSTMENT_ALL_SIZES;
}
