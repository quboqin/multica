// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CreativeComparisonWorkspace, containedImageBounds, normalizeCreativeAnnotation, normalizeCreativeAnnotationInImage } from "./creative-comparison-workspace";

describe("CreativeComparisonWorkspace", () => {
  const props = {
    source: { label: "竞品原图", url: "https://cdn.example/source.png" },
    result: { id: "final-1", label: "正式成图", finalUrl: "https://cdn.example/final.png", baseUrl: "https://cdn.example/base.png", size: "1080x1080", variant: "V01" },
    assets: [
      { id: "final-1", label: "正式成图", finalUrl: "https://cdn.example/final.png", baseUrl: "https://cdn.example/base.png", size: "1080x1080", variant: "V01" },
      { id: "final-2", label: "正式成图", finalUrl: "https://cdn.example/final-2.png", size: "1200x628", variant: "V02" },
    ],
    onAssetChange: vi.fn(),
    onAdjust: vi.fn(),
    onAnnotation: vi.fn(),
  };

  it("switches comparison modes and preserves a selected delivery", () => {
    render(<CreativeComparisonWorkspace {...props} />);
    fireEvent.click(screen.getByRole("button", { name: "滑杆" }));
    expect(screen.getByLabelText("对比滑杆")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "叠加" }));
    expect(screen.queryByLabelText("对比滑杆")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("V02"));
    expect(props.onAssetChange).toHaveBeenCalledWith("final-2");
  });

  it("disables acceptance and explains which final delivery condition is pending", () => {
    render(<CreativeComparisonWorkspace {...props} acceptance={{ enabled: false, status: "等待双路 QC：technical 通过，visual 待完成" }} />);
    expect(screen.getByRole("button", { name: "接受" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("等待双路 QC：technical 通过，visual 待完成");
  });

  it("normalizes point and rectangle annotations independently of canvas pixels", () => {
    expect(normalizeCreativeAnnotation({ x: 80, y: 20 }, { x: 20, y: 70 }, { width: 100, height: 100 }, "rect")).toEqual({ kind: "rect", x: 0.2, y: 0.2, width: 0.6, height: 0.5 });
    expect(normalizeCreativeAnnotation({ x: 150, y: -4 }, { x: 0, y: 0 }, { width: 100, height: 100 }, "point")).toEqual({ kind: "point", x: 1, y: 0, width: 0, height: 0 });
  });

  it("excludes object-contain letterboxing from persisted image coordinates", () => {
    expect(containedImageBounds({ width: 200, height: 200 }, { width: 200, height: 100 })).toEqual({ left: 0, top: 50, width: 200, height: 100 });
    expect(normalizeCreativeAnnotationInImage({ x: 50, y: 75 }, { x: 150, y: 125 }, { width: 200, height: 200 }, { width: 200, height: 100 }, "rect")).toEqual({ kind: "rect", x: 0.25, y: 0.25, width: 0.5, height: 0.5 });
  });
});
