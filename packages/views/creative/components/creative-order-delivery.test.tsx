// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Attachment, CreativeOrder, CreativeOrderItem, CreativeOrderQCReport, CreativeOrderVariant } from "@multica/core/types";
import {
  adoptedCreativeOrderVariant,
  CreativeOrderDeliveryCandidates,
  creativeOrderActionableWorkflowFailures,
  creativeOrderStage,
  creativeOrderStatusLabel,
  creativeVariantArchiveEntries,
  creativeVariantAdoptionReadiness,
  creativeVariantDeliveryAssets,
  creativeVariantIsInProgress,
  creativeVariantNeedsManualAction,
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
    copy_snapshot: { creative_type: "num", visual_direction: { theme: "RATE DOWN" } },
    direction: "突出免息利益点",
    adopted_variant_id: adoptedVariantId,
    adopted_at: adoptedVariantId ? "2026-08-05T10:00:00Z" : "",
    adopted_by: adoptedVariantId ? "user-1" : "",
    variants: [variant("v01"), variant("v02", false), variant("v03")],
  } as unknown as CreativeOrderItem;
}

function qcReport(input: Pick<CreativeOrderQCReport, "id" | "variant_id" | "revision" | "lane" | "status" | "findings" | "updated_at">): CreativeOrderQCReport {
  return { ...input, trigger_evidence_kind: "creative_order_variant_qc", trigger_evidence_ref_id: input.variant_id, created_at: input.updated_at };
}

