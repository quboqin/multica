import { describe, expect, it } from "vitest";
import type { CreativeMaterialCandidate, CreativeResource } from "../types/creative";
import { parseCreativeCopyLibraryConfig, recommendCreativeCopy, validateCustomCopyFinancialFacts } from "./copy-library";

const resource: CreativeResource = {
  id: "library-1", workspace_id: "workspace-1", kind: "copy_library", name: "Library", description: "", status: "published",
  version: 4, published_version: 3, created_by: "user-1", created_at: "", updated_at: "",
  config: {},
  published_config: {
    schema_version: 2, market: "Indonesia", locale: "id-ID",
    source: { name: "Feishu", url: "https://example.test", sync_status: "pending", note: "" },
    product_facts: [{ id: "fact-1", key: "limit", label: "Limit", value: "80000000", copy_text: "Rp80.000.000", source: "approved", status: "approved" }],
    fragments: [
      { id: "fragment-num-headline", key: "num-headline", name: "Headline", creative_types: ["num"], role: "headline", text: "Pinjaman Fleksibel Tanpa Ribet", tags: [], status: "approved" },
      { id: "fragment-num-benefit", key: "num-benefit", name: "Benefit", creative_types: ["num"], role: "benefit", text: "Limit hingga {{fact.limit.copy_text}}", tags: [], status: "approved" },
      { id: "fragment-plan-headline", key: "plan-headline", name: "Plan", creative_types: ["repayment_plan"], role: "headline", text: "Pilih Tenor Sesuai Kebutuhan", tags: [], status: "approved" },
    ],
    recipes: [
      { id: "recipe-num", key: "num", name: "NUM", creative_type: "num", description: "", fragment_ids: { headline: ["fragment-num-headline"], benefit: ["fragment-num-benefit"] }, match_tags: ["额度", "limit"], status: "approved" },
      { id: "recipe-plan", key: "plan", name: "Plan", creative_type: "repayment_plan", description: "", fragment_ids: { headline: ["fragment-plan-headline"] }, match_tags: ["分期", "tenor"], status: "approved" },
    ],
    calculation_rules: [], recommendation_policy: { type_weight: 1000, tag_weight: 80, concise_weight: 1, default_creative_type: "num" },
  },
};

const candidate = {
  id: "candidate-1", title: "Limit pinjaman", tags: [], media_names: [],
} as unknown as CreativeMaterialCandidate;

