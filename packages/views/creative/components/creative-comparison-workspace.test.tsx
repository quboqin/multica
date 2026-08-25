// @vitest-environment jsdom

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CreativeComparisonWorkspace, containedImageBounds, normalizeCreativeAnnotation, normalizeCreativeAnnotationInImage, transformedContainedImageBounds } from "./creative-comparison-workspace";

describe("CreativeComparisonWorkspace", () => {
  const props = {
    source: { label: "竞品原图", url: "https://cdn.example/source.png" },
    result: { id: "final-1", label: "正式成图", finalUrl: "https://cdn.example/final.png", baseUrl: "https://cdn.example/base.png", size: "1080x1080", variant: "V01" },
    assets: [
      { id: "final-1", label: "正式成图", finalUrl: "https://cdn.example/final.png", baseUrl: "https://cdn.example/base.png", size: "1080x1080", variant: "V01" },
      { id: "final-2", label: "V01 横版", finalUrl: "https://cdn.example/final-2.png", size: "1200x628", variant: "V01" },
      { id: "final-3", label: "V01 竖版", finalUrl: "https://cdn.example/final-3.png", size: "800x1000", variant: "V01" },
      { id: "final-4", label: "V02 方形", finalUrl: "https://cdn.example/final-4.png", size: "1080x1080", variant: "V02" },
      { id: "final-5", label: "V02 横版", finalUrl: "https://cdn.example/final-5.png", size: "1200x628", variant: "V02" },
      { id: "final-6", label: "V02 竖版", finalUrl: "https://cdn.example/final-6.png", size: "800x1000", variant: "V02" },
      { id: "final-7", label: "V03 方形", finalUrl: "https://cdn.example/final-7.png", size: "1080x1080", variant: "V03" },
      { id: "final-8", label: "V03 横版", finalUrl: "https://cdn.example/final-8.png", size: "1200x628", variant: "V03" },
      { id: "final-9", label: "V03 竖版", finalUrl: "https://cdn.example/final-9.png", size: "800x1000", variant: "V03" },
    ],
    onAssetChange: vi.fn(),
    onAdjust: vi.fn(),
    onAnnotations: vi.fn().mockResolvedValue(true),
  };

  it("keeps one side-by-side comparison and switches variants and sizes immediately", () => {
    render(<CreativeComparisonWorkspace {...props} />);
    expect(screen.getByTestId("creative-preview-toolbar")).toHaveClass("flex-nowrap", "overflow-x-auto");
    expect(screen.getByText("原图 / 成图")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "滑杆" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "叠加" })).not.toBeInTheDocument();
    const variants = screen.getByRole("group", { name: "创意变体" });
    const sizes = screen.getByRole("group", { name: "成图尺寸" });
    fireEvent.click(within(variants).getByRole("button", { name: "V02" }));
    expect(props.onAssetChange).toHaveBeenLastCalledWith("final-4");
    expect(within(variants).getByRole("button", { name: "V02" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByAltText("V02 方形")).toHaveAttribute("src", "https://cdn.example/final-4.png");

    fireEvent.click(within(sizes).getByRole("button", { name: "横版" }));
    expect(props.onAssetChange).toHaveBeenLastCalledWith("final-5");
    expect(within(sizes).getByRole("button", { name: "横版" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByAltText("V02 横版")).toHaveAttribute("src", "https://cdn.example/final-5.png");

    fireEvent.click(within(variants).getByRole("button", { name: "V03" }));
    expect(props.onAssetChange).toHaveBeenLastCalledWith("final-8");
    expect(screen.getByAltText("V03 横版")).toHaveAttribute("src", "https://cdn.example/final-8.png");

    fireEvent.click(within(sizes).getByRole("button", { name: "竖版" }));
    expect(props.onAssetChange).toHaveBeenLastCalledWith("final-9");
    fireEvent.click(within(variants).getByRole("button", { name: "V01" }));
    expect(props.onAssetChange).toHaveBeenLastCalledWith("final-3");
    fireEvent.click(within(sizes).getByRole("button", { name: "方形" }));
    expect(props.onAssetChange).toHaveBeenLastCalledWith("final-1");
    expect(screen.getByAltText("正式成图")).toHaveAttribute("src", "https://cdn.example/final.png");
  });

  it("shows the previous revision as the before image for a precise adjustment", () => {
    render(<CreativeComparisonWorkspace {...props} comparisonMode="adjustment" source={{ label: "调整前 · r1", url: "https://cdn.example/before.png" }} />);
    expect(screen.getByText("调整前 / 调整后")).toBeInTheDocument();
    expect(screen.getByTestId("creative-comparison-source-pane")).toHaveAttribute("title", "查看调整前成图");
    expect(screen.getByTestId("creative-comparison-result-pane")).toHaveAttribute("title", "查看调整后成图");
    expect(screen.getByAltText("调整前 · r1")).toHaveAttribute("src", "https://cdn.example/before.png");
    expect(screen.getByText("调整后 · 正式成图")).toBeInTheDocument();
  });

  it("disables acceptance and explains which final delivery condition is pending", () => {
    render(<CreativeComparisonWorkspace {...props} acceptance={{ enabled: false, status: "等待视觉质检完成" }} />);
    expect(screen.getByRole("button", { name: "接受" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("等待视觉质检完成");
  });

  it("opens generation information for the current result", () => {
    const onViewInfo = vi.fn();
    render(<CreativeComparisonWorkspace {...props} onViewInfo={onViewInfo} />);
    fireEvent.click(screen.getByRole("button", { name: "生成信息" }));
    expect(onViewInfo).toHaveBeenCalledTimes(1);
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

  it("submits annotation adjustments for the selected variant scope", async () => {
    const onAnnotations = vi.fn().mockResolvedValue(true);
    render(<CreativeComparisonWorkspace {...props} onAnnotations={onAnnotations} annotationScopes={["size", "variant"]} />);
    const scopeGroup = screen.getByRole("group", { name: "调整范围" });
    expect(within(scopeGroup).getByRole("button", { name: "当前尺寸" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(within(scopeGroup).getByRole("button", { name: "当前变体三尺寸" }));

    const surface = screen.getByTestId("creative-annotation-surface");
    Object.defineProperty(surface, "getBoundingClientRect", { value: () => ({ x: 0, y: 0, left: 0, top: 0, right: 400, bottom: 400, width: 400, height: 400, toJSON: () => ({}) }) });
    Object.defineProperty(surface, "setPointerCapture", { value: vi.fn() });
    fireEvent.click(screen.getByRole("button", { name: "点标注" }));
    fireEvent.pointerDown(surface, { clientX: 100, clientY: 100, pointerId: 1 });
    fireEvent.pointerUp(surface, { clientX: 100, clientY: 100, pointerId: 1 });
    fireEvent.change(screen.getByRole("textbox", { name: "标注 1 调整说明" }), { target: { value: "三个尺寸都改标题" } });
    fireEvent.click(screen.getByRole("button", { name: "完成标注" }));
    fireEvent.click(screen.getByRole("button", { name: "提交 1 处调整" }));

    await waitFor(() => expect(onAnnotations).toHaveBeenCalledTimes(1));
    expect(onAnnotations.mock.calls[0]?.[0]?.[0]).toMatchObject({ scope: "variant", comment: "三个尺寸都改标题" });
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

  it("zooms only when the wheel is over the generated image pane", async () => {
    render(<CreativeComparisonWorkspace {...props} />);
    const sourceWheel = new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: -100 });
    act(() => screen.getByTestId("creative-comparison-source-pane").dispatchEvent(sourceWheel));
    expect(sourceWheel.defaultPrevented).toBe(false);
    expect(screen.getByText("100%")).toBeInTheDocument();

    const resultWheel = new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: -100 });
    act(() => screen.getByTestId("creative-comparison-result-pane").dispatchEvent(resultWheel));
    expect(resultWheel.defaultPrevented).toBe(true);
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
