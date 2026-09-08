// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Attachment, CreativeOrder, CreativeOrderItem, CreativeOrderQCReport, CreativeOrderVariant } from "@multica/core/types";
import {
  CreativeOrderDeliveryCandidates,
  creativeGalleryVariantIds,
  creativeOrderActionableWorkflowFailures,
  creativeOrderStage,
  creativeOrderStatusLabel,
  creativeVariantArchiveEntries,
  creativeVariantAdoptionReadiness,
  creativeVariantDeliveryAssets,
  creativeVariantIsInProgress,
  creativeVariantNeedsManualAction,
  creativeVariantPreviewAssets,
  creativeVariantQCDetails,
  creativeVariantRiskAdoptionReadiness,
} from "./creative-order-delivery";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

afterEach(cleanup);

const sizes = ["1080x1080", "1200x628", "800x1000"];

function variant(id: string, complete = true): CreativeOrderVariant {
  return {
    id,
    variant_key: id.toUpperCase(),
    revision: 2,
    active_revision: 2,
    staging_revision: 2,
    status: complete ? "completed" : "running",
    qc_status: complete ? "passed" : "pending",
    qc_recovery_used: false,
    qc_recovery_available: false,
    assets: sizes.flatMap((size) => [
      { id: `${id}-${size}-old`, variant_id: id, size_key: size, revision: 1, stage: "delivered", status: "completed", attachment_id: `${id}-${size}-old`, updated_at: "2026-08-01T00:00:00Z" },
      { id: `${id}-${size}-prime`, variant_id: id, size_key: size, revision: 2, stage: "primed", status: "completed", attachment_id: `${id}-${size}-prime`, updated_at: "2026-08-02T00:00:00Z" },
      { id: `${id}-${size}`, variant_id: id, size_key: size, revision: 2, stage: "delivered", status: complete || size !== "800x1000" ? "completed" : "running", attachment_id: `${id}-${size}`, updated_at: "2026-08-02T00:00:00Z" },
    ]),
    diagnostic_assets: [],
    qc_reports: [
      { id: `${id}-technical`, variant_id: id, revision: 2, lane: "technical", status: complete ? "passed" : "pending" },
      { id: `${id}-visual`, variant_id: id, revision: 2, lane: "visual", status: complete ? "passed" : "pending" },
    ],
  } as unknown as CreativeOrderVariant;
}

function item(adoptedVariantId = ""): CreativeOrderItem {
  return {
    id: "item-1",
    candidate_id: "candidate-1",
    copy_snapshot: { creative_type: "num", delivery_naming: { type: "num" }, visual_direction: { theme: "RATE DOWN" } },
    direction: "突出免息利益点",
    adopted_variant_id: adoptedVariantId,
    adopted_at: adoptedVariantId ? "2026-08-05T10:00:00Z" : "",
    adopted_by: adoptedVariantId ? "user-1" : "",
    variants: [variant("v01"), variant("v02", false), variant("v03")],
  } as unknown as CreativeOrderItem;
}

function qcReport(input: Pick<CreativeOrderQCReport, "id" | "variant_id" | "revision" | "lane" | "status" | "findings" | "updated_at">): CreativeOrderQCReport {
  return { ...input, attempt: 1, trigger_evidence_kind: "creative_order_variant_qc", trigger_evidence_ref_id: input.variant_id, created_at: input.updated_at };
}

