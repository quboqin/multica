// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { CreativeGenerationInfoDialog, creativeAssetLineage, creativeGenerationInfo } from "./creative-generation-info-dialog";

afterEach(cleanup);

function asset(input: Partial<CreativeOrderAsset> & Pick<CreativeOrderAsset, "id" | "stage">): CreativeOrderAsset {
  const { id, stage, ...overrides } = input;
  return {
    id,
    variant_id: "variant-1",
    asset_family_id: "family-1",
    size_key: "1080x1080",
    revision: 2,
    stage,
    attachment_id: `${input.id}-attachment`,
    derived_from_asset_id: "",
    metadata: {},
    evidence: {},
    status: "completed",
    created_at: "2026-08-05T10:00:00Z",
    updated_at: "2026-08-05T10:00:00Z",
    ...overrides,
  };
}

function fixture() {
  const generated = asset({
    id: "generated-1",
    stage: "generated",
    metadata: { model: "gpt-image-2", provider: "OpenAI", prompt: "完整提示词：保留蓝绿色信息卡片，并使用已审核文案。\nApproved repayment rows: Jumlah Pinjaman Rp80.000.000" },
    evidence: { request_id: "request-1234567890", attempts: 2, prompt_sha256: "cli-returned-prompt-hash" },
  });
  const primed = asset({
    id: "primed-1",
    stage: "primed",
    derived_from_asset_id: generated.id,
    metadata: { market_pack_name: "AdaKami Indonesia", market_pack_version: 12 },
    evidence: { package_contract_version: 1 },
  });
  const delivered = asset({ id: "delivered-1", stage: "delivered", derived_from_asset_id: primed.id, updated_at: "2026-08-05T12:30:00Z" });
  const variant = {
    id: "variant-1",
    variant_key: "V01",
    revision: 2,
    brief: { prime_layout_contract: { version: 2, hard_regions: [{ id: "logo" }], backdrop_rule: "quiet" } },
    assets: [generated, primed, delivered],
    qc_reports: [],
  } as unknown as CreativeOrderVariant;
  const item = {
    id: "item-1",
    direction: "保留原图信息机制，突出额度利益点",
    copy_snapshot: {
      headline: "Pinjaman Fleksibel Tanpa Ribet",
      benefit: "Limit hingga Rp80.000.000",
      cta: "AJUKAN SEKARANG",
      pre_adaptation: { repayment_plan_selections: [{ values: { principal: "Rp80.000.000", tenor: "6 Bulan", monthly_installment: "Rp14.000.000" } }] },
    },
    variants: [variant],
  } as unknown as CreativeOrderItem;
  return { generated, primed, delivered, variant, item };
}

describe("creative generation information", () => {
  it("resolves delivered, Prime, and generated records into business information", () => {
    const { delivered, variant, item } = fixture();
    const info = creativeGenerationInfo(item, variant, delivered);

    expect(info.lineage.map((entry) => entry.stage)).toEqual(["generated", "primed", "delivered"]);
    expect(info).toMatchObject({
      direction: "保留原图信息机制，突出额度利益点",
      model: "gpt-image-2",
      provider: "OpenAI",
      marketResource: "AdaKami Indonesia",
      marketRule: "Prime 布局合同 v2 · 1 个保护区 · 背景规则 quiet",
      prompt: "完整提示词：保留蓝绿色信息卡片，并使用已审核文案。\nApproved repayment rows: Jumlah Pinjaman Rp80.000.000",
      attempts: "2",
    });
    expect(info.copyLines).toContainEqual({ label: "主标题", value: "Pinjaman Fleksibel Tanpa Ribet" });
    expect(info.repaymentPlans).toEqual([{ label: "Rp80.000.000 / 6 Bulan", value: "Rp14.000.000" }]);
  });

  it("stops cyclic lineage and tolerates malformed metadata and evidence", () => {
    const { delivered, primed, variant, item } = fixture();
    delivered.derived_from_asset_id = primed.id;
    primed.derived_from_asset_id = delivered.id;
    delivered.metadata = "invalid" as unknown as Record<string, unknown>;
    primed.evidence = null as unknown as Record<string, unknown>;
    item.copy_snapshot = { pre_adaptation: { repayment_plan_selections: [null, { values: [] }] } };

    expect(creativeAssetLineage(variant, delivered).map((entry) => entry.id)).toEqual([primed.id, delivered.id]);
    expect(() => creativeGenerationInfo(item, variant, delivered)).not.toThrow();
    expect(creativeGenerationInfo(item, variant, delivered).repaymentPlans).toEqual([]);
  });

  it("reads the exact model prompt from the generated asset trace without requiring a separate hash", () => {
    const { generated, delivered, variant, item } = fixture();
    generated.metadata = { model: "gpt-image-2", prompt: "Create an unbranded V02 concept from the approved plan." };
    generated.evidence = { request_id: "request-legacy", attempts: 1 };
    delivered.metadata = { full_prompt: "Planner summary copied onto the delivery asset" };

    const info = creativeGenerationInfo(item, variant, delivered);

    expect(info.prompt).toBe("Create an unbranded V02 concept from the approved plan.");
    render(<CreativeGenerationInfoDialog open onOpenChange={vi.fn()} item={item} variant={variant} asset={delivered} imageUrl="https://cdn.example/delivered.png" />);
    expect(screen.getByText("Create an unbranded V02 concept from the approved plan.")).toBeInTheDocument();
    expect(screen.queryByText("Planner summary copied onto the delivery asset")).not.toBeInTheDocument();
  });

  it("renders the complete trace-verified prompt directly in the image detail", () => {
    const { delivered, variant, item } = fixture();
    render(<CreativeGenerationInfoDialog open onOpenChange={vi.fn()} item={item} variant={variant} asset={delivered} imageUrl="https://cdn.example/delivered.png" />);

    expect(screen.getByRole("heading", { name: "成图详情" })).toBeInTheDocument();
    expect(screen.getByText("保留原图信息机制，突出额度利益点")).toBeInTheDocument();
    expect(screen.getByText("Pinjaman Fleksibel Tanpa Ribet")).toBeInTheDocument();
    expect(screen.getByText("AdaKami Indonesia · v12")).toBeInTheDocument();
    expect(screen.getByText("Prime 布局合同 v2 · 1 个保护区 · 背景规则 quiet")).toBeInTheDocument();
    expect(screen.getByText("完整模型提示词")).toBeInTheDocument();
    expect(screen.getByText(/Approved repayment rows/)).toHaveClass("whitespace-pre-wrap", "break-words");
    expect(screen.getByAltText("V01 方形 · 1080x1080 成图")).toBeInTheDocument();
  });
});
