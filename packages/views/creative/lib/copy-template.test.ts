import { describe, expect, it } from "vitest";
import type { CreativeCopyLibraryConfig, CreativeProductFact } from "@multica/core/types";
import { appendCopyFactToken, copyLibraryDraftError, copyTemplateFactKeys, resolveCopyTemplateForBusiness } from "./copy-template";

const facts: CreativeProductFact[] = [
  { id: "limit", key: "limit_max", label: "最高额度", value: "80000000", copy_text: "Rp80.000.000", source: "业务审核", status: "approved" },
  { id: "rate", key: "interest_rate_from", label: "起始利率", value: "0.03%", copy_text: "0,03%*", source: "业务审核", status: "approved" },
];

describe("business copy template editing", () => {
  it("shows resolved Indonesian copy and the facts used without exposing template syntax", () => {
    const preview = resolveCopyTemplateForBusiness(
      "Limit hingga {{fact.limit_max.copy_text}}\nBunga mulai dari {{fact.interest_rate_from.copy_text}}",
      facts,
    );

    expect(preview.text).toBe("Limit hingga Rp80.000.000\nBunga mulai dari 0,03%*");
    expect(preview.usedFacts.map((fact) => fact.key)).toEqual(["limit_max", "interest_rate_from"]);
    expect(preview.missingFactKeys).toEqual([]);
  });

  it("keeps missing or unapproved facts visible as a blocking business problem", () => {
    const preview = resolveCopyTemplateForBusiness("Tenor {{fact.tenor_range.copy_text}}", facts);
    expect(preview.text).toBe("Tenor [缺少事实：tenor_range]");
    expect(preview.missingFactKeys).toEqual(["tenor_range"]);
  });

  it("inserts and discovers product facts without requiring users to type tokens", () => {
    expect(appendCopyFactToken("Limit hingga", "limit_max")).toBe("Limit hingga {{fact.limit_max.copy_text}}");
    expect(appendCopyFactToken("", "interest_rate_from")).toBe("{{fact.interest_rate_from.copy_text}}");
    expect(copyTemplateFactKeys("{{fact.limit_max.copy_text}} / {{fact.limit_max.value}}")).toEqual(["limit_max"]);
  });

  it("blocks publishing when approved copy refers to a missing business fact", () => {
    const config: CreativeCopyLibraryConfig = {
      schema_version: 2,
      market: "Indonesia",
      locale: "id-ID",
      source: { name: "业务文案", url: "", sync_status: "synced", note: "" },
      product_facts: facts,
      fragments: [{ id: "tenor", key: "tenor", name: "期限", creative_types: ["repayment_plan"], role: "benefit", usage: "core", text: "Tenor {{fact.tenor_range.copy_text}}", tags: [], status: "approved" }],
      recipes: [
        { id: "num", key: "num", name: "NUM", creative_type: "num", description: "", fragment_ids: {}, match_tags: [], status: "approved" },
        { id: "plan", key: "plan", name: "Plan", creative_type: "repayment_plan", description: "", fragment_ids: {}, match_tags: [], status: "approved" },
      ],
      calculation_rules: [],
      recommendation_policy: { type_weight: 1000, tag_weight: 80, concise_weight: 1, default_creative_type: "num" },
    };

    expect(copyLibraryDraftError(config)).toBe("已审核文案片段“期限”缺少已审核产品事实“tenor_range”");
  });
});
