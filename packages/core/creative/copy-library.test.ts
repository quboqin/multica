import { describe, expect, it } from "vitest";
import type { CreativeResource } from "../types/creative";
import { parseCreativeCopyLibraryConfig, selectCreativeRepaymentPlan, validateCustomCopyFinancialFacts } from "./copy-library";

const resource: CreativeResource = {
  id: "library-1", workspace_id: "workspace-1", kind: "copy_library", name: "Library", description: "", status: "published",
  version: 4, published_version: 3, created_by: "user-1", created_at: "", updated_at: "", config: {},
  published_config: {
    schema_version: 4, market: "Indonesia", locale: "id-ID",
    source: { name: "Feishu", url: "https://example.test", sync_status: "pending", note: "" },
    fragments: [
      { id: "fragment-num-headline", key: "num-headline", name: "Headline", creative_types: ["num", "repayment_plan"], role: "headline", text: "Pinjaman Fleksibel Tanpa Ribet", tags: [], usage: "core", status: "approved" },
      { id: "fragment-num-benefit", key: "num-benefit", name: "Benefit", creative_types: ["num", "repayment_plan"], role: "benefit", text: "Limit hingga Rp80.000.000", tags: ["limit"], usage: "core", status: "approved" },
      { id: "fragment-plan-headline", key: "plan-headline", name: "Plan", content_group: "repayment_headline", creative_types: ["repayment_plan"], role: "headline", text: "Pilih Tenor Sesuai Kebutuhan", tags: ["cicilan"], usage: "core", status: "approved" },
    ],
    recipes: [],
    repayment_plan: {
      labels: { principal: "Jumlah Pinjaman", tenor: "Periode Cicilan", monthly_installment: "Cicilan per Bulan", total_interest: "Total Bunga", total_repayment: "Total Pembayaran" },
      entries: [
        { id: "plan-5m-3", key: "plan-5000000-3", principal: 5_000_000, tenor_months: 3, monthly_installment: 1_711_667, total_interest: 135_001, total_repayment: 5_135_001, source: "approved table", status: "approved" },
        { id: "plan-5m-6", key: "plan-5000000-6", principal: 5_000_000, tenor_months: 6, monthly_installment: 878_333, total_interest: 269_998, total_repayment: 5_269_998, source: "approved table", status: "approved" },
        { id: "plan-10m-12", key: "plan-10000000-12", principal: 10_000_000, tenor_months: 12, monthly_installment: 923_333, total_interest: 1_079_996, total_repayment: 11_079_996, source: "approved table", status: "approved" },
      ],
    },
  },
};

describe("composable copy library", () => {
  it("parses malformed config without throwing", () => {
    expect(parseCreativeCopyLibraryConfig({ fragments: null, repayment_plan: null })).toMatchObject({ schema_version: 4, fragments: [], locale: "id-ID", repayment_plan: { entries: [] } });
  });

  it("looks up only an approved amount and tenor pair", () => {
    const config = parseCreativeCopyLibraryConfig(resource.published_config!);
    expect(selectCreativeRepaymentPlan(config, { principal: 5_000_000, tenorMonths: 6 })?.display).toMatchObject({ principal: "Rp5.000.000", tenor: "6 Bulan", monthlyInstallment: "Rp878.333" });
    expect(selectCreativeRepaymentPlan(config, { principal: 20_000_000, tenorMonths: 6 })).toBeNull();
    expect(selectCreativeRepaymentPlan(config, { principal: 5_000_000, tenorMonths: 9 })).toBeNull();
  });

  it("allows financial text only when it appears in approved copy or the repayment table", () => {
    const approved = validateCustomCopyFinancialFacts({ headline: "", subheadline: "", benefit: "Limit hingga Rp80.000.000", supporting: "", cta: "", legal_text: "" }, resource);
    const planned = validateCustomCopyFinancialFacts({ headline: "", subheadline: "", benefit: "Cicilan Rp878.333 untuk 6 Bulan", supporting: "", cta: "", legal_text: "" }, resource);
    const invented = validateCustomCopyFinancialFacts({ headline: "", subheadline: "", benefit: "Limit hingga Rp99.000.000", supporting: "", cta: "", legal_text: "" }, resource);
    expect(approved.allowed).toBe(true);
    expect(planned.allowed).toBe(true);
    expect(invented.allowed).toBe(false);
  });

});