describe("creative order stage", () => {
  it("keeps a ready variant actionable even when another workflow failed", () => {
    const order = {
      status: "partial",
      derived_status: "action_required",
      workflow_failures: [{ task_id: "failed-task" }],
      items: [item()],
    } as unknown as CreativeOrder;

    expect(creativeOrderStage(order)).toMatchObject({
      key: "review",
      label: "待验收",
      action: "选择最终方案",
      readyVariants: 2,
    });
  });

  it("moves adopted orders to delivery", () => {
    const adopted = item("v01");
    adopted.variants = [variant("v01")];
    expect(creativeOrderStage({ items: [adopted], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "delivered",
      action: "查看并下载",
    });
  });

  it("uses aggregate list status when order details are not loaded", () => {
    expect(creativeOrderStage({ status: "queued", derived_status: "awaiting_adoption", items: [], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "review",
      label: "待验收",
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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

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
    expect(creativeVariantAdoptionReadiness(stale)).toEqual({ ready: true, status: "三尺寸、品牌组件与质检均已完成，可以采用" });
    expect(creativeOrderActionableWorkflowFailures(order)).toEqual([]);
    expect(creativeOrderStage(order)).toMatchObject({ key: "review", label: "待验收", readyVariants: 1 });
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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} showDirectionDetails={false} />);

    expect(screen.getByAltText("原图 竞品原图")).toHaveAttribute("src", "https://cdn.example/source.png");
    expect(screen.getByText("每张成图旁都可查看完整模型提示词、冻结文案、生成记录和版本溯源。")).toBeInTheDocument();
    expect(screen.getByText("等待任务")).toBeInTheDocument();
    expect(screen.getByText("等待后台创建生成任务，完成后这里会出现成图和验收入口。")).toBeInTheDocument();
  });

  it("collapses the whole material package when requested", () => {
    const orderItem = item();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} defaultOpen={false} />);

    const section = screen.getByTestId("creative-order-item-item-1");
    expect(section).not.toHaveAttribute("open");
    expect(screen.queryByText("查看三套候选方案")).not.toBeInTheDocument();
  });

  it("offers review actions and disables adoption for incomplete variants", () => {
    const orderItem = item();
    const onAdopt = vi.fn();
    const onAssetSelect = vi.fn();
    const onAssetInfo = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={onAdopt} onAssetSelect={onAssetSelect} onAssetInfo={onAssetInfo} />);

    const actions = screen.getAllByRole("button", { name: "采用此变体" });
    expect(actions).toHaveLength(2);
    expect(actions[0]).toBeEnabled();
    expect(screen.getByRole("button", { name: "尚不可采用" })).toBeDisabled();
    expect(screen.getAllByRole("button", { name: "查看并标注" })).toHaveLength(3);
    expect(screen.getAllByText("处理中").length).toBeGreaterThan(0);
    expect(screen.getAllByAltText(/方形主预览/)).toHaveLength(3);
    expect(screen.queryAllByAltText(/横版主预览|竖版主预览/)).toHaveLength(0);
    fireEvent.click(screen.getAllByRole("button", { name: "查看方形成图详情" })[0]!);
    expect(onAssetSelect).not.toHaveBeenCalled();
    expect(onAssetInfo).toHaveBeenCalledWith("v01-1080x1080");

    fireEvent.click(actions[1]!);
    expect(onAdopt).toHaveBeenCalledWith("v03");
  });

  it("keeps cancelled order results visible but disables adoption", () => {
    const orderItem = item();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} disabled />);

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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getAllByText("未完成").length).toBeGreaterThan(0);
    expect(screen.getByText("底部 Prime 固定贴片区域被模型内容占用，未注册三尺寸成图。")).toBeInTheDocument();
    expect(screen.getByText("可以直接重试这个方案；也可以查看其他候选或标注调整。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试失败步骤" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "尚不可采用" })).toBeDisabled();
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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} onRetryVariant={onRetryVariant} />);

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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} onRetryVariant={onRetryVariant} />);

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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByAltText("V01 过程图片 返工输出")).toBeInTheDocument();
    expect(screen.getByText("过程图片")).toBeInTheDocument();
    expect(screen.queryByText(/已保留 1 张过程图片/)).not.toBeInTheDocument();
    expect(screen.getAllByText("未完成").length).toBeGreaterThan(0);
    expect(screen.getByText("方形图两次模型输出均将 CTA 放入底部 Prime 固定排除区。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /方形过程图/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /方形待成图/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: "尚不可采用" })).toBeDisabled();
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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /查看过程图片/ }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("V01 · 过程图片")).toBeInTheDocument();
    expect(screen.getByText("方形 · 1080x1080 · 贴片预览检查")).toBeInTheDocument();
    expect(screen.getByText("prime-collision-preview-1080x1080.png")).toBeInTheDocument();
  });

  it("separates direct adjustment before, after, current process, and reused process images", () => {
    const ready = variant("v01");
    ready.brief = { creative_direct_edit_delivery: { skip_qc: true, target_size: "1080x1080", source_revision: 1 } };
    ready.diagnostic_assets = [
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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /查看过程图片/ }));

    expect(screen.getByText("前后对照")).toBeInTheDocument();
    expect(screen.getByText("调整前原图")).toBeInTheDocument();
    expect(screen.getByText("调整后结果")).toBeInTheDocument();
    expect(screen.getByText("本次调整过程图")).toBeInTheDocument();
    expect(screen.getByText("视觉质检返工触发图")).toBeInTheDocument();
    expect(screen.getByText("沿用上一版过程图")).toBeInTheDocument();
    expect(screen.getByText("方形 · 1080x1080 · 模型改图回图")).toBeInTheDocument();
    expect(screen.getByText("竖版 · 800x1000 · 失败贴片成图")).toBeInTheDocument();
    expect(screen.getByText("横版 · 1200x628 · 沿用上一版")).toBeInTheDocument();
    expect(screen.getAllByText(/北京时间/).length).toBeGreaterThan(0);
    expect(screen.getByText(/2026\/08\/02 08:02:00/)).toBeInTheDocument();
  });

  it("lets users ignore a QC reminder and adopt the current revision", () => {
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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={onAdopt} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("视觉质检未通过")).toBeInTheDocument();
    expect(screen.getByText("800x1000：右上角品牌组件遮挡标题")).toBeInTheDocument();
    expect(screen.getByText("人物边缘略有锯齿")).toBeInTheDocument();
    expect(screen.queryByText("r1 旧问题不应展示")).not.toBeInTheDocument();
    expect(screen.getByText("检测记录")).toBeInTheDocument();
    expect(screen.getByText("可采用")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "采用此变体" }));
    expect(onAdopt).toHaveBeenCalledWith(adjusted.id, {
      acknowledged: true,
      reason: "用户确认采用当前成图",
    });
  });

  it("exposes visual QC retry for recoverable failed QC variants", () => {
    const failed = variant("v01");
    failed.status = "action_required";
    failed.qc_status = "failed";
    failed.qc_recovery_available = true;
    failed.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: failed.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: failed.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    const orderItem = item();
    orderItem.variants = [failed];
    const onRetryVariant = vi.fn();

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} onRetryVariant={onRetryVariant} />);

    fireEvent.click(screen.getByRole("button", { name: "重新质检" }));
    expect(onRetryVariant).toHaveBeenCalledWith(failed, { kind: "qc", taskId: "", label: "重新质检" });
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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(creativeVariantNeedsManualAction(failed)).toBe(false);
    expect(creativeOrderActionableWorkflowFailures(order)).toEqual([]);
    expect(creativeOrderStage(order)).toMatchObject({ key: "review", label: "待验收", readyVariants: 1 });
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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

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

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("四角品牌或商店区域疑似被画面内容遮挡。")).toBeInTheDocument();
    expect(screen.queryByText("corner_overlap")).not.toBeInTheDocument();
  });

  it("allows risk adoption only when the current failed QC revision has all brand component sizes", () => {
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
    expect(creativeVariantRiskAdoptionReadiness(missingBrandComponents)).toMatchObject({ allowed: false, status: "品牌组件完成 2/3，暂不可采用" });

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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("检测记录")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "采用此变体" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();
  });

  it("promotes the adopted variant with the source and all three final sizes", () => {
    const orderItem = item("v01");
    const onAssetInfo = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} onAssetInfo={onAssetInfo} />);

    expect(screen.getByTestId("creative-adopted-variant")).toHaveTextContent("最终采用方案");
    expect(screen.getByAltText("原图 竞品原图")).toBeInTheDocument();
    expect(screen.getByAltText("最终采用方案 1080x1080")).toBeInTheDocument();
    expect(screen.getByAltText("最终采用方案 1200x628")).toBeInTheDocument();
    expect(screen.getByAltText("最终采用方案 800x1000")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "下载交付包" })).toBeEnabled();
    expect(screen.getAllByRole("button", { name: /查看.*成图详情/ })).toHaveLength(9);
    fireEvent.click(screen.getAllByRole("button", { name: "查看横版成图详情" })[0]!);
    expect(onAssetInfo).toHaveBeenCalledWith("v01-1200x628");
    expect(screen.getByText("查看其他候选")).toBeInTheDocument();
  });
});

