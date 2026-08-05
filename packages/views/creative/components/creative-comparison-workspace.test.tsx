// @vitest-environment jsdom

import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CreativeComparisonWorkspace, containedImageBounds, normalizeCreativeAnnotation, normalizeCreativeAnnotationInImage, transformedContainedImageBounds } from "./creative-comparison-workspace";

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
    onAnnotations: vi.fn().mockResolvedValue(true),
  };

  it("keeps one side-by-side comparison and switches variants and sizes", () => {
    render(<CreativeComparisonWorkspace {...props} />);
    expect(screen.getByTestId("creative-preview-toolbar")).toHaveClass("flex-nowrap", "overflow-x-auto");
    expect(screen.getByText("原图 / 成图")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "滑杆" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "叠加" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("V02"));
    expect(props.onAssetChange).toHaveBeenCalledWith("final-2");
    fireEvent.click(screen.getByText("横版"));
    expect(props.onAssetChange).toHaveBeenCalledWith("final-2");
  });

  it("disables acceptance and explains which final delivery condition is pending", () => {
    render(<CreativeComparisonWorkspace {...props} acceptance={{ enabled: false, status: "等待双路 QC：technical 通过，visual 待完成" }} />);
    expect(screen.getByRole("button", { name: "接受" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("等待双路 QC：technical 通过，visual 待完成");
  });

  it("keeps multiple numbered annotations and submits them as one adjustment", async () => {
    const onAnnotations = vi.fn().mockResolvedValue(true);
    render(<CreativeComparisonWorkspace {...props} onAnnotations={onAnnotations} />);
    const surface = screen.getByTestId("creative-annotation-surface");
    Object.defineProperty(surface, "getBoundingClientRect", { value: () => ({ x: 0, y: 0, left: 0, top: 0, right: 400, bottom: 400, width: 400, height: 400, toJSON: () => ({}) }) });
    Object.defineProperty(surface, "setPointerCapture", { value: vi.fn() });

    fireEvent.click(screen.getByRole("button", { name: "框选标注" }));
    fireEvent.pointerDown(surface, { clientX: 40, clientY: 50, pointerId: 1 });
    fireEvent.pointerUp(surface, { clientX: 140, clientY: 160, pointerId: 1 });
    fireEvent.change(screen.getByRole("textbox", { name: "标注 1 调整说明" }), { target: { value: "放大标题" } });
    fireEvent.click(screen.getByRole("button", { name: "完成标注" }));

    fireEvent.click(screen.getByRole("button", { name: "框选标注" }));
    fireEvent.pointerDown(surface, { clientX: 220, clientY: 220, pointerId: 2 });
    fireEvent.pointerUp(surface, { clientX: 340, clientY: 350, pointerId: 2 });
    fireEvent.change(screen.getByRole("textbox", { name: "标注 2 调整说明" }), { target: { value: "移除装饰" } });
    fireEvent.click(screen.getByRole("button", { name: "完成标注" }));

    expect(screen.getByText("已标注 2 处")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "提交 2 处调整" }));
    await waitFor(() => expect(onAnnotations).toHaveBeenCalledTimes(1));
    expect(onAnnotations.mock.calls[0]?.[0]).toHaveLength(2);
  });

  it("shows the rectangle while the pointer is still moving", () => {
    render(<CreativeComparisonWorkspace {...props} />);
    const surface = screen.getByTestId("creative-annotation-surface");
    Object.defineProperty(surface, "getBoundingClientRect", { value: () => ({ x: 0, y: 0, left: 0, top: 0, right: 400, bottom: 400, width: 400, height: 400, toJSON: () => ({}) }) });
    Object.defineProperty(surface, "setPointerCapture", { value: vi.fn() });
    fireEvent.click(screen.getByRole("button", { name: "框选标注" }));
    fireEvent.pointerDown(surface, { clientX: 40, clientY: 50, pointerId: 1 });
    fireEvent.pointerMove(surface, { clientX: 140, clientY: 160, pointerId: 1 });
    expect(screen.getByTestId("creative-drawing-annotation")).toBeInTheDocument();
  });

  it("captures the wheel inside the canvas and zooms without scrolling the page", async () => {
    render(<CreativeComparisonWorkspace {...props} />);
    const stage = screen.getByTestId("creative-comparison-stage");
    const wheel = new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: -100 });
    act(() => stage.dispatchEvent(wheel));
    expect(wheel.defaultPrevented).toBe(true);
    await waitFor(() => expect(screen.getByText("115%")).toBeInTheDocument());
  });

  it("normalizes point and rectangle annotations independently of canvas pixels", () => {
    expect(normalizeCreativeAnnotation({ x: 80, y: 20 }, { x: 20, y: 70 }, { width: 100, height: 100 }, "rect")).toEqual({ kind: "rect", x: 0.2, y: 0.2, width: 0.6, height: 0.5 });
    expect(normalizeCreativeAnnotation({ x: 150, y: -4 }, { x: 0, y: 0 }, { width: 100, height: 100 }, "point")).toEqual({ kind: "point", x: 1, y: 0, width: 0, height: 0 });
  });

  it("excludes object-contain letterboxing from persisted image coordinates", () => {
    expect(containedImageBounds({ width: 200, height: 200 }, { width: 200, height: 100 })).toEqual({ left: 0, top: 50, width: 200, height: 100 });
    expect(normalizeCreativeAnnotationInImage({ x: 50, y: 75 }, { x: 150, y: 125 }, { width: 200, height: 200 }, { width: 200, height: 100 }, "rect")).toEqual({ kind: "rect", x: 0.25, y: 0.25, width: 0.5, height: 0.5 });
  });

  it("keeps annotation geometry attached to the image while zooming and panning", () => {
    expect(transformedContainedImageBounds(
      { width: 200, height: 200 },
      { width: 200, height: 100 },
      2,
      { x: 10, y: -5 },
    )).toEqual({ left: -90, top: -5, width: 400, height: 200 });
  });
});
