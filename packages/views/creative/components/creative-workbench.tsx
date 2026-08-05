"use client";

import { ArrowRight, CheckCircle2, ImageIcon, PackageCheck, TriangleAlert } from "lucide-react";
import type { CreativeMaterialCandidate, CreativeOrder } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { formatCreativeDateTime } from "../lib/creative-time";

export interface CreativeWorkbenchProps {
  candidates?: CreativeMaterialCandidate[];
  orders?: CreativeOrder[];
  onOpenMaterialLibrary: () => void;
  onOpenOrder: (orderId: string) => void;
}

type WorkbenchOrderState = "attention" | "review" | "delivered" | "inactive";

const SELECTABLE_MATERIAL_STATUSES = new Set(["unseen", "new", "viewed", "shortlisted"]);
const WORKBENCH_COPY = {
  title: "创意工作台",
  emptyActions: "当前没有需要你处理的事项",
  emptyDeliveries: "完成采用后，交付会出现在这里",
  openDelivery: "查看并下载",
  resolveIssue: "处理问题",
  reviewCreative: "验收成图",
} as const;

export function CreativeWorkbench({
  candidates = [],
  orders = [],
  onOpenMaterialLibrary,
  onOpenOrder,
}: CreativeWorkbenchProps) {
  const candidateById = new Map(candidates.map((candidate) => [candidate.id, candidate]));
  const pendingMaterials = candidates.filter((candidate) => SELECTABLE_MATERIAL_STATUSES.has(candidate.status));
  const classifiedOrders = orders.map((order) => ({ order, state: creativeWorkbenchOrderState(order) }));
  const actionOrders = classifiedOrders
    .filter((entry) => entry.state === "attention" || entry.state === "review")
    .sort(compareActionOrders);
  const deliveredOrders = classifiedOrders
    .filter((entry) => entry.state === "delivered")
    .map((entry) => entry.order)
    .sort((left, right) => timestamp(right.updated_at) - timestamp(left.updated_at));

  return (
    <div className="mx-auto w-full max-w-[1440px] space-y-8" data-testid="creative-workbench">
      <header className="border-b pb-4">
        <h2 className="text-lg font-semibold">{WORKBENCH_COPY.title}</h2>
        <div className="mt-4 grid grid-cols-3 divide-x border-y bg-muted/20">
          <WorkbenchMetric
            testId="creative-workbench-pending-materials"
            value={pendingMaterials.length}
            label="待选素材"
          />
          <WorkbenchMetric
            testId="creative-workbench-pending-orders"
            value={actionOrders.length}
            label="待验收 / 需处理"
          />
          <WorkbenchMetric
            testId="creative-workbench-deliveries"
            value={deliveredOrders.length}
            label="最近交付"
          />
        </div>
      </header>

      <section aria-label="现在需要处理">
        <SectionHeading title="现在需要处理" count={actionOrders.length + (pendingMaterials.length > 0 ? 1 : 0)} />
        <div className="mt-2 border-y">
          {actionOrders.filter((entry) => entry.state === "attention").map(({ order }) => (
            <OrderActionRow
              key={order.id}
              order={order}
              candidateTitle={creativeOrderTitle(order, candidateById)}
              state="attention"
              onOpenOrder={onOpenOrder}
            />
          ))}
          {pendingMaterials.length > 0 && (
            <ActionRow
              icon={<ImageIcon className="h-4 w-4" />}
              tone="default"
              label="待选素材"
              title={`${pendingMaterials.length} 条素材等待筛选`}
              description="选择值得继续制作的素材，并记录不采用的原因"
              action={(
                <Button size="sm" variant="outline" onClick={onOpenMaterialLibrary}>
                  {`筛选 ${pendingMaterials.length} 条素材`}
                  <ArrowRight className="h-4 w-4" />
                </Button>
              )}
            />
          )}
          {actionOrders.filter((entry) => entry.state === "review").map(({ order }) => (
            <OrderActionRow
              key={order.id}
              order={order}
              candidateTitle={creativeOrderTitle(order, candidateById)}
              state="review"
              onOpenOrder={onOpenOrder}
            />
          ))}
          {actionOrders.length === 0 && pendingMaterials.length === 0 && (
            <div className="flex min-h-24 items-center gap-3 px-4 py-5 text-sm text-muted-foreground">
              <CheckCircle2 className="h-4 w-4" />
              {WORKBENCH_COPY.emptyActions}
            </div>
          )}
        </div>
      </section>

      <section aria-label="最近交付">
        <SectionHeading title="最近交付" count={deliveredOrders.length} />
        <div className="mt-2 border-y">
          {deliveredOrders.slice(0, 4).map((order) => (
            <ActionRow
              key={order.id}
              icon={<PackageCheck className="h-4 w-4" />}
              tone="success"
              label="已采用"
              title={creativeOrderTitle(order, candidateById)}
              description={`${order.items.length} 个素材已采用 · ${formatCreativeDateTime(order.updated_at)}`}
              action={(
                <Button size="sm" variant="outline" onClick={() => onOpenOrder(order.id)}>
                  {WORKBENCH_COPY.openDelivery}
                  <ArrowRight className="h-4 w-4" />
                </Button>
              )}
            />
          ))}
          {deliveredOrders.length === 0 && (
            <div className="flex min-h-24 items-center gap-3 px-4 py-5 text-sm text-muted-foreground">
              <PackageCheck className="h-4 w-4" />
              {WORKBENCH_COPY.emptyDeliveries}
            </div>
          )}
        </div>
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
    return "attention";
  }

  return "inactive";
}