describe("creative order delivery selection", () => {
  it("renders backend adoption states as business labels", () => {
    expect(creativeOrderStatusLabel("partial")).toBe("生成中");
    expect(creativeOrderStatusLabel("awaiting_adoption")).toBe("待验收");
    expect(creativeOrderStatusLabel("completed")).toBe("已采用");
    expect(creativeOrderStatusLabel("cancelled")).toBe("已结束");
  });

  it("resolves only the persisted adopted variant", () => {
    expect(adoptedCreativeOrderVariant(item())?.id).toBeUndefined();
    expect(adoptedCreativeOrderVariant(item("v03"))?.id).toBe("v03");
  });

  it("uses the current revision delivered asset for each required size", () => {
    const selected = creativeVariantDeliveryAssets(variant("v01"));
    expect(selected.map((asset) => asset.id)).toEqual([
      "v01-1080x1080",
      "v01-1200x628",
      "v01-800x1000",
    ]);
  });

  it("never falls back to a Prime asset when the current delivery is incomplete", () => {
    const candidate = variant("v01");
    candidate.assets = candidate.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "delivered" && asset.size_key === "800x1000"));
    expect(creativeVariantDeliveryAssets(candidate).map((asset) => asset.id)).toEqual([
      "v01-1080x1080",
      "v01-1200x628",
    ]);
    expect(creativeVariantArchiveEntries(candidate, attachmentMap({ variants: [candidate] } as CreativeOrderItem))).toHaveLength(2);
    expect(creativeVariantAdoptionReadiness(candidate)).toEqual({ ready: false, status: "等待正式交付：已完成 2/3 个尺寸" });
  });

  it("requires a complete current-revision brand component package before adoption", () => {
    const candidate = variant("v01");
    candidate.assets = candidate.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));

    expect(creativeVariantDeliveryAssets(candidate)).toHaveLength(3);
    expect(creativeVariantAdoptionReadiness(candidate)).toEqual({ ready: false, status: "等待品牌组件合成：已完成 2/3 个尺寸" });
  });

  it("accepts warning QC, lets failed QC be adopted with a reminder, and keeps pending lanes blocked", () => {
    const warning = variant("v01");
    warning.qc_reports = [
      { id: "technical-warning", variant_id: warning.id, revision: 2, lane: "technical", status: "warning", findings: { blocking_failures: [] } },
      { id: "visual-passed", variant_id: warning.id, revision: 2, lane: "visual", status: "passed", findings: {} },
    ] as CreativeOrderQCReport[];
    expect(creativeVariantAdoptionReadiness(warning).ready).toBe(true);

    const failed = structuredClone(warning);
    failed.status = "action_required";
    failed.qc_reports[1]!.status = "failed";
    expect(creativeVariantAdoptionReadiness(failed)).toEqual({ ready: true, status: "可查看并采用当前成图" });

    const pending = structuredClone(warning);
    pending.qc_reports[1]!.status = "pending";
    expect(creativeVariantAdoptionReadiness(pending).ready).toBe(false);
  });

  it("builds a three-file archive manifest with stable size names", () => {
    const orderItem = item("v01");
    const selected = adoptedCreativeOrderVariant(orderItem)!;
    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem), undefined, orderItem).map((entry) => entry.filename)).toEqual([
      "08_P_AK_MY_20260802_NUM_RATE DOWN_SX_AI_11.png",
      "08_P_AK_MY_20260802_NUM_RATE DOWN_SX_AI_191.png",
      "08_P_AK_MY_20260802_NUM_RATE DOWN_SX_AI_45.png",
    ]);
  });

  it("uses the frozen order naming rule for delivery files", () => {
    const orderItem = item("v01");
    const selected = adoptedCreativeOrderVariant(orderItem)!;
    const order = {
      created_at: "2026-08-19T01:02:03Z",
      input_snapshot: {
        market_pack: {
          config: {
            brand: "AdaKami",
            market: "Indonesia",
            naming_rule: "{month}_P_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}",
            naming_size_abbreviations: { "1080x1080": "11", "1200x628": "191", "800x1000": "45" },
          },
        },
      },
    } as unknown as CreativeOrder;

    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem), order, orderItem).map((entry) => entry.filename)).toEqual([
      "08_P_AK_MY_20260802_NUM_RATE DOWN_SX_AI_11.png",
      "08_P_AK_MY_20260802_NUM_RATE DOWN_SX_AI_191.png",
      "08_P_AK_MY_20260802_NUM_RATE DOWN_SX_AI_45.png",
    ]);
  });

  it("uses the video rule, size abbreviation, and duration without adding image placeholders", () => {
    const orderItem = item("v01");
    const selected = adoptedCreativeOrderVariant(orderItem)!;
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
});
