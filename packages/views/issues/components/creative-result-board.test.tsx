import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CreativeMaterialCandidate } from "@multica/core/types";

import {
  creativeProductionProgress,
  creativeMaterialPoolCandidatesForScope,
  creativeResultCandidates,
  groupCreativeDeliveriesByCandidate,
  inferCreativeFeedbackDecision,
  IssueResultBoard,
  mergeCreativeMaterialPoolCandidates,
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
  it("keeps selected candidates out of the result board until production starts", () => {
    const selected = { ...candidate, status: "selected" } as CreativeMaterialCandidate;
    expect(creativeResultCandidates([selected], [], [])).toEqual([]);
    expect(creativeResultCandidates([selected], [{ candidate_id: selected.id, work_issue_id: "work-1" }] as never, [])).toEqual([selected]);
  });

  it("merges the full material pool and separates current-run assets from pending history", () => {
    const current = { ...candidate, id: "current", source_issue_id: "issue-1", is_new_in_run: true, status: "new" } as CreativeMaterialCandidate;
    const historical = { ...candidate, id: "historical", source_issue_id: "issue-old", is_new_in_run: false, status: "new" } as CreativeMaterialCandidate;
    const merged = mergeCreativeMaterialPoolCandidates([current], [current, historical]);

    expect(merged.map((value) => value.id)).toEqual(["current", "historical"]);
    expect(creativeMaterialPoolCandidatesForScope(merged, new Set([current.id]), "issue-1", "current")).toEqual([current]);
    expect(creativeMaterialPoolCandidatesForScope(merged, new Set([current.id]), "issue-1", "pending")).toEqual([historical]);
  });

  it("keeps the selected source visible before the first delivery", () => {
    const onOpenBoardPreview = vi.fn();
    const onPreview = vi.fn();

    render(
      <IssueResultBoard
        archiveName="ADC-new-results"
        assets={[]}
        candidates={[{ ...candidate, status: "selected" } as CreativeMaterialCandidate]}
        candidateByAttachment={new Map()}
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

    expect(screen.getAllByText("等待首批成图").length).toBeGreaterThan(0);
    expect(screen.getByText("0/9")).toBeInTheDocument();
    expect(screen.getAllByAltText(candidate.title).every((image) => image.getAttribute("src") === candidate.archived_url)).toBe(true);
    fireEvent.click(screen.getByTitle("放大修图看板"));
    expect(onOpenBoardPreview).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByTitle("查看竞品原图大图"));
    expect(onPreview).toHaveBeenLastCalledWith(expect.objectContaining({ url: candidate.archived_url }));
  });

  it("summarizes blocked production on the parent candidate instead of rendering a child workflow surface", () => {
    const progress = creativeProductionProgress(
      [{ candidate_id: candidate.id, work_issue_id: "work-1" }] as never,
      new Map([["work-1", [{
        id: "production-2",
        identifier: "ADC-301",
        title: "图像编辑 · V02 · 三尺寸",
        status: "blocked",
        metadata: { workflow: "creative_production", variant: "V02" },
      }]]]) as never,
    );

    expect(progress).toEqual([expect.objectContaining({
      candidate_id: candidate.id,
      work_issue_id: "work-1",
      tasks: [expect.objectContaining({ identifier: "ADC-301", status: "blocked", variant: "V02" })],
    })]);
  });

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
    expect(screen.getByTestId("creative-preview-toolbar")).toHaveClass("pr-14");
    expect(screen.getByTitle("查看竞品原图大图")).toHaveClass("h-full", "min-h-0");
    expect(screen.getByTitle("查看修图结果大图")).toHaveClass("h-full", "min-h-0");
    expect(screen.queryByTitle("放大修图看板")).not.toBeInTheDocument();
  });

  it("keeps three variants in a horizontal selector and compares one result at full width", () => {
    const sizes = ["1080x1080", "1200x628", "800x1000"];
    const assets = [1, 2, 3].flatMap((variant) =>
      sizes.map((size) => ({
        ...asset,
        id: `final-${variant}-${size}`,
        filename: `August_AdaKami_Indonesia_candidate-42_V${String(variant).padStart(2, "0")}_${size}_v1.png`,
        url: `https://cdn.example.com/v${variant}-${size}.png`,
      })),
    );

    const onAssetChange = vi.fn();
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
        onAssetChange={onAssetChange}
        onPreview={vi.fn()}
        onOpenBoardPreview={vi.fn()}
        onAdjust={vi.fn()}
        onCandidateFeedback={vi.fn()}
      />,
    );

    expect(screen.getByTestId("creative-material-strip")).toHaveClass("overflow-x-auto");
    expect(screen.getByTestId("creative-variant-strip")).toHaveClass("overflow-x-auto");
    const board = screen.getByTestId("creative-comparison-board");
    expect(board).toHaveClass("min-w-[760px]");
    expect(screen.getByText("竞品原图")).toBeInTheDocument();
    expect(screen.getByText("V01")).toBeInTheDocument();
    expect(screen.getByText("V02")).toBeInTheDocument();
    expect(screen.getByText("V03")).toBeInTheDocument();
    expect(screen.getByText("当前结果 · V01")).toBeInTheDocument();
    expect(screen.getAllByTitle("查看修图结果大图")).toHaveLength(1);
    expect(screen.getAllByText("1080x1080")).toHaveLength(1);

    fireEvent.click(screen.getByText("V02"));
    expect(onAssetChange).toHaveBeenCalledWith("final-2-1080x1080");
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

  it("uses structured deliveries for production Prime filenames", () => {
    const assets = [1, 2, 3].flatMap((variant) =>
      ["1080x1080", "1200x628", "800x1000"].map((size) => ({
        ...asset,
        id: `prime-${variant}-${size}`,
        filename: `V0${variant}_${size}_r2_prime.png`,
      })),
    );
    const deliveries = new Map(assets.map((item) => {
      const match = item.filename.match(/^V0(\d)_(1080x1080|1200x628|800x1000)/);
      return [item.id, {
        candidate_id: candidate.id,
        variant: Number(match?.[1]),
        size: match?.[2],
        revision: 2,
      }];
    }));

    const groups = groupCreativeDeliveriesByCandidate(
      assets,
      new Map(assets.map((item) => [item.id, candidate.id])),
      [candidate],
      deliveries as never,
    );

    expect(groups).toHaveLength(1);
    expect(groups[0]?.groups).toHaveLength(3);
    expect(groups[0]?.groups.map((group) => group.variant)).toEqual([1, 2, 3]);
    expect(groups[0]?.groups.every((group) => group.assets.length === 3)).toBe(true);
  });

  it("understands natural-language variant preservation and replanning", () => {
    expect(inferCreativeFeedbackDecision(
      "V01 保留，V02 和 V03 重做。保持还款计划和蓝白主色。",
      [1, 2, 3],
    )).toEqual({ variants: [2, 3], mode: "replan" });
  });
});
