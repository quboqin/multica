// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CreativeOrderWorkflowFailure } from "@multica/core/types";
import { CreativeOrderFailureNotice, creativeOrderWaitingMessage } from "./creative-studio-page";

const failure: CreativeOrderWorkflowFailure = {
  task_id: "task-1",
  agent_id: "agent-1",
  workflow: "creative_qc_visual",
  scope: "variant",
  subject_id: "variant-1",
  item_key: "variant-b",
  trigger_evidence_kind: "creative_order_variant_qc",
  trigger_evidence_ref_id: "variant-1",
  failure_reason: "provider_rate_limited",
  error: "429 Too Many Requests",
  failed_at: "2026-08-04T12:00:00Z",
  retryable: true,
};

describe("CreativeOrderFailureNotice", () => {
  it("shows the failed step and recovery instead of only waiting for the first asset", () => {
    const onRetry = vi.fn();
    render(<>
      <CreativeOrderFailureNotice failures={[failure]} retryingTaskId="" onRetry={onRetry} />
      <div>{creativeOrderWaitingMessage([failure])}</div>
    </>);

    expect(screen.getByRole("alert", { name: "创意流程失败" })).toHaveTextContent("失败步骤：视觉质检");
    expect(screen.getByText("429 Too Many Requests")).toBeInTheDocument();
    expect(screen.getByText("失败步骤恢复后，首批成图会自动出现在这里。")).toBeInTheDocument();
    expect(screen.queryByText("正在等待首批成图。各变体完成后会立即出现在这里。")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "重试失败步骤" }));
    expect(onRetry).toHaveBeenCalledWith(failure);
  });

  it("keeps non-retryable failures visible without offering a misleading action", () => {
    render(<CreativeOrderFailureNotice failures={[{ ...failure, retryable: false }]} retryingTaskId="" onRetry={vi.fn()} />);

    expect(screen.getByText("需要人工处理")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试失败步骤" })).not.toBeInTheDocument();
  });
});
