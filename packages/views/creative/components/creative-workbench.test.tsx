// @vitest-environment jsdom

import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type {
  CreativeMaterialCandidate,
  CreativeOrder,
  CreativeOrderItem,
  CreativeOrderVariant,
} from "@multica/core/types";
import { CreativeWorkbench } from "./creative-workbench";

function candidate(id: string, status: string, title: string): CreativeMaterialCandidate {
  return {
    id,
    workspace_id: "workspace-1",
    connector_id: "connector-1",
    external_id: id,
    dedupe_key: id,
    competitor: "Kredit Pintar",
    title,
    asset_type: "image",
    preview_url: "",
    resource_url: "",
    poster_url: "",
    original_url: "",
    archived_url: "",
    archive_status: "",
    archive_error: "",
    duration_days: 7,
    impression_estimate: 12000,
    media_names: ["Meta"],
    area_names: ["Indonesia"],
    language_names: ["Indonesian"],
    platform_names: ["Facebook"],
    status,
    tags: [],
    note: "",
    selected_at: null,
    first_seen_at: "2026-08-01T02:00:00Z",
    last_seen_at: "2026-08-01T02:00:00Z",
    created_at: "2026-08-01T02:00:00Z",
    updated_at: "2026-08-01T02:00:00Z",
    source_attachment_id: "attachment-1",
    source_issue_id: "issue-1",
    source_run_id: "run-1",
    is_new_in_run: true,
  };
}

function variant(id: string, status = "completed"): CreativeOrderVariant {
  return {
    id,
    order_item_id: "item-1",
    variant_key: "V01",
    brief: {},
    revision: 1,
    status,
    qc_status: status === "completed" ? "passed" : "pending",
    qc_recovery_used: false,
    qc_recovery_available: false,
    prime_repair_used: false,
    prime_repair_available: false,
    created_at: "2026-08-01T02:00:00Z",
    updated_at: "2026-08-01T02:00:00Z",
    assets: [],
    qc_reports: [],
  };
}

function item({
  id,
  candidateId,
  adopted = false,
  withCompletedVariant = true,
}: {
  id: string;
  candidateId: string;
  adopted?: boolean;
  withCompletedVariant?: boolean;
}): CreativeOrderItem {
  const readyVariant = variant(`${id}-variant`, withCompletedVariant ? "completed" : "running");
  return {
    id,
    order_id: "order-1",
    candidate_id: candidateId,
    source_analysis_id: "analysis-1",
    copy_snapshot: {},
    direction: "",
    status: withCompletedVariant ? "completed" : "running",
    adopted_variant_id: adopted ? readyVariant.id : "",
    adopted_at: adopted ? "2026-08-05T05:00:00Z" : "",
    adopted_by: adopted ? "member-1" : "",
    created_at: "2026-08-01T02:00:00Z",
    updated_at: "2026-08-05T05:00:00Z",
    variants: [readyVariant],
  };
}

function order({
  id,
  status,
  orderItems,
  failed = false,
  updatedAt = "2026-08-05T05:00:00Z",
}: {
  id: string;
  status: string;
  orderItems: CreativeOrderItem[];
  failed?: boolean;
  updatedAt?: string;
}): CreativeOrder {
  return {
    id,
    workspace_id: "workspace-1",
    issue_id: `issue-${id}`,
    status,
    derived_status: status,
    input_snapshot: {},
    trigger_evidence_kind: "manual",
    trigger_evidence_ref_id: "member-1",
    created_by: "member-1",
    created_at: "2026-08-01T02:00:00Z",
    updated_at: updatedAt,
    workflow_failures: failed ? [{
      task_id: "task-1",
      agent_id: "agent-1",
      workflow: "creative_generation",
      scope: "variant",
      subject_id: "variant-1",
      item_key: "V01",
      trigger_evidence_kind: "creative_order",
      trigger_evidence_ref_id: id,
      failure_reason: "provider_rate_limited",
      error: "429",
      failed_at: updatedAt,
      retryable: true,
    }] : [],
    items: orderItems,
  };
}

