import { describe, expect, it } from "vitest";
import type { CreativeCopyLibraryConfig } from "@multica/core/types";
import { copyLibraryDraftError } from "./copy-template";

function config(): CreativeCopyLibraryConfig {
  return {
    schema_version: 4, market: "Indonesia", locale: "id-ID", source: { name: "业务文案", url: "", sync_status: "synced", note: "" },
    fragments: [
      { id: "headline", key: "headline", name: "标题", creative_types: ["num"], role: "headline", usage: "core", text: "Pinjaman fleksibel", tags: [], status: "approved" },
      { id: "benefit", key: "benefit", name: "卖点", creative_types: ["num"], role: "benefit", usage: "core", text: "Limit hingga Rp80.000.000", tags: [], status: "approved" },
    ],
    recipes: [],
    repayment_plan: { labels: { principal: "Jumlah Pinjaman", tenor: "Tenor", monthly_installment: "Cicilan", total_interest: "Total Bunga", total_repayment: "Total Pembayaran" }, entries: [{ id: "plan", key: "plan-5m-3", principal: 5_000_000, tenor_months: 3, monthly_installment: 1_711_667, total_interest: 135_001, total_repayment: 5_135_001, source: "业务审核", status: "approved" }] },
  };
}

describe("business copy library editing", () => {
  it("accepts final copy and approved repayment rows", () => expect(copyLibraryDraftError(config())).toBe(""));
  it("allows a library without headline copy", () => {
    const value = config();
    value.fragments = value.fragments.filter((fragment) => fragment.role !== "headline");
    expect(copyLibraryDraftError(value)).toBe("");
  });
  it("blocks template variables in an approved copy", () => {
    const value = config();
    value.fragments[1]!.text = "Limit hingga {{fact.limit.copy_text}}";
    expect(copyLibraryDraftError(value)).toContain("不能使用变量");
  });
  it("blocks duplicate amount-and-tenor rows", () => {
    const value = config();
    value.repayment_plan.entries.push({ ...value.repayment_plan.entries[0]!, id: "second", key: "second" });
    expect(copyLibraryDraftError(value)).toContain("还款计划重复");
  });
});
