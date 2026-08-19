import { describe, expect, it } from "vitest";
import type { CreativeFeedbackDashboard } from "@multica/core/types";
import { creativeWorkflowInsights, formatCreativeDuration } from "./creative-feedback-insights";

describe("creative feedback insights", () => {
  it("calculates workflow rates without inventing data for empty totals", () => {
    const workflow = {
      candidate_selected: 3,
      candidate_rejected: 1,
      copy_accepted: 0,
      copy_replaced: 0,
      asset_reported: 0,
      qc_accepted: 4,
      qc_missed_issue: 1,
      qc_false_positive: 0,
      image_generation_success: 3,
      image_generation_total: 4,
      image_generation_failed: 1,
      image_generation_in_progress: 2,
      image_generation_duration_seconds: null,
      image_generation_duration_package_count: 0,
      three_size_qc_success: 1,
      three_size_qc_total: 3,
      first_delivery_count: 2,
      first_delivery_total: 4,
      production_adopted: 2,
      production_adoption_eligible: 5,
      feedback_reasons: [],
    } satisfies CreativeFeedbackDashboard["workflow"];

    expect(creativeWorkflowInsights(workflow).map((insight) => insight.rate)).toEqual([75, 75, null, 40]);
  });

  it("formats complete package durations for the dashboard", () => {
    expect(formatCreativeDuration(null)).toBe("-");
    expect(formatCreativeDuration(42)).toBe("42 秒");
    expect(formatCreativeDuration(368)).toBe("6 分 8 秒");
    expect(formatCreativeDuration(3660)).toBe("1 小时 1 分");
  });
});