function WorkbenchMetric({ testId, value, label }: { testId: string; value: number; label: string }) {
  return (
    <div className="min-w-0 px-4 py-3 sm:px-5" data-testid={testId}>
      <span className="block text-xl font-semibold tabular-nums">{value}</span>
      <span className="mt-0.5 block min-h-8 text-xs leading-4 text-muted-foreground sm:min-h-0">{label}</span>
    </div>
  );
}

function SectionHeading({ title, count }: { title: string; count: number }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <h2 className="text-sm font-semibold">{title}</h2>
      <span className="text-xs tabular-nums text-muted-foreground">{`${count} 项`}</span>
    </div>
  );
}

function OrderActionRow({
  order,
  candidateTitle,
  state,
  onOpenOrder,
}: {
  order: CreativeOrder;
  candidateTitle: string;
  state: "attention" | "review";
  onOpenOrder: (orderId: string) => void;
}) {
  const failureCount = order.workflow_failures?.length ?? 0;
  const reviewableItems = order.items.filter((item) => item.variants.some((variant) =>
    variant.status === "completed" || variant.qc_status === "passed",
  )).length;

  if (state === "attention") {
    return (
      <ActionRow
        icon={<TriangleAlert className="h-4 w-4" />}
        tone="danger"
        label="需要处理"
        title={candidateTitle}
        description={failureCount > 0
          ? `${failureCount} 个环节需要处理，已完成的结果不受影响`
          : "生成遇到问题，已完成的结果仍可查看"}
        action={(
          <Button size="sm" variant="outline" onClick={() => onOpenOrder(order.id)}>
            {WORKBENCH_COPY.resolveIssue}
            <ArrowRight className="h-4 w-4" />
          </Button>
        )}
      />
    );
  }

  return (
    <ActionRow
      icon={<ImageIcon className="h-4 w-4" />}
      tone="default"
      label="等待验收"
      title={candidateTitle}
      description={`${Math.max(reviewableItems, 1)} 个素材已出图，等待选择最终方案`}
      action={(
        <Button size="sm" onClick={() => onOpenOrder(order.id)}>
          {WORKBENCH_COPY.reviewCreative}
          <ArrowRight className="h-4 w-4" />
        </Button>
      )}
    />
  );
}

function ActionRow({
  icon,
  tone,
  label,
  title,
  description,
  action,
}: {
  icon: React.ReactNode;
  tone: "default" | "danger" | "success";
  label: string;
  title: string;
  description: string;
  action: React.ReactNode;
}) {
  const toneClass = tone === "danger"
    ? "text-destructive"
    : tone === "success"
      ? "text-success"
      : "text-foreground";

  return (
    <div className="grid min-h-20 grid-cols-1 items-center gap-3 border-t px-4 py-3 first:border-t-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:gap-4">
      <div className="flex min-w-0 items-start gap-3">
        <span className={`mt-0.5 shrink-0 ${toneClass}`} aria-hidden="true">{icon}</span>
        <div className="min-w-0">
          <div className="flex min-w-0 items-baseline gap-2">
            <span className={`shrink-0 text-[11px] font-medium ${toneClass}`}>{label}</span>
            <h3 className="truncate text-sm font-medium">{title}</h3>
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">{description}</p>
        </div>
      </div>
      <div className="shrink-0 justify-self-start sm:justify-self-end">{action}</div>
    </div>
  );
}

function creativeOrderTitle(
  order: CreativeOrder,
  candidateById: Map<string, CreativeMaterialCandidate>,
): string {
  const firstCandidate = order.items[0] ? candidateById.get(order.items[0].candidate_id) : undefined;
  const baseTitle = firstCandidate?.title || firstCandidate?.competitor || `订单 ${order.id.slice(0, 8)}`;
  return order.items.length > 1 ? `${baseTitle} 等 ${order.items.length} 个素材` : baseTitle;
}

function compareActionOrders(
  left: { order: CreativeOrder; state: WorkbenchOrderState },
  right: { order: CreativeOrder; state: WorkbenchOrderState },
): number {
  if (left.state !== right.state) return left.state === "attention" ? -1 : 1;
  return timestamp(right.order.updated_at) - timestamp(left.order.updated_at);
}

function timestamp(value: string): number {
  const parsed = new Date(value).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}
