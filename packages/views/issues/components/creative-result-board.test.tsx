import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CreativeMaterialCandidate } from "@multica/core/types";

import {
  groupCreativeDeliveriesByCandidate,
  inferCreativeFeedbackDecision,
  IssueResultBoard,
  ResultBoardPreviewDialog,
} from "./creative-material-pool";

const asset = {
  id: "final-1",
  filename: "August_AdaKami_Indonesia_candidate-42_V01_1080x1080_v1.png",
  url: "https://cdn.example.com/final.png",
  download_url: "https://cdn.example.com/final-download.png",
  markdown_url: null,
};

const candidate = {
  id: "candidate-42",
  competitor: "Easycash",
  title: "Low-rate competitor creative",
  asset_type: "image",
  archived_url: "https://cdn.example.com/source.png",
  preview_url: "",
  poster_url: "",
  resource_url: "",
  original_url: "https://appgrowing.example.com/source",
} as unknown as CreativeMaterialCandidate;

describe("creative result board preview", () => {
  it("exposes board, source, and result enlargement actions", () => {
    const onOpenBoardPreview = vi.fn();
    const onPreview = vi.fn();

    render(
      <IssueResultBoard
        archiveName="ADC-135-results"
        assets={[asset]}
        activeAsset={asset}
        candidates={[candidate]}
        candidateByAttachment={new Map([[asset.id, candidate.id]])}
        deliveryByAttachment={new Map()}
        expanded
        onExpandedChange={vi.fn()}
        onAssetChange={vi.fn()}
        onPreview={onPreview}
        onOpenBoardPreview={onOpenBoardPreview}
        onAdjust={vi.fn()}
        onCandidateFeedback={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTitle("放大修图看板"));
    expect(onOpenBoardPreview).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByTitle("查看竞品原图大图"));
    expect(onPreview).toHaveBeenLastCalledWith(
      expect.objectContaining({ url: candidate.archived_url }),
    );

    fireEvent.click(screen.getByTitle("查看修图结果大图"));
    expect(onPreview).toHaveBeenLastCalledWith(
      expect.objectContaining({ url: asset.url }),
    );
  });

  it("renders a large comparison workspace without a nested enlargement action", () => {
    render(
      <ResultBoardPreviewDialog
        open
        onOpenChange={vi.fn()}
        archiveName="ADC-135-results"
        assets={[asset]}
        activeAsset={asset}
        candidates={[candidate]}
        candidateByAttachment={new Map([[asset.id, candidate.id]])}
        deliveryByAttachment={new Map()}
        onAssetChange={vi.fn()}
        onPreview={vi.fn()}
        onAdjust={vi.fn()}
        onCandidateFeedback={vi.fn()}
      />,
    );

    expect(screen.getByText("修图结果看板预览")).toBeInTheDocument();
    expect(screen.getByTitle("查看竞品原图大图")).toBeInTheDocument();
    expect(screen.getByTitle("查看修图结果大图")).toBeInTheDocument();
    expect(screen.queryByTitle("放大修图看板")).not.toBeInTheDocument();
  });

  it("keeps the source and three creative variants in one horizontal board", () => {
    const sizes = ["1080x1080", "1200x628", "800x1000"];
    const assets = [1, 2, 3].flatMap((variant) =>
      sizes.map((size) => ({
        ...asset,
        id: `final-${variant}-${size}`,
        filename: `August_AdaKami_Indonesia_candidate-42_V${String(variant).padStart(2, "0")}_${size}_v1.png`,
        url: `https://cdn.example.com/v${variant}-${size}.png`,
      })),
    );

    render(
      <IssueResultBoard
        archiveName="ADC-135-results"
        assets={assets}
        activeAsset={assets[0]}
        candidates={[candidate]}
        candidateByAttachment={new Map(assets.map((item) => [item.id, candidate.id]))}
        deliveryByAttachment={new Map()}
        expanded
        onExpandedChange={vi.fn()}
        onAssetChange={vi.fn()}
        onPreview={vi.fn()}
        onOpenBoardPreview={vi.fn()}
        onAdjust={vi.fn()}
        onCandidateFeedback={vi.fn()}
      />,
    );

    expect(screen.getByTestId("creative-material-strip")).toHaveClass("overflow-x-auto");
    const board = screen.getByTestId("creative-variant-board");
    expect(board).toHaveClass("min-w-[920px]");
    expect(screen.getByText("竞品原图")).toBeInTheDocument();
    expect(screen.getByText("原始尺寸")).toBeInTheDocument();
    expect(screen.getByText("V01")).toBeInTheDocument();
    expect(screen.getByText("V02")).toBeInTheDocument();
    expect(screen.getByText("V03")).toBeInTheDocument();
    expect(screen.getAllByText("1080x1080")).toHaveLength(3);
  });

  it("groups variants under their source material", () => {
    const secondAsset = {
      ...asset,
      id: "final-2",
      filename: "August_AdaKami_Indonesia_candidate-99_V01_1080x1080_v1.png",
    };
    const secondCandidate = { ...candidate, id: "candidate-99", title: "Repayment plan" };
    const groups = groupCreativeDeliveriesByCandidate(
      [asset, secondAsset],
      new Map([[asset.id, candidate.id], [secondAsset.id, secondCandidate.id]]),
      [candidate, secondCandidate],
    );

    expect(groups).toHaveLength(2);
    expect(groups[0]?.candidateId).toBe(candidate.id);
    expect(groups[1]?.candidateId).toBe(secondCandidate.id);
    expect(groups[1]?.groups[0]?.variant).toBe(1);
  });

  it("understands natural-language variant preservation and replanning", () => {
    expect(inferCreativeFeedbackDecision(
      "V01 保留，V02 和 V03 重做。保持还款计划和蓝白主色。",
      [1, 2, 3],
    )).toEqual({ variants: [2, 3], mode: "replan" });
  });
});
