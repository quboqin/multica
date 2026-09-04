"use client";

import type { CreativeMaterialCandidate, CreativeOrder } from "@multica/core/types";
import { useT } from "../../i18n";

export interface CreativeWorkbenchProps {
  candidates?: CreativeMaterialCandidate[];
  orders?: CreativeOrder[];
  excludedCandidateIds?: readonly string[];
  averageGenerationDuration?: string;
  generationDurationPackageCount?: number;
  onOpenMaterialLibrary: (filter?: "available" | "analyze") => void;
  onOpenOrder: (orderId: string) => void;
}

type WorkbenchOrderState = "review" | "running" | "attention" | "delivered" | "inactive";

export function CreativeWorkbench({
  candidates = [],
  orders = [],
  excludedCandidateIds = [],
  averageGenerationDuration = "-",
  generationDurationPackageCount = 0,
}: CreativeWorkbenchProps) {
  const { t } = useT("creative");
  const excludedCandidateIdSet = new Set(excludedCandidateIds);
  const pendingMaterials = candidates.filter((candidate) => !excludedCandidateIdSet.has(candidate.id));
  const reviewOrders = orders.filter(isGalleryReadyOrder).sort(compareActionOrders);
  const runningOrders = orders.filter(isRunningOrder).sort(compareActionOrders);
  const deliveredThisWeek = orders
    .filter(isDeliveredOrder)
    .sort(compareActionOrders)
    .filter((order) => timestamp(order.updated_at) >= startOfLocalWeekTimestamp());

  return (
    <div className="mx-auto w-full min-w-0 max-w-[1440px] space-y-4" data-testid="creative-workbench">
      <header className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t(($) => $.workbench.title)}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t(($) => $.workbench.subtitle)}</p>
        </div>
      </header>

      <section aria-label={t(($) => $.workbench.overview)} className="grid min-w-0 grid-cols-2 divide-x divide-y border bg-background sm:grid-cols-5 sm:divide-y-0">
        <WorkbenchMetric testId="creative-workbench-materials" value={pendingMaterials.length} label={t(($) => $.workbench.materials)} />
        <WorkbenchMetric testId="creative-workbench-running" value={runningOrders.length} label={t(($) => $.workbench.generating)} />
        <WorkbenchMetric testId="creative-workbench-reviews" value={reviewOrders.length} label={t(($) => $.workbench.readyToAdopt)} />
        <WorkbenchMetric testId="creative-workbench-deliveries" value={deliveredThisWeek.length} label={t(($) => $.workbench.weeklyDelivery)} />
        <WorkbenchMetric
          testId="creative-workbench-average-duration"
          value={averageGenerationDuration}
          label={t(($) => $.workbench.averageDuration)}
          hint={generationDurationPackageCount > 0 ? t(($) => $.workbench.basedOnPackages, { count: generationDurationPackageCount }) : undefined}
        />
      </section>
    </div>
  );
}

export function creativeWorkbenchOrderState(order: CreativeOrder): WorkbenchOrderState {
  if (isCancelledOrder(order)) return "inactive";
  if (isDeliveredOrder(order)) return "delivered";
  if (isGalleryReadyOrder(order)) return "review";
  if (isRunningOrder(order)) return "running";
  if (isAttentionRequiredOrder(order)) return "attention";
  return "inactive";
}

function isCancelledOrder(order: CreativeOrder): boolean {
  return order.status === "cancelled" || (order.derived_status || order.status) === "cancelled";
}

function isDeliveredOrder(order: CreativeOrder): boolean {
  const status = order.derived_status || order.status;
  return status === "completed";
}

function isGalleryReadyOrder(order: CreativeOrder): boolean {
  if (isCancelledOrder(order) || isDeliveredOrder(order)) return false;
  const status = order.derived_status || order.status;
  if (status === "awaiting_adoption") return true;
  return order.items.some((item) => item.variants.some((variant) =>
    variant.status === "completed"
    || variant.qc_status === "passed"
    || variant.assets.some((asset) => asset.status === "completed" && asset.stage === "delivered"),
  ));
}

function isRunningOrder(order: CreativeOrder): boolean {
  if (isCancelledOrder(order)) return false;
  const status = order.derived_status || order.status;
  return status === "queued" || status === "running" || status === "partial";
}

function isAttentionRequiredOrder(order: CreativeOrder): boolean {
  if (isCancelledOrder(order)) return false;
  const status = order.derived_status || order.status;
  if (isRunningOrder(order)) return false;
  return status === "action_required" || status === "failed" || (order.workflow_failures?.length ?? 0) > 0;
}

function WorkbenchMetric({ testId, value, label, hint }: { testId: string; value: number | string; label: string; hint?: string }) {
  return (
    <div className="min-w-0 px-5 py-4" data-testid={testId}>
      <span className="block text-2xl font-semibold tabular-nums">{value}</span>
      <span className="mt-0.5 block text-xs leading-4 text-muted-foreground">{label}</span>
      {hint && <span className="mt-0.5 block truncate text-[11px] leading-4 text-muted-foreground">{hint}</span>}
    </div>
  );
}

function compareActionOrders(left: CreativeOrder, right: CreativeOrder): number {
  return timestamp(right.updated_at) - timestamp(left.updated_at);
}

function timestamp(value: string): number {
  const parsed = new Date(value).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}

function startOfLocalWeekTimestamp(date = new Date()): number {
  const start = new Date(date);
  const day = start.getDay();
  const daysSinceMonday = day === 0 ? 6 : day - 1;
  start.setHours(0, 0, 0, 0);
  start.setDate(start.getDate() - daysSinceMonday);
  return start.getTime();
}
