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
    prime_repair_used: false,
    prime_repair_available: false,
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
    direction: "突出免息利益点",
    adopted_variant_id: adoptedVariantId,
    adopted_at: adoptedVariantId ? "2026-08-05T10:00:00Z" : "",
    adopted_by: adoptedVariantId ? "user-1" : "",
    variants: [variant("v01"), variant("v02", false), variant("v03")],
  } as CreativeOrderItem;
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
    } as CreativeOrder;

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

  it("treats delegated Prime handoff as running instead of a manual action", () => {
    const handoff = variant("v01", false);
    handoff.status = "action_required";
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
    handoff.action_required = {
      task_id: "production-task",
      workflow: "creative_production",
      failure_reason: "agent_reported_action_required",
      detail: "已委派唯一 Prime 任务：prime-task；Prime 状态：queued。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [handoff];
    const order = {
      status: "partial",
      derived_status: "action_required",
      workflow_failures: [{
        task_id: "production-task",
        workflow: "creative_production",
        subject_id: handoff.id,
        failure_reason: "agent_reported_action_required",
        error: "已委派唯一 Prime 任务：prime-task；Prime 状态：queued。",
        retryable: true,
      }],
      items: [orderItem],
    } as CreativeOrder;

    expect(creativeVariantNeedsManualAction(handoff)).toBe(false);
    expect(creativeVariantIsInProgress(handoff)).toBe(true);
    expect(creativeVariantAdoptionReadiness(handoff)).toEqual({ ready: false, status: "成图已完成，正在贴片：已完成 0/3 个尺寸" });
    expect(creativeOrderActionableWorkflowFailures(order)).toEqual([]);
    expect(creativeOrderStage(order)).toMatchObject({ key: "generating", label: "生成中" });
  });

  it("treats completed Prime registration as waiting for QC instead of a repairable failure", () => {
    const handoff = variant("v01", false);
    handoff.status = "action_required";
    handoff.assets = handoff.assets.filter((asset) => asset.stage === "primed" || asset.stage === "generated");
    handoff.action_required = {
      task_id: "prime-task",
      workflow: "creative_prime",
      failure_reason: "agent_reported_action_required",
      detail: "已完成 `87848071:v01:r1` 的 Prime 合成与登记。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [handoff];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(creativeVariantNeedsManualAction(handoff)).toBe(false);
    expect(creativeVariantIsInProgress(handoff)).toBe(true);
    expect(screen.getByText("处理中")).toBeInTheDocument();
    expect(screen.getByText("贴片已完成，等待质检：技术质检待完成、视觉质检待完成")).toBeInTheDocument();
    expect(screen.queryByText("需要处理")).not.toBeInTheDocument();
    expect(screen.queryByText("品牌贴片提醒")).not.toBeInTheDocument();
  });

  it("treats qc-finalize pending on the other lane as QC progress", () => {
    const pending = variant("v01", false);
    pending.status = "action_required";
    pending.assets = pending.assets.filter((asset) => asset.stage === "primed" || asset.stage === "generated");
    pending.action_required = {
      task_id: "visual-qc-task",
      workflow: "creative_qc_visual",
      failure_reason: "agent_reported_action_required",
      detail: "`qc-finalize` 结果：`pending`，因 technical lane 尚未完成；未修改 Issue 或其他 Variant。",
      failed_at: "2026-08-09T10:00:00Z",
      retryable: true,
    };
    const orderItem = item();
    orderItem.variants = [pending];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(creativeVariantNeedsManualAction(pending)).toBe(false);
    expect(creativeVariantIsInProgress(pending)).toBe(true);
    expect(screen.getByText("处理中")).toBeInTheDocument();
    expect(screen.getByText("贴片已完成，等待质检：技术质检待完成、视觉质检待完成")).toBeInTheDocument();
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
      detail: "已委派唯一 Prime 任务：prime-task；Prime 状态：queued。",
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
    expect(creativeVariantAdoptionReadiness(stale)).toEqual({ ready: true, status: "三尺寸、贴片与质检均已完成，可以采用" });
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
  it("shows the source image and final prompt while the backend has not created variants", () => {
    const orderItem = item();
    orderItem.variants = [];
    orderItem.direction = "保留白底和红色还款表；最终画面文字使用已审核文案。";

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={new Map()} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} showDirectionDetails={false} />);

    expect(screen.getByAltText("原图 竞品原图")).toHaveAttribute("src", "https://cdn.example/source.png");
    expect(screen.getByText("最终出图提示词")).toBeInTheDocument();
    expect(screen.getByText("保留白底和红色还款表；最终画面文字使用已审核文案。")).toBeInTheDocument();
    expect(screen.getByText("等待任务")).toBeInTheDocument();
    expect(screen.getByText("等待后台创建生成任务，完成后这里会出现过程图和验收入口。")).toBeInTheDocument();
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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={onAdopt} onAssetSelect={vi.fn()} />);

    const actions = screen.getAllByRole("button", { name: "采用此变体" });
    expect(actions).toHaveLength(2);
    expect(actions[0]).toBeEnabled();
    expect(screen.getByRole("button", { name: "尚不可采用" })).toBeDisabled();
    expect(screen.getAllByRole("button", { name: "查看并标注" })).toHaveLength(3);
    expect(screen.getByText("等待质检：技术质检待完成、视觉质检待完成")).toBeInTheDocument();
    expect(screen.getAllByAltText(/方形主预览/)).toHaveLength(3);
    expect(screen.queryAllByAltText(/横版主预览|竖版主预览/)).toHaveLength(0);

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

    expect(screen.getAllByText("生成未完成").length).toBeGreaterThan(0);
    expect(screen.getByText("成图生成未完成")).toBeInTheDocument();
    expect(screen.getByText("底部 Prime 固定贴片区域被模型内容占用，未注册三尺寸成图。")).toBeInTheDocument();
    expect(screen.getByText("后台已记录该步骤未补齐；不用手动重试，可查看其他候选、标注调整或重新发起。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试失败步骤" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "尚不可采用" })).toBeDisabled();
  });

  it("does not expose workflow retry actions to business users", () => {
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
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("后台已记录该步骤未补齐；不用手动重试，可查看其他候选、标注调整或重新发起。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试失败步骤" })).not.toBeInTheDocument();
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
      size_key: "1080x1080",
      revision: blocked.revision,
      label: "返工输出",
      filename: "square-model-rework.png",
      url: "/api/creative/orders/order-1/variants/v01/diagnostic-assets/diagnostic-1",
      created_at: "2026-08-09T10:00:00Z",
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

    expect(screen.getByAltText("V01 诊断图 返工输出")).toBeInTheDocument();
    expect(screen.getByText("未登记诊断图")).toBeInTheDocument();
    expect(screen.getByText("已保留 1 张未登记模型输出，仅用于判断停止原因，不参与采用或交付。")).toBeInTheDocument();
    expect(screen.getByText("生成未完成：方形图两次模型输出均将 CTA 放入底部 Prime 固定排除区。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /方形诊断图/ })).toBeEnabled();
    expect(screen.getByRole("button", { name: "尚不可采用" })).toBeDisabled();
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
          blocking_failures: [{ size_key: "800x1000", reason: "右上角二维码遮挡标题" }],
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
    expect(screen.getByText("800x1000：右上角二维码遮挡标题")).toBeInTheDocument();
    expect(screen.getByText("人物边缘略有锯齿")).toBeInTheDocument();
    expect(screen.queryByText("r1 旧问题不应展示")).not.toBeInTheDocument();
    expect(screen.getByText("有系统提醒")).toBeInTheDocument();
    expect(screen.getByText("系统提醒：视觉质检未通过，仍可查看、标注或忽略提醒采用")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "忽略提醒并采用" }));
    expect(onAdopt).toHaveBeenCalledWith(adjusted.id, {
      acknowledged: true,
      reason: "用户确认忽略系统提醒并采用",
    });
  });

  it("shows qc-finalize failures as system reminders without exposing internal lane text", () => {
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
    expect(screen.getByText("技术质检未通过")).toBeInTheDocument();
    expect(screen.getByText("质检未通过，需要人工确认当前成图是否可用")).toBeInTheDocument();
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

    expect(screen.getByText("技术质检没有同步具体失败明细；请人工复核文件完整性、贴片、二维码和渠道要求。")).toBeInTheDocument();
    expect(screen.getByText("视觉质检没有同步具体失败明细；请人工复核文字可读性、遮挡、数值一致性和整体画面质量。")).toBeInTheDocument();
    expect(screen.queryByText("报告未提供具体失败原因")).not.toBeInTheDocument();
  });

  it("translates common QC failure codes", () => {
    const failed = variant("v01");
    failed.status = "action_required";
    failed.qc_reports = [
      qcReport({
        id: "technical-r2",
        variant_id: failed.id,
        revision: 2,
        lane: "technical",
        status: "failed",
        findings: { blocking_failures: [{ code: "qr_independent_redecode_failed" }] },
        updated_at: "2026-08-05T00:00:00Z",
      }),
    ];
    const orderItem = item();
    orderItem.variants = [failed];

    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("二维码无法独立识别，可能影响渠道验收。")).toBeInTheDocument();
    expect(screen.queryByText("qr_independent_redecode_failed")).not.toBeInTheDocument();
  });

  it("allows risk adoption only when the current failed QC revision has all Prime sizes", () => {
    const recoverable = variant("v01");
    recoverable.status = "action_required";
    recoverable.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: recoverable.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: recoverable.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    recoverable.qc_recovery_available = true;
    expect(creativeVariantRiskAdoptionReadiness(recoverable).allowed).toBe(true);

    const missingPrime = structuredClone(recoverable);
    missingPrime.assets = missingPrime.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));
    expect(creativeVariantRiskAdoptionReadiness(missingPrime)).toMatchObject({ allowed: false, status: "系统提醒：贴片仅完成 2/3 个尺寸，暂不可采用" });

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

    expect(screen.getByText("有系统提醒")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "忽略提醒并采用" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();
  });

  it("does not expose Prime repair actions to business users", () => {
    const repairable = variant("v01");
    repairable.status = "action_required";
    repairable.prime_repair_available = true;
    repairable.qc_recovery_available = true;
    repairable.qc_reports[1]!.status = "failed";
    const primed = repairable.assets.filter((asset) => asset.revision === repairable.revision && asset.stage === "primed");
    repairable.assets = [...primed, ...primed.map((asset) => ({
      ...asset,
      id: `${asset.id}-generated`,
      stage: "generated" as const,
      attachment_id: `${asset.attachment_id}-generated`,
    }))];
    const orderItem = item();
    orderItem.variants = [repairable];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("有系统提醒")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "修复贴片并重新质检" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新质检" })).not.toBeInTheDocument();
  });

  it("keeps Prime repair exhaustion as a process exception without a repair button", () => {
    const exhausted = variant("v01", false);
    exhausted.status = "action_required";
    exhausted.prime_repair_used = true;
    exhausted.qc_reports = [];
    const orderItem = item();
    orderItem.variants = [exhausted];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getAllByText("有系统提醒").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "修复贴片并重新质检" })).not.toBeInTheDocument();
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
    expect(screen.getAllByRole("button", { name: /查看 .* 生成信息/ })).toHaveLength(3);
    fireEvent.click(screen.getByRole("button", { name: "查看 1200x628 生成信息" }));
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

  it("requires an independent current-revision Prime package before adoption", () => {
    const candidate = variant("v01");
    candidate.assets = candidate.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));

    expect(creativeVariantDeliveryAssets(candidate)).toHaveLength(3);
    expect(creativeVariantAdoptionReadiness(candidate)).toEqual({ ready: false, status: "等待贴片：已完成 2/3 个尺寸" });
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
    failed.qc_reports[0]!.status = "failed";
    expect(creativeVariantAdoptionReadiness(failed)).toEqual({ ready: true, status: "系统提醒：技术质检未通过，仍可查看、标注或忽略提醒采用" });

    const pending = structuredClone(warning);
    pending.qc_reports[0]!.status = "pending";
    expect(creativeVariantAdoptionReadiness(pending).ready).toBe(false);
  });

  it("builds a three-file archive manifest with stable size names", () => {
    const orderItem = item("v01");
    const selected = adoptedCreativeOrderVariant(orderItem)!;
    expect(creativeVariantArchiveEntries(selected, attachmentMap(orderItem)).map((entry) => entry.filename)).toEqual([
      "V01_1080x1080.png",
      "V01_1200x628.png",
      "V01_800x1000.png",
    ]);
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
