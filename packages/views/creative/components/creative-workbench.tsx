"use client";

import type { CreativeMaterialCandidate, CreativeOrder } from "@multica/core/types";

export interface CreativeWorkbenchProps {
  candidates?: CreativeMaterialCandidate[];
  orders?: CreativeOrder[];
  excludedCandidateIds?: readonly string[];
  onOpenMaterialLibrary: (filter?: "available" | "analyze") => void;
  onOpenOrder: (orderId: string) => void;
}

type WorkbenchOrderState = "review" | "running" | "delivered" | "inactive";

const WORKBENCH_COPY = {
  title: "创意工作台",
  subtitle: "查看素材、生成、验收和交付的当前总览。",
} as const;

export function CreativeWorkbench({
  candidates = [],
  orders = [],
  excludedCandidateIds = [],
}: CreativeWorkbenchProps) {
  const excludedCandidateIdSet = new Set(excludedCandidateIds);
  const pendingMaterials = candidates.filter((candidate) => !excludedCandidateIdSet.has(candidate.id));
  const classifiedOrders = orders.map((order) => ({ order, state: creativeWorkbenchOrderState(order) }));
  const reviewOrders = classifiedOrders.filter((entry) => entry.state === "review").sort(compareActionOrders);
  const runningOrders = classifiedOrders.filter((entry) => entry.state === "running").sort(compareActionOrders);
  const deliveredOrders = classifiedOrders
    .filter((entry) => entry.state === "delivered")
    .map((entry) => entry.order)
    .sort((left, right) => timestamp(right.updated_at) - timestamp(left.updated_at));
  const deliveredThisWeek = deliveredOrders.filter((order) => timestamp(order.updated_at) >= startOfLocalWeekTimestamp());

  return (
    <div className="mx-auto w-full min-w-0 max-w-[1440px] space-y-4" data-testid="creative-workbench">
      <header className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{WORKBENCH_COPY.title}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{WORKBENCH_COPY.subtitle}</p>
        </div>
      </header>

      <section aria-label="工作台概览" className="grid min-w-0 grid-cols-2 divide-x divide-y border bg-background sm:grid-cols-4 sm:divide-y-0">
        <WorkbenchMetric testId="creative-workbench-materials" value={pendingMaterials.length} label="素材待筛" />
        <WorkbenchMetric testId="creative-workbench-running" value={runningOrders.length} label="出图中" />
        <WorkbenchMetric testId="creative-workbench-reviews" value={reviewOrders.length} label="待确认成图" />
        <WorkbenchMetric testId="creative-workbench-deliveries" value={deliveredThisWeek.length} label="本周交付" />
      </section>
    </div>
  );
}

export function creativeWorkbenchOrderState(order: CreativeOrder): WorkbenchOrderState {
  const status = order.derived_status || order.status;
  if (status === "cancelled" || order.status === "cancelled") return "inactive";
  if (status === "completed" || (order.items.length > 0 && order.items.every((item) => Boolean(item.adopted_variant_id)))) return "delivered";
  if (status === "awaiting_adoption") return "review";
  const hasReviewableResult = order.items.some((item) => item.variants.some((variant) =>
    variant.status === "completed"
    || variant.qc_status === "passed"
    || variant.assets.some((asset) => asset.status === "completed" && asset.stage === "delivered"),
  ));
  if (hasReviewableResult) return "review";

  if ((order.workflow_failures?.length ?? 0) > 0 || status === "action_required" || status === "failed") {
    return "review";
  }
  if (status === "queued" || status === "running" || status === "partial" || order.items.some((item) => item.variants.some((variant) => variant.status === "queued" || variant.status === "running" || variant.status === "partial"))) {
    return "running";
  }

  return "inactive";
}

function WorkbenchMetric({ testId, value, label }: { testId: string; value: number; label: string }) {
  return (
    <div className="min-w-0 px-5 py-4" data-testid={testId}>
      <span className="block text-2xl font-semibold tabular-nums">{value}</span>
      <span className="mt-0.5 block text-xs leading-4 text-muted-foreground">{label}</span>
    </div>
  );
}

function compareActionOrders(
  left: { order: CreativeOrder; state: WorkbenchOrderState },
  right: { order: CreativeOrder; state: WorkbenchOrderState },
): number {
  return timestamp(right.order.updated_at) - timestamp(left.order.updated_at);
}

function timestamp(value: string): number {
  const parsed = new Date(value).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}

function startOfLocalWeekTimestamp(date = new Date()): number {
  const start = new Date(date);
  const day = start.getDay();
  const diff = day === 0 ? 6 : day - 1;
  start.setHours(0, 0, 0, 0);
  start.setDate(start.getDate() - diff);
  return start.getTime();
}
