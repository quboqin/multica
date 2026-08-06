// @vitest-environment node

import { strFromU8, unzipSync } from "fflate";
import { describe, expect, it } from "vitest";
import type { CreativeCopyLibraryConfig } from "@multica/core/types";
import { copyLibraryWorkbookSheets, createCopyLibraryWorkbook, parseCopyLibraryWorkbook } from "./copy-library-xlsx";

const headers = {
  "库设置": ["key", "value"],
  "产品事实": ["id", "key", "label", "value", "copy_text", "source", "status"],
  "文案片段": ["id", "key", "name", "creative_types", "role", "text", "tags", "status"],
  "组合方案": ["id", "key", "name", "creative_type", "description", "match_tags", "status", "headline_fragment_ids", "subheadline_fragment_ids", "benefit_fragment_ids", "supporting_fragment_ids", "cta_fragment_ids", "legal_fragment_ids"],
  "计算规则": ["id", "key", "name", "expression", "input_fact_keys", "output_fact_key", "source", "status"],
};

function sheet(sheetName: keyof typeof headers, rows: string[][]) { return { sheetName, headers: headers[sheetName], rows }; }

describe("copy library XLSX v2", () => {
  it("imports all four v2 domains while keeping calculation rules out of product facts", () => {
    const preview = parseCopyLibraryWorkbook([
      sheet("库设置", [["market", "Indonesia"], ["locale", "id-ID"]]),
      sheet("产品事实", [["fact-limit", "limit", "Limit", "80000000", "Rp80.000.000", "approved sheet", "approved"]]),
      sheet("文案片段", [["fragment-headline", "headline-limit", "Headline", "num", "headline", "Limit hingga {{fact.limit.copy_text}}", "limit", "approved"]]),
      sheet("组合方案", [["recipe-num", "num-limit", "NUM limit", "num", "", "limit", "approved", "fragment-headline", "", "", "", "", ""]]),
      sheet("计算规则", [["rule-interest", "interest-calc", "Interest", "principal * rate", "limit,rate", "monthly_interest", "calculator.xlsx", "draft"]]),
    ], "adakami-copy-v2.xlsx");

    expect(preview.errors).toEqual([]);
    expect(preview.config.product_facts).toEqual([expect.objectContaining({ key: "limit", status: "approved" })]);
    expect(preview.config.calculation_rules).toEqual([expect.objectContaining({ key: "interest-calc", output_fact_key: "monthly_interest", status: "draft" })]);
    expect(preview.config.product_facts.find((fact) => fact.key === "monthly_interest")).toBeUndefined();
    expect(preview.warnings.join("\n")).toContain("不会自动生成产品事实");
  });

  it("keeps invalid references in review instead of producing a silently usable draft", () => {
    const preview = parseCopyLibraryWorkbook([
      sheet("产品事实", []),
      sheet("文案片段", []),
      sheet("组合方案", [["recipe-num", "num-limit", "NUM limit", "num", "", "", "draft", "missing-fragment", "", "", "", "", ""]]),
      sheet("计算规则", []),
    ]);

    expect(preview.config.recipes).toEqual([]);
    expect(preview.errors.join("\n")).not.toContain("组合方案");
    expect(preview.warnings.join("\n")).toContain("未提供“库设置”工作表");
  });

  it("exports a multi-sheet v2 workbook with every business domain", () => {
    const config: CreativeCopyLibraryConfig = {
      schema_version: 2, market: "Indonesia", locale: "id-ID",
      source: { name: "Business source", url: "https://example.test", sync_status: "synced", note: "Reviewed" },
      product_facts: [{ id: "fact-limit", key: "limit", label: "Limit", value: "80000000", copy_text: "Rp80.000.000", source: "approved", status: "approved" }],
      fragments: [{ id: "fragment-headline", key: "headline-limit", name: "Headline", creative_types: ["num"], role: "headline", usage: "core", text: "Limit", tags: ["limit"], status: "approved" }],
      recipes: [{ id: "recipe-num", key: "num-limit", name: "NUM", creative_type: "num", description: "", fragment_ids: { headline: ["fragment-headline"] }, match_tags: ["limit"], status: "approved" }],
      calculation_rules: [{ id: "rule-interest", key: "interest", name: "Interest", expression: "principal * rate", input_fact_keys: ["limit"], output_fact_key: "monthly_interest", source: "calculator", status: "draft" }],
      recommendation_policy: { type_weight: 1000, tag_weight: 80, concise_weight: 1, default_creative_type: "num" },
    };
    const sheets = copyLibraryWorkbookSheets(config);
    expect(sheets.map((sheet) => sheet.name)).toEqual(["库设置", "产品事实", "文案片段", "计算规则"]);
    expect(sheets.find((sheet) => sheet.name === "文案片段")?.headers).toContain("usage");

    const workbook = createCopyLibraryWorkbook(config);
    expect([...workbook.slice(0, 2)]).toEqual([0x50, 0x4b]);
    const archive = unzipSync(workbook);
    expect(Object.keys(archive)).toContain("xl/workbook.xml");
    expect(strFromU8(archive["xl/workbook.xml"]!)).toContain("计算规则");
    expect(strFromU8(archive["xl/worksheets/sheet4.xml"]!)).toContain("monthly_interest");
  });
});
