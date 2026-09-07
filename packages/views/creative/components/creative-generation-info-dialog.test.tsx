// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { CreativeGenerationInfoDialog, creativeAssetLineage, creativeGenerationInfo } from "./creative-generation-info-dialog";
import { renderWithI18n } from "../../test/i18n";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));

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
  it("shows actual model integration and the exact template independently of frozen configuration", () => {
    const { delivered, primed, variant, item } = fixture();
    variant.brief = { prime_composition: { mode: "deterministic" } };
    primed.evidence = { composition_mode: "model_integrated", template_family_id: "light_background", template_source_role: "prime_light_square", template_attachment_id: "template-123" };
    const info = creativeGenerationInfo(item, variant, delivered);
    expect(info.primeConfigured.mode).toBe("deterministic");
    expect(info.primeActual).toMatchObject({ mode: "model_integrated", templateAttachmentId: "template-123" });
    renderWithI18n(<CreativeGenerationInfoDialog open onOpenChange={vi.fn()} item={item} variant={variant} asset={delivered} imageUrl="https://cdn.example/delivered.png" />);
    expect(screen.getByText("Platform overlay")).toBeInTheDocument();
    expect(screen.getByText("Model integration")).toBeInTheDocument();
    expect(screen.getByText("Actual processing differs from the frozen order mode")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View actual template" })).toHaveAttribute("href", expect.stringContaining("template-123"));
  });

  it("never infers execution from the current brief or borrows evidence from another revision", () => {
    const { delivered, variant, item } = fixture();
    variant.revision = 3;
    variant.brief = { prime_composition: { mode: "model_integrated" } };
    variant.assets.push(asset({ id: "new-prime", stage: "primed", revision: 3, evidence: { composition_mode: "model_integrated" } }));
    const info = creativeGenerationInfo(item, variant, delivered);
    expect(info.primeConfigured.mode).toBe("unknown");
    expect(info.primeActual.mode).toBe("unknown");
  });

  it("resolves delivered, Prime, and generated records into business information", () => {
    const { delivered, variant, item } = fixture();
    const info = creativeGenerationInfo(item, variant, delivered);

    expect(info.lineage.map((entry) => entry.stage)).toEqual(["generated", "primed", "delivered"]);
    expect(info).toMatchObject({
      direction: "保留原图信息机制，突出额度利益点",
      model: "gpt-image-2",
      provider: "OpenAI",
      marketResource: "AdaKami Indonesia · v12",
      marketRule: "Prime layout contract v2 · 1 protected regions · Backdrop rule quiet",
      prompt: "完整提示词：保留蓝绿色信息卡片，并使用已审核文案。\nApproved repayment rows: Jumlah Pinjaman Rp80.000.000",
      attempts: "2",
    });
    expect(info.copyLines).toContainEqual({ field: "headline", label: "Headline", value: "Pinjaman Fleksibel Tanpa Ribet" });
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
    renderWithI18n(<CreativeGenerationInfoDialog open onOpenChange={vi.fn()} item={item} variant={variant} asset={delivered} imageUrl="https://cdn.example/delivered.png" />);
    expect(screen.getByText("Create an unbranded V02 concept from the approved plan.")).toBeInTheDocument();
    expect(screen.queryByText("Planner summary copied onto the delivery asset")).not.toBeInTheDocument();
  });

  it("renders the complete trace-verified prompt directly in the image detail", () => {
    const { delivered, variant, item } = fixture();
    renderWithI18n(<CreativeGenerationInfoDialog open onOpenChange={vi.fn()} item={item} variant={variant} asset={delivered} imageUrl="https://cdn.example/delivered.png" />);

    expect(screen.getByRole("heading", { name: "Creative details" })).toBeInTheDocument();
    expect(screen.getByText("保留原图信息机制，突出额度利益点")).toBeInTheDocument();
    expect(screen.getByText("Pinjaman Fleksibel Tanpa Ribet")).toBeInTheDocument();
    expect(screen.getByText("AdaKami Indonesia · v12")).toBeInTheDocument();
    expect(screen.getByText("Prime layout contract v2 · 1 protected regions · Backdrop rule quiet")).toBeInTheDocument();
    expect(screen.getByText("Full model prompt")).toBeInTheDocument();
    expect(screen.getByText(/Approved repayment rows/)).toHaveClass("whitespace-pre-wrap", "break-words");
    expect(screen.getByAltText("V01 Square - 1080x1080 creative")).toBeInTheDocument();
  });
});
