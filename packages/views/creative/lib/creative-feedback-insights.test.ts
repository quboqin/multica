import { describe, expect, it } from "vitest";
import type { CreateCreativeFeedbackResponse, CreativeFeedbackMetrics } from "@multica/core/types";
import { creativeFeedbackInsights, creativeFeedbackReasonSummary } from "./creative-feedback-insights";

describe("creative feedback insights", () => {
  it("calculates business rates without inventing data for empty totals", () => {
    const metrics = {
      candidate_selected: 3,
      candidate_rejected: 1,
      copy_accepted: 0,
      copy_replaced: 0,
      variant_accepted: 2,
      variant_needs_revision: 2,
      asset_accepted: 0,
      asset_reported: 0,
      qc_accepted: 4,
      qc_missed_issue: 1,
      qc_false_positive: 0,
    } satisfies CreativeFeedbackMetrics;

    expect(creativeFeedbackInsights(metrics).map((insight) => insight.rate)).toEqual([75, null, 50, 80]);
  });

  it("counts active feedback reasons and ignores undone events", () => {
    const events = [
      { id: "one", event_type: "decision", reason_codes: ["low_quality", "other"], undo_of_id: "" },
      { id: "two", event_type: "decision", reason_codes: ["low_quality"], undo_of_id: "" },
      { id: "undo", event_type: "undo", reason_codes: [], undo_of_id: "two" },
    ] as CreateCreativeFeedbackResponse[];

    expect(creativeFeedbackReasonSummary(events)).toEqual([
      { code: "low_quality", count: 1 },
      { code: "other", count: 1 },
    ]);
  });
});
