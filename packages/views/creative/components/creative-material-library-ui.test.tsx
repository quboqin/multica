// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { CreativeMaterialCandidate } from "@multica/core/types";
import { compiledProductionPrompt, creativeMaterialProductionState, effectiveNumericLayouts, effectiveRepaymentPlanSelections, frozenCopySnapshot, groupTextReplacementsByLocation, hasCompleteTextReplacements, hasPendingReplacementConfirmation, latestAnalyses, latestCompletedAnalyses, manualCopySnapshot, normalizedVisualBounds, overallProductionRequirement, Field, MaterialBatchSelectionBar, MaterialBatchSidebar, MaterialTile, materialCandidateDisplayDetails, materialImportNotice, orderDraftWithPreAdaptation, preparedPreAdaptation, SelectedMaterialStrip, selectedMaterialReadinessLabel, sourceAnalysisAdaptationErrorCode, sourceAnalysisAdaptationStatus, sourceAnalysisNeedsVisualUpgrade, sourceTextBlockVisualBounds, visualReviewRegions, type PreparedPreAdaptation } from "./creative-material-library";
import { userFacingProductionPrompt } from "../lib/creative-production-prompt";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

const availableProductionState = { status: "available", label: "可用", selectable: true, active: false } as const;
const failedProductionState = { status: "failed", label: "处理失败", selectable: false, active: false } as const;

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
    const candidate = {
      id: "abcdef12-3456-7890-abcd-ef1234567890",
      title: "重复标题",
      asset_type: "image",
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

    render(<MaterialTile candidate={candidate} analysisState={{ ready: true, status: "completed", error: "", version: 1 }} productionState={availableProductionState} selected={false} decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onRetryAnalysis={vi.fn()} />);

    expect(screen.getByText("素材 ID abcdef12")).toBeInTheDocument();
    expect(screen.getByText("6 天")).toBeInTheDocument();
    expect(screen.getByText("1.3 万")).toBeInTheDocument();
    expect(screen.getAllByText("Meta")).toHaveLength(2);
    expect(screen.getByText("可用")).toBeInTheDocument();
    expect(screen.queryByText("已完成分析 v1")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "选择出图素材" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "直接改图" })).not.toBeInTheDocument();
  });

  it("keeps an in-progress local selection visible on its material card", () => {
    const candidate = {
      id: "candidate-1",
      title: "已选素材",
      asset_type: "image",
      connector_id: "manual_upload",
      archive_status: "completed",
      analysis_status: "completed",
      archived_url: "https://cdn.example/source.png",
      tags: [], area_names: [], language_names: [], platform_names: [], media_names: [], note: "",
    } as unknown as CreativeMaterialCandidate;

    render(<MaterialTile candidate={candidate} analysisState={{ ready: true, status: "completed", error: "", version: 1 }} productionState={availableProductionState} selected decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onRetryAnalysis={vi.fn()} />);

    expect(screen.getByText("已选")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "取消选择" })).toBeEnabled();
  });

  it("allows a selected material to be cancelled from the top order strip", () => {
    const onSelect = vi.fn();
    const onDeselect = vi.fn();
    const candidate = {
      id: "abcdef12-3456-7890-abcd-ef1234567890",
      title: "顶部可取消素材",
      competitor: "参考来源",
      connector_id: "manual_upload",
      archive_status: "completed",
      archived_url: "https://cdn.example/source.png",
    } as unknown as CreativeMaterialCandidate;

    render(<SelectedMaterialStrip candidates={[candidate]} activeCandidateId={candidate.id} readinessByCandidateId={new Map([[candidate.id, "ready"]])} deselecting={false} onSelect={onSelect} onDeselect={onDeselect} />);

    expect(screen.getByRole("button", { name: "取消选择素材" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "取消选择素材" }));
    expect(onDeselect).toHaveBeenCalledWith(candidate.id);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("folds copy preparation into the material readiness label", () => {
    expect(selectedMaterialReadinessLabel("ready")).toBe("可用");
    expect(selectedMaterialReadinessLabel("analyzing")).toBe("分析中");
    expect(selectedMaterialReadinessLabel("manual_required")).toBe("待人工确认");
    expect(selectedMaterialReadinessLabel("failed")).toBe("处理失败");
  });

  it("labels failed pre-adaptation separately from image analysis failure", () => {
    const state = creativeMaterialProductionState(
      { asset_type: "image", archived_url: "https://cdn.example/source.png", archive_status: "completed", status: "active" } as any,
      { ready: true, status: "completed", error: "", version: 3 },
      undefined,
      false,
      { ready: false, active: false, failed: true, reason: "Invalid numeric layout" },
    );

    expect(state).toMatchObject({ status: "failed", label: "适配失败", reason: "Invalid numeric layout" });
  });

  it("labels manual pre-adaptation handoff without offering it as available", () => {
    const state = creativeMaterialProductionState(
      { asset_type: "image", archived_url: "https://cdn.example/source.png", archive_status: "completed", status: "active" } as any,
      { ready: true, status: "completed", error: "", version: 3 },
      undefined,
      false,
      { ready: false, active: false, failed: false, manualRequired: true, reason: "预适配已重试一次仍未通过" },
    );

    expect(state).toMatchObject({ status: "manual_required", label: "待人工确认", selectable: false, reason: "预适配已重试一次仍未通过" });
  });

  it("keeps a local batch visible before the user confirms copy", () => {
    const onConfirm = vi.fn();
    const onClear = vi.fn();
    const candidate = {
      id: "abcdef12-3456-7890-abcd-ef1234567890",
      title: "待比较素材",
      competitor: "参考来源",
      connector_id: "manual_upload",
      archive_status: "completed",
      archived_url: "https://cdn.example/source.png",
    } as unknown as CreativeMaterialCandidate;

    render(<MaterialBatchSelectionBar candidates={[candidate]} onConfirm={onConfirm} onClear={onClear} />);

    const selection = within(screen.getByRole("region", { name: "已选出图素材" }));
    expect(selection.getByText("已选 1 张素材")).toBeInTheDocument();
    expect(selection.getByText("已在素材卡片中标记，可继续筛选和比较。")).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.click(selection.getByRole("button", { name: "下一步：确认文案" }));
    expect(onConfirm).toHaveBeenCalledOnce();

    fireEvent.click(selection.getByRole("button", { name: "清空选择" }));
    expect(onClear).toHaveBeenCalledOnce();
  });

  it("uses collection batches as a left-side navigation scope", () => {
    const onSelect = vi.fn();

    render(<MaterialBatchSidebar
      activeRunId="run-2"
      onSelect={onSelect}
      runs={[
        { id: "run-1", connector_id: "appgrowing", query_summary: "发薪日素材", imported_count: 12, total_count: 18, candidate_metrics: { selected: 3 }, created_at: "2026-08-09T10:00:00Z" } as any,
        { id: "run-2", connector_id: "appgrowing", query_summary: "世界杯素材", imported_count: 30, total_count: 30, candidate_metrics: { selected: 0 }, created_at: "2026-08-10T10:00:00Z" } as any,
      ]}
    />);

    const sidebar = screen.getByLabelText("采集批次");
    expect(within(sidebar).getByText("采集批次")).toBeInTheDocument();
    expect(within(sidebar).getByText("选择批次后，右侧继续用状态和搜索筛选。")).toBeInTheDocument();
    expect(within(sidebar).getByRole("button", { name: /世界杯素材/ })).toHaveAttribute("aria-pressed", "true");
    expect(within(sidebar).getByText("命中 18 张 · 入库 12 张 · 已选 3 张")).toBeInTheDocument();

    fireEvent.click(within(sidebar).getByRole("button", { name: /发薪日素材/ }));
    fireEvent.click(within(sidebar).getByRole("button", { name: /全部素材/ }));

    expect(onSelect).toHaveBeenNthCalledWith(1, "run-1");
    expect(onSelect).toHaveBeenNthCalledWith(2, "");
  });

  it("offers a failed historical material a direct reanalysis action", () => {
    const onRetryAnalysis = vi.fn();
    const candidate = { id: "candidate-1", title: "分析失败素材", connector_id: "manual_upload", archive_status: "completed", analysis_status: "failed", tags: [], area_names: [], language_names: [], platform_names: [], media_names: [], note: "" } as unknown as CreativeMaterialCandidate;

    render(<MaterialTile candidate={candidate} analysisState={{ ready: false, status: "failed", error: "task unavailable", version: 1 }} productionState={failedProductionState} selected={false} decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onRetryAnalysis={onRetryAnalysis} />);

    fireEvent.click(screen.getByRole("button", { name: "重新分析素材" }));
    expect(onRetryAnalysis).toHaveBeenCalledOnce();
  });

  it("shows the material preparation failure reason on the card", () => {
    const candidate = { id: "candidate-1", title: "预适配失败素材", connector_id: "manual_upload", archive_status: "completed", analysis_status: "completed", archived_url: "https://cdn.example/source.png", tags: [], area_names: [], language_names: [], platform_names: [], media_names: [], note: "" } as unknown as CreativeMaterialCandidate;

    render(<MaterialTile candidate={candidate} analysisState={{ ready: true, status: "completed", error: "", version: 2 }} productionState={{ ...failedProductionState, reason: "数值布局类型未被服务端接受" }} selected={false} decision="" busy={false} onToggle={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} onOpen={vi.fn()} onRetryAnalysis={vi.fn()} />);

    expect(screen.getByText("数值布局类型未被服务端接受")).toBeInTheDocument();
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

  it("keeps media source separate from platform metadata", () => {
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

    expect(materialCandidateDisplayDetails(platformFallback).media).toBe("源数据未提供");
    expect(materialCandidateDisplayDetails(platformFallback).platforms).toBe("iOS");
    expect(materialCandidateDisplayDetails(connectorFallback).media).toBe("源数据未提供");
    expect(materialCandidateDisplayDetails(connectorFallback).languages).toBe("源数据未提供");
  });

  it("binds form labels to their controls", () => {
    render(<Field label="市场资源包"><select><option value="market-1">市场包</option></select></Field>);

    const control = screen.getByLabelText("市场资源包");
    expect(control).toHaveAttribute("id");
    expect(screen.getByText("市场资源包")).toHaveAttribute("for", control.id);
  });

  it("uses the model pre-adaptation draft without a code recommendation", () => {
    const initial = orderDraftWithPreAdaptation(undefined, "first prompt");
    const preserved = orderDraftWithPreAdaptation({ ...initial, productionPrompt: "edited prompt" }, "new prompt");

    expect(initial.mode).toBe("pre_adaptation");
    expect(initial.productionPrompt).toBe("first prompt");
    expect(preserved.productionPrompt).toBe("edited prompt");
  });

  it("requires every ordinary source text block to have an approved replacement", () => {
    const adaptation = preparedPreAdaptation({
      result: {
        adaptation: {
          status: "completed",
          summary: "ready",
          result: {
            market_pack_id: "market-1", market_pack_version: 2,
            copy_library_id: "library-1", copy_library_version: 3,
            text_replacements: [
              { block_id: "banner", location: "中部横幅", role: "benefit", source_text: "Flexible", replacement_text: "", source_keys: [], status: "missing", note: "missing approved copy" },
            ],
            numeric_layouts: [{ id: "top-table", source_block_ids: ["top-left"], location: "左上卡片", layout_kind: "table", scenario_ids: ["scenario-1"], target_columns: ["principal"], render_instruction: "Jumlah Pinjaman Rp30.000.000" }],
          },
        },
      },
    } as any, { id: "market-1", published_version: 2 }, { id: "library-1", published_version: 3 });

    expect(adaptation?.textReplacements).toHaveLength(1);
    expect(adaptation?.numericLayouts).toHaveLength(1);
    expect(hasCompleteTextReplacements(adaptation!.textReplacements)).toBe(false);
    expect(hasCompleteTextReplacements(adaptation!.textReplacements, { banner: "Cicilan fleksibel" })).toBe(true);
  });

  it("normalizes semantic numeric layout hints in prepared adaptations", () => {
    const adaptation = preparedPreAdaptation({
      result: {
        text_blocks: [
          { id: "amount", visual_bounds: { x: 100, y: 100, width: 300, height: 80 } },
          { id: "tenor-3", visual_bounds: { x: 100, y: 220, width: 140, height: 60 } },
          { id: "tenor-6", visual_bounds: { x: 260, y: 220, width: 140, height: 60 } },
          { id: "payment", visual_bounds: { x: 560, y: 520, width: 220, height: 60 } },
        ],
        adaptation: {
          status: "completed",
          summary: "ready",
          result: {
            market_pack_id: "market-1", market_pack_version: 2,
            copy_library_id: "library-1", copy_library_version: 3,
            text_replacements: [],
            repayment_plan_selections: [{ id: "row-6", plan_key: "8m-6", principal: 8_000_000, tenor_months: 6, values: { principal: "Rp8.000.000", tenor: "6 Bulan", total_interest: "Rp432.000", total_repayment: "Rp8.432.000", monthly_installment: "Rp1.405.333" } }],
            numeric_layouts: [
              { id: "amount-layout", source_block_ids: ["amount"], location: "金额框", layout_kind: "single_value", scenario_ids: ["row-6"], target_columns: ["principal"], render_instruction: "展示 Rp8.000.000。" },
              { id: "tenor-layout", source_block_ids: ["tenor-3", "tenor-6"], location: "期限按钮", layout_kind: "option_buttons", scenario_ids: ["row-6"], target_columns: ["tenor"], render_instruction: "展示 6 Bulan。" },
              { id: "row-layout", source_block_ids: ["payment"], location: "表格行", layout_kind: "table_row", scenario_ids: ["row-6"], target_columns: ["monthly_installment"], render_instruction: "展示 Rp1.405.333。" },
              { id: "principal-layout", source_block_ids: ["amount"], location: "本金区", layout_kind: "principal", scenario_ids: ["row-6"], target_columns: ["principal"], render_instruction: "展示 Rp8.000.000。" },
              { id: "semantic-tenor-layout", source_block_ids: ["tenor-3", "tenor-6"], location: "期限区", layout_kind: "tenor", scenario_ids: ["row-6"], target_columns: ["tenor"], render_instruction: "展示 6 Bulan。" },
              { id: "repayment-table-layout", source_block_ids: ["payment"], location: "还款表", layout_kind: "repayment_table", scenario_ids: ["row-6"], target_columns: ["monthly_installment"], render_instruction: "展示 Rp1.405.333。" },
            ],
            production_prompt: "展示 Rp8.000.000。展示 6 Bulan。展示 Rp1.405.333。",
          },
        },
      },
    } as any, { id: "market-1", published_version: 2 }, { id: "library-1", published_version: 3 });

    expect(adaptation?.numericLayouts.map((layout) => layout.layoutKind)).toEqual(["single_value", "option_buttons", "table_row", "single_value", "option_buttons", "table"]);
  });

  it("keeps calculated and recommended gaps pending until a person chooses their source", () => {
    const adaptation = preparedPreAdaptation({
      result: {
        text_blocks: [
          { id: "daily", semantic_kind: "daily_interest_amount", visual_bounds: { x: 100, y: 100, width: 200, height: 60 } },
          { id: "headline", semantic_kind: "copy", visual_bounds: { x: 100, y: 200, width: 400, height: 60 } },
        ],
        adaptation: {
          status: "completed",
          summary: "two pending choices",
          result: {
            market_pack_id: "market-1", market_pack_version: 2,
            copy_library_id: "library-1", copy_library_version: 3,
            text_replacements: [
              { block_id: "daily", location: "日息区", role: "benefit", source_text: "Rp10.000", replacement_text: "Rp1.500 per hari", source_keys: [], status: "calculated", calculation: { formula: "本金 × 日利率", inputs: ["Rp5.000.000", "0,03%"], result: "Rp1.500 per hari" } },
              { block_id: "headline", location: "顶部标题", role: "headline", source_text: "Dana cepat", replacement_text: "Dana fleksibel", source_keys: [], status: "recommended", recommendation_basis: ["当前订单通用利益点"] },
            ],
          },
        },
      },
    } as any, { id: "market-1", published_version: 2 }, { id: "library-1", published_version: 3 });

    expect(adaptation?.textReplacements[0]).toMatchObject({ semanticKind: "daily_interest_amount", status: "calculated", calculation: { formula: "本金 × 日利率" } });
    expect(hasPendingReplacementConfirmation(adaptation!.textReplacements[0]!)).toBe(true);
    expect(hasPendingReplacementConfirmation(adaptation!.textReplacements[0]!, {}, { daily: { kind: "calculation" } })).toBe(false);
    const draft = {
      ...orderDraftWithPreAdaptation(undefined, adaptation!.productionPrompt),
      replacementSources: { daily: { kind: "calculation" as const } },
    };
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, adaptation!, draft, "analysis-1");
    expect(frozen.pre_adaptation?.text_replacements[0]).toMatchObject({
      block_id: "daily",
      replacement_text: "Rp1.500 per hari",
      source_kind: "calculation",
      calculation: { result: "Rp1.500 per hari" },
    });
    expect(hasPendingReplacementConfirmation(adaptation!.textReplacements[1]!)).toBe(true);
    expect(hasPendingReplacementConfirmation(adaptation!.textReplacements[1]!, { headline: "我方手动标题" }, { headline: { kind: "manual" } })).toBe(false);
  });

  it("requires a legacy source analysis to be upgraded before it can be prepared", () => {
    const legacy = {
      id: "analysis-v1", candidate_id: "candidate-1", analysis_version: 1, status: "completed",
      result: { text_blocks: [{ id: "headline", source_text: "Dana cepat" }] },
    } as any;
    const updating = { ...legacy, id: "analysis-v2", analysis_version: 2, status: "pending", result: {} };

    expect(sourceAnalysisNeedsVisualUpgrade(legacy)).toBe(true);
    expect(latestCompletedAnalyses([legacy, updating]).has("candidate-1")).toBe(false);
    expect(latestAnalyses([legacy, updating]).get("candidate-1")).toMatchObject({ id: "analysis-v2", status: "pending" });
  });

  it("links each replacement to the normalized source-image text bounds", () => {
    const adaptation = preparedPreAdaptation({
      result: {
        text_blocks: [
          { id: "headline", visual_bounds: { x: 120, y: 80, width: 640, height: 90 } },
          { id: "invalid", visual_bounds: { x: 900, y: 100, width: 200, height: 50 } },
        ],
        adaptation: {
          status: "completed",
          summary: "ready",
          result: {
            market_pack_id: "market-1", market_pack_version: 2,
            copy_library_id: "library-1", copy_library_version: 3,
            text_replacements: [
              { block_id: "headline", location: "顶部标题", role: "headline", source_text: "Bunga flat", replacement_text: "Bunga mulai dari 0,03%", source_keys: ["headline"], status: "ready", note: "" },
              { block_id: "invalid", location: "底部脚注", role: "legal", source_text: "Terms", replacement_text: "", source_keys: [], status: "missing", note: "" },
            ],
          },
        },
      },
    } as any, { id: "market-1", published_version: 2 }, { id: "library-1", published_version: 3 });

    expect(adaptation?.textReplacements[0]?.visualBounds).toEqual({ x: 120, y: 80, width: 640, height: 90 });
    expect(adaptation?.textReplacements[1]?.visualBounds).toBeUndefined();
  });

  it("drops malformed source-image bounds instead of drawing a misleading hotspot", () => {
    expect(normalizedVisualBounds({ x: 0, y: 0, width: 1, height: 1 })).toEqual({ x: 0, y: 0, width: 1, height: 1 });
    expect(normalizedVisualBounds({ x: 999, y: 0, width: 2, height: 1 })).toBeNull();
    expect(normalizedVisualBounds({ x: 0, y: 0, width: 0, height: 1 })).toBeNull();
    expect(sourceTextBlockVisualBounds({ text_blocks: [{ id: "bad", visual_bounds: { x: "0", y: 0, width: 1, height: 1 } }] }).size).toBe(0);
  });

  it("renders text and repayment data as distinct visual regions instead of two checklists", () => {
    const regions = visualReviewRegions({
      status: "completed", summary: "ready", decision: "", preferredFragmentKeys: [], reasons: [], gaps: [], analysisHighlights: [], productionPrompt: "prompt",
      visualRegions: [
        { id: "rate-card", location: "右侧利率卡", kind: "copy", sourceBlockIds: ["rate-label", "rate-value"], visualBounds: { x: 620, y: 220, width: 230, height: 160 } },
        { id: "repayment-table", location: "底部还款表", kind: "numeric", sourceBlockIds: ["table-principal", "table-tenor"], visualBounds: { x: 80, y: 600, width: 800, height: 250 } },
      ],
      textReplacements: [
        { blockId: "rate-label", visualRegionId: "rate-card", location: "右侧利率卡", role: "benefit", sourceText: "Bunga mulai", replacementText: "Bunga mulai dari", sourceKeys: ["rate-label"], status: "ready", note: "" },
        { blockId: "rate-value", visualRegionId: "rate-card", location: "右侧利率卡", role: "benefit", sourceText: "0,01%", replacementText: "0,03%*", sourceKeys: ["rate-value"], status: "ready", note: "" },
      ],
      repaymentPlanSelections: [{ id: "row-1", planKey: "plan-1", principal: 5_000_000, tenorMonths: 3, values: { principal: "Rp5.000.000", tenor: "3 Bulan", monthlyInstallment: "Rp1.711.667", totalInterest: "Rp135.001", totalRepayment: "Rp5.135.001" } }],
      numericLayouts: [{ id: "table", visualRegionId: "repayment-table", sourceBlockIds: ["table-principal", "table-tenor"], location: "底部还款表", layoutKind: "table", scenarioIds: ["row-1"], targetColumns: ["principal", "tenor"], renderInstruction: "Use the approved table row." }],
    });

    expect(regions).toHaveLength(2);
    expect(regions[0]).toMatchObject({ id: "rate-card", kind: "copy", textReplacements: [{ blockId: "rate-label" }, { blockId: "rate-value" }], numericLayouts: [] });
    expect(regions[1]).toMatchObject({ id: "repayment-table", kind: "numeric", textReplacements: [], numericLayouts: [{ id: "table" }] });
  });

  it("turns an unfilled source block into an explicit removal instruction", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed", summary: "ready", decision: "", preferredFragmentKeys: [], reasons: [], gaps: [], analysisHighlights: [],
      textReplacements: [{ blockId: "footer", location: "底部脚注", role: "legal", sourceText: "Competitor legal text", replacementText: "", sourceKeys: [], status: "missing", note: "No approved legal text" }],
      repaymentPlanSelections: [], numericLayouts: [], productionPrompt: "Keep the source layout.",
    };
    const draft = orderDraftWithPreAdaptation(undefined, adaptation.productionPrompt);
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, adaptation, draft, "analysis-1");

    expect(compiledProductionPrompt(adaptation, {}, draft.productionPrompt)).toContain("留空并移除原文");
    expect(frozen.pre_adaptation?.text_replacements[0]).toMatchObject({ status: "missing", replacement_text: "" });
  });

  it("does not duplicate frozen replacement and numeric instructions when submitting", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed", summary: "ready", decision: "", preferredFragmentKeys: [], reasons: [], gaps: [], analysisHighlights: [],
      textReplacements: [{ blockId: "headline", location: "顶部标题", role: "headline", sourceText: "Competitor", replacementText: "Pembiayaan Fleksibel", sourceKeys: ["headline"], status: "ready", note: "" }],
      repaymentPlanSelections: [],
      numericLayouts: [{ id: "amount-card", sourceBlockIds: ["amount"], location: "金额卡", layoutKind: "single_card", scenarioIds: ["row-1"], targetColumns: ["principal"], renderInstruction: "Render the approved principal Rp10.000.000." }],
      productionPrompt: "Keep the original hierarchy. Use Pembiayaan Fleksibel. Render the approved principal Rp10.000.000.",
    };

    const prompt = compiledProductionPrompt(adaptation, {});

    expect(overallProductionRequirement(adaptation)).toBe("Keep the original hierarchy.");
    expect(prompt).toContain("Keep the original hierarchy.");
    expect(prompt).toContain("顶部标题（标题）：Pembiayaan Fleksibel");
    expect(prompt).toContain("金额卡：Render the approved principal Rp10.000.000.");
    expect(prompt).not.toContain("Use Pembiayaan Fleksibel.");
  });

  it("does not append numeric layout again when its financial values are already in the base prompt", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed", summary: "ready", decision: "", preferredFragmentKeys: [], reasons: [], gaps: [], analysisHighlights: [],
      textReplacements: [],
      repaymentPlanSelections: [],
      numericLayouts: [{ id: "table", sourceBlockIds: ["amount"], location: "还款表", layoutKind: "table", scenarioIds: ["row-1"], targetColumns: ["principal", "tenor", "monthly_installment"], renderInstruction: "Render Jumlah Pinjaman Rp5.000.000, Periode Cicilan 12 Bulan, Cicilan per Bulan Rp461.667." }],
      productionPrompt: "Keep a compact table with Jumlah Pinjaman Rp5.000.000, Periode Cicilan 12 Bulan, and Cicilan per Bulan Rp461.667.",
    };

    const prompt = compiledProductionPrompt(adaptation, {}, adaptation.productionPrompt);

    expect(prompt).not.toContain("数值与还款版式：");
  });

  it("keeps global generation requirements separate from edited block text", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed", summary: "ready", decision: "", preferredFragmentKeys: [], reasons: [], gaps: [], analysisHighlights: [],
      textReplacements: [{ blockId: "payment-label", location: "下部摘要表第四行左侧", role: "supporting", sourceText: "Pembayaran", replacementText: "Total pembayaran", sourceKeys: ["payment-label"], status: "ready", note: "" }],
      repaymentPlanSelections: [{ id: "row-1", planKey: "4m-3", principal: 4_000_000, tenorMonths: 3, values: { principal: "Rp4.000.000", tenor: "3 Bulan", totalInterest: "Rp108.000", totalRepayment: "Rp4.108.000", monthlyInstallment: "Rp1.369.333" } }],
      numericLayouts: [],
      productionPrompt: "保留白底，红黑高对比的信息型借款界面和既有阅读顺序。表头使用 Pinjaman、3 Bulan、6 Bulan、12 Bulan；本金与月供行依次为 Rp4.000.000 对应 Rp1.369.333。移除底部原竞品条款。下部四行摘要表左侧依次使用“Pinjaman”“Cicilan”“Bunga”“Total pembayaran”。",
    };
    const draft = {
      ...orderDraftWithPreAdaptation(undefined, adaptation),
      textOverrides: { "payment-label": "Total pembayaran222" },
    };
    const prompt = compiledProductionPrompt(adaptation, draft.textOverrides, draft.productionPrompt);

    expect(draft.productionPrompt).toContain("保留白底");
    expect(draft.productionPrompt).not.toContain("Rp4.000.000");
    expect(draft.productionPrompt).not.toContain("移除底部");
    expect(draft.productionPrompt).not.toContain("Total pembayaran");
    expect(prompt).not.toContain("“Total pembayaran”");
    expect(prompt).toContain("下部摘要表第四行左侧（辅助说明）：Total pembayaran222");
  });

  it("groups replacement blocks by their displayed location without losing card fields", () => {
    const groups = groupTextReplacementsByLocation([
      { blockId: "top-left-principal", location: "左上还款卡", role: "plan_field", sourceText: "Rp10.000.000", replacementText: "Rp4.000.000", sourceKeys: ["plan_4m"], status: "ready", note: "" },
      { blockId: "top-left-monthly", location: "左上还款卡", role: "plan_field", sourceText: "Rp923.333", replacementText: "Rp369.333", sourceKeys: ["plan_4m_monthly"], status: "ready", note: "" },
      { blockId: "banner", location: "中部横幅", role: "benefit", sourceText: "Flexible", replacementText: "Cicilan Ringan", sourceKeys: ["benefit"], status: "ready", note: "" },
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0]).toMatchObject({ location: "左上还款卡" });
    expect(groups[0]!.replacements.map((replacement) => replacement.blockId)).toEqual(["top-left-principal", "top-left-monthly"]);
    expect(groups[1]!.replacements.map((replacement) => replacement.blockId)).toEqual(["banner"]);
  });

  it("freezes an edited production prompt together with the filled text map", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed",
      summary: "ready",
      decision: "",
      preferredFragmentKeys: [],
      reasons: [],
      gaps: [],
      analysisHighlights: [],
      textReplacements: [{ blockId: "top-left", location: "左上还款卡", role: "plan_field", sourceText: "Rp10.000.000", replacementText: "Rp4.000.000", sourceKeys: ["plan_4m"], status: "ready" as const, note: "" }],
      repaymentPlanSelections: [{ id: "target-30m-3", planKey: "30m-3", principal: 30_000_000, tenorMonths: 3, values: { principal: "Rp30.000.000", tenor: "3 Bulan", totalInterest: "Rp810.000", totalRepayment: "Rp30.810.000", monthlyInstallment: "Rp10.270.000" } }],
      numericLayouts: [{ id: "top-table", sourceBlockIds: ["top-left"], location: "左上还款卡", layoutKind: "table", scenarioIds: ["target-30m-3"], targetColumns: ["principal", "tenor", "monthly_installment"], renderInstruction: "Jumlah Pinjaman Rp30.000.000; Periode Cicilan 3 Bulan; Cicilan per Bulan Rp10.270.000" }],
      productionPrompt: "Use the original prompt.",
    };
    const draft = {
      ...orderDraftWithPreAdaptation(undefined, adaptation.productionPrompt),
      productionPrompt: "Use the edited production prompt.",
      textOverrides: { "top-left": "Rp9.000.000" },
      replacementSources: { "top-left": { kind: "manual" as const } },
    };
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, adaptation, draft, "analysis-1");

    expect(draft.productionPrompt).toBe("Use the edited production prompt.");
    expect(compiledProductionPrompt(adaptation, draft.textOverrides, `${draft.productionPrompt}\nApproved repayment rows: Jumlah Pinjaman Rp30.000.000`)).toContain("Use the edited production prompt.");
    expect(compiledProductionPrompt(adaptation, draft.textOverrides, `${draft.productionPrompt}\nApproved repayment rows: Jumlah Pinjaman Rp30.000.000`)).not.toContain("Approved repayment rows");
    expect(frozen.pre_adaptation?.production_prompt).toContain("Use the edited production prompt.");
    expect(frozen.pre_adaptation?.text_replacements[0]).toMatchObject({ replacement_text: "Rp9.000.000", source_kind: "manual" });
    expect(frozen.pre_adaptation?.production_prompt).toContain("左上还款卡（金额/期限）：Rp9.000.000");
    expect(frozen.pre_adaptation?.production_prompt).toContain("Jumlah Pinjaman Rp30.000.000");
    expect(frozen.pre_adaptation?.numeric_layouts[0]).toMatchObject({ layout_kind: "table", scenario_ids: ["target-30m-3"] });
  });

  it("switches numeric layouts to another approved repayment plan as one frozen choice", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed",
      summary: "ready",
      decision: "",
      preferredFragmentKeys: [],
      reasons: [],
      gaps: [],
      analysisHighlights: [],
      textReplacements: [],
      repaymentPlanSelections: [{ id: "row-1", planKey: "5m-3", principal: 5_000_000, tenorMonths: 3, values: { principal: "Rp5.000.000", tenor: "3 Bulan", totalInterest: "Rp135.001", totalRepayment: "Rp5.135.001", monthlyInstallment: "Rp1.711.667" } }],
      numericLayouts: [{ id: "summary-row", sourceBlockIds: ["amount"], location: "下部摘要表", layoutKind: "table_row", scenarioIds: ["row-1"], targetColumns: ["principal", "tenor", "total_repayment"], renderInstruction: "展示 Rp5.000.000、3 Bulan、Rp5.135.001。" }],
      productionPrompt: "保留原表格结构。",
    };
    const repaymentPlanOverrides = {
      "row-1": { planKey: "8m-6", principal: 8_000_000, tenorMonths: 6, values: { principal: "Rp8.000.000", tenor: "6 Bulan", totalInterest: "Rp431.998", totalRepayment: "Rp8.431.998", monthlyInstallment: "Rp1.405.333" } },
    };
    const draft = { ...orderDraftWithPreAdaptation(undefined, adaptation), repaymentPlanOverrides };
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, adaptation, draft, "analysis-1");
    const prompt = compiledProductionPrompt(adaptation, {}, draft.productionPrompt, repaymentPlanOverrides);

    expect(effectiveRepaymentPlanSelections(adaptation, repaymentPlanOverrides)[0]).toMatchObject({ planKey: "8m-6", principal: 8_000_000, tenorMonths: 6 });
    expect(prompt).toContain("Rp8.000.000");
    expect(prompt).toContain("Rp8.431.998");
    expect(prompt).not.toContain("Rp5.000.000");
    expect(frozen.pre_adaptation?.repayment_plan_selections[0]).toMatchObject({ plan_key: "8m-6", principal: 8_000_000, tenor_months: 6 });
    expect(frozen.pre_adaptation?.numeric_layouts[0]?.render_instruction).toContain("Rp8.431.998");
    expect(frozen.pre_adaptation?.numeric_layouts[0]?.render_instruction).not.toContain("Rp5.135.001");
  });

  it("freezes deleted and added repayment rows from the order draft", () => {
    const adaptation: PreparedPreAdaptation = {
      status: "completed",
      summary: "ready",
      decision: "",
      preferredFragmentKeys: [],
      reasons: [],
      gaps: [],
      analysisHighlights: [],
      textReplacements: [],
      repaymentPlanSelections: [
        { id: "row-4m-6", planKey: "4m-6", principal: 4_000_000, tenorMonths: 6, values: { principal: "Rp4.000.000", tenor: "6 Bulan", totalInterest: "Rp216.002", totalRepayment: "Rp4.216.002", monthlyInstallment: "Rp702.667" } },
        { id: "row-5m-6", planKey: "5m-6", principal: 5_000_000, tenorMonths: 6, values: { principal: "Rp5.000.000", tenor: "6 Bulan", totalInterest: "Rp269.998", totalRepayment: "Rp5.269.998", monthlyInstallment: "Rp878.333" } },
      ],
      numericLayouts: [{ id: "six-month-column", sourceBlockIds: ["b6"], location: "6 个月月供列", layoutKind: "table", scenarioIds: ["row-4m-6", "row-5m-6"], targetColumns: ["principal", "tenor", "monthly_installment"], renderInstruction: "展示两行 6 个月月供。" }],
      productionPrompt: "保留原表格结构。",
    };
    const addedPlan = { planKey: "8m-12", principal: 8_000_000, tenorMonths: 12, values: { principal: "Rp8.000.000", tenor: "12 Bulan", totalInterest: "Rp864.004", totalRepayment: "Rp8.864.004", monthlyInstallment: "Rp738.667" } };
    const numericLayoutDrafts = {
      "six-month-column": {
        removedScenarioIds: ["row-4m-6"],
        addedScenarios: { "added-six-month-column-8m-12": addedPlan },
      },
    };
    const draft = { ...orderDraftWithPreAdaptation(undefined, adaptation), numericLayoutDrafts };
    const prompt = compiledProductionPrompt(adaptation, {}, draft.productionPrompt, {}, numericLayoutDrafts);
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, adaptation, draft, "analysis-1");

    expect(effectiveNumericLayouts(adaptation, numericLayoutDrafts)[0]?.scenarioIds).toEqual(["row-5m-6", "added-six-month-column-8m-12"]);
    expect(effectiveRepaymentPlanSelections(adaptation, {}, numericLayoutDrafts).map((selection) => selection.planKey)).toEqual(["5m-6", "8m-12"]);
    expect(prompt).toContain("Rp8.000.000");
    expect(prompt).not.toContain("Rp4.000.000");
    expect(frozen.pre_adaptation?.numeric_layouts[0]?.scenario_ids).toEqual(["row-5m-6", "added-six-month-column-8m-12"]);
    expect(frozen.pre_adaptation?.repayment_plan_selections.map((selection) => selection.plan_key)).toEqual(["5m-6", "8m-12"]);
  });

  it("keeps the overall generation requirement editable while hiding audit-only repayment rows", () => {
    const prompt = "保留绿色主题，可增强球赛氛围。\nApproved repayment rows: Jumlah Pinjaman: Rp4.000.000";

    expect(userFacingProductionPrompt(prompt)).toBe("保留绿色主题，可增强球赛氛围。");
    expect(orderDraftWithPreAdaptation(undefined, prompt).productionPrompt).toBe("保留绿色主题，可增强球赛氛围。");
  });

  it("does not treat a legacy generic adaptation as a completed text plan", () => {
    const adaptation = preparedPreAdaptation({
      result: {
        adaptation: {
          status: "completed",
          summary: "generic copy recommendation",
          result: {
            market_pack_id: "market-1", market_pack_version: 2,
            copy_library_id: "library-1", copy_library_version: 3,
            preferred_fragment_keys: ["headline_generic"],
          },
        },
      },
    } as any, { id: "market-1", published_version: 2 }, { id: "library-1", published_version: 3 });

    expect(adaptation).toBeNull();
  });

  it("distinguishes a missing adaptation from a saved unavailable result", () => {
    expect(sourceAnalysisAdaptationStatus({ result: {} } as any)).toBe("");
    expect(sourceAnalysisAdaptationStatus({ result: { adaptation: { status: "unavailable" } } } as any)).toBe("unavailable");
    expect(sourceAnalysisAdaptationErrorCode({ result: { adaptation: { error_code: "manual_confirmation_required" } } } as any)).toBe("manual_confirmation_required");
  });

  it("keeps the inferred creative type when the user manually adjusts copy", () => {
    const snapshot = manualCopySnapshot({
      mode: "manual",
      direction: "",
      textOverrides: {},
      manualHeadline: "Pilih tenor",
      manualSubheadline: "",
      manualBenefit: "Cicilan sesuai kebutuhan",
      manualSupporting: "",
      manualCta: "Ajukan sekarang",
    }, "repayment_plan");

    expect(snapshot).toMatchObject({ creative_type: "repayment_plan", status: "user_custom" });
  });
});
