// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  CreativeMaterialCandidate,
  CreativeOrder,
  CreativeOrderItem,
  CreativeOrderVariant,
} from "@multica/core/types";
import { CreativeWorkbench } from "./creative-workbench";
import copy from "../../locales/zh-Hans/creative.json";

vi.mock("../../i18n", () => ({ useT: () => ({ t: (selector: (value: typeof copy) => string, values: Record<string, string | number> = {}) => Object.entries(values).reduce((text, [key, value]) => text.replaceAll(`{{${key}}}`, String(value)), selector(copy)) }) }));

afterEach(() => cleanup());

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
    active_revision: status === "completed" ? 1 : 0,
    staging_revision: 1,
    status,
    qc_status: status === "completed" ? "passed" : "pending",
    qc_recovery_used: false,
    qc_recovery_available: false,
    created_at: "2026-08-01T02:00:00Z",
    updated_at: "2026-08-01T02:00:00Z",
    assets: [],
    diagnostic_assets: [],
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
  it("waits for a measured package instead of showing a historical duration", () => {
    render(<CreativeWorkbench averageGenerationDuration="40 分钟" generationDurationPackageCount={0} onOpenMaterialLibrary={vi.fn()} onOpenOrder={vi.fn()} />);
    const metric = screen.getByTestId("creative-workbench-average-duration");
    expect(metric).toHaveTextContent("待统计");
    expect(metric).not.toHaveTextContent("40 分钟");
    expect(metric).toHaveAttribute("title", expect.stringContaining("最近7天新下单"));
  });

  it("shows only business-facing work that needs a decision", () => {
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
      excludedCandidateIds={["candidate-selected", "candidate-rejected"]}
      averageGenerationDuration="6 分 8 秒"
      generationDurationPackageCount={3}
      onOpenMaterialLibrary={vi.fn()}
      onOpenOrder={vi.fn()}
    />);

    expect(screen.getByTestId("creative-workbench-materials")).toHaveTextContent("2");
    expect(screen.getByTestId("creative-workbench-running")).toHaveTextContent("1");
    expect(screen.getByTestId("creative-workbench-reviews")).toHaveTextContent("1");
    expect(screen.getByTestId("creative-workbench-deliveries")).toHaveTextContent("0");
    expect(screen.getByTestId("creative-workbench-average-duration")).toHaveTextContent("6 分 8 秒");
    expect(screen.getByTestId("creative-workbench-average-duration")).toHaveTextContent("基于 3 套");
    expect(screen.getByRole("region", { name: "工作台概览" })).toBeInTheDocument();
    expect(screen.getByText("查看素材、生成、验收和本周交付的当前总览。")).toBeInTheDocument();
    expect(screen.queryByText("需处理异常")).not.toBeInTheDocument();
    expect(screen.queryByText("Dana cepat")).not.toBeInTheDocument();
    expect(screen.queryByText("Pinjaman ringan")).not.toBeInTheDocument();
    expect(screen.queryByText("Sudah dipilih")).not.toBeInTheDocument();
  });

  it("does not count adopted results as pending adoption", () => {
    const delivered = order({
      id: "order-delivered",
      status: "completed",
      orderItems: [item({ id: "item-delivered", candidateId: "candidate-delivered", adopted: true })],
      updatedAt: new Date().toISOString(),
    });
    const deliveredLastWeek = order({
      id: "order-delivered-last-week",
      status: "completed",
      orderItems: [item({ id: "item-delivered-last-week", candidateId: "candidate-delivered-last-week", adopted: true })],
      updatedAt: new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString(),
    });

    render(<CreativeWorkbench
      candidates={[candidate("candidate-delivered", "selected", "Tawaran Lebaran")]}
      orders={[delivered, deliveredLastWeek]}
      onOpenMaterialLibrary={vi.fn()}
      onOpenOrder={vi.fn()}
    />);

    expect(screen.getByTestId("creative-workbench-reviews")).toHaveTextContent("0");
    expect(screen.getByTestId("creative-workbench-deliveries")).toHaveTextContent("1");
    expect(screen.getByTestId("creative-workbench-running")).toHaveTextContent("0");
    expect(screen.queryByText("Tawaran Lebaran")).not.toBeInTheDocument();
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

    expect(screen.getByTestId("creative-workbench-running")).toHaveTextContent("0");
    expect(screen.queryByText(/需要你处理/)).not.toBeInTheDocument();
  });

  it("does not count a stale queued order as active generation", () => {
    const stalled = order({
      id: "order-stalled",
      status: "queued",
      orderItems: [item({ id: "item-stalled", candidateId: "candidate-stalled", withCompletedVariant: false })],
    });
    stalled.derived_status = "action_required";

    render(<CreativeWorkbench candidates={[]} orders={[stalled]} onOpenMaterialLibrary={vi.fn()} onOpenOrder={vi.fn()} />);

    expect(screen.getByTestId("creative-workbench-running")).toHaveTextContent("0");
    expect(screen.getByTestId("creative-workbench-deliveries")).toHaveTextContent("0");
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

    expect(screen.getByTestId("creative-workbench-reviews")).toHaveTextContent("1");
    expect(screen.getByTestId("creative-workbench-deliveries")).toHaveTextContent("0");
    expect(screen.queryByRole("button", { name: /确认/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /处理/ })).not.toBeInTheDocument();
  });

  it("keeps empty states calm and does not expose implementation concepts", () => {
    render(<CreativeWorkbench
      candidates={[]}
      orders={[]}
      onOpenMaterialLibrary={vi.fn()}
      onOpenOrder={vi.fn()}
    />);

    expect(screen.getByRole("region", { name: "工作台概览" })).toBeInTheDocument();
    expect(screen.queryByText(/WorkUnit|Task Batch|Agent/)).not.toBeInTheDocument();
  });
});
