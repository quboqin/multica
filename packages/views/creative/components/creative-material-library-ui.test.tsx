// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { describe, expect, it, vi } from "vitest";
import type { CreativeMaterialCandidate, CreativeVisualDirection } from "@multica/core/types";
import {
  adoptedGalleryComplete,
  creativeMaterialProductionState,
  creativeMaterialSelectionActionLabel,
  canRetryMaterialAnalysis,
  effectiveNumericLayouts,
  effectiveRepaymentPlanSelections,
  frozenCopySnapshot,
  hasPendingReplacementConfirmation,
  manualRepaymentPlanChoice,
  manualCopySnapshot,
  orderDraftWithPreAdaptation,
  pendingNumericLayouts,
  preparedPreAdaptation,
  resolveDirectEditMarketPack,
  sourceAnalysisHasNoEditableCopy,
  submitCreativeDirectEdit,
  visualDirectionFromAnalysis,
  visualDirectionSummary,
  type OrderItemDraft,
  type PreparedPreAdaptation,
} from "./creative-material-library";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

const direction: CreativeVisualDirection = {
  schema_version: 1,
  theme: "灵活融资",
  style_tags: ["红黑高对比", "信息型"],
  must_preserve: ["原图阅读顺序", "主体层次"],
  avoid: ["底部遮挡", "竞品品牌识别"],
};

function adaptation(overrides: Partial<PreparedPreAdaptation> = {}): PreparedPreAdaptation {
  return {
    status: "completed",
    summary: "ready",
    decision: "",
    preferredFragmentKeys: [],
    reasons: [],
    gaps: [],
    analysisHighlights: ["信息层级明确", "主体关系清楚", "底部需要留出组件空间"],
    analysisResult: { theme: direction.theme, visual_type: "信息型", palette_anchors: direction.style_tags, must_preserve: direction.must_preserve, avoid: direction.avoid },
    textReplacements: [{ blockId: "headline", location: "顶部标题", role: "headline", sourceText: "Flexible", replacementText: "Pembiayaan Fleksibel", sourceKeys: ["headline"], status: "ready", note: "" }],
    repaymentPlanSelections: [{ id: "row-1", planKey: "5m-3", principal: 5_000_000, tenorMonths: 3, values: { principal: "Rp5.000.000", tenor: "3 Bulan", totalInterest: "Rp135.001", totalRepayment: "Rp5.135.001", monthlyInstallment: "Rp1.711.667" } }],
    numericLayouts: [{ id: "table", sourceBlockIds: ["amount"], location: "下部还款表", layoutKind: "table", scenarioIds: ["row-1"], targetColumns: ["principal", "tenor", "monthly_installment"], renderInstruction: "展示冻结还款方案并保持表格层级。" }],
    ...overrides,
  };
}

function draft(overrides: Partial<OrderItemDraft> = {}): OrderItemDraft {
  return {
    mode: "pre_adaptation",
    visualDirection: direction,
    textOverrides: {},
    replacementSources: {},
    repaymentPlanOverrides: {},
    numericLayoutDrafts: {},
    manualHeadline: "",
    manualSubheadline: "",
    manualBenefit: "",
    manualSupporting: "",
    manualCta: "",
    ...overrides,
  };
}

