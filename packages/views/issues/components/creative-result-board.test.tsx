import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CreativeMaterialCandidate } from "@multica/core/types";

import { IssueResultBoard, ResultBoardPreviewDialog } from "./creative-material-pool";

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
      />,
    );

    expect(screen.getByText("修图结果看板预览")).toBeInTheDocument();
    expect(screen.getByTitle("查看竞品原图大图")).toBeInTheDocument();
    expect(screen.getByTitle("查看修图结果大图")).toBeInTheDocument();
    expect(screen.queryByTitle("放大修图看板")).not.toBeInTheDocument();
  });
});
