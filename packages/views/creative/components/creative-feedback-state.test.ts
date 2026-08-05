import { describe, expect, it } from "vitest";
import type { CreativeOrder, CreativeOrderAsset, CreativeOrderVariant, CreativeSourceAnalysis } from "@multica/core/types";
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
  recoveryForSubmissionKey,
} from "./creative-material-library";
import { creativeOrderAcceptanceStatus, creativeStudioPath, creativeVariantAcceptanceReadiness, latestVariantFeedback, selectCreativeReviewAssets } from "./creative-studio-page";

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
      { id: `${id}-visual`, variant_id: id, lane: "visual", revision: 2, status: "warning" },
    ],
  } as CreativeOrderVariant;
}

describe("creative feedback state", () => {
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

  it("prefers a matching completed analysis over a newer crawl-run pending status", () => {
    const candidate = { id: "candidate-1", analysis_status: "pending", analysis_error: "" };
    const analyses = [
      { id: "completed-v1", candidate_id: "candidate-1", analysis_version: 1, status: "completed", completed_at: "2026-08-04T10:00:00Z", created_at: "2026-08-04T09:59:00Z" },
      { id: "running-v2", candidate_id: "candidate-1", analysis_version: 2, status: "running", completed_at: "", created_at: "2026-08-04T10:01:00Z" },
    ] as CreativeSourceAnalysis[];

    expect(materialAnalysisState(candidate, analyses)).toEqual({ ready: true, status: "completed", error: "", version: 1 });
    expect(materialAnalysisState({ ...candidate, id: "candidate-without-analysis", analysis_status: "running" }, analyses))
      .toEqual({ ready: false, status: "running", error: "", version: 0 });
  });

  it("marks an order accepted only after every variant has an accepted decision", () => {
    const order = {
      id: "order-1",
      items: [{ variants: [readyVariant("variant-1"), readyVariant("variant-2")] }],
    } as unknown as CreativeOrder;
    const decisions = latestVariantFeedback([
      { id: "accept-1", subject_id: "variant-1", event_type: "decision", decision: "accepted", undo_of_id: "", created_at: "2026-08-04T10:00:00Z" },
      { id: "abandon-2", subject_id: "variant-2", event_type: "decision", decision: "abandoned", undo_of_id: "", created_at: "2026-08-04T10:01:00Z" },
    ]);

    expect(creativeOrderAcceptanceStatus(order, decisions)).toBe("待验收");
    decisions.set("variant-2", "accepted");
    expect(creativeOrderAcceptanceStatus(order, decisions)).toBe("已验收");
  });

  it("only allows acceptance after all three Prime and delivered sizes pass both QC lanes", () => {
    const ready = readyVariant("variant-ready");
    expect(creativeVariantAcceptanceReadiness(ready)).toEqual({
      ready: true,
      status: "三尺寸、Prime 与双路 QC 均已完成，可以验收",
    });

    const missingPrime = { ...ready, assets: ready.assets.filter((asset) => !(asset.stage === "primed" && asset.size_key === "800x1000")) };
    expect(creativeVariantAcceptanceReadiness(missingPrime)).toEqual({ ready: false, status: "等待 Prime：已完成 2/3 个尺寸" });

    const failedQC = { ...ready, qc_reports: ready.qc_reports.map((report) => report.lane === "visual" ? { ...report, status: "failed" } : report) };
    expect(creativeVariantAcceptanceReadiness(failedQC)).toEqual({ ready: false, status: "QC 未通过：technical 通过，visual 失败" });

    const missingDelivery = { ...ready, assets: ready.assets.filter((asset) => !(asset.stage === "delivered" && asset.size_key === "1200x628")) };
    expect(creativeVariantAcceptanceReadiness(missingDelivery)).toEqual({ ready: false, status: "等待正式交付：已完成 2/3 个尺寸" });
  });

  it("keeps the selected order in the creative studio URL and preserves unrelated query state", () => {
    expect(creativeStudioPath("/acme/creative", new URLSearchParams("source=inbox&tab=copy"), "orders", "order-1"))
      .toBe("/acme/creative?source=inbox&tab=orders&order=order-1");
    expect(creativeStudioPath("/acme/creative", new URLSearchParams("tab=orders&order=order-1"), "discovery"))
      .toBe("/acme/creative");
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

  it("records why the user did not adopt the recommended copy", () => {
    expect(recommendedCopyDecisionFeedbackInput({
      submissionKey: "submission-1",
      issueId: "issue-1",
      orderId: "order-1",
      candidateId: "candidate-1",
      recommendedCopyId: "copy-recommended",
      selectedCopyId: "copy-selected",
      replacementReason: "facts_inapplicable",
    })).toEqual({
      idempotency_key: creativeFeedbackIdempotencyKey("submission-1", "recommended_copy", "copy-recommended", "replacement", "replaced", "candidate-1"),
      issue_id: "issue-1",
      subject_type: "recommended_copy",
      subject_id: "copy-recommended",
      event_type: "replacement",
      decision: "replaced",
      reason_codes: ["facts_inapplicable"],
      context_snapshot: { candidate_id: "candidate-1", order_id: "order-1", replacement_copy_id: "copy-selected" },
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
