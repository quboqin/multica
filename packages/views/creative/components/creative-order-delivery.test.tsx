// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Attachment, CreativeOrder, CreativeOrderItem, CreativeOrderQCReport, CreativeOrderVariant } from "@multica/core/types";
import {
  adoptedCreativeOrderVariant,
  CreativeOrderDeliveryCandidates,
  creativeOrderStage,
  creativeOrderStatusLabel,
  creativeVariantArchiveEntries,
  creativeVariantAdoptionReadiness,
  creativeVariantCanRepairPrime,
  creativeVariantCanRetryQC,
  creativeVariantDeliveryAssets,
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
    qc_reports: [
      { id: `${id}-technical`, variant_id: id, revision: 2, lane: "technical", status: complete ? "passed" : "pending" },
      { id: `${id}-visual`, variant_id: id, revision: 2, lane: "visual", status: complete ? "passed" : "pending" },
    ],
  } as CreativeOrderVariant;
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
      label: "需要处理",
    });
  });

  it("treats cancelled orders as retained history instead of active work", () => {
    expect(creativeOrderStage({ status: "cancelled", derived_status: "cancelled", items: [], workflow_failures: [] } as unknown as CreativeOrder)).toMatchObject({
      key: "cancelled",
      label: "已结束",
      action: "查看记录",
    });
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
  it("offers one adoption action per candidate and explains why an incomplete variant is disabled", () => {
    const orderItem = item();
    const onAdopt = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={onAdopt} onAssetSelect={vi.fn()} />);

    const actions = screen.getAllByRole("button", { name: "采用此变体" });
    expect(actions).toHaveLength(3);
    expect(actions[0]).toBeEnabled();
    expect(actions[1]).toBeDisabled();
    expect(screen.getByText("等待双路 QC：technical 待完成，visual 待完成")).toBeInTheDocument();
    expect(screen.getAllByAltText(/方形主预览/)).toHaveLength(3);
    expect(screen.queryAllByAltText(/横版主预览|竖版主预览/)).toHaveLength(0);

    fireEvent.click(actions[2]!);
    expect(onAdopt).toHaveBeenCalledWith("v03");
  });

  it("keeps cancelled order results visible but disables adoption", () => {
    const orderItem = item();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onAssetSelect={vi.fn()} disabled />);

    expect(screen.getAllByRole("button", { name: "订单已结束" })).toHaveLength(3);
    expect(screen.getAllByRole("button", { name: "订单已结束" }).every((button) => button.hasAttribute("disabled"))).toBe(true);
  });

  it("requires an explicit reason before adopting the current failed QC revision", () => {
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
    const onRetryQC = vi.fn();
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={onAdopt} onRetryQC={onRetryQC} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("视觉质检未通过 · r2")).toBeInTheDocument();
    expect(screen.getByText("800x1000：右上角二维码遮挡标题")).toBeInTheDocument();
    expect(screen.getByText("人物边缘略有锯齿")).toBeInTheDocument();
    expect(screen.queryByText("r1 旧问题不应展示")).not.toBeInTheDocument();
    expect(screen.getByText("QC 未通过")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "确认风险后采用" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "重新执行双路质检" }));
    expect(onRetryQC).toHaveBeenCalledWith(adjusted.id);

    fireEvent.click(screen.getByRole("button", { name: "确认风险后采用" }));
    expect(screen.getByRole("heading", { name: "确认带问题采用 V01？" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "确认风险并采用" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: "带问题采用原因" }), { target: { value: "投放档期已锁定，已确认二维码区域不影响当前渠道。" } });
    fireEvent.click(screen.getByRole("button", { name: "确认风险并采用" }));
    expect(onAdopt).toHaveBeenCalledWith(adjusted.id, {
      acknowledged: true,
      reason: "投放档期已锁定，已确认二维码区域不影响当前渠道。",
    });
  });

  it("limits QC recovery to the current failed QC revision with all Prime sizes", () => {
    const recoverable = variant("v01");
    recoverable.status = "action_required";
    recoverable.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: recoverable.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: recoverable.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    recoverable.qc_recovery_available = true;
    expect(creativeVariantCanRetryQC(recoverable)).toBe(true);

    const missingPrime = structuredClone(recoverable);
    missingPrime.assets = missingPrime.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));
    expect(creativeVariantCanRetryQC(missingPrime)).toBe(false);

    const previousFailure = structuredClone(recoverable);
    previousFailure.qc_reports = [qcReport({ id: "visual-r1", variant_id: previousFailure.id, revision: 1, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-04T00:00:00Z" })];
    expect(creativeVariantCanRetryQC(previousFailure)).toBe(false);

    const exhausted = structuredClone(recoverable);
    exhausted.qc_recovery_used = true;
    expect(creativeVariantCanRetryQC(exhausted)).toBe(false);
    expect(creativeVariantRiskAdoptionReadiness(exhausted).allowed).toBe(true);

    const missingPrimeForAdoption = structuredClone(exhausted);
    missingPrimeForAdoption.assets = missingPrimeForAdoption.assets.filter((asset) => !(asset.revision === 2 && asset.stage === "primed" && asset.size_key === "800x1000"));
    expect(creativeVariantRiskAdoptionReadiness(missingPrimeForAdoption)).toMatchObject({ allowed: false, status: "QC 未通过，且 Prime 仅完成 2/3 个尺寸" });
  });

  it("shows an exhausted QC recovery as an explicit manual action", () => {
    const exhausted = variant("v01");
    exhausted.status = "action_required";
    exhausted.qc_recovery_used = true;
    exhausted.qc_reports = [
      qcReport({ id: "technical-r2", variant_id: exhausted.id, revision: 2, lane: "technical", status: "passed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
      qcReport({ id: "visual-r2", variant_id: exhausted.id, revision: 2, lane: "visual", status: "failed", findings: {}, updated_at: "2026-08-05T00:00:00Z" }),
    ];
    const orderItem = item();
    orderItem.variants = [exhausted];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onRetryQC={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("双路 QC 恢复不可用，需修复 Prime 包或人工处理")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新执行双路质检" })).not.toBeInTheDocument();
  });

  it("offers a single Prime package repair only when the backend marks the current revision available", () => {
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
    expect(creativeVariantCanRepairPrime(repairable)).toBe(true);
    const onRepairPrime = vi.fn();
    const onRetryQC = vi.fn();
    const orderItem = item();
    orderItem.variants = [repairable];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onRepairPrime={onRepairPrime} onRetryQC={onRetryQC} onAssetSelect={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "修复 Prime 包并重新质检" }));
    expect(onRepairPrime).toHaveBeenCalledWith(repairable.id);
    expect(screen.queryByRole("button", { name: "重新执行双路质检" })).not.toBeInTheDocument();
    expect(screen.queryByText("双路 QC 恢复不可用，需修复 Prime 包或人工处理")).not.toBeInTheDocument();

    const exhausted = structuredClone(repairable);
    exhausted.prime_repair_used = true;
    expect(creativeVariantCanRepairPrime(exhausted)).toBe(false);
  });

  it("shows manual handling after the only Prime package repair was used", () => {
    const exhausted = variant("v01");
    exhausted.status = "action_required";
    exhausted.prime_repair_used = true;
    const orderItem = item();
    orderItem.variants = [exhausted];
    render(<CreativeOrderDeliveryCandidates orderId="order-1" item={orderItem} source={{ label: "竞品原图", url: "https://cdn.example/source.png" }} attachments={attachmentMap(orderItem)} adoptingVariantId="" onAdopt={vi.fn()} onRepairPrime={vi.fn()} onAssetSelect={vi.fn()} />);

    expect(screen.getByText("Prime 包已修复一次，需人工处理")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "修复 Prime 包并重新质检" })).not.toBeInTheDocument();
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
    expect(creativeOrderStatusLabel("awaiting_adoption")).toBe("待采用");
    expect(creativeOrderStatusLabel("completed")).toBe("已完成");
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
    expect(creativeVariantAdoptionReadiness(candidate)).toEqual({ ready: false, status: "等待 Prime：已完成 2/3 个尺寸" });
  });

  it("accepts warning QC but keeps failed and pending lanes blocked", () => {
    const warning = variant("v01");
    warning.qc_reports = [
      { id: "technical-warning", variant_id: warning.id, revision: 2, lane: "technical", status: "warning", findings: { blocking_failures: [] } },
      { id: "visual-passed", variant_id: warning.id, revision: 2, lane: "visual", status: "passed", findings: {} },
    ] as CreativeOrderQCReport[];
    expect(creativeVariantAdoptionReadiness(warning).ready).toBe(true);

    const failed = structuredClone(warning);
    failed.qc_reports[0]!.status = "failed";
    expect(creativeVariantAdoptionReadiness(failed).ready).toBe(false);

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
