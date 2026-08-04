// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CreativeMaterialCandidate } from "@multica/core/types";
import { Field, MaterialTile, orderDraftWithRecommendation } from "./creative-material-library";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

describe("CreativeMaterialLibrary UI", () => {
  it("shows a short material id on every tile", () => {
    const candidate = {
      id: "abcdef12-3456-7890-abcd-ef1234567890",
      title: "重复标题",
      competitor: "参考来源",
      connector_id: "manual_upload",
      area_names: [],
      tags: [],
      archive_status: "completed",
      analysis_status: "pending",
      archived_url: "https://cdn.example/source.png",
      source_attachment_id: "attachment-1",
    } as unknown as CreativeMaterialCandidate;

    render(<MaterialTile candidate={candidate} analysisState={{ ready: true, status: "completed", error: "", version: 1 }} selected={false} decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onDirectEdit={vi.fn()} />);

    expect(screen.getByText("素材 ID abcdef12")).toBeInTheDocument();
    expect(screen.getByText("已分析 v1")).toBeInTheDocument();
    expect(screen.queryByText("等待分析")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "选择素材" })).toBeEnabled();
  });

  it("binds form labels to their controls", () => {
    render(<Field label="市场资源包"><select><option value="market-1">市场包</option></select></Field>);

    const control = screen.getByLabelText("市场资源包");
    expect(control).toHaveAttribute("id");
    expect(screen.getByText("市场资源包")).toHaveAttribute("for", control.id);
  });

  it("fills the recommendation when copy entries arrive after the order draft", () => {
    const initial = orderDraftWithRecommendation(undefined, "");
    const filled = orderDraftWithRecommendation(initial, "copy-recommended");
    const preserved = orderDraftWithRecommendation({ ...filled, copyEntryId: "copy-user-selected" }, "copy-recommended");

    expect(initial.copyEntryId).toBe("");
    expect(filled.copyEntryId).toBe("copy-recommended");
    expect(preserved.copyEntryId).toBe("copy-user-selected");
  });
});