describe("CreativeWorkbench", () => {
  it("shows only business-facing work that needs a decision", () => {
    const onOpenMaterialLibrary = vi.fn();
    const onOpenOrder = vi.fn();
    const candidates = [
      candidate("candidate-new", "new", "Dana cepat"),
      candidate("candidate-viewed", "viewed", "Pinjaman ringan"),
      candidate("candidate-selected", "selected", "Sudah dipilih"),
      candidate("candidate-rejected", "rejected", "Tidak dipakai"),
    ];
    const needsAttention = order({
      id: "order-attention",
      status: "action_required",
      orderItems: [item({ id: "item-attention", candidateId: "candidate-new", withCompletedVariant: false })],
      failed: true,
    });
    const awaitingReview = order({
      id: "order-review",
      status: "awaiting_adoption",
      orderItems: [item({ id: "item-review", candidateId: "candidate-viewed" })],
    });
    const generating = order({
      id: "order-running",
      status: "running",
      orderItems: [item({ id: "item-running", candidateId: "candidate-selected", withCompletedVariant: false })],
    });

    render(<CreativeWorkbench
      candidates={candidates}
      orders={[generating, awaitingReview, needsAttention]}
      onOpenMaterialLibrary={onOpenMaterialLibrary}
      onOpenOrder={onOpenOrder}
    />);

    expect(screen.getByTestId("creative-workbench-pending-materials")).toHaveTextContent("2");
    expect(screen.getByTestId("creative-workbench-pending-orders")).toHaveTextContent("2");
    expect(screen.getByTestId("creative-workbench-deliveries")).toHaveTextContent("0");

    const queue = screen.getByRole("region", { name: "现在需要处理" });
    expect(within(queue).getByText("Dana cepat")).toBeInTheDocument();
    expect(within(queue).getByText("Pinjaman ringan")).toBeInTheDocument();
    expect(within(queue).queryByText("Sudah dipilih")).not.toBeInTheDocument();

    fireEvent.click(within(queue).getByRole("button", { name: "筛选 2 条素材" }));
    fireEvent.click(within(queue).getByRole("button", { name: "处理问题" }));
    fireEvent.click(within(queue).getByRole("button", { name: "验收成图" }));
    expect(onOpenMaterialLibrary).toHaveBeenCalledOnce();
    expect(onOpenOrder).toHaveBeenNthCalledWith(1, "order-attention");
    expect(onOpenOrder).toHaveBeenNthCalledWith(2, "order-review");
  });

  it("lists adopted orders as recent deliveries with a direct download action", () => {
    const onOpenOrder = vi.fn();
    const delivered = order({
      id: "order-delivered",
      status: "completed",
      orderItems: [item({ id: "item-delivered", candidateId: "candidate-delivered", adopted: true })],
    });

    render(<CreativeWorkbench
      candidates={[candidate("candidate-delivered", "selected", "Tawaran Lebaran")]}
      orders={[delivered]}
      onOpenMaterialLibrary={vi.fn()}
      onOpenOrder={onOpenOrder}
    />);

    expect(screen.getByTestId("creative-workbench-deliveries")).toHaveTextContent("1");
    const deliveries = screen.getByRole("region", { name: "最近交付" });
    expect(within(deliveries).getByText("Tawaran Lebaran")).toBeInTheDocument();
    expect(within(deliveries).getByText(/1 个素材已采用/)).toBeInTheDocument();

    fireEvent.click(within(deliveries).getByRole("button", { name: "查看并下载" }));
    expect(onOpenOrder).toHaveBeenCalledWith("order-delivered");
  });

  it("does not keep cancelled order history in the action queue", () => {
    const cancelled = order({
      id: "order-cancelled",
      status: "cancelled",
      orderItems: [],
      failed: true,
    });
    cancelled.derived_status = "cancelled";

    render(<CreativeWorkbench candidates={[]} orders={[cancelled]} onOpenMaterialLibrary={vi.fn()} onOpenOrder={vi.fn()} />);

    expect(screen.getByTestId("creative-workbench-pending-orders")).toHaveTextContent("0");
    expect(screen.getByRole("region", { name: "现在需要处理" })).toHaveTextContent("当前没有需要你处理的事项");
  });

  it("keeps completed results reviewable when another step failed", () => {
    const onOpenOrder = vi.fn();
    const partial = order({
      id: "order-partial",
      status: "action_required",
      orderItems: [item({ id: "item-partial", candidateId: "candidate-partial" })],
      failed: true,
    });

    render(<CreativeWorkbench candidates={[candidate("candidate-partial", "selected", "可验收素材")]} orders={[partial]} onOpenMaterialLibrary={vi.fn()} onOpenOrder={onOpenOrder} />);

    expect(screen.getByRole("button", { name: "验收成图" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "处理问题" })).not.toBeInTheDocument();
  });

  it("keeps empty states calm and does not expose implementation concepts", () => {
    render(<CreativeWorkbench
      candidates={[]}
      orders={[]}
      onOpenMaterialLibrary={vi.fn()}
      onOpenOrder={vi.fn()}
    />);

    expect(screen.getByText("当前没有需要你处理的事项")).toBeInTheDocument();
    expect(screen.getByText("完成采用后，交付会出现在这里")).toBeInTheDocument();
    expect(screen.queryByText(/WorkUnit|Task Batch|Agent/)).not.toBeInTheDocument();
  });
});
