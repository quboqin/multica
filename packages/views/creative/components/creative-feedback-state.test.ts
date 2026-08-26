import { describe, expect, it } from "vitest";
import type { CreateCreativeFeedbackResponse, CreativeOrder, CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant, CreativeSourceAnalysis } from "@multica/core/types";
import {
  canCreateDirectEdit,
  candidateDecisionFeedbackInput,
  candidateFeedbackIdempotencyKey,
  creativeFeedbackIdempotencyKey,
  creativeOrderItemInput,
  creativeSquadMemberSnapshot,
  creativeSubmissionKey,
  latestCompletedAnalyses,
  latestCandidateFeedback,
  materialAnalysisState,
  recommendedCopyDecisionFeedbackInput,
  recommendedCopyViewedFeedbackInput,
  recoveryForSubmissionKey,
} from "./creative-material-library";
import { creativeVariantAdoptionReadiness } from "./creative-order-delivery";
import {
  creativeAnnotationAdjustmentSummary,
  creativeOrderAdjustmentIssueDescription,
  creativeOrderAdjustmentIssueMetadata,
  creativeOrderAdjustmentIssueRequest,
  creativeOrderAdjustmentIssueTitle,
  creativeOrderAdoptionStatus,
  creativeOrderSquadId,
  creativeStudioPath,
  selectCreativeReviewAssets,
} from "./creative-studio-page";
import { creativeAdjustmentCanRetry, creativeAdjustmentProgress, creativeAdjustmentTarget, creativeAdjustmentTimeline, latestOrderAdjustmentFeedback } from "../lib/creative-adjustment-progress";

function readyVariant(id: string): CreativeOrderVariant {
  const sizes = ["1080x1080", "1200x628", "800x1000"];
  return {
    id,
    revision: 2,
    status: "completed",
    assets: sizes.flatMap((size) => [
      { id: `${id}-${size}-prime`, variant_id: id, size_key: size, revision: 2, stage: "primed", status: "completed", attachment_id: `${id}-${size}-attachment` },
      { id: `${id}-${size}-delivered`, variant_id: id, size_key: size, revision: 2, stage: "delivered", status: "completed", attachment_id: `${id}-${size}-attachment` },
    ]),
    qc_reports: [
      { id: `${id}-technical`, variant_id: id, lane: "technical", revision: 2, status: "passed" },
      { id: `${id}-visual`, variant_id: id, lane: "visual", revision: 2, status: "passed" },
    ],
  } as CreativeOrderVariant;
}