describe("CreativeMaterialLibrary contracts", () => {
  it("resolves exactly one published default market pack for formal direct edits", () => {
    const marketPack = (id: string) => ({
      id,
      name: `市场配置 ${id}`,
      kind: "market_pack",
      published_version: 3,
      published_config: { pre_adaptation_default: true },
    });

    expect(resolveDirectEditMarketPack([], false)).toMatchObject({ status: "missing" });
    expect(resolveDirectEditMarketPack([marketPack("one"), marketPack("two")] as never, false)).toMatchObject({ status: "ambiguous" });
    expect(resolveDirectEditMarketPack([marketPack("one")] as never, false)).toMatchObject({
      status: "ready",
      marketPack: { id: "one", published_version: 3 },
    });
  });

  it("freezes the formal direct edit before assigning the squad", async () => {
    const calls: string[] = [];
    const recoveries: unknown[] = [];
    const apiClient = {
      createIssue: vi.fn(async () => { calls.push("createIssue"); return { id: "issue-1" }; }),
      updateIssue: vi.fn(async () => { calls.push("updateIssue"); return { id: "issue-1" }; }),
      putCreativeIssueContext: vi.fn(async () => { calls.push("putCreativeIssueContext"); return {}; }),
      createCreativeDirectEdit: vi.fn(async () => { calls.push("createCreativeDirectEdit"); return { order: { id: "order-1" } }; }),
      setIssueMetadataKey: vi.fn(async () => { calls.push("setIssueMetadataKey"); return { metadata: {} }; }),
    };

    const result = await submitCreativeDirectEdit(apiClient as never, {
      candidate: { id: "candidate-1", title: "参考素材", competitor: "", source_attachment_id: "attachment-1" },
      instruction: "只调整标题位置",
      targetSize: "1080x1080",
      deliveryMode: "publish",
      squadId: "squad-1",
      submissionKey: "submission-1",
      marketPackId: "market-pack-1",
      recovery: { issueId: "", orderId: "", submissionKey: "", marketPackId: "" },
      onRecovery: (recovery) => recoveries.push(recovery),
    });

    expect(result).toEqual({ issueId: "issue-1", orderId: "order-1" });
    expect(calls).toEqual(["createIssue", "putCreativeIssueContext", "createCreativeDirectEdit", "setIssueMetadataKey", "updateIssue"]);
    expect(apiClient.updateIssue).toHaveBeenCalledWith("issue-1", { assignee_type: "squad", assignee_id: "squad-1" });
    expect(apiClient.putCreativeIssueContext).toHaveBeenCalledWith("issue-1", { market_pack_id: "market-pack-1", squad_id: "squad-1" });
    expect(recoveries).toEqual([
      { issueId: "issue-1", orderId: "", submissionKey: "submission-1", marketPackId: "market-pack-1" },
      { issueId: "issue-1", orderId: "order-1", submissionKey: "submission-1", marketPackId: "market-pack-1" },
    ]);
  });

  it("blocks a formal direct edit without a unique market pack before creating an issue", async () => {
    const apiClient = {
      createIssue: vi.fn(),
      updateIssue: vi.fn(),
      putCreativeIssueContext: vi.fn(),
      createCreativeDirectEdit: vi.fn(),
      setIssueMetadataKey: vi.fn(),
    };

    await expect(submitCreativeDirectEdit(apiClient as never, {
      candidate: { id: "candidate-1", title: "参考素材", competitor: "", source_attachment_id: "attachment-1" },
      instruction: "只调整标题位置",
      targetSize: "1080x1080",
      deliveryMode: "publish",
      squadId: "squad-1",
      submissionKey: "submission-1",
      marketPackId: "",
      recovery: { issueId: "", orderId: "", submissionKey: "", marketPackId: "" },
      onRecovery: vi.fn(),
    })).rejects.toThrow("正式投放必须绑定唯一的已发布默认市场配置");
    expect(apiClient.createIssue).not.toHaveBeenCalled();
  });

  it("keeps preview direct edits independent from market-pack configuration", async () => {
    const apiClient = {
      createIssue: vi.fn(async () => ({ id: "issue-1" })),
      updateIssue: vi.fn(async () => ({ id: "issue-1" })),
      putCreativeIssueContext: vi.fn(),
      createCreativeDirectEdit: vi.fn(async () => ({ order: { id: "order-1" } })),
      setIssueMetadataKey: vi.fn(async () => ({ metadata: {} })),
    };

    await expect(submitCreativeDirectEdit(apiClient as never, {
      candidate: { id: "candidate-1", title: "参考素材", competitor: "", source_attachment_id: "attachment-1" },
      instruction: "只调整标题位置",
      targetSize: "1080x1080",
      deliveryMode: "preview",
      squadId: "squad-1",
      submissionKey: "submission-1",
      marketPackId: "",
      recovery: { issueId: "", orderId: "", submissionKey: "", marketPackId: "" },
      onRecovery: vi.fn(),
    })).resolves.toEqual({ issueId: "issue-1", orderId: "order-1" });
    expect(apiClient.putCreativeIssueContext).not.toHaveBeenCalled();
  });

  it("recovers a created direct-edit order without repeating earlier submission steps", async () => {
    const apiClient = {
      createIssue: vi.fn(),
      updateIssue: vi.fn(),
      putCreativeIssueContext: vi.fn(),
      createCreativeDirectEdit: vi.fn(),
      setIssueMetadataKey: vi.fn(async () => ({ metadata: {} })),
    };

    await expect(submitCreativeDirectEdit(apiClient as never, {
      candidate: { id: "candidate-1", title: "参考素材", competitor: "", source_attachment_id: "attachment-1" },
      instruction: "只调整标题位置",
      targetSize: "1080x1080",
      deliveryMode: "publish",
      squadId: "squad-1",
      submissionKey: "submission-1",
      marketPackId: "market-pack-1",
      recovery: { issueId: "issue-1", orderId: "order-1", submissionKey: "submission-1", marketPackId: "market-pack-1" },
      onRecovery: vi.fn(),
    })).resolves.toEqual({ issueId: "issue-1", orderId: "order-1" });
    expect(apiClient.createIssue).not.toHaveBeenCalled();
    expect(apiClient.updateIssue).toHaveBeenCalledWith("issue-1", { assignee_type: "squad", assignee_id: "squad-1" });
    expect(apiClient.putCreativeIssueContext).not.toHaveBeenCalled();
    expect(apiClient.createCreativeDirectEdit).not.toHaveBeenCalled();
    expect(apiClient.setIssueMetadataKey).toHaveBeenCalledWith("issue-1", "creative_order_id", "order-1");
  });

  it("treats a complete single-size precise edit as a downloadable adopted package", () => {
    const attachmentID = "precise-square-attachment";
    const variant = {
      id: "precise-variant",
      variant_key: "direct_edit",
      revision: 2,
      active_revision: 2,
      staging_revision: 0,
      candidate_state: "selected",
      revisions: [{ revision: 2, expected_sizes: ["1080x1080"] }],
      assets: [{
        id: "precise-square-delivered",
        variant_id: "precise-variant",
        size_key: "1080x1080",
        revision: 2,
        stage: "delivered",
        status: "completed",
        attachment_id: attachmentID,
      }],
    };
    const item = { id: "precise-item", candidate_id: "candidate-1", copy_snapshot: {}, variants: [variant] };
    const galleryItem = {
      id: "precise-gallery",
      label: "精准改图",
      order: { id: "precise-order", input_snapshot: {} },
      item,
      variant,
      adoptedAt: "2026-08-27T00:00:00Z",
    };
    const attachments = new Map([[attachmentID, { id: attachmentID, filename: "precise.png", url: "https://cdn.example/precise.png", download_url: "https://cdn.example/precise.png" }]]);

    expect(adoptedGalleryComplete(galleryItem as never, attachments as never)).toBe(true);
    expect(adoptedGalleryComplete(galleryItem as never, new Map())).toBe(false);
  });

  it("derives a small editable visual direction from source analysis", () => {
    expect(visualDirectionFromAnalysis({
      theme: "灵活融资",
      visual_type: "信息型",
      palette_anchors: ["红黑高对比", "现代", "红黑高对比", "过量"],
      must_preserve: ["阅读顺序", "主体层次", "第三项", "第四项", "第五项"],
      avoid: ["遮挡", "竞品品牌"],
    })).toEqual({
      schema_version: 1,
      theme: "灵活融资",
      style_tags: ["信息型", "红黑高对比", "现代"],
      must_preserve: ["阅读顺序", "主体层次", "第三项", "第四项"],
      avoid: ["遮挡", "竞品品牌"],
    });
  });

  it("keeps app UI phone anchors out of the default visual direction", () => {
    expect(visualDirectionFromAnalysis({
      theme: "移动端借款",
      visual_type: "信息型",
      app_ui_detected: true,
      app_ui_replacement_needed: true,
      must_preserve: ["借款促销核心语义", "左文右人和手机展示的阅读顺序", "移动端申请场景", "绿色视觉层级"],
      visual_anchors: ["右侧人物的手持手机姿势", "左文右人构图"],
    }).must_preserve).toEqual(["借款促销核心语义", "绿色视觉层级", "左文右人构图"]);
  });

  it("preserves user visual edits when the adaptation refreshes", () => {
    const initial = orderDraftWithPreAdaptation(undefined, adaptation());
    const edited = orderDraftWithPreAdaptation({ ...initial, visualDirection: { ...direction, theme: "用户主题" } }, adaptation({ analysisResult: { theme: "系统主题" } }));
    expect(initial.visualDirection.theme).toBe("灵活融资");
    expect(edited.visualDirection.theme).toBe("用户主题");
  });

  it("freezes structured direction with copy and numeric layout, never a page prompt", () => {
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, adaptation(), draft(), "analysis-1");
    expect(frozen.visual_direction).toEqual(direction);
    expect(frozen.pre_adaptation?.text_replacements[0]).toMatchObject({ replacement_text: "Pembiayaan Fleksibel", status: "ready" });
    expect(frozen.pre_adaptation?.numeric_layouts[0]).toMatchObject({ scenario_ids: ["row-1"], layout_kind: "table" });
    expect("production_prompt" in (frozen.pre_adaptation ?? {})).toBe(false);
  });

  it("freezes the user selected app UI reference for planning", () => {
    const selected = { resourceFileId: "file-1", attachmentId: "attachment-1", label: "AdaKami 首页", reason: "业务确认使用首页额度界面" };
    const frozen = frozenCopySnapshot(
      { headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any,
      adaptation({
        analysisResult: {
          theme: direction.theme,
          app_ui_detected: true,
          app_ui_replacement_needed: true,
          app_ui_type: "贷款额度页",
          app_ui_visual_characteristics: "竖向手机屏幕大部分可见",
          app_ui_bounds: { x: 600, y: 300, width: 220, height: 420 },
        },
      }),
      draft({ appUIReference: selected }),
      "analysis-1",
    );
    expect(frozen.pre_adaptation?.app_ui_replacement).toMatchObject({
      required: true,
      selected: true,
      resource_file_id: "file-1",
      attachment_id: "attachment-1",
      reason: "业务确认使用首页额度界面",
      source_screen: { app_ui_type: "贷款额度页", bounds: { x: 600, y: 300, width: 220, height: 420 } },
    });
  });

  it("keeps manual copy under the same visual direction contract", () => {
    const snapshot = manualCopySnapshot(draft({ mode: "manual", manualHeadline: "Pilih tenor", manualBenefit: "Cicilan sesuai kebutuhan", manualCta: "Ajukan sekarang" }), "repayment_plan");
    expect(snapshot).toMatchObject({ creative_type: "repayment_plan", status: "user_custom", visual_direction: direction });
  });

  it("keeps generated recommendations pending until the user confirms them", () => {
    const recommendation = {
      blockId: "headline",
      location: "顶部标题",
      role: "headline",
      sourceText: "Pinjaman Cepat",
      replacementText: "Solusi finansial untuk kebutuhanmu",
      sourceKeys: [],
      status: "recommended" as const,
      note: "平台生成候选",
      recommendationBasis: ["没有兼容的已审核片段"],
    };
    expect(hasPendingReplacementConfirmation(recommendation)).toBe(true);
    expect(hasPendingReplacementConfirmation(recommendation, {}, { headline: { kind: "recommendation" } })).toBe(false);
    expect(hasPendingReplacementConfirmation(recommendation, { headline: "用户改写" }, { headline: { kind: "manual" } })).toBe(false);
  });

  it("summarizes only the structured visual direction for the legacy direction column", () => {
    expect(visualDirectionSummary(direction)).toBe("主题：灵活融资\n风格：红黑高对比、信息型\n保留：原图阅读顺序、主体层次\n避免：底部遮挡、竞品品牌识别");
  });

  it("switches a numeric layout as one approved repayment choice", () => {
    const replacement = { planKey: "8m-6", principal: 8_000_000, tenorMonths: 6, values: { principal: "Rp8.000.000", tenor: "6 Bulan", totalInterest: "Rp431.998", totalRepayment: "Rp8.431.998", monthlyInstallment: "Rp1.405.333" } };
    const updated = effectiveRepaymentPlanSelections(adaptation(), { "row-1": replacement });
    expect(updated[0]).toMatchObject({ planKey: "8m-6", principal: 8_000_000 });
    expect(effectiveNumericLayouts(adaptation(), { table: { removedScenarioIds: [], addedScenarios: {} } })[0]?.scenarioIds).toEqual(["row-1"]);
  });

  it("restores a repayment-plan selector for missing numeric regions without copying source values", () => {
    const missingNumeric = adaptation({
      visualRegions: [
        { id: "principal-region", location: "中部本金列", kind: "numeric", sourceBlockIds: ["principal-1", "principal-2"], visualBounds: { x: 50, y: 250, width: 200, height: 400 } },
        { id: "monthly-region", location: "中部月供列", kind: "numeric", sourceBlockIds: ["monthly-1", "monthly-2"], visualBounds: { x: 300, y: 250, width: 200, height: 400 } },
      ],
      textReplacements: [
        { blockId: "principal-1", visualRegionId: "principal-region", location: "中部本金列", role: "plan_field", semanticKind: "principal", sourceText: "Rp1.000.000", replacementText: "", sourceKeys: [], status: "missing", note: "" },
        { blockId: "principal-2", visualRegionId: "principal-region", location: "中部本金列", role: "plan_field", semanticKind: "principal", sourceText: "Rp3.000.000", replacementText: "", sourceKeys: [], status: "missing", note: "" },
        { blockId: "monthly-1", visualRegionId: "monthly-region", location: "中部月供列", role: "plan_field", semanticKind: "monthly_installment", sourceText: "Rp196.667", replacementText: "", sourceKeys: [], status: "missing", note: "" },
        { blockId: "monthly-2", visualRegionId: "monthly-region", location: "中部月供列", role: "plan_field", semanticKind: "monthly_installment", sourceText: "Rp590.000", replacementText: "", sourceKeys: [], status: "missing", note: "" },
      ],
      repaymentPlanSelections: [],
      numericLayouts: [],
    });
    const layouts = pendingNumericLayouts(missingNumeric);
    expect(layouts).toHaveLength(2);
    expect(layouts[0]).toMatchObject({ id: "pending-numeric:principal-region", sourceBlockIds: ["principal-1", "principal-2"], targetColumns: ["principal"], scenarioIds: [] });
    expect(layouts[1]).toMatchObject({ id: "pending-numeric:monthly-region", sourceBlockIds: ["monthly-1", "monthly-2"], targetColumns: ["monthly_installment"], scenarioIds: [] });

    const selectedPlan = { planKey: "8m-6", principal: 8_000_000, tenorMonths: 6, values: { principal: "Rp8.000.000", tenor: "6 Bulan", totalInterest: "Rp431.998", totalRepayment: "Rp8.431.998", monthlyInstallment: "Rp1.405.333" } };
    const selected = effectiveNumericLayouts(missingNumeric, {
      "pending-numeric:principal-region": { addedScenarios: { "pending-row": selectedPlan } },
    });
    expect(selected.find((layout) => layout.id === "pending-numeric:principal-region")?.scenarioIds).toEqual(["pending-row"]);
    expect(effectiveRepaymentPlanSelections(missingNumeric, {}, {
      "pending-numeric:principal-region": { addedScenarios: { "pending-row": selectedPlan } },
    })).toMatchObject([{ id: "pending-row", planKey: "8m-6", principal: 8_000_000 }]);
  });

  it("freezes selected pending numeric blocks as a layout instead of duplicate text replacements", () => {
    const missingNumeric = adaptation({
      visualRegions: [{ id: "principal-region", location: "中部本金列", kind: "numeric", sourceBlockIds: ["principal-1"], visualBounds: { x: 50, y: 250, width: 200, height: 120 } }],
      textReplacements: [{ blockId: "principal-1", visualRegionId: "principal-region", location: "中部本金列", role: "plan_field", semanticKind: "principal", sourceText: "Rp1.000.000", replacementText: "", sourceKeys: [], status: "missing", note: "" }],
      repaymentPlanSelections: [],
      numericLayouts: [],
    });
    const plan = { planKey: "8m-6", principal: 8_000_000, tenorMonths: 6, values: { principal: "Rp8.000.000", tenor: "6 Bulan", totalInterest: "Rp431.998", totalRepayment: "Rp8.431.998", monthlyInstallment: "Rp1.405.333" } };
    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, missingNumeric, draft({ numericLayoutDrafts: { "pending-numeric:principal-region": { addedScenarios: { "pending-row": plan } } } }), "analysis-1");
    expect(frozen.pre_adaptation?.text_replacements).toEqual([]);
    expect(frozen.pre_adaptation?.repayment_plan_selections).toMatchObject([{ id: "pending-row", plan_key: "8m-6" }]);
    expect(frozen.pre_adaptation?.numeric_layouts).toMatchObject([{ id: "pending-numeric:principal-region", source_block_ids: ["principal-1"], scenario_ids: ["pending-row"], target_columns: ["principal"] }]);
    expect(frozen.pre_adaptation?.numeric_layouts[0]?.render_instruction).toContain("Rp8.000.000");
  });

  it("freezes a manually entered single principal when a benefit amount was misclassified as repayment", () => {
    const missingNumeric = adaptation({
      visualRegions: [{ id: "hero-principal-region", location: "中右黄色额度徽章本金区", kind: "numeric", sourceBlockIds: ["hero-max-principal"], visualBounds: { x: 488, y: 367, width: 419, height: 115 } }],
      textReplacements: [{ blockId: "hero-max-principal", visualRegionId: "hero-principal-region", location: "中右黄色额度徽章主体", role: "plan_field", semanticKind: "principal", sourceText: "Rp100 Juta", replacementText: "", sourceKeys: [], status: "missing", note: "" }],
      repaymentPlanSelections: [],
      numericLayouts: [],
    });
    const layout = pendingNumericLayouts(missingNumeric)[0]!;
    const manualPlan = manualRepaymentPlanChoice(layout.id, { principal: "Rp100 Juta" });

    expect(manualPlan).toMatchObject({ principal: 100_000_000, tenorMonths: 0, values: { principal: "Rp100 Juta", tenor: "", monthlyInstallment: "" } });

    const frozen = frozenCopySnapshot({ headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal_text: "", fragments: [], repayment_plan_entries: [] } as any, missingNumeric, draft({ numericLayoutDrafts: { [layout.id]: { addedScenarios: { "manual-hero-principal": manualPlan } } } }), "analysis-1");
    expect(frozen.pre_adaptation?.text_replacements).toEqual([]);
    expect(frozen.pre_adaptation?.repayment_plan_selections).toMatchObject([{ id: "manual-hero-principal", plan_key: expect.stringContaining("manual-"), principal: 100_000_000, values: { principal: "Rp100 Juta" } }]);
    expect(frozen.pre_adaptation?.numeric_layouts).toMatchObject([{ id: "pending-numeric:hero-principal-region", source_block_ids: ["hero-max-principal"], scenario_ids: ["manual-hero-principal"], target_columns: ["principal"] }]);
    expect(frozen.pre_adaptation?.numeric_layouts[0]?.render_instruction).toContain("借款金额 Rp100 Juta");
  });

  it("does not mark a generic adaptation as ready", () => {
    expect(preparedPreAdaptation({ result: { adaptation: { status: "completed", summary: "generic", result: { market_pack_id: "market-1", market_pack_version: 1, copy_library_id: "library-1", copy_library_version: 1 } } } } as any, { id: "market-1" }, { id: "library-1" })).toBeNull();
  });

  it("keeps the latest completed adaptation usable across resource version changes", () => {
    const result = preparedPreAdaptation({
      result: {
        adaptation: {
          status: "completed",
          summary: "ready",
          result: {
            market_pack_id: "market-1",
            market_pack_version: 1,
            copy_library_id: "library-1",
            copy_library_version: 1,
            text_replacements: [{ block_id: "headline", location: "标题", role: "headline", source_text: "Cash", replacement_text: "Pembiayaan Fleksibel", source_keys: ["headline"], status: "ready" }],
          },
        },
      },
    } as any, { id: "market-1" }, { id: "library-1" });

    expect(result).toMatchObject({ status: "completed", textReplacements: [{ blockId: "headline", replacementText: "Pembiayaan Fleksibel" }] });
  });

  it("keeps a material with a failed adaptation visibly blocked", () => {
    const candidate = { asset_type: "image", archived_url: "https://cdn.example/source.png", archive_status: "completed", status: "active" } as unknown as CreativeMaterialCandidate;
    expect(creativeMaterialProductionState(candidate, { ready: true, status: "completed", error: "", version: 1 }, undefined, false, { ready: false, active: false, failed: true, reason: "Invalid numeric layout" })).toMatchObject({ status: "failed", label: "适配失败" });
  });

  it("keeps reanalysis available for archived images while waiting or already available", () => {
    const candidate = { asset_type: "image", archived_url: "https://cdn.example/source.png" } as CreativeMaterialCandidate;
    expect(canRetryMaterialAnalysis(candidate, { status: "analyzing" })).toBe(true);
    expect(canRetryMaterialAnalysis(candidate, { status: "available" })).toBe(true);
    expect(canRetryMaterialAnalysis(candidate, { status: "failed" })).toBe(true);
    expect(canRetryMaterialAnalysis(candidate, { status: "generated" })).toBe(true);
    expect(canRetryMaterialAnalysis(candidate, { status: "rejected" })).toBe(false);
    expect(canRetryMaterialAnalysis(candidate, { status: "unsupported" })).toBe(false);
    expect(canRetryMaterialAnalysis({ asset_type: "video", archived_url: "https://cdn.example/source.mp4" } as CreativeMaterialCandidate, { status: "available" })).toBe(false);
    expect(canRetryMaterialAnalysis({ asset_type: "image", archived_url: "" } as CreativeMaterialCandidate, { status: "available" })).toBe(false);
  });

  it("keeps generated production state separate from analysis while allowing reselection", () => {
    const candidate = { asset_type: "image", archived_url: "https://cdn.example/source.png" } as CreativeMaterialCandidate;
    const generated = { status: "generated", label: "已生成", selectable: true, active: false } as const;
    const analyzing = { status: "analyzing", label: "分析中", selectable: false, active: true } as const;
    expect(creativeMaterialProductionState(candidate, { ready: true, status: "completed", error: "", version: 1 }, undefined, true, { ready: true, active: false, failed: false })).toMatchObject(generated);
    expect(creativeMaterialSelectionActionLabel(false, generated)).toBe("选择出图素材");
    expect(creativeMaterialSelectionActionLabel(false, analyzing)).toBe("等待分析完成");
    expect(creativeMaterialSelectionActionLabel(true, generated)).toBe("取消选择");
  });

  it("treats a static source without editable text as unavailable instead of retryable", () => {
    const candidate = { asset_type: "image", archived_url: "https://cdn.example/source.png", archive_status: "completed", status: "active" } as unknown as CreativeMaterialCandidate;
    const analysis = { status: "completed", result: { text_blocks: [] } } as any;
    expect(sourceAnalysisHasNoEditableCopy(analysis)).toBe(true);
    expect(creativeMaterialProductionState(candidate, { ready: true, status: "completed", error: "", version: 1 }, undefined, false, { ready: false, active: false, failed: false, unavailable: true, reason: "no editable copy" })).toMatchObject({ status: "unsupported", label: "无可配置文案", selectable: false, active: false });
  });
});
