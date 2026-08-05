// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CreativeOrderWorkflowFailure } from "@multica/core/types";
import { CreativeOrderFailureNotice, creativeOrderWaitingMessage, workflowFailureRetryMode } from "./creative-studio-page";

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

afterEach(cleanup);

describe("CreativeOrderFailureNotice", () => {
  it("shows the failed step and recovery instead of only waiting for the first asset", () => {
    const onRetry = vi.fn();
    render(<>
      <CreativeOrderFailureNotice failures={[failure]} qcRecoveryAvailableVariantIds={new Set(["variant-1"])} retryingTaskId="" onRetry={onRetry} />
      <div>{creativeOrderWaitingMessage([failure])}</div>
    </>);

    expect(screen.getByRole("alert", { name: "创意流程失败" })).toHaveTextContent("失败步骤：视觉质检");
    expect(screen.getByText("生成服务暂时繁忙，当前步骤未完成。")).toBeInTheDocument();
    expect(screen.getByText("技术详情")).toBeInTheDocument();
    expect(screen.getByText("失败步骤恢复后，首批成图会自动出现在这里。")).toBeInTheDocument();
    expect(screen.queryByText("正在等待首批成图。各变体完成后会立即出现在这里。")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "重新执行双路质检" }));
    expect(onRetry).toHaveBeenCalledWith(failure);
  });

  it("keeps non-retryable failures visible without offering a misleading action", () => {
    render(<CreativeOrderFailureNotice failures={[{ ...failure, workflow: "creative_production", retryable: false }]} retryingTaskId="" onRetry={vi.fn()} />);

    expect(screen.getByText("该步骤已无重试次数，请结束订单后重新发起")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新执行双路质检" })).not.toBeInTheDocument();
  });

  it("turns a wrapped codex process failure into business copy and keeps raw logs collapsed", () => {
    render(<CreativeOrderFailureNotice failures={[{
      ...failure,
		workflow: "creative_production",
      failure_reason: "codex_semantic_inactivity",
      error: "codex semantic inactivity timeout; Exit code: 1 Output: unknown flag: --output",
      retryable: false,
    }]} retryingTaskId="" onRetry={vi.fn()} />);

    expect(screen.getByText("智能体运行在生成开始前异常退出，当前步骤未完成。")).toBeInTheDocument();
    const details = screen.getByText("技术详情").closest("details");
    expect(details).not.toHaveAttribute("open");
    expect(details).toHaveTextContent("unknown flag: --output");
  });

  it("collapses a local failure when other creative results remain available", () => {
    render(<CreativeOrderFailureNotice failures={[failure]} retryingTaskId="" onRetry={vi.fn()} compact />);

    expect(screen.getByRole("alert")).toHaveTextContent("1 个局部步骤需要处理");
    expect(screen.getByRole("alert")).toHaveTextContent("其他已完成成图不受影响");
  });

  it("retains failure history without retry actions after an order is closed", () => {
    render(<CreativeOrderFailureNotice failures={[failure]} retryingTaskId="" onRetry={vi.fn()} closed />);

    expect(screen.getByText("订单已结束，不再继续处理")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新执行双路质检" })).not.toBeInTheDocument();
  });

  it("always routes QC action-required results through atomic dual-lane recovery", () => {
    const reported = { ...failure, failure_reason: "agent_reported_action_required", error: "OPENAI_API_KEY is not set" };
    expect(workflowFailureRetryMode(reported)).toBe("qc_recovery");
    expect(workflowFailureRetryMode({ ...reported, task_id: "" })).toBe("qc_recovery");
    expect(workflowFailureRetryMode({ ...reported, workflow: "creative_qc" })).toBe("qc_recovery");
    expect(workflowFailureRetryMode({ ...reported, workflow: "creative_production" })).toBe("creative_recovery");
    expect(workflowFailureRetryMode(failure)).toBe("qc_recovery");
  });

  it("does not offer a single-lane retry when atomic QC recovery is unavailable", () => {
    const reported = { ...failure, failure_reason: "agent_reported_action_required", error: "Prime package contract is incomplete" };
    render(<CreativeOrderFailureNotice failures={[reported]} retryingTaskId="" onRetry={vi.fn()} />);

    expect(screen.getByText("双路 QC 恢复不可用，需修复 Prime 包或人工处理")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /重试|质检/ })).not.toBeInTheDocument();
  });

  it("shows one clear recovery action for both independent QC lanes", () => {
    const onRetry = vi.fn();
    render(<CreativeOrderFailureNotice failures={[failure, { ...failure, task_id: "task-2", workflow: "creative_qc_technical" }]} qcRecoveryAvailableVariantIds={new Set(["variant-1"])} retryingTaskId="" onRetry={onRetry} />);

    expect(screen.getAllByRole("button", { name: "重新执行双路质检" })).toHaveLength(1);
    expect(screen.getByText("此变体的双路质检可由上方恢复操作统一重跑")).toBeInTheDocument();
  });

  it("uses the generic running label for a Prime retry", () => {
    render(<CreativeOrderFailureNotice failures={[{ ...failure, workflow: "creative_prime" }]} retryingTaskId="task-1" onRetry={vi.fn()} />);

    expect(screen.getByRole("button", { name: "正在重试失败步骤" })).toBeDisabled();
  });

  it("uses the dual-lane running label for a QC recovery", () => {
    render(<CreativeOrderFailureNotice failures={[failure]} qcRecoveryAvailableVariantIds={new Set(["variant-1"])} retryingTaskId="" retryingQCVariantId="variant-1" onRetry={vi.fn()} />);

    expect(screen.getByRole("button", { name: "正在重新执行双路质检" })).toBeDisabled();
  });
});