describe("creative feedback state", () => {
  it("builds one structured order adjustment issue from the canvas target", () => {
    const input = {
      request: "标题上移，保留整体样式",
      orderId: "order-1",
      itemId: "item-2",
      variantId: "variant-3",
      variantKey: "V01",
      assetId: "asset-4",
      attachmentId: "019fb932-3b59-77ab-841a-c57598f81097",
      sizeKey: "1080x1080",
      scope: "size",
      sourceRevision: 1,
    } as const;
    const description = creativeOrderAdjustmentIssueDescription(input);
    const metadata = creativeOrderAdjustmentIssueMetadata(input);

    expect(creativeOrderAdjustmentIssueTitle(input)).toBe("V01 / 方形 精准调整 · R2");
    expect(description).toContain("creative_order_id: order-1");
    expect(description).toContain("creative_order_item_id: item-2");
    expect(description).toContain("variant_id: variant-3");
    expect(description).toContain("asset_id: asset-4");
    expect(description).toContain("attachment_id: 019fb932-3b59-77ab-841a-c57598f81097");
    expect(description).toContain("**目标成图：** 订单 `order-1` · `V01` · 方形 `1080x1080` · `r1` · 成图 `asset-4`");
    expect(description).toContain("![目标成图：V01 方形 r1](/api/attachments/019fb932-3b59-77ab-841a-c57598f81097/download)");
    expect(description).toContain("Process only this size");
    expect(metadata).toMatchObject({
      workflow: "creative_adjustment",
      creative_adjustment_source: "creative_order",
      creative_scope: "size",
      creative_source_revision: 1,
      creative_revision: 2,
    });
    expect(creativeOrderAdjustmentIssueRequest(input, "issue-parent", "squad-1")).toMatchObject({
      title: "V01 / 方形 精准调整 · R2",
      parent_issue_id: "issue-parent",
      assignee_type: "squad",
      assignee_id: "squad-1",
      status: "backlog",
      allow_duplicate: true,
      metadata,
    });
  });

  it("marks order adjustment issues that apply to all three variant sizes", () => {
    const input = {
      request: "三个尺寸都把标题上移",
      orderId: "order-1",
      itemId: "item-2",
      variantId: "variant-3",
      variantKey: "V01",
      assetId: "asset-4",
      attachmentId: "019fb932-3b59-77ab-841a-c57598f81097",
      sizeKey: "1080x1080",
      scope: "variant",
      sourceRevision: 1,
    } as const;

    expect(creativeOrderAdjustmentIssueTitle(input)).toBe("V01 / 三尺寸 精准调整 · R2");
    expect(creativeOrderAdjustmentIssueDescription(input)).toContain("scope: variant");
    expect(creativeOrderAdjustmentIssueDescription(input)).toContain("Apply this adjustment across all expected sizes");
    expect(creativeOrderAdjustmentIssueMetadata(input)).toMatchObject({
      creative_scope: "variant",
      creative_size: "1080x1080",
    });
  });

  it("keeps order adjustment scope on the current canvas size", () => {
    expect(creativeAnnotationAdjustmentSummary([
      { kind: "rect", x: 0, y: 0, width: 0.1, height: 0.1, issueType: "other", scope: "order", comment: "标题上移" },
      { kind: "point", x: 0.2, y: 0.3, width: 0, height: 0, issueType: "artifact", scope: "variant", comment: "按钮提亮" },
    ])).toBe("标注 1（当前尺寸）：标题上移\n标注 2（当前尺寸）：按钮提亮");

    expect(creativeOrderSquadId({ input_snapshot: { squad_snapshot: { squad_id: "squad-1" } } })).toBe("squad-1");
  });

  it("keeps the latest order adjustment visible and derives its progress from the variant revision", () => {
    const older = { id: "f1", decision: "needs_revision", created_at: "2026-08-05T10:00:00Z", context_snapshot: { order_id: "order-1", revision: 1, size_key: "1080x1080" } };
    const latest = { id: "f2", decision: "needs_revision", created_at: "2026-08-05T10:01:00Z", context_snapshot: { order_id: "order-1", revision: 1, size_key: "1080x1080" } };
    const ignored = { id: "f3", decision: "needs_revision", created_at: "2026-08-05T10:02:00Z", context_snapshot: { order_id: "order-2", revision: 1, size_key: "1080x1080" } };

    expect(latestOrderAdjustmentFeedback([older, ignored, latest] as never, "order-1")?.id).toBe("f2");
    expect(creativeAdjustmentProgress({ revision: 1, status: "action_required" } as CreativeOrderVariant, latest as never)).toBe("调整未启动");
    expect(creativeAdjustmentCanRetry({ revision: 1, status: "action_required" } as CreativeOrderVariant, latest as never)).toBe(true);
    expect(creativeAdjustmentProgress({ revision: 2, status: "partial" } as CreativeOrderVariant, latest as never)).toBe("当前尺寸调整中 · r2");
    expect(creativeAdjustmentProgress({ revision: 2, status: "partial" } as CreativeOrderVariant, { ...latest, context_snapshot: { ...latest.context_snapshot, scope: "variant" } } as never)).toBe("三尺寸调整中 · r2");
    expect(creativeAdjustmentProgress({ revision: 2, status: "action_required" } as CreativeOrderVariant, latest as never)).toBe("调整需要处理 · r2");
    expect(creativeAdjustmentProgress({ revision: 2, status: "completed" } as CreativeOrderVariant, latest as never)).toBe("调整已完成 · r2");
  });

  it("follows a replacement variant record to the latest revision of the same item and variant key", () => {
    const event = { context_snapshot: { variant_id: "v01-r1", revision: 1 } } as unknown as CreateCreativeFeedbackResponse;
    const items = [{
      id: "item-1",
      variants: [
        { id: "v01-r1", variant_key: "V01", revision: 1, status: "completed" },
        { id: "v01-r2", variant_key: "V01", revision: 2, status: "completed" },
        { id: "v02-r1", variant_key: "V02", revision: 1, status: "completed" },
      ],
    }] as CreativeOrderItem[];

    expect(creativeAdjustmentTarget(items, event)?.variant.id).toBe("v01-r2");
  });

  it("derives the user-facing adjustment steps from order assets and QC", () => {
    const event = { context_snapshot: { revision: 1 } } as never;
    const variant = {
      revision: 2,
      status: "partial",
      assets: ["1080x1080", "1200x628", "800x1000"].flatMap((size) => [
        { revision: 2, stage: "generated", status: "completed", size_key: size },
        { revision: 2, stage: "primed", status: "completed", size_key: size },
      ]),
      qc_reports: [{ revision: 2, lane: "technical", status: "passed" }],
    } as CreativeOrderVariant;
    expect(creativeAdjustmentTimeline(variant, event).map((step) => [step.key, step.status])).toEqual([
      ["submitted", "done"],
      ["planned", "done"],
      ["generated", "done"],
      ["primed", "done"],
      ["qc", "current"],
      ["completed", "pending"],
    ]);
  });

  it("tracks precise single-size adjustment progress without waiting for three sizes", () => {
    const event = { context_snapshot: { revision: 1, scope: "size", size_key: "1080x1080" } } as never;
    const variant = {
      revision: 2,
      status: "partial",
      assets: [
        { revision: 2, stage: "generated", status: "completed", size_key: "1080x1080" },
        { revision: 2, stage: "primed", status: "completed", size_key: "1080x1080" },
      ],
      qc_reports: [{ revision: 2, lane: "technical", status: "passed" }],
    } as CreativeOrderVariant;

    expect(creativeAdjustmentTimeline(variant, event).map((step) => [step.label, step.status])).toEqual([
      ["已提交", "done"],
      ["精准调整", "done"],
      ["当前尺寸生成", "done"],
      ["品牌组件合成", "done"],
      ["视觉质检", "current"],
      ["完成", "pending"],
    ]);
  });

  it("uses the newest non-undone candidate decision", () => {
    const decisions = latestCandidateFeedback([
      { id: "selected", subject_id: "candidate-1", event_type: "decision", decision: "selected", undo_of_id: "", created_at: "2026-08-04T10:00:00Z" },
      { id: "rejected", subject_id: "candidate-1", event_type: "decision", decision: "rejected", undo_of_id: "", created_at: "2026-08-04T10:01:00Z" },
      { id: "undo", subject_id: "candidate-1", event_type: "undo", decision: "", undo_of_id: "rejected", created_at: "2026-08-04T10:02:00Z" },
    ]);

    expect(decisions.get("candidate-1")).toMatchObject({ id: "selected", decision: "selected" });
  });

  it("uses the highest completed analysis version for each candidate", () => {
    const analyses = [
      { id: "v1", candidate_id: "candidate-1", analysis_version: 1, status: "completed", completed_at: "2026-08-04T10:00:00Z" },
      { id: "v2-pending", candidate_id: "candidate-1", analysis_version: 2, status: "running", completed_at: "" },
      { id: "v3", candidate_id: "candidate-1", analysis_version: 3, status: "completed", completed_at: "2026-08-04T10:02:00Z" },
    ] as CreativeSourceAnalysis[];

    expect(latestCompletedAnalyses(analyses).get("candidate-1")?.id).toBe("v3");
  });

  it("treats a newer source-analysis rerun as the current material analysis state", () => {
    const candidate = { id: "candidate-1", analysis_status: "pending", analysis_error: "" };
    const analyses = [
      { id: "completed-v1", candidate_id: "candidate-1", analysis_version: 1, status: "completed", completed_at: "2026-08-04T10:00:00Z", created_at: "2026-08-04T09:59:00Z" },
      { id: "running-v2", candidate_id: "candidate-1", analysis_version: 2, status: "running", completed_at: "", created_at: "2026-08-04T10:01:00Z" },
    ] as CreativeSourceAnalysis[];

    expect(materialAnalysisState(candidate, analyses)).toEqual({ ready: false, status: "running", error: "", version: 2 });
    expect(materialAnalysisState({ ...candidate, id: "candidate-without-analysis", analysis_status: "running" }, analyses))
      .toEqual({ ready: false, status: "running", error: "", version: 0 });
  });

  it("marks an order adopted only after every item has one persisted variant selection", () => {
    const order = {
      id: "order-1",
      items: [
        { adopted_variant_id: "variant-1", variants: [readyVariant("variant-1"), readyVariant("variant-2")] },
        { adopted_variant_id: "", variants: [readyVariant("variant-3"), readyVariant("variant-4")] },
      ],
    } as unknown as CreativeOrder;

    expect(creativeOrderAdoptionStatus(order)).toBe("待选择");
    order.items[1]!.adopted_variant_id = "variant-4";
    expect(creativeOrderAdoptionStatus(order)).toBe("已采用");
  });

  it("allows acceptance after three sizes are ready and treats failed QC as a user reminder", () => {
    const ready = readyVariant("variant-ready");
    expect(creativeVariantAdoptionReadiness(ready)).toEqual({
      ready: true,
      status: "三尺寸、品牌组件与质检均已完成，可以采用",
    });

    const missingBrandComponents = { ...ready, assets: ready.assets.filter((asset) => !(asset.stage === "primed" && asset.size_key === "800x1000")) };
    expect(creativeVariantAdoptionReadiness(missingBrandComponents)).toEqual({ ready: false, status: "等待品牌组件合成：已完成 2/3 个尺寸" });

    const failedQC = { ...ready, qc_reports: ready.qc_reports.map((report) => report.lane === "visual" ? { ...report, status: "failed" } : report) };
    expect(creativeVariantAdoptionReadiness(failedQC)).toEqual({ ready: true, status: "可查看并采用当前成图" });

    const warnedQC = { ...ready, qc_reports: ready.qc_reports.map((report) => report.lane === "visual" ? { ...report, status: "warning" } : report) };
    expect(creativeVariantAdoptionReadiness(warnedQC)).toEqual({ ready: true, status: "三尺寸、品牌组件与质检均已完成，可以采用" });

    const missingDelivery = { ...ready, assets: ready.assets.filter((asset) => !(asset.stage === "delivered" && asset.size_key === "1200x628")) };
    expect(creativeVariantAdoptionReadiness(missingDelivery)).toEqual({ ready: false, status: "等待正式交付：已完成 2/3 个尺寸" });
  });

  it("keeps the selected order in the creative studio URL and preserves unrelated query state", () => {
    expect(creativeStudioPath("/acme/creative", new URLSearchParams("source=inbox&tab=copy"), "orders", "order-1"))
      .toBe("/acme/creative?source=inbox&tab=orders&order=order-1");
    expect(creativeStudioPath("/acme/creative", new URLSearchParams("tab=orders&order=order-1"), "home"))
      .toBe("/acme/creative");
    expect(creativeStudioPath("/acme/creative", new URLSearchParams("source=workbench"), "materials", "", "available", "crawl-run-1"))
      .toBe("/acme/creative?source=workbench&tab=materials&run=crawl-run-1");
    expect(creativeStudioPath("/acme/creative", new URLSearchParams("source=workbench&tab=materials"), "resources", "", "available", "", "copy"))
      .toBe("/acme/creative?source=workbench&tab=resources&resource=copy");
  });

  it("requires a stable source, a request, and a squad before direct edit submits", () => {
    expect(canCreateDirectEdit({ source_attachment_id: "" }, "调整文字", "squad-1")).toBe(false);
    expect(canCreateDirectEdit({ source_attachment_id: "attachment-1" }, " ", "squad-1")).toBe(false);
    expect(canCreateDirectEdit({ source_attachment_id: "attachment-1" }, "调整文字", "")).toBe(false);
    expect(canCreateDirectEdit({ source_attachment_id: "attachment-1" }, "调整文字", "squad-1")).toBe(true);
  });

  it("derives a stable, payload-specific submission key for refresh recovery", async () => {
    await expect(creativeSubmissionKey({ mode: "direct", candidate: "candidate-1", request: "调整标题" })).resolves.toBe(
      await creativeSubmissionKey({ mode: "direct", candidate: "candidate-1", request: "调整标题" }),
    );
    await expect(creativeSubmissionKey({ mode: "direct", candidate: "candidate-1", request: "调整标题" })).resolves.not.toBe(
      await creativeSubmissionKey({ mode: "direct", candidate: "candidate-1", request: "调整 CTA" }),
    );
  });

  it("never reuses a partially created issue after submission inputs change", () => {
    const recovery = { issueId: "issue-1", orderId: "order-1", submissionKey: "submission-1" };

    expect(recoveryForSubmissionKey(recovery, "submission-1")).toBe(recovery);
    expect(recoveryForSubmissionKey(recovery, "submission-2")).toEqual({ issueId: "", orderId: "", submissionKey: "" });
  });

  it("derives stable feedback keys per submission, subject, event, decision, and candidate", () => {
    const first = creativeFeedbackIdempotencyKey("submission-1", "recommended_copy", "copy-1", "decision", "accepted", "candidate-1");
    expect(first).toBe(creativeFeedbackIdempotencyKey("submission-1", "recommended_copy", "copy-1", "decision", "accepted", "candidate-1"));
    expect(first).not.toBe(creativeFeedbackIdempotencyKey("submission-1", "recommended_copy", "copy-1", "decision", "accepted", "candidate-2"));
    expect(first).not.toBe(creativeFeedbackIdempotencyKey("submission-1", "recommended_copy", "copy-1", "replacement", "replaced", "candidate-1"));
  });

  it("records each recommended-copy view once per analysis and library version", () => {
    const copySnapshot = {
      schema_version: 3 as const, id: "22000000-0000-4000-8000-000000000001", library_id: "library-1", library_version: 54,
      composition_id: "22000000-0000-4000-8000-000000000001", composition_key: "dynamic-num", creative_type: "num" as const,
      headline: "Headline", subheadline: "", benefit: "Benefit", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [],
      recommendation: { score: 1, reasons: [], matched_signals: [] }, status: "approved" as const,
    };
    const input = recommendedCopyViewedFeedbackInput({
      candidateId: "candidate-1",
      sourceAnalysisId: "analysis-2",
      compositionId: "22000000-0000-4000-8000-000000000001",
      libraryId: "library-1",
      libraryVersion: 54,
      rank: 1,
      recommendationReasons: ["创意类型：还款计划", "匹配信号：分期"],
      copySnapshot,
    });

    expect(input).toEqual({
      idempotency_key: "creative:feedback:recommended-copy-viewed:candidate-1:analysis-2:library-1:54:22000000-0000-4000-8000-000000000001",
      issue_id: "",
      subject_type: "recommended_copy",
      subject_id: "22000000-0000-4000-8000-000000000001",
      event_type: "viewed",
      decision: "",
      context_snapshot: {
        candidate_id: "candidate-1",
        source_analysis_id: "analysis-2",
        copy_library_id: "library-1",
        copy_library_version: 54,
        composition_id: "22000000-0000-4000-8000-000000000001",
        rank: 1,
        recommendation_reasons: ["创意类型：还款计划", "匹配信号：分期"],
        copy_snapshot: copySnapshot,
      },
    });
    expect(input.idempotency_key).not.toBe(recommendedCopyViewedFeedbackInput({
      candidateId: "candidate-1",
      sourceAnalysisId: "analysis-3",
      compositionId: "22000000-0000-4000-8000-000000000001",
      libraryId: "library-1",
      libraryVersion: 54,
      rank: 1,
      recommendationReasons: [],
      copySnapshot,
    }).idempotency_key);
  });

  it("keeps squad roles as display data without interpreting localized role names", () => {
    expect(creativeSquadMemberSnapshot([
      { member_type: "agent", member_id: "agent-1", role: "任意业务称呼" },
      { member_type: "agent", member_id: "agent-2", role: "Another label" },
      { member_type: "member", member_id: "member-1", role: "Owner" },
    ])).toEqual([
      { agent_id: "agent-1", role: "任意业务称呼" },
      { agent_id: "agent-2", role: "Another label" },
    ]);
  });

  it("preserves material rejection reasons and retry identity in feedback", () => {
    const idempotencyKey = candidateFeedbackIdempotencyKey("candidate-1", "rejected", "action-1");
    expect(candidateDecisionFeedbackInput(
      { id: "candidate-1", source_run_id: "crawl-1", analysis_status: "completed" },
      "rejected",
      ["low_quality"],
      "图片模糊",
      idempotencyKey,
    )).toEqual({
      idempotency_key: idempotencyKey,
      issue_id: "",
      subject_type: "candidate",
      subject_id: "candidate-1",
      event_type: "decision",
      decision: "rejected",
      reason_codes: ["low_quality"],
      comment: "图片模糊",
      context_snapshot: { crawl_run_id: "crawl-1", analysis_status: "completed" },
    });
  });

  it("freezes the user's creative idea in the submitted order item", () => {
    expect(creativeOrderItemInput("candidate-1", "analysis-2", { headline: "已审核文案" }, "  保留人物，突出 CTA  ")).toEqual({
      candidate_id: "candidate-1",
      source_analysis_id: "analysis-2",
      copy_snapshot: { headline: "已审核文案" },
      direction: "保留人物，突出 CTA",
    });
  });

  it("records which dynamic Top 3 copy the user adopted", () => {
    expect(recommendedCopyDecisionFeedbackInput({
      submissionKey: "submission-1",
      issueId: "issue-1",
      orderId: "order-1",
      candidateId: "candidate-1",
      selectedCopyId: "copy-selected",
      selectedRank: 2,
      libraryId: "library-1",
      libraryVersion: 54,
    })).toEqual({
      idempotency_key: creativeFeedbackIdempotencyKey("submission-1", "recommended_copy", "copy-selected", "decision", "accepted", "candidate-1"),
      issue_id: "issue-1",
      subject_type: "recommended_copy",
      subject_id: "copy-selected",
      event_type: "decision",
      decision: "accepted",
      context_snapshot: { candidate_id: "candidate-1", order_id: "order-1", selected_rank: 2, copy_library_id: "library-1", copy_library_version: 54 },
    });
  });

  it("keeps the newest visible asset for every variant and size", () => {
    const assets = [
      { id: "v1-old", variant_id: "v1", size_key: "1080x1080", revision: 1, stage: "delivered", status: "completed", attachment_id: "a1", updated_at: "2026-08-01" },
      { id: "v1-new", variant_id: "v1", size_key: "1080x1080", revision: 2, stage: "generated", status: "completed", attachment_id: "a2", updated_at: "2026-08-02" },
      { id: "v1-wide", variant_id: "v1", size_key: "1200x628", revision: 1, stage: "primed", status: "completed", attachment_id: "a3", updated_at: "2026-08-01" },
      { id: "v2-square", variant_id: "v2", size_key: "1080x1080", revision: 1, stage: "generated", status: "completed", attachment_id: "a4", updated_at: "2026-08-01" },
      { id: "ignored", variant_id: "v2", size_key: "800x1000", revision: 1, stage: "generated", status: "running", attachment_id: "a5", updated_at: "2026-08-01" },
    ] as CreativeOrderAsset[];

    expect(selectCreativeReviewAssets(assets).map((asset) => asset.id)).toEqual(["v1-new", "v1-wide", "v2-square"]);
  });

  it("shows review assets in the business variant order instead of UUID order", () => {
    const assets = [
      { id: "asset-v03", variant_id: "aaa-v03", size_key: "1080x1080", revision: 1, stage: "primed", status: "completed", attachment_id: "a3", updated_at: "2026-08-01" },
      { id: "asset-v01", variant_id: "zzz-v01", size_key: "1080x1080", revision: 1, stage: "primed", status: "completed", attachment_id: "a1", updated_at: "2026-08-01" },
      { id: "asset-v02", variant_id: "mmm-v02", size_key: "1080x1080", revision: 1, stage: "primed", status: "completed", attachment_id: "a2", updated_at: "2026-08-01" },
    ] as CreativeOrderAsset[];

    expect(selectCreativeReviewAssets(assets, ["zzz-v01", "mmm-v02", "aaa-v03"]).map((asset) => asset.id)).toEqual([
      "asset-v01",
      "asset-v02",
      "asset-v03",
    ]);
  });
});