describe("composable copy library", () => {
  it("parses malformed config without throwing", () => {
    expect(parseCreativeCopyLibraryConfig({ fragments: null, recipes: "bad" })).toMatchObject({
      schema_version: 2, fragments: [], recipes: [], product_facts: [], locale: "id-ID",
    });
  });

  it("ranks by inferred creative type and freezes assembled facts", () => {
    const ranked = recommendCreativeCopy(candidate, resource, {
      theme: "", theme_elements: [], primary_benefit: "额度", secondary_benefits: [], benefit_value: "",
    });
    expect(ranked[0]?.recipe.id).toBe("recipe-num");
    expect(ranked[0]?.snapshot).toMatchObject({
      schema_version: 2,
      library_id: "library-1",
      library_version: 3,
      creative_type: "num",
      headline: "Pinjaman Fleksibel Tanpa Ribet",
      benefit: "Limit hingga Rp80.000.000",
      product_facts: [{ key: "limit", copy_text: "Rp80.000.000" }],
      fragments: [
        { id: "fragment-num-headline", role: "headline", text: "Pinjaman Fleksibel Tanpa Ribet" },
        { id: "fragment-num-benefit", role: "benefit", text: "Limit hingga Rp80.000.000" },
      ],
    });
    expect(ranked[0]?.reasons).toContain("创意类型：NUM 数字利益点");
  });

  it("keeps NUM and repayment-plan recipes as explicit recommendation types", () => {
    const ranked = recommendCreativeCopy(candidate, resource, {
      theme: "", theme_elements: [], primary_benefit: "Pilihan tenor dan cicilan", secondary_benefits: [], benefit_value: "",
    });

    expect(ranked[0]?.recipe.id).toBe("recipe-plan");
    expect(ranked[0]?.snapshot.creative_type).toBe("repayment_plan");
  });

  it("skips a recipe that cannot resolve an approved fact without blocking a fact-free recipe", () => {
    const changed = structuredClone(resource);
    changed.published_config!.product_facts = [];

    const ranked = recommendCreativeCopy(candidate, changed, {
      theme: "", theme_elements: [], primary_benefit: "额度", secondary_benefits: [], benefit_value: "",
    });

    expect(ranked.map((item) => item.recipe.id)).toEqual(["recipe-plan"]);
    expect(JSON.stringify(ranked)).not.toContain("缺少产品事实");
  });

  it("does not execute unaudited calculation expressions", () => {
    const changed = structuredClone(resource);
    changed.published_config!.calculation_rules = [{ id: "rule", key: "danger", name: "Do not run", expression: "1 / 0", input_fact_keys: [], output_fact_key: "broken", source: "unknown", status: "approved" }];
    expect(recommendCreativeCopy(candidate, changed, { theme: "", theme_elements: [], primary_benefit: "额度", secondary_benefits: [], benefit_value: "" })[0]?.snapshot.product_facts)
      .toEqual([{ key: "limit", label: "Limit", value: "80000000", copy_text: "Rp80.000.000", source: "approved" }]);
  });

  it("allows only approved financial facts in custom copy", () => {
    const approved = validateCustomCopyFinancialFacts({
      headline: "Pinjaman fleksibel",
      subheadline: "",
      benefit: "Limit hingga Rp 80.000.000",
      supporting: "",
      cta: "Ajukan sekarang",
      legal_text: "",
    }, resource);
    const unapproved = validateCustomCopyFinancialFacts({
      headline: "Pinjaman fleksibel",
      subheadline: "",
      benefit: "Limit hingga Rp99.000.000 dengan bunga 1%",
      supporting: "",
      cta: "Ajukan sekarang",
      legal_text: "",
    }, resource);

    expect(approved).toMatchObject({ allowed: true, unapproved: [] });
    expect(unapproved.allowed).toBe(false);
    expect(unapproved.unapproved).toEqual(expect.arrayContaining(["Rp99.000.000", "1%"]));
  });

  it("allows fact-free custom copy without a published library but blocks invented facts", () => {
    const factFree = validateCustomCopyFinancialFacts({
      headline: "Pinjaman fleksibel",
      subheadline: "",
      benefit: "Sesuaikan dengan kebutuhanmu",
      supporting: "",
      cta: "Ajukan sekarang",
      legal_text: "",
    }, undefined);
    const invented = validateCustomCopyFinancialFacts({
      headline: "Pinjaman fleksibel",
      subheadline: "",
      benefit: "Limit hingga Rp99.000.000",
      supporting: "",
      cta: "Ajukan sekarang",
      legal_text: "",
    }, undefined);

    expect(factFree).toEqual({ allowed: true, unapproved: [], message: "" });
    expect(invented.allowed).toBe(false);
  });

  it("keeps using the published revision while a newer draft exists", () => {
    const withDraft = structuredClone(resource);
    withDraft.version = 5;
    withDraft.status = "draft";
    withDraft.config = { fragments: [{ text: "UNPUBLISHED" }] };
    const first = recommendCreativeCopy(candidate, withDraft, { theme: "", theme_elements: [], primary_benefit: "额度", secondary_benefits: [], benefit_value: "" })[0]!.snapshot;
    expect(first.library_version).toBe(3);
    expect(first.headline).toBe("Pinjaman Fleksibel Tanpa Ribet");

    withDraft.published_version = 5;
    withDraft.published_config = structuredClone(resource.published_config);
    const published = withDraft.published_config!.fragments as Array<Record<string, unknown>>;
    published[0] = { ...published[0], text: "Headline v5" };
    const second = recommendCreativeCopy(candidate, withDraft, { theme: "", theme_elements: [], primary_benefit: "额度", secondary_benefits: [], benefit_value: "" })[0]!.snapshot;
    expect(second.library_version).toBe(5);
    expect(second.headline).toBe("Headline v5");
    expect(first.headline).toBe("Pinjaman Fleksibel Tanpa Ribet");
  });
});
