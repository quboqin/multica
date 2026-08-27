// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { CreativeStagingRepairWorkspace, creativeStagingRepairEntries, creativeVariantRevisionExpectedSizes } from "./creative-staging-repair-workspace";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

function asset(input: Partial<CreativeOrderAsset> & Pick<CreativeOrderAsset, "id" | "revision" | "stage">): CreativeOrderAsset {
  return {
    variant_id: "variant-1",
    asset_family_id: "family-1",
    size_key: "800x1000",
    attachment_id: input.id,
    derived_from_asset_id: "",
    metadata: {},
    evidence: {},
    status: "completed",
    created_at: "2026-08-27T00:00:00Z",
    updated_at: "2026-08-27T00:00:00Z",
    ...input,
  };
}

function failedStagingItem(): CreativeOrderItem {
  const variant = {
    id: "variant-1",
    order_item_id: "item-1",
    variant_key: "V01",
    brief: {},
    revision: 3,
    status: "action_required",
    active_revision: 2,
    staging_revision: 3,
    qc_status: "failed",
    qc_recovery_used: false,
    qc_recovery_available: false,
    created_at: "",
    updated_at: "",
    revisions: [
      { revision: 2, brief: {}, status: "active", expected_sizes: ["1080x1080", "1200x628", "800x1000"], activated_at: "", created_at: "", updated_at: "" },
      { revision: 3, brief: {}, status: "action_required", expected_sizes: ["800x1000"], activated_at: "", created_at: "", updated_at: "" },
    ],
    assets: [
      asset({ id: "active-r2", revision: 2, stage: "delivered" }),
      asset({ id: "staging-r3-base", revision: 3, stage: "generated" }),
      asset({ id: "staging-r3-prime", revision: 3, stage: "primed" }),
    ],
    image_operations: [],
    diagnostic_assets: [],
    qc_reports: [],
  } as CreativeOrderVariant;
  return {
    id: "item-1",
    order_id: "order-1",
    candidate_id: "candidate-1",
    source_analysis_id: "analysis-1",
    copy_snapshot: {},
    direction: "",
    status: "ready",
    adopted_variant_id: variant.id,
    adopted_at: "",
    adopted_by: "",
    created_at: "",
    updated_at: "",
    variants: [variant],
  };
}

describe("CreativeStagingRepairWorkspace", () => {
  it("selects only completed Prime/final assets from a failed staging revision", () => {
    const item = failedStagingItem();
    const variant = item.variants[0]!;

    expect(creativeVariantRevisionExpectedSizes(variant, 3)).toEqual(["800x1000"]);
    expect(creativeStagingRepairEntries([item])).toMatchObject([{
      asset: { id: "staging-r3-prime", revision: 3, stage: "primed" },
      baseAsset: { id: "staging-r3-base" },
      activeAsset: { id: "active-r2", revision: 2 },
      expectedSizes: ["800x1000"],
    }]);
  });

  it("keeps staging out of delivery actions and submits annotations against the staging asset", async () => {
    const item = failedStagingItem();
    const attachments = new Map(item.variants[0]!.assets.map((entry) => [entry.attachment_id, {
      id: entry.attachment_id,
      filename: `${entry.id}.png`,
      url: `https://cdn.example/${entry.id}.png`,
      download_url: `https://cdn.example/${entry.id}.png`,
      markdown_url: "",
      content_type: "image/png",
    }]));
    const onAnnotations = vi.fn().mockResolvedValue(true);
    render(<CreativeStagingRepairWorkspace items={[item]} attachments={attachments} onAnnotations={onAnnotations} />);

    fireEvent.click(screen.getByRole("button", { name: /查看并标注 V01 · r3 · 竖版/ }));
    expect(screen.getByRole("dialog")).toHaveTextContent("线上 r2 保持不变");
    expect(screen.queryByRole("button", { name: "下载当前成图" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "接受" })).not.toBeInTheDocument();

    const surface = screen.getByTestId("creative-annotation-surface");
    Object.defineProperty(surface, "getBoundingClientRect", { value: () => ({ x: 0, y: 0, left: 0, top: 0, right: 400, bottom: 400, width: 400, height: 400, toJSON: () => ({}) }) });
    Object.defineProperty(surface, "setPointerCapture", { value: vi.fn() });
    fireEvent.click(screen.getByRole("button", { name: "点标注" }));
    fireEvent.pointerDown(surface, { clientX: 100, clientY: 100, pointerId: 1 });
    fireEvent.pointerUp(surface, { clientX: 100, clientY: 100, pointerId: 1 });
    fireEvent.change(screen.getByRole("textbox", { name: "标注 1 调整说明" }), { target: { value: "提高标题对比度" } });
    fireEvent.click(screen.getByRole("button", { name: "完成标注" }));
    fireEvent.click(screen.getByRole("button", { name: "提交 1 处调整" }));

    await waitFor(() => expect(onAnnotations).toHaveBeenCalledTimes(1));
    expect(onAnnotations.mock.calls[0]?.[0]).toMatchObject({ id: "staging-r3-prime", revision: 3, stage: "primed" });
    expect(onAnnotations.mock.calls[0]?.[1]?.[0]).toMatchObject({ kind: "point", comment: "提高标题对比度", scope: "size" });
  });
});
