// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CreativeMaterialCandidate } from "@multica/core/types";
import { manualCopySnapshot, Field, MaterialTile, materialCandidateDisplayDetails, materialImportNotice, orderDraftWithRecommendation } from "./creative-material-library";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

describe("CreativeMaterialLibrary UI", () => {
  it("explains whether automatic reference analysis was queued after import", () => {
    expect(materialImportNotice({
      id: "candidate-1",
      analysis: { action: "queued", status: "pending", warning: "", crawl_run_id: "run-1", analysis_agent_id: "agent-1", task_id: "task-1" },
    })).toEqual({ level: "success", message: "素材已导入，参考分析已排队" });
    expect(materialImportNotice({
      id: "candidate-1",
      analysis: { action: "unavailable", status: "failed", warning: "missing agent", crawl_run_id: "run-1", analysis_agent_id: "", task_id: "" },
    })).toEqual({ level: "warning", message: "素材已导入，但尚未配置参考分析智能体" });
  });

  it("shows a short material id on every tile", () => {
    const onDirectEdit = vi.fn();
    const candidate = {
      id: "abcdef12-3456-7890-abcd-ef1234567890",
      title: "重复标题",
      competitor: "参考来源",
      connector_id: "manual_upload",
      duration_days: 6,
      impression_estimate: 13000,
      media_names: ["Meta"],
      area_names: [],
      language_names: [],
      platform_names: ["Android"],
      tags: [],
      note: "",
      archive_status: "completed",
      analysis_status: "pending",
      archived_url: "https://cdn.example/source.png",
      source_attachment_id: "attachment-1",
    } as unknown as CreativeMaterialCandidate;

    render(<MaterialTile candidate={candidate} analysisState={{ ready: true, status: "completed", error: "", version: 1 }} selected={false} decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onDirectEdit={onDirectEdit} onRetryAnalysis={vi.fn()} />);

    expect(screen.getByText("素材 ID abcdef12")).toBeInTheDocument();
    expect(screen.getByText("6 天")).toBeInTheDocument();
    expect(screen.getByText("1.3 万")).toBeInTheDocument();
    expect(screen.getByText("Meta")).toBeInTheDocument();
    expect(screen.getByText("已分析 v1")).toBeInTheDocument();
    expect(screen.queryByText("等待分析")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "选择素材" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "直接改图" }));
    expect(onDirectEdit).toHaveBeenCalledOnce();
  });

  it("offers a failed historical material a direct reanalysis action", () => {
    const onRetryAnalysis = vi.fn();
    const candidate = { id: "candidate-1", title: "分析失败素材", connector_id: "manual_upload", archive_status: "completed", analysis_status: "failed", tags: [], area_names: [], language_names: [], platform_names: [], media_names: [], note: "" } as unknown as CreativeMaterialCandidate;

    render(<MaterialTile candidate={candidate} analysisState={{ ready: false, status: "failed", error: "task unavailable", version: 1 }} selected={false} decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onDirectEdit={vi.fn()} onRetryAnalysis={onRetryAnalysis} />);

    fireEvent.click(screen.getByRole("button", { name: "重新分析素材" }));
    expect(onRetryAnalysis).toHaveBeenCalledOnce();
  });

  it("reads collected details from loose source metadata before declaring them missing", () => {
    const candidate = {
      connector_id: "appgrowing",
      duration_days: null,
      impression_estimate: null,
      media_names: [],
      area_names: [],
      language_names: [],
      platform_names: [],
      tags: [],
      note: "",
      raw: JSON.stringify({
        duration_days: "4",
        impressions: 16400,
        media: ["TikTok"],
        market: "Indonesia",
        language: "Bahasa Indonesia",
        tags: ["借贷"],
        remark: "重点参考",
      }),
    } as unknown as CreativeMaterialCandidate;

    expect(materialCandidateDisplayDetails(candidate)).toEqual({
      duration: "4 天",
      impressions: "1.6 万",
      media: "TikTok",
      market: "Indonesia",
      languages: "Bahasa Indonesia",
      platforms: "源数据未提供",
      tags: "借贷",
      note: "重点参考",
    });
  });

  it("labels platform and connector fallbacks instead of presenting inferred media as source data", () => {
    const platformFallback = {
      connector_id: "appgrowing",
      media_names: [],
      area_names: [],
      language_names: [],
      platform_names: ["iOS"],
      tags: [],
      note: "",
      duration_days: null,
      impression_estimate: null,
    } as unknown as CreativeMaterialCandidate;
    const connectorFallback = { ...platformFallback, platform_names: [] } as unknown as CreativeMaterialCandidate;

    expect(materialCandidateDisplayDetails(platformFallback).media).toBe("iOS（投放平台）");
    expect(materialCandidateDisplayDetails(connectorFallback).media).toBe("AppGrowing（采集来源）");
    expect(materialCandidateDisplayDetails(connectorFallback).languages).toBe("源数据未提供");
  });

  it("binds form labels to their controls", () => {
    render(<Field label="市场资源包"><select><option value="market-1">市场包</option></select></Field>);

    const control = screen.getByLabelText("市场资源包");
    expect(control).toHaveAttribute("id");
    expect(screen.getByText("市场资源包")).toHaveAttribute("for", control.id);
  });

  it("does not choose a dynamic recommendation on the user's behalf", () => {
    const initial = orderDraftWithRecommendation(undefined, "");
    const filled = orderDraftWithRecommendation(initial, "copy-recommended");
    const preserved = orderDraftWithRecommendation({ ...filled, selectedRecommendationId: "copy-user-selected" }, "copy-recommended");

    expect(initial.selectedRecommendationId).toBe("");
    expect(filled.selectedRecommendationId).toBe("");
    expect(preserved.selectedRecommendationId).toBe("copy-user-selected");
  });

  it("keeps the inferred creative type when the user manually adjusts copy", () => {
    const snapshot = manualCopySnapshot({
      mode: "manual",
      selectedRecommendationId: "",
      direction: "",
      manualHeadline: "Pilih tenor",
      manualSubheadline: "",
      manualBenefit: "Cicilan sesuai kebutuhan",
      manualSupporting: "",
      manualCta: "Ajukan sekarang",
    }, "repayment_plan");

    expect(snapshot).toMatchObject({ creative_type: "repayment_plan", status: "user_custom" });
  });
});