describe("creative order stage", () => {
  it("uses actual candidate progress instead of inferring selection from completed square images", () => {
    const orderItem = item();
    orderItem.variants = Array.from({ length: 6 }, (_, index) => ({ ...variant(`c${index}`), candidate_state: "candidate" }));
    orderItem.candidate_progress = { state: "planning_incomplete", target: 6, expected: 8, planned: 6, generated: 6, primed: 6, settled: 6, plan_task_id: "plan", plan_status: "completed", selection_task_id: "", selection_status: "" };
    expect(creativeOrderStage({ status: "running", derived_status: "awaiting_selection", items: [orderItem], input_snapshot: { target_variant_count: 6 }, workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({ label: "候选方案未齐", detail: "候选方案 6/8，方图 6，贴片 6" });
    orderItem.candidate_progress.state = "selection_queued";
    expect(creativeOrderStage({ status: "running", items: [orderItem], input_snapshot: { target_variant_count: 6 }, workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({ label: "候选比较中" });
  });
  it("shows nine of ten ready sets without counting two reserves as deliveries", () => {
    const orderItem = item();
    orderItem.variants = Array.from({ length: 12 }, (_, index) => ({
      ...variant(`c${String(index + 1).padStart(2, "0")}`, index !== 9),
      candidate_state: index < 10 ? "selected" : "reserve",
      selection_rank: index + 1,
    })) as CreativeOrderVariant[];
    const incomplete = orderItem.variants[9]!;
    incomplete.assets = incomplete.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));
    const order = { status: "running", derived_status: "partial", input_snapshot: { target_variant_count: 10 }, items: [orderItem], workflow_failures: [] } as unknown as CreativeOrder;
    expect(creativeOrderStage(order)).toMatchObject({ key: "review", readyVariants: 9, totalVariants: 10, detail: "9/10 套成图可分别加入成图库" });
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "文案库", url: "" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);
    expect(screen.getByText("C10")).toBeInTheDocument();
    expect(screen.queryByText("C11")).not.toBeInTheDocument();
    expect(screen.queryByText("C12")).not.toBeInTheDocument();
  });

  it("keeps exploratory candidates out of the delivery page until three selections are committed", () => {
    const orderItem = item();
    orderItem.variants = ["c01", "c02", "c03", "c04", "c05"].map((id) => ({
      ...variant(id),
      variant_key: id.toUpperCase(),
      candidate_state: "candidate",
      selection_rank: null,
    })) as unknown as CreativeOrderVariant[];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByTestId("creative-order-candidate-selection-pending")).toBeInTheDocument();
    expect(screen.queryByText("C01")).not.toBeInTheDocument();
    expect(screen.queryByText("C05")).not.toBeInTheDocument();
  });

  it("shows only the three selected variants after candidate comparison", () => {
    const orderItem = item();
    orderItem.variants = ["c01", "c02", "c03", "c04", "c05"].map((id, index) => ({
      ...variant(id),
      variant_key: id.toUpperCase(),
      candidate_state: index < 3 ? "selected" : index === 3 ? "reserve" : "rejected",
      selection_rank: index < 3 ? index + 1 : index === 3 ? 4 : null,
    })) as CreativeOrderVariant[];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("C01")).toBeInTheDocument();
    expect(screen.getByText("C02")).toBeInTheDocument();
    expect(screen.getByText("C03")).toBeInTheDocument();
    expect(screen.queryByText("C04")).not.toBeInTheDocument();
    expect(screen.queryByText("C05")).not.toBeInTheDocument();
  });

  it("keeps a ready variant actionable even when another workflow failed", () => {
    const order = {
      status: "partial",
      derived_status: "action_required",
      workflow_failures: [{ task_id: "failed-task" }],
      items: [item()],
    } as unknown as CreativeOrder;

    expect(creativeOrderStage(order)).toMatchObject({
      key: "review",
      label: "可入图库",
      action: "查看成图",
      readyVariants: 3,
    });
  });

  it("moves completed orders to delivery", () => {
    expect(creativeOrderStage({ status: "completed", items: [item()], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "delivered",
      action: "查看并下载",
    });
  });

  it("uses aggregate list status when order details are not loaded", () => {
    expect(creativeOrderStage({ status: "queued", derived_status: "awaiting_adoption", items: [], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "review",
      label: "可入图库",
    });
    expect(creativeOrderStage({ status: "queued", derived_status: "action_required", items: [], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "attention",
      label: "待验收",
    });
  });

  it("treats cancelled orders as retained history instead of active work", () => {
    expect(creativeOrderStage({ status: "cancelled", derived_status: "cancelled", items: [], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "cancelled",
      label: "已结束",
      action: "查看记录",
    });
  });

  it("treats background brand composition as running instead of a manual action", () => {
    const handoff = variant("v01", false);
    handoff.active_revision = 0;
    handoff.status = "running";
    handoff.assets = sizes.map((size) => ({
      id: `${handoff.id}-${size}-generated`,
      variant_id: handoff.id,
      size_key: size,
      revision: handoff.revision,
      stage: "generated",
      status: "completed",
      attachment_id: `${handoff.id}-${size}-generated`,
      updated_at: "2026-08-05T00:00:00Z",
    })) as CreativeOrderVariant["assets"];
    const orderItem = item();
    orderItem.variants = [handoff];
    const order = {
      status: "partial",
      derived_status: "running",
      workflow_failures: [],
      items: [orderItem],
    } as unknown as CreativeOrder;

    expect(creativeVariantNeedsManualAction(handoff)).toBe(false);
    expect(creativeVariantIsInProgress(handoff)).toBe(true);
    expect(creativeVariantAdoptionReadiness(handoff)).toEqual({ ready: false, status: "成图已完成，正在合成品牌组件：已完成 0/3 个尺寸" });
    expect(creativeOrderActionableWorkflowFailures(order)).toEqual([]);
    expect(creativeOrderStage(order)).toMatchObject({ key: "generating", label: "生成中" });
  });

  it("keeps an active production continuation out of the failure state", () => {
    const continuing = variant("v01", false);
    continuing.active_revision = 0;
    continuing.status = "running";
    continuing.assets = [{
      id: "v01-generated",
      variant_id: continuing.id,
      size_key: "1080x1080",
      revision: continuing.revision,
      stage: "generated",
      status: "completed",
      attachment_id: "v01-generated",
      updated_at: "2026-08-05T00:00:00Z",
    }] as CreativeOrderVariant["assets"];
    continuing.action_required = {
      task_id: "production-parent",
      workflow: "creative_production",
      failure_reason: "agent_reported_action_required",
      detail: "父任务已完成，缺少横版与竖版，续跑任务正在执行。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };

    expect(creativeVariantNeedsManualAction(continuing)).toBe(false);
    expect(creativeVariantIsInProgress(continuing)).toBe(true);
    expect(creativeVariantAdoptionReadiness(continuing)).toEqual({ ready: false, status: "成图生成中：已完成 1/3 个尺寸" });

    const orderItem = item();
    orderItem.variants = [continuing];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("等待成图")).toBeInTheDocument();
    expect(screen.queryByText("生成失败")).not.toBeInTheDocument();
    expect(screen.queryByText("成图生成失败")).not.toBeInTheDocument();
  });

  it("treats completed brand composition as waiting for QC instead of a repairable failure", () => {
    const handoff = variant("v01", false);
    handoff.status = "running";
    handoff.assets = handoff.assets.filter((asset) => asset.stage === "primed" || asset.stage === "generated");
    const orderItem = item();
    orderItem.variants = [handoff];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(creativeVariantNeedsManualAction(handoff)).toBe(false);
    expect(creativeVariantIsInProgress(handoff)).toBe(true);
    expect(screen.getAllByText("处理中").length).toBeGreaterThan(0);
    expect(screen.queryByText("需要处理")).not.toBeInTheDocument();
    expect(screen.queryByText("品牌组件提醒")).not.toBeInTheDocument();
  });

  it("treats qc-finalize pending as QC progress", () => {
    const pending = variant("v01", false);
    pending.status = "action_required";
    pending.assets = pending.assets.filter((asset) => asset.stage === "primed" || asset.stage === "generated");
    pending.action_required = {
      task_id: "visual-qc-task",
      workflow: "creative_qc_visual",
      failure_reason: "agent_reported_action_required",
      detail: "`qc-finalize` 结果：`pending`，视觉质检仍在同步中；未修改 Issue 或其他 Variant。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [pending];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(creativeVariantNeedsManualAction(pending)).toBe(false);
    expect(creativeVariantIsInProgress(pending)).toBe(true);
    expect(screen.getAllByText("处理中").length).toBeGreaterThan(0);
    expect(screen.queryByText("需要处理")).not.toBeInTheDocument();
    expect(screen.queryByText("视觉质检异常")).not.toBeInTheDocument();
  });

  it("ignores stale action-required state after the current delivery passed QC", () => {
    const stale = variant("v01");
    stale.status = "action_required";
    stale.action_required = {
      task_id: "production-task",
      workflow: "creative_production",
      failure_reason: "agent_reported_action_required",
      detail: "已完成无品牌底图，正在后台合成品牌组件。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [stale];
    const order = {
      status: "partial",
      derived_status: "action_required",
      workflow_failures: [{
        task_id: "production-task",
        workflow: "creative_production",
        subject_id: stale.id,
        failure_reason: "agent_reported_action_required",
        error: "stale production failure",
        retryable: true,
      }],
      items: [orderItem],
    } as CreativeOrder;

    expect(creativeVariantNeedsManualAction(stale)).toBe(false);
    expect(creativeVariantAdoptionReadiness(stale)).toEqual({ ready: true, status: "三尺寸品牌组件已完成，可以加入成图库" });
    expect(creativeOrderActionableWorkflowFailures(order)).toEqual([]);
    expect(creativeOrderStage(order)).toMatchObject({ key: "review", label: "可入图库", readyVariants: 1 });
  });
});

function attachmentMap(orderItem: CreativeOrderItem): Map<string, Attachment> {
  return new Map(orderItem.variants.flatMap((entry) => entry.assets.map((asset) => [asset.attachment_id, {
    id: asset.attachment_id,
    filename: `${entry.variant_key}_${asset.size_key}.png`,
    url: `https://cdn.example/${asset.attachment_id}.png`,
    download_url: `https://cdn.example/${asset.attachment_id}.png`,
    markdown_url: "",
  } as Attachment])));
}

describe("CreativeOrderDeliveryCandidates", () => {
  it("shows the source image and explains where image-specific prompts are available before variants exist", () => {
    const orderItem = item();
    orderItem.variants = [];
    orderItem.direction = "保留白底和红色还款表；最终画面文字使用已审核文案。";

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} showDirectionDetails={false} />);

    expect(document.querySelector('img[src="https://cdn.example/source.png"]')).toBeInTheDocument();
    expect(screen.getByTestId("creative-order-item-item-1")).toBeInTheDocument();
  });

  it("collapses the whole material package when requested", () => {
    const orderItem = item();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} defaultOpen={false} />);

    const section = screen.getByTestId("creative-order-item-item-1");
    expect(section).not.toHaveAttribute("open");
    expect(screen.queryByText("查看三套候选方案")).not.toBeInTheDocument();
  });

  it("offers gallery actions and disables adding incomplete variants", () => {
    const orderItem = item();
    const incomplete = orderItem.variants[1]!;
    incomplete.assets = incomplete.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));
    const onAdopt = vi.fn();
    const onAssetSelect = vi.fn();
    const onAssetInfo = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={onAdopt} onAssetSelect={onAssetSelect} onAssetInfo={onAssetInfo} />);

    const actions = screen.getAllByRole("button", { name: "添加到成图库" });
    expect(actions).toHaveLength(2);
    expect(actions[0]).toBeEnabled();
    expect(screen.getByRole("button", { name: "尚不可入库" })).toBeDisabled();
    expect(screen.getAllByRole("button", { name: "查看并标注" })).toHaveLength(3);
    expect(screen.getByText("等待成图")).toBeInTheDocument();
    expect(screen.getAllByAltText(/方形主预览/)).toHaveLength(3);
    expect(screen.queryAllByAltText(/横版主预览|竖版主预览/)).toHaveLength(0);
    fireEvent.click(screen.getAllByRole("button", { name: "查看方形成图详情" })[0]!);
    expect(onAssetSelect).not.toHaveBeenCalled();
    expect(onAssetInfo).toHaveBeenCalledWith("v01-1080x1080");

    fireEvent.click(actions[1]!);
    expect(onAdopt).toHaveBeenCalledWith("v03");
  });

  it("keeps cancelled order results visible but disables adding to gallery", () => {
    const orderItem = item();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} disabled />);

    expect(screen.getAllByRole("button", { name: "订单已结束" })).toHaveLength(3);
    expect(screen.getAllByRole("button", { name: "订单已结束" }).every((button) => button.hasAttribute("disabled"))).toBe(true);
  });

  it("shows the backend blocker reason as a read-only process exception", () => {
    const blocked = variant("v01", false);
    blocked.status = "action_required";
    blocked.qc_reports = [];
    blocked.action_required = {
      task_id: "task-1",
      workflow: "creative_production",
      failure_reason: "agent_reported_action_required",
      detail: "底部 Prime 固定贴片区域被模型内容占用，未注册三尺寸成图。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [blocked];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getAllByText("未完成").length).toBeGreaterThan(0);
    expect(screen.getByText("底部 Prime 固定贴片区域被模型内容占用，未注册三尺寸成图。")).toBeInTheDocument();
    expect(screen.getByText("可以直接重试这个方案；也可以查看成图或标注调整。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试失败步骤" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加到成图库" })).toBeEnabled();
  });

  it("exposes workflow retry on each retryable blocked variant", () => {
    const blocked = variant("v01", false);
    blocked.status = "action_required";
    blocked.qc_reports = [];
    blocked.action_required = {
      task_id: "task-1",
      workflow: "creative_production",
      failure_reason: "agent_reported_action_required",
      detail: "方形图两次模型输出均将 CTA 放入底部 Prime 固定排除区。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [blocked];
    const onRetryVariant = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} onRetryVariant={onRetryVariant} />);

    fireEvent.click(screen.getByRole("button", { name: "重试此方案" }));
    expect(onRetryVariant).toHaveBeenCalledWith(blocked, { kind: "workflow", taskId: "task-1", label: "重试此方案" });
  });

  it("exposes Prime composition retry when brand component composition is blocked", () => {
    const blocked = variant("v01", false);
    blocked.status = "action_required";
    blocked.qc_reports = [];
    blocked.action_required = {
      task_id: "",
      workflow: "brand_components",
      failure_reason: "brand_composition_failed",
      detail: "compose brand components: Prime composer requires Python with Pillow, OpenCV, and NumPy",
      failed_at: "2026-08-20T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [blocked];
    const onRetryVariant = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} onRetryVariant={onRetryVariant} />);

    fireEvent.click(screen.getByRole("button", { name: "重试品牌组件合成" }));
    expect(onRetryVariant).toHaveBeenCalledWith(blocked, { kind: "prime", taskId: "", label: "重试品牌组件合成" });
  });

  it("shows diagnostic model output when a stopped variant has no registered assets", () => {
    const blocked = variant("v01", false);
    blocked.status = "action_required";
    blocked.assets = [];
    blocked.qc_reports = [];
    blocked.diagnostic_assets = [{
      id: "diagnostic-1",
      variant_id: blocked.id,
      task_id: "task-1",
      attachment_id: "attachment-1",
      size_key: "1080x1080",
      revision: blocked.revision,
      workflow: "creative_production",
      label: "返工输出",
      filename: "square-model-rework.png",
      metadata: {},
      url: "/api/attachments/attachment-1/download",
      created_at: "2026-08-09T10:00:00Z",
      updated_at: "2026-08-09T10:00:00Z",
    }];
    blocked.action_required = {
      task_id: "task-1",
      workflow: "creative_production",
      failure_reason: "agent_reported_action_required",
      detail: "方形图两次模型输出均将 CTA 放入底部 Prime 固定排除区。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [blocked];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByAltText("V01 过程图片 返工输出")).toBeInTheDocument();
    expect(screen.getByText("过程图片")).toBeInTheDocument();
    expect(screen.queryByText(/已保留 1 张过程图片/)).not.toBeInTheDocument();
    expect(screen.getAllByText("未完成").length).toBeGreaterThan(0);
    expect(screen.getByText("方形图两次模型输出均将 CTA 放入底部 Prime 固定排除区。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /方形过程图/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /方形待成图/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: "尚不可入库" })).toBeDisabled();
  });

  it("shows an unadopted direct-edit result as a viewable preview", () => {
    const blocked = variant("v01", false);
    blocked.status = "action_required";
    blocked.assets = [];
    blocked.qc_reports = [];
    blocked.diagnostic_assets = [{
      id: "diagnostic-source",
      variant_id: blocked.id,
      task_id: "task-1",
      attachment_id: "attachment-source",
      size_key: "1080x1080",
      revision: blocked.revision,
      workflow: "creative_production",
      label: "模型原图",
      filename: "source.png",
      metadata: {},
      url: "/api/attachments/attachment-source/download",
      created_at: "2026-09-02T10:00:00Z",
      updated_at: "2026-09-02T10:00:00Z",
    }, {
      id: "diagnostic-preview",
      variant_id: blocked.id,
      task_id: "task-1",
      attachment_id: "attachment-preview",
      size_key: "1080x1080",
      revision: blocked.revision,
      workflow: "creative_direct_edit",
      label: "直接改图尝试 2 · 未采用",
      filename: "direct-edit-preview.png",
      metadata: { accepted: false },
      url: "/api/attachments/attachment-preview/download",
      created_at: "2026-09-02T10:01:00Z",
      updated_at: "2026-09-02T10:01:00Z",
    }];
    blocked.action_required = {
      task_id: "task-1",
      workflow: "creative_direct_edit",
      failure_reason: "creative_output_missing",
      detail: "direct image edit did not register a completed target generated asset",
      failed_at: "2026-09-02T10:02:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [blocked];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByAltText("V01 过程图片 直接改图尝试 2 · 未采用")).toBeInTheDocument();
    expect(screen.getByText("改后预览 · 未采用")).toBeInTheDocument();
    expect(screen.getByText("已生成改后预览，但未通过交付验收；请查看预览图片。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /查看改后预览/ })).toBeInTheDocument();
  });

  it("opens process images even when the variant already has preview assets", () => {
    const ready = variant("v01");
    ready.diagnostic_assets = [{
      id: "diagnostic-1",
      variant_id: ready.id,
      task_id: "task-1",
      attachment_id: "attachment-1",
      size_key: "1080x1080",
      revision: ready.revision,
      workflow: "creative_production",
      label: "贴片预览检查",
      filename: "prime-collision-preview-1080x1080.png",
      metadata: {},
      url: "/api/attachments/attachment-1/download",
      created_at: "2026-08-09T10:00:00Z",
      updated_at: "2026-08-09T10:00:00Z",
    }];
    const orderItem = item();
    orderItem.variants = [ready];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /查看过程图片/ }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("V01 · 过程图片")).toBeInTheDocument();
    expect(screen.getByText("方形 · 1080x1080 · 贴片预览检查")).toBeInTheDocument();
    expect(screen.getByText("prime-collision-preview-1080x1080.png")).toBeInTheDocument();
  });

  it("separates direct adjustment before, after, current process, and reused process images", () => {
    const ready = variant("v01");
    ready.active_revision = 1;
    ready.staging_revision = 0;
    ready.brief = { creative_direct_edit_delivery: { final_visual_validation: true, target_size: "1080x1080", source_revision: 1 } };
    ready.diagnostic_assets = [
      {
        id: "diagnostic-composed-source",
        variant_id: ready.id,
        task_id: "task-source",
        attachment_id: "diagnostic-composed-source-attachment",
        size_key: "1080x1080",
        revision: 1,
        workflow: "brand_components",
        label: "Prime 合成成图",
        filename: "source-composed-1080x1080.png",
        metadata: {},
        url: "/api/attachments/diagnostic-composed-source-attachment/download",
        created_at: "2026-08-01T00:02:30Z",
        updated_at: "2026-08-01T00:02:30Z",
      },
      {
        id: "diagnostic-target",
        variant_id: ready.id,
        task_id: "task-1",
        attachment_id: "diagnostic-target-attachment",
        size_key: "1080x1080",
        revision: ready.revision,
        workflow: "creative_production",
        label: "模型原图",
        filename: "direct-edit-model-1080x1080.png",
        metadata: {},
        url: "/api/attachments/diagnostic-target-attachment/download",
        created_at: "2026-08-02T00:01:00Z",
        updated_at: "2026-08-02T00:02:00Z",
      },
      {
        id: "diagnostic-composed-target",
        variant_id: ready.id,
        task_id: "task-1",
        attachment_id: "diagnostic-composed-target-attachment",
        size_key: "1080x1080",
        revision: ready.revision,
        workflow: "brand_components",
        label: "Prime 合成成图",
        filename: "direct-edit-composed-1080x1080.png",
        metadata: {},
        url: "/api/attachments/diagnostic-composed-target-attachment/download",
        created_at: "2026-08-02T00:02:30Z",
        updated_at: "2026-08-02T00:02:30Z",
      },
      {
        id: "diagnostic-reused",
        variant_id: ready.id,
        task_id: "task-1",
        attachment_id: "diagnostic-reused-attachment",
        size_key: "1200x628",
        revision: ready.revision,
        workflow: "creative_production",
        label: "规范化底图",
        filename: "reused-landscape.png",
        metadata: { order_adjustment: { source_revision: 1, reused_for_adjustment: true } },
        url: "/api/attachments/diagnostic-reused-attachment/download",
        created_at: "2026-08-02T00:03:00Z",
        updated_at: "2026-08-02T00:04:00Z",
      },
      {
        id: "diagnostic-qc-visual-rework",
        variant_id: ready.id,
        task_id: "task-2",
        attachment_id: "diagnostic-qc-visual-rework-attachment",
        size_key: "800x1000",
        revision: ready.revision,
        workflow: "qc_visual_rework",
        label: "视觉质检失败触发图",
        filename: "qc-visual-failure-800-1000.png",
        metadata: { qc_visual_rework: true },
        url: "/api/attachments/diagnostic-qc-visual-rework-attachment/download",
        created_at: "2026-08-02T00:05:00Z",
        updated_at: "2026-08-02T00:06:00Z",
      },
    ];
    const orderItem = item();
    orderItem.variants = [ready];
    const onAdoptProcessImage = vi.fn().mockResolvedValue(undefined);

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} onAdoptProcessImage={onAdoptProcessImage} />);

    fireEvent.click(screen.getByRole("button", { name: /查看过程图片/ }));

    expect(screen.getByText("前后对照")).toBeInTheDocument();
    expect(screen.getByText("调整前贴片结果")).toBeInTheDocument();
    expect(screen.getByText("调整后贴片结果")).toBeInTheDocument();
    expect(screen.getByText("本次调整过程图")).toBeInTheDocument();
    expect(screen.getByText("视觉质检返工触发图")).toBeInTheDocument();
    expect(screen.getByText("沿用上一版过程图")).toBeInTheDocument();
    expect(screen.getByText("方形 · 1080x1080 · 模型改图回图")).toBeInTheDocument();
    expect(screen.getByText("竖版 · 800x1000 · 失败贴片成图")).toBeInTheDocument();
    expect(screen.getByText("横版 · 1200x628 · 沿用上一版")).toBeInTheDocument();
    expect(screen.getAllByText(/北京时间/).length).toBeGreaterThan(0);
    expect(screen.getByText(/2026\/08\/02 08:02:00/)).toBeInTheDocument();
    expect(within(screen.getByText("调整前贴片结果").closest("figure")!).getByRole("button", { name: "当前成图" })).toBeDisabled();
    expect(screen.queryByText("正在更新")).not.toBeInTheDocument();
    fireEvent.click(within(screen.getByText("调整后贴片结果").closest("figure")!).getByRole("button", { name: "采用此结果" }));
    expect(onAdoptProcessImage).toHaveBeenCalledWith(ready.id, "diagnostic-composed-target");
    expect(screen.queryByRole("button", { name: "采用此图" })).not.toBeInTheDocument();
  });

  it("lets users add the current revision to the gallery despite a QC reminder", () => {
    const adjusted = variant("v01");
    adjusted.status = "action_required";
    adjusted.qc_status = "failed";
    adjusted.qc_reports = [
      qcReport({
        id: "visual-r1",
        variant_id: adjusted.id,
        revision: 1,
        lane: "visual",
        status: "failed",
        findings: { blocking_failures: ["r1 旧问题不应展示"] },
        updated_at: "2026-08-04T00:00:00Z",
      }),
      qcReport({
        id: "technical-r2",
        variant_id: adjusted.id,
        revision: 2,
        lane: "technical",
        status: "passed",
        findings: { blocking_failures: [], quality_warnings: [] },
        updated_at: "2026-08-05T00:00:00Z",
      }),
      qcReport({
        id: "visual-r2",
        variant_id: adjusted.id,
        revision: 2,
        lane: "visual",
        status: "failed",
        findings: {
          blocking_failures: [{ size_key: "800x1000", reason: "右上角品牌组件遮挡标题" }],
          quality_warnings: ["人物边缘略有锯齿"],
        },
        updated_at: "2026-08-05T00:00:00Z",
      }),
    ];
    adjusted.qc_recovery_available = true;
    const orderItem = item();
    orderItem.variants = [adjusted];

    const onAdopt = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={onAdopt} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("视觉质检未通过")).toBeInTheDocument();
    expect(screen.getByText("800x1000：右上角品牌组件遮挡标题")).toBeInTheDocument();
    expect(screen.getByText("人物边缘略有锯齿")).toBeInTheDocument();
    expect(screen.queryByText("r1 旧问题不应展示")).not.toBeInTheDocument();
    expect(screen.getByText("检测记录")).toBeInTheDocument();
    expect(screen.getByText("可加入成图库")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "添加到成图库" }));
    expect(onAdopt).toHaveBeenCalledWith(adjusted.id);
  });

  it("exposes visual QC retry for recoverable failed QC variants", () => {
    const failed = variant("v01");
    failed.status = "failed";
    failed.qc_status = "failed";
    failed.qc_recovery_available = true;
    failed.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: failed.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: failed.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    const orderItem = item();
    orderItem.variants = [failed];
    const onRetryVariant = vi.fn();

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} onRetryVariant={onRetryVariant} />);

    fireEvent.click(screen.getByRole("button", { name: "rerun_qc" }));
    expect(onRetryVariant).toHaveBeenCalledWith(failed, { kind: "qc", taskId: "", label: "rerun_qc" });
  });

  it("ignores legacy technical QC failures when visual QC passed", () => {
    const failed = variant("v01");
    failed.status = "action_required";
    failed.qc_status = "failed";
    failed.qc_reports = [
      qcReport({
        id: "technical-r2",
        variant_id: failed.id,
        revision: 2,
        lane: "technical",
        status: "failed",
        findings: {
          blocking_failures: ["已调用 qc-finalize。当前 Variant 因 technical lane 为 failed，整体结果为 action_required。"],
        },
        updated_at: "2026-08-05T00:00:00Z",
      }),
      qcReport({
        id: "visual-r2",
        variant_id: failed.id,
        revision: 2,
        lane: "visual",
        status: "passed",
        findings: {},
        updated_at: "2026-08-05T00:00:00Z",
      }),
    ];
    const orderItem = item();
    orderItem.variants = [failed];
    const order = {
      status: "partial",
      derived_status: "action_required",
      workflow_failures: [{
        task_id: "qc-task",
        workflow: "creative_qc_technical",
        subject_id: failed.id,
        failure_reason: "agent_reported_action_required",
        error: "已调用 qc-finalize。当前 Variant 因 technical lane 为 failed，整体结果为 action_required。",
        retryable: true,
      }],
      items: [orderItem],
    } as CreativeOrder;

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(creativeVariantNeedsManualAction(failed)).toBe(false);
    expect(creativeOrderActionableWorkflowFailures(order)).toEqual([]);
    expect(creativeOrderStage(order)).toMatchObject({ key: "review", label: "可入图库", readyVariants: 1 });
    expect(creativeVariantAdoptionReadiness(failed)).toMatchObject({ ready: true });
    expect(screen.queryByText("质检未通过，需要人工确认当前成图是否可用")).not.toBeInTheDocument();
    expect(screen.queryByText(/qc-finalize|technical lane|action_required/)).not.toBeInTheDocument();
  });

  it("shows a business fallback when a failed QC report has no blocking detail", () => {
    const failed = variant("v01");
    failed.status = "action_required";
    failed.qc_status = "failed";
    failed.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: failed.id, revision: 2, lane: "technical", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: failed.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    const orderItem = item();
    orderItem.variants = [failed];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("视觉质检没有同步具体失败明细；请人工复核文字可读性、遮挡、数值一致性和整体画面质量。")).toBeInTheDocument();
    expect(screen.queryByText("报告未提供具体失败原因")).not.toBeInTheDocument();
  });

  it("translates common QC failure codes", () => {
    const failed = variant("v01");
    failed.status = "action_required";
    failed.qc_reports = [
      qcReport({
        id: "visual-r2",
        variant_id: failed.id,
        revision: 2,
        lane: "visual",
        status: "failed",
        findings: { blocking_failures: [{ code: "corner_overlap" }] },
        updated_at: "2026-08-05T00:00:00Z",
      }),
    ];
    const orderItem = item();
    orderItem.variants = [failed];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("四角品牌或商店区域疑似被画面内容遮挡。")).toBeInTheDocument();
    expect(screen.queryByText("corner_overlap")).not.toBeInTheDocument();
  });

  it("allows adding a risk-reviewed variant only when all brand component sizes are present", () => {
    const recoverable = variant("v01");
    recoverable.status = "action_required";
    recoverable.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: recoverable.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: recoverable.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    recoverable.qc_recovery_available = true;
    expect(creativeVariantRiskAdoptionReadiness(recoverable).allowed).toBe(true);

    const missingBrandComponents = structuredClone(recoverable);
    missingBrandComponents.assets = missingBrandComponents.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));
    expect(creativeVariantRiskAdoptionReadiness(missingBrandComponents)).toMatchObject({ allowed: false, status: "品牌组件完成 2/3，暂不可入库" });

    const previousFailure = structuredClone(recoverable);
    previousFailure.qc_reports = [qcReport({ id: "visual-r1", variant_id: previousFailure.id, revision: 1, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-04T00:00:00Z" })];
    expect(creativeVariantRiskAdoptionReadiness(previousFailure).allowed).toBe(false);

    const exhausted = structuredClone(recoverable);
    exhausted.qc_recovery_used = true;
    expect(creativeVariantRiskAdoptionReadiness(exhausted).allowed).toBe(true);
  });

  it("does not expose QC recovery actions after automatic recovery was used", () => {
    const exhausted = variant("v01");
    exhausted.status = "action_required";
    exhausted.qc_recovery_used = true;
    exhausted.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: exhausted.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: exhausted.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    const orderItem = item();
    orderItem.variants = [exhausted];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("检测记录")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加到成图库" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();
  });

  it("keeps all three selected variants independently addable and records feedback per variant", async () => {
    const orderItem = item();
    orderItem.variants = [variant("v01"), variant("v02"), variant("v03")];
    const onAddToGallery = vi.fn();
    const onCollectFeedback = vi.fn().mockResolvedValue(undefined);
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set(["v01"])} onAddToGallery={onAddToGallery} onCollectFeedback={onCollectFeedback} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("V01")).toBeInTheDocument();
    expect(screen.getByText("V02")).toBeInTheDocument();
    expect(screen.getByText("V03")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "已入图库" })).toBeDisabled();
    const galleryButtons = screen.getAllByRole("button", { name: "添加到成图库" });
    expect(galleryButtons).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "收集反馈" })).toHaveLength(3);

    fireEvent.click(galleryButtons[1]!);
    expect(onAddToGallery).toHaveBeenCalledWith("v03");

    fireEvent.click(screen.getAllByRole("button", { name: "收集反馈" })[1]!);
    fireEvent.change(screen.getByPlaceholderText("补充具体原因或建议"), { target: { value: "人物不一致" } });
    fireEvent.click(screen.getByRole("button", { name: "记录反馈" }));
    await waitFor(() => expect(onCollectFeedback).toHaveBeenCalledWith(orderItem.variants[1], "visual_direction_mismatch", "人物不一致"));
  });
});

