import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CreativeEditVariant } from "@multica/core/types";
import { CreativeVariantFeedback } from "./creative-variant-feedback";

const baseVariant: CreativeEditVariant = {
  id: "variant-1",
  job_id: "job-1",
  candidate_id: "candidate-1",
  variant_index: 1,
  title: "Variant 1",
  description: "",
  qc_status: "passed",
  created_at: "2026-07-16T01:00:00Z",
  assets: [],
  feedback: [],
};

describe("CreativeVariantFeedback", () => {
  it("submits a structured revision decision", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(
      <CreativeVariantFeedback
        variant={baseVariant}
        submitting={false}
        onSubmit={onSubmit}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "需要调整" }));
    fireEvent.click(screen.getByRole("button", { name: "利益点不匹配" }));
    fireEvent.change(screen.getByPlaceholderText(/补充具体调整建议/), {
      target: { value: "保留原图构图，只替换核心金额。" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提交反馈" }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({
      decision: "needs_revision",
      reason_codes: ["benefit_mismatch"],
      suggestion: "保留原图构图，只替换核心金额。",
    }));
  });

  it("shows confirmation, history, and the captured process snapshot", () => {
    render(
      <CreativeVariantFeedback
        variant={{
          ...baseVariant,
          feedback: [{
            id: "feedback-1",
            workspace_id: "workspace-1",
            issue_id: "issue-1",
            job_id: "job-1",
            candidate_id: "candidate-1",
            variant_id: "variant-1",
            decision: "needs_revision",
            reason_codes: ["benefit_mismatch"],
            suggestion: "主利益点应改成提额。",
            process_snapshot: {
              job_stage: "quality_check_complete",
              job_progress: 100,
              variant_qc_status: "passed",
            },
            created_by: "user-1",
            created_by_name: "张真",
            created_at: "2026-07-16T02:30:00Z",
          }],
        }}
        submitting={false}
        onSubmit={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(screen.getByText("最近确认")).toBeTruthy();
    expect(screen.getByText("反馈历史")).toBeTruthy();
    expect(screen.getByText("张真")).toBeTruthy();
    expect(screen.getByText("利益点不匹配")).toBeTruthy();
    expect(screen.getByText("主利益点应改成提额。")).toBeTruthy();
    expect(screen.getByText(/quality_check_complete · QC passed · 100%/)).toBeTruthy();
  });
});
