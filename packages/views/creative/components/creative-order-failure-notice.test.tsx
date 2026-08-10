// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
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

afterEach(cleanup);

describe("CreativeOrderFailureNotice", () => {
  it("shows failed steps as read-only process records", () => {
    render(<>
      <CreativeOrderFailureNotice failures={[failure]} />
      <div>{creativeOrderWaitingMessage([failure])}</div>
    </>);

    expect(screen.getByRole("status", { name: "创意过程提醒" })).toHaveTextContent("过程步骤：视觉质检");
    expect(screen.getByText("生成服务暂时繁忙，当前步骤未完成。")).toBeInTheDocument();
    expect(screen.getByText("技术详情")).toBeInTheDocument();
    expect(screen.getByText("目前没有可验收成图；可结束订单后重新发起。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /重试|质检|修复/ })).not.toBeInTheDocument();
  });

  it("keeps non-retryable failures visible without offering an action", () => {
    render(<CreativeOrderFailureNotice failures={[{ ...failure, workflow: "creative_production", retryable: false }]} />);

    expect(screen.getByText("后台已记录该步骤未补齐；不用手动重试，请查看其他候选、标注调整或结束订单后重新发起。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /重试|质检|修复/ })).not.toBeInTheDocument();
  });

  it("turns a wrapped codex process failure into business copy and keeps raw logs collapsed", () => {
    render(<CreativeOrderFailureNotice failures={[{
      ...failure,
      workflow: "creative_production",
      failure_reason: "codex_semantic_inactivity",
      error: "codex semantic inactivity timeout; Exit code: 1 Output: unknown flag: --output",
      retryable: false,
    }]} />);

    expect(screen.getByText("智能体运行在生成开始前异常退出，当前步骤未完成。")).toBeInTheDocument();
    const details = screen.getByText("技术详情").closest("details");
    expect(details).not.toHaveAttribute("open");
    expect(details).toHaveTextContent("unknown flag: --output");
  });

  it("collapses a local failure when other creative results remain available", () => {
    render(<CreativeOrderFailureNotice failures={[failure]} compact />);

    expect(screen.getByRole("status")).toHaveTextContent("1 条系统提醒");
    expect(screen.getByRole("status")).toHaveTextContent("可用成图不受影响");
  });

  it("retains failure history without retry actions after an order is closed", () => {
    render(<CreativeOrderFailureNotice failures={[failure]} closed />);

    expect(screen.getByText("订单已结束，过程记录保留。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /重试|质检|修复/ })).not.toBeInTheDocument();
  });

  it("does not expose per-lane QC recovery controls", () => {
    render(<CreativeOrderFailureNotice failures={[failure, { ...failure, task_id: "task-2", workflow: "creative_qc_technical" }]} />);

    expect(screen.getAllByText(/过程步骤：/)).toHaveLength(2);
    expect(screen.queryByRole("button", { name: /重试|质检|修复/ })).not.toBeInTheDocument();
  });
});