describe("creative order delivery selection", () => {
  it("renders backend gallery states as business labels", () => {
    expect(creativeOrderStatusLabel("partial")).toBe("生成中");
    expect(creativeOrderStatusLabel("awaiting_adoption")).toBe("可入图库");
    expect(creativeOrderStatusLabel("completed")).toBe("成图完成");
    expect(creativeOrderStatusLabel("cancelled")).toBe("已结束");
  });

  it("keeps every non-undone gallery acceptance", () => {
    const accepted = creativeGalleryVariantIds([
      { id: "feedback-1", undo_of_id: "", subject_type: "variant", subject_id: "v01", event_type: "decision", decision: "accepted" },
      { id: "feedback-2", undo_of_id: "", subject_type: "variant", subject_id: "v03", event_type: "decision", decision: "accepted" },
      { id: "feedback-3", undo_of_id: "feedback-2", subject_type: "variant", subject_id: "v03", event_type: "undo", decision: "" },
    ]);

    expect([...accepted]).toEqual(["v01"]);
  });

  it("uses the current revision delivered asset for each required size", () => {
    const selected = creativeVariantDeliveryAssets(variant("v01"));
    expect(selected.map((asset) => asset.id)).toEqual([
      "v01-1080x1080",
      "v01-1200x628",
      "v01-800x1000",
    ]);
  });

  it("uses a revision's persisted size contract for a single-size precise edit", () => {
    const preciseEdit = variant("direct");
    preciseEdit.brief = { creative_direct_edit_delivery: { final_visual_validation: true } };
    preciseEdit.revisions = [{
      revision: 2,
      brief: preciseEdit.brief,
      status: "completed",
      expected_sizes: ["1080x1080"],
      activated_at: "2026-08-02T00:00:00Z",
      created_at: "2026-08-02T00:00:00Z",
      updated_at: "2026-08-02T00:00:00Z",
    }];
    preciseEdit.assets = preciseEdit.assets.filter((asset) => asset.revision === 2
      && asset.size_key === "1080x1080"
      && (asset.stage === "primed" || asset.stage === "delivered"));
    const directItem = item(preciseEdit.id);
    directItem.variants = [preciseEdit];

    expect(creativeVariantDeliveryAssets(preciseEdit)).toHaveLength(1);
    expect(creativeVariantArchiveEntries(preciseEdit, attachmentMap(directItem))).toHaveLength(1);
    expect(creativeVariantAdoptionReadiness(preciseEdit)).toEqual({
      ready: true,
      status: "目标尺寸品牌组件已完成，可以加入成图库",
    });

    render(<CreativeOrderDeliveryCandidates orderId="order-direct" item={directItem} source={{ label: "原素材", url: "https://cdn.example/source.png" }} attachments={attachmentMap(directItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);
    expect(screen.getByAltText("DIRECT 方形主预览")).toBeInTheDocument();
    expect(screen.queryByAltText("DIRECT 横版主预览")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加到成图库" })).toBeEnabled();
  });

  it("does not let a legacy direct-edit flag bypass Prime completeness", () => {
    const preciseEdit = variant("direct");
    preciseEdit.brief = { creative_direct_edit_delivery: { skip_qc: true } };
    preciseEdit.revisions = [{
      revision: 2,
      brief: preciseEdit.brief,
      status: "active",
      expected_sizes: ["1080x1080"],
      activated_at: "2026-08-02T00:00:00Z",
      created_at: "2026-08-02T00:00:00Z",
      updated_at: "2026-08-02T00:00:00Z",
    }];
    preciseEdit.assets = preciseEdit.assets.filter((asset) => asset.revision === 2 && asset.size_key === "1080x1080");
    preciseEdit.qc_reports = [];

    expect(creativeVariantAdoptionReadiness(preciseEdit)).toMatchObject({ ready: true });
    preciseEdit.assets = preciseEdit.assets.filter((asset) => asset.stage !== "primed");
    expect(creativeVariantAdoptionReadiness(preciseEdit)).toMatchObject({ ready: false });
  });

  it("allows a complete unactivated Prime package into the gallery but not directly into an archive", () => {
    const working = variant("v01");
    working.active_revision = 0;

    expect(creativeVariantDeliveryAssets(working)).toEqual([]);
    expect(creativeVariantPreviewAssets(working).map((asset) => asset.revision)).toEqual([2, 2, 2]);
    expect(creativeVariantAdoptionReadiness(working).ready).toBe(true);
  });

  it("keeps the active delivery addable when a newer revision failed before its primary result arrived", () => {
    const stagingFailed = variant("v01");
    stagingFailed.active_revision = 1;
    stagingFailed.staging_revision = 2;
    stagingFailed.status = "action_required";
    stagingFailed.assets = stagingFailed.assets.filter((asset) => !(asset.revision === 2 && asset.size_key === "1080x1080"));
    stagingFailed.action_required = {
      task_id: "direct-edit-task",
      workflow: "creative_production",
      failure_reason: "missing_direct_edit_output",
      detail: "direct image edit did not register a completed target generated asset",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [stagingFailed];

    expect(creativeVariantAdoptionReadiness(stagingFailed)).toEqual({ ready: true, status: "r2 修订草稿尚未生效；可将当前 r1 成图加入成图库" });
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByTestId("creative-staging-result-missing")).toHaveTextContent("调整结果尚未返回，当前仍展示已交付成图。");
    expect(screen.getByRole("button", { name: "加入当前 r1 成图" })).toBeEnabled();
  });

  it("keeps archives on delivered assets while allowing a complete Prime package into the gallery", () => {
    const candidate = variant("v01");
    candidate.assets = candidate.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "delivered" && asset.size_key === "800x1000"));
    expect(creativeVariantDeliveryAssets(candidate).map((asset) => asset.id)).toEqual([
      "v01-1080x1080",
      "v01-1200x628",
    ]);
    expect(creativeVariantArchiveEntries(candidate, attachmentMap({ variants: [candidate] } as CreativeOrderItem))).toHaveLength(2);
    expect(creativeVariantAdoptionReadiness(candidate)).toEqual({ ready: true, status: "三尺寸品牌组件已完成，可以加入成图库" });
  });

  it("requires a complete current-revision Prime package before adding to gallery", () => {
    const candidate = variant("v01");
    candidate.assets = candidate.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));

    expect(creativeVariantDeliveryAssets(candidate)).toHaveLength(3);
    expect(creativeVariantAdoptionReadiness(candidate)).toEqual({ ready: false, status: "等待品牌组件合成：已完成 2/3 个尺寸" });
  });

  it("keeps a complete Prime package addable regardless of QC state", () => {
    const warning = variant("v01");
    warning.qc_reports = [
      { id: "technical-warning", variant_id: warning.id, revision: 2, lane: "technical", status: "warning", findings: { blocking_failures: [] } },
      { id: "visual-passed", variant_id: warning.id, revision: 2, lane: "visual", status: "passed", findings: {} },
    ] as CreativeOrderQCReport[];
    expect(creativeVariantAdoptionReadiness(warning).ready).toBe(true);

    const failed = structuredClone(warning);
    failed.status = "action_required";
    failed.qc_reports[1]!.status = "failed";
    expect(creativeVariantAdoptionReadiness(failed)).toEqual({ ready: true, status: "三尺寸品牌组件已完成，可以加入成图库" });

    const pending = structuredClone(warning);
    pending.qc_reports[1]!.status = "pending";
    expect(creativeVariantAdoptionReadiness(pending)).toEqual({ ready: true, status: "三尺寸品牌组件已完成，可以加入成图库" });
  });

  it("builds a three-file archive manifest with stable size names", () => {
    const orderItem = item("v01");
    const selected = orderItem.variants[0]!;
    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem), undefined, orderItem).map((entry) => entry.filename)).toEqual([
      "20260802_P_AK_MY_Num_Regular_ALL_AI_11_1.png",
      "20260802_P_AK_MY_Num_Regular_ALL_AI_191_2.png",
      "20260802_P_AK_MY_Num_Regular_ALL_AI_45_3.png",
    ]);
  });

  it("uses the frozen order naming rule for delivery files", () => {
    const orderItem = item("v01");
    const selected = orderItem.variants[0]!;
    const order = {
      created_at: "2026-08-19T01:02:03Z",
      input_snapshot: {
        market_pack: {
          config: {
            brand: "AdaKami",
            market: "Indonesia",
            naming_rule: "{production_date}_P_AK_MY_{type}_Regular_ALL_AI_{size}_{material_number}",
            naming_size_abbreviations: { "1080x1080": "11", "1200x628": "191", "800x1000": "45" },
          },
        },
      },
    } as unknown as CreativeOrder;

    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem), order, orderItem).map((entry) => entry.filename)).toEqual([
      "20260802_P_AK_MY_Num_Regular_ALL_AI_11_1.png",
      "20260802_P_AK_MY_Num_Regular_ALL_AI_191_2.png",
      "20260802_P_AK_MY_Num_Regular_ALL_AI_45_3.png",
    ]);
  });

  it("uses the confirmed delivery type instead of repayment-plan selections", () => {
    const orderItem = item("v01");
    orderItem.copy_snapshot = { creative_type: "repayment_plan", delivery_naming: { type: "num" } };
    const selected = orderItem.variants[0]!;

    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem), undefined, orderItem)[0]?.filename).toBe(
      "20260802_P_AK_MY_Num_Regular_ALL_AI_11_1.png",
    );

    orderItem.copy_snapshot = { creative_type: "num", delivery_naming: { type: "repayment_plan" } };
    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem), undefined, orderItem)[0]?.filename).toBe(
      "20260802_P_AK_MY_Repayment Plan_Regular_ALL_AI_11_1.png",
    );
  });

  it("uses the video rule, size abbreviation, and duration without adding image placeholders", () => {
    const orderItem = item("v01");
    const selected = orderItem.variants[0]!;
    const asset = selected.assets.find((candidate) => candidate.revision === selected.revision && candidate.stage === "delivered" && candidate.size_key === "1080x1080")!;
    asset.metadata = { duration_seconds: 6 };
    const attachments = attachmentMap(orderItem);
    const attachment = attachments.get(asset.attachment_id)!;
    attachments.set(asset.attachment_id, { ...attachment, filename: "source.mp4", content_type: "video/mp4" });

    expect(creativeVariantArchiveEntries(selected, attachments, undefined, orderItem).find((entry) => entry.asset.id === asset.id)?.filename).toBe(
      "08_V_AK_MY_20260802_NUM_RATE DOWN_SX_AI_11_6.mp4",
    );
  });

  it("selects one deterministic report per lane for the active revision", () => {
    const candidate = variant("v01");
    candidate.qc_reports = [
      qcReport({ id: "visual-r1", variant_id: candidate.id, revision: 1, lane: "visual", status: "failed", findings: { blocking_failures: ["旧版问题"] }, updated_at: "2026-08-04T00:00:00Z" }),
      qcReport({ id: "visual-r2-old", variant_id: candidate.id, revision: 2, lane: "visual", status: "failed", findings: { blocking_failures: ["较早结果"] }, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2-new", variant_id: candidate.id, revision: 2, lane: "visual", status: "failed", findings: { blocking_failures: ["最终结果"] }, updated_at: "2026-08-05T00:01:00Z" }),
    ];

    expect(creativeVariantQCDetails(candidate)).toEqual([expect.objectContaining({
      lane: "visual",
      blockingFailures: ["最终结果"],
    })]);
  });

  it("keeps the active delivery usable while a newer staging revision needs work", () => {
    const candidate = variant("v01");
    candidate.revision = 3;
    candidate.active_revision = 2;
    candidate.staging_revision = 3;
    candidate.candidate_state = "selected";
    candidate.selection_rank = 1;
    candidate.status = "action_required";
    candidate.qc_recovery_available = true;
    candidate.revisions = [
      { revision: 2, brief: {}, status: "active", expected_sizes: sizes, activated_at: "2026-08-05T00:00:00Z", created_at: "2026-08-05T00:00:00Z", updated_at: "2026-08-05T00:00:00Z" },
      { revision: 3, brief: {}, status: "action_required", expected_sizes: ["800x1000"], activated_at: "", created_at: "2026-08-06T00:00:00Z", updated_at: "2026-08-06T00:01:00Z" },
    ];
    candidate.action_required = {
      task_id: "r3-task",
      workflow: "creative_qc_visual",
      failure_reason: "agent_reported_action_required",
      detail: "r3 竖版仍需调整",
      failed_at: "2026-08-06T00:00:00Z",
      retryable: true,
    };
    candidate.assets.push({
      id: "v01-r3-800x1000-prime",
      variant_id: candidate.id,
      size_key: "800x1000",
      revision: 3,
      stage: "primed",
      status: "completed",
      attachment_id: "v01-r3-800x1000-prime",
      updated_at: "2026-08-06T00:00:00Z",
    } as CreativeOrderVariant["assets"][number]);
    candidate.qc_reports.push(qcReport({
      id: "visual-r3",
      variant_id: candidate.id,
      revision: 3,
      lane: "visual",
      status: "failed",
      findings: { blocking_failures: ["竖版白字对比不足"] },
      updated_at: "2026-08-06T00:01:00Z",
    }));

    expect(creativeVariantDeliveryAssets(candidate).map((asset) => asset.revision)).toEqual([2, 2, 2]);
    expect(creativeVariantPreviewAssets(candidate).map((asset) => asset.id)).toEqual([
      "v01-1080x1080",
      "v01-1200x628",
      "v01-800x1000",
    ]);
    expect(creativeVariantAdoptionReadiness(candidate)).toMatchObject({ ready: true });
    expect(creativeVariantNeedsManualAction(candidate)).toBe(false);
    expect(creativeVariantQCDetails(candidate)).toEqual([
      expect.objectContaining({ lane: "visual", status: "passed", blockingFailures: [] }),
    ]);

    const orderItem = item();
    orderItem.variants = [candidate];
    const onAssetSelect = vi.fn();
    const onRetryVariant = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={onAssetSelect} onRetryVariant={onRetryVariant} />);
    expect(screen.getByText("当前成图")).toBeInTheDocument();
    expect(screen.getByText("正在更新")).toBeInTheDocument();
    const cover = screen.getByAltText("V01 方形主预览");
    expect(cover).toHaveAttribute("src", "https://cdn.example/v01-1080x1080.png");
    const card = cover.closest("article");
    expect(card).not.toBeNull();
    fireEvent.click(within(card!).getByRole("button", { name: "横版可查看" }));
    expect(onAssetSelect).toHaveBeenCalledWith("v01-1200x628");
    expect(within(card!).getByRole("button", { name: "竖版可查看" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "加入当前 r2 成图" })).toBeEnabled();
    expect(screen.getByText("调整需要处理")).toBeInTheDocument();
    expect(screen.getByText("r3 竖版仍需调整")).toBeInTheDocument();
    expect(screen.getByText("竖版白字对比不足")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "rerun_qc" }));
    expect(onRetryVariant).toHaveBeenCalledWith(candidate, { kind: "qc", taskId: "", label: "rerun_qc" });

  });

  it("keeps exploratory primary images and reserves out of the delivery page", () => {
    const candidate = variant("c01");
    candidate.revision = 1;
    candidate.active_revision = 0;
    candidate.staging_revision = 1;
    candidate.candidate_state = "candidate";
    candidate.selection_rank = 0;
    candidate.primary_size = "1200x628";
    candidate.status = "completed";
    candidate.assets = [{
      id: "c01-landscape-prime",
      variant_id: candidate.id,
      size_key: "1200x628",
      revision: 1,
      stage: "primed",
      status: "completed",
      attachment_id: "c01-landscape-prime",
      updated_at: "2026-08-06T00:00:00Z",
    } as CreativeOrderVariant["assets"][number]];
    candidate.qc_reports = [];

    const reserve = structuredClone(candidate);
    reserve.id = "c02";
    reserve.variant_key = "C02";
    reserve.candidate_state = "reserve";
    reserve.selection_rank = 4;
    reserve.assets = reserve.assets.map((asset) => ({ ...asset, id: "c02-landscape-prime", variant_id: reserve.id, attachment_id: "c02-landscape-prime" }));

    const orderItem = item();
    orderItem.variants = [reserve, candidate];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByTestId("creative-order-candidate-selection-pending")).toBeInTheDocument();
    expect(screen.queryByAltText("C01 横版主预览")).not.toBeInTheDocument();
    expect(screen.queryByText("后备第 4 名")).not.toBeInTheDocument();
    expect(creativeVariantAdoptionReadiness(reserve)).toEqual({ ready: false, status: "后备方案暂不参与交付" });
    expect(creativeVariantNeedsManualAction(reserve)).toBe(false);
  });

  it("treats an unknown provider receipt as ongoing work instead of a failure", () => {
    const pending = variant("v01", false);
    pending.revision = 3;
    pending.active_revision = 0;
    pending.staging_revision = 3;
    pending.candidate_state = "selected";
    pending.status = "action_required";
    pending.action_required = {
      task_id: "late-task",
      workflow: "creative_production",
      failure_reason: "empty_result",
      detail: "尚未收到最终回执",
      failed_at: "2026-08-06T00:00:38Z",
      retryable: true,
    };
    pending.image_operations = [{
      id: "operation-1",
      variant_id: pending.id,
      size_key: "800x1000",
      revision: 3,
      operation_kind: "generation",
      idempotency_key: "v01:r3:800x1000",
      status: "unknown",
      attempts: [],
    } as unknown as NonNullable<CreativeOrderVariant["image_operations"]>[number]];

    expect(creativeVariantIsInProgress(pending)).toBe(true);
    expect(creativeVariantNeedsManualAction(pending)).toBe(false);
  });

  it("shows candidate comparison progress before three variants are selected", () => {
    const variants = Array.from({ length: 5 }, (_, index) => {
      const candidate = variant(`c0${index + 1}`, false);
      candidate.candidate_state = "candidate";
      candidate.primary_size = index % 2 === 0 ? "1200x628" : "800x1000";
      candidate.active_revision = 0;
      candidate.staging_revision = 1;
      candidate.status = "completed";
      candidate.revisions = [{
        revision: 1,
        brief: {},
        status: "completed",
        expected_sizes: sizes,
        activated_at: "",
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      }];
      candidate.assets = [{
        id: `${candidate.id}-primary`,
        variant_id: candidate.id,
        size_key: candidate.primary_size,
        revision: 1,
        stage: "primed",
        status: "completed",
        attachment_id: `${candidate.id}-primary`,
        updated_at: "2026-08-06T00:00:00Z",
      } as CreativeOrderVariant["assets"][number]];
      return candidate;
    });
    const orderItem = item();
    orderItem.variants = variants;
    const candidateOrder = {
      status: "running",
      derived_status: "awaiting_selection",
      workflow_failures: [],
      items: [orderItem],
    } as unknown as CreativeOrder;

    expect(creativeOrderStage(candidateOrder)).toMatchObject({
      key: "generating",
      label: "候选待筛选",
      detail: "候选主画面已就绪，等待筛选任务",
      totalVariants: 0,
    });

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);
    expect(screen.getByTestId("creative-order-candidate-selection-pending")).toBeInTheDocument();
    expect(screen.queryByText("5/5 张成图")).not.toBeInTheDocument();
  });

  it("counts selected targets and only the primary size for reserves and rejected candidates", () => {
    const selected = variant("s01", false);
    selected.active_revision = 0;
    selected.staging_revision = 2;
    selected.candidate_state = "selected";
    selected.revisions = [{ revision: 2, brief: {}, status: "running", expected_sizes: sizes, activated_at: "", created_at: "", updated_at: "" }];
    selected.assets = sizes.map((size) => ({
      id: `s01-${size}-generated`,
      variant_id: selected.id,
      size_key: size,
      revision: 2,
      stage: "generated",
      status: "completed",
      attachment_id: `s01-${size}-generated`,
      updated_at: "2026-08-06T00:00:00Z",
    })) as CreativeOrderVariant["assets"];
    selected.qc_reports = [];

    const reserve = variant("r01", false);
    reserve.active_revision = 0;
    reserve.staging_revision = 2;
    reserve.candidate_state = "reserve";
    reserve.primary_size = "1200x628";
    reserve.revisions = [{ revision: 2, brief: {}, status: "completed", expected_sizes: sizes, activated_at: "", created_at: "", updated_at: "" }];
    reserve.assets = reserve.assets.filter((asset) => asset.revision === 2 && asset.stage === "primed" && asset.size_key === reserve.primary_size);
    reserve.status = "completed";
    reserve.qc_reports = [];

    const rejected = structuredClone(reserve);
    rejected.id = "x01";
    rejected.variant_key = "X01";
    rejected.candidate_state = "rejected";
    rejected.primary_size = "800x1000";
    rejected.assets = [{
      ...reserve.assets[0]!,
      id: "x01-800x1000-prime",
      variant_id: rejected.id,
      size_key: rejected.primary_size,
      attachment_id: "x01-800x1000-prime",
    }];

    const orderItem = item();
    orderItem.variants = [selected, reserve, rejected];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} galleryVariantIds={new Set()} onAddToGallery={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getAllByText(/3\/3/)).toHaveLength(1);
    expect(screen.queryByText("R01")).not.toBeInTheDocument();
    expect(screen.queryByText("X01")).not.toBeInTheDocument();
  });
});
