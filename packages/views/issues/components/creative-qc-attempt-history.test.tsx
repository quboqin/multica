// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  CreativeQcAttemptHistory,
  extractCreativeQcAttemptRounds,
} from "./creative-qc-attempt-history";

const processData = {
  candidates: [{
    candidate_id: "candidate-1",
    submissions: [{
      submission_index: 1,
      status: "partial",
      process: {
        artifacts: {
          source_url: "https://source.example/image.jpg?auth_key=secret",
        },
        rounds: [
          {
            round: 1,
            verdicts: [{
              candidate_index: 1,
              verdict: "fail",
              issues: [{ type: "financial", message: "额度文案重复。" }],
              artifacts: {
                image: "attempt_1_1.png",
                image_url: "http://127.0.0.1:8010/files/lean-1/attempt_1_1.png",
                qc: "attempt_1_1_qc.json",
              },
            }],
          },
          {
            round: 2,
            verdicts: [{
              concept_index: 1,
              candidate_index: 1,
              size: "800x1000",
              verdict: "pass",
              issues: [],
              prompt_artifact: "prompt_attempt_2_1_800x1000.txt",
              artifacts: {
                image: "attempt_2_1_800x1000.png",
                image_url: "http://127.0.0.1:8010/files/lean-1/attempt_2_1_800x1000.png",
              },
            }, {
              concept_index: 1,
              size: "1200x628",
              verdict: "generation_error",
              provider_returned_size: "1024x1024",
              requested_model_size: "1200x640",
              exact_output: "1200x628",
              issues: [{ type: "generation", message: "模型返回尺寸不符合横版要求。" }],
              artifacts: {
                image: "attempt_2_1_1200x628_raw.avif",
                image_url: "http://127.0.0.1:8010/files/lean-1/attempt_2_1_1200x628_raw.avif",
              },
            }],
          },
        ],
      },
    }],
  }],
};

describe("CreativeQcAttemptHistory", () => {
  it("parses old concept rounds and model-native size rounds", () => {
    const rounds = extractCreativeQcAttemptRounds(processData, "candidate-1");
    expect(rounds).toHaveLength(2);
    expect(rounds[0]?.attempts[0]).toMatchObject({
      conceptIndex: 1,
      size: "主图",
      verdict: "fail",
      imageArtifact: "attempt_1_1.png",
    });
    expect(rounds[1]?.attempts.map((attempt) => attempt.size)).toEqual([
      "800x1000",
      "1200x628",
    ]);
    expect(rounds[1]?.attempts[1]).toMatchObject({
      imageUrl: "http://127.0.0.1:8010/files/lean-1/attempt_2_1_1200x628_raw.avif",
      providerReturnedSize: "1024x1024",
      requestedModelSize: "1200x640",
      exactOutput: "1200x628",
    });
  });

  it("shows passed and failed images with Chinese QC conclusions without exposing signed sources", () => {
    const onPreview = vi.fn();
    render(
      <CreativeQcAttemptHistory
        processData={processData}
        candidateId="candidate-1"
        candidateName="Kredit Pintar"
        onPreview={onPreview}
      />,
    );

    expect(screen.getByText("QC 每轮记录")).toBeInTheDocument();
    expect(screen.getByText("批次 1 · 第 1 轮")).toBeInTheDocument();
    expect(screen.getByText("批次 1 · 第 2 轮")).toBeInTheDocument();
    expect(screen.getByText("额度文案重复。")).toBeInTheDocument();
    expect(screen.getByText("模型返回尺寸不符合横版要求。")).toBeInTheDocument();
    expect(screen.getByText("生成失败")).toBeInTheDocument();
    expect(screen.getByText("模型返回 1024x1024")).toBeInTheDocument();
    expect(screen.getByText("要求模型尺寸 1200x640")).toBeInTheDocument();
    expect(screen.getByText("交付尺寸 1200x628")).toBeInTheDocument();
    expect(screen.getAllByText("未通过")).toHaveLength(1);
    expect(screen.getByText("通过")).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("auth_key");
    expect(document.querySelector('img[src*="auth_key"]')).toBeNull();

    fireEvent.click(screen.getByRole("button", {
      name: "预览第 1 轮概念 1 主图 尝试图",
    }));
    expect(onPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "http://127.0.0.1:8010/files/lean-1/attempt_1_1.png",
    }));
  });
});
