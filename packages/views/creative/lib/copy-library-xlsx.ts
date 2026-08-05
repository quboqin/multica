import { strToU8, zipSync } from "fflate";
import type {
  CreativeCopyCalculationRule,
  CreativeCopyFragment,
  CreativeCopyFragmentRole,
  CreativeCopyLibraryConfig,
  CreativeCopyRecipe,
  CreativeCopyStatus,
  CreativeProductFact,
  CreativeType,
} from "@multica/core/types";
import type { SpreadsheetData } from "./xlsx-copy-import";
import { readSpreadsheetWorkbook } from "./xlsx-copy-import";

export const COPY_LIBRARY_WORKBOOK_VERSION = "2";

const FACT_SHEET = "产品事实";
const FRAGMENT_SHEET = "文案片段";
const RECIPE_SHEET = "组合方案";
const RULE_SHEET = "计算规则";
const SETTINGS_SHEET = "库设置";
const ROLES: CreativeCopyFragmentRole[] = ["headline", "subheadline", "benefit", "supporting", "cta", "legal"];

type WorkbookSheet = { name: string; headers: string[]; rows: string[][] };

export type CopyLibraryImportPreview = {
  config: CreativeCopyLibraryConfig;
  errors: string[];
  warnings: string[];
  sourceName: string;
  counts: { productFacts: number; fragments: number; recipes: number; calculationRules: number };
};

export async function readCopyLibraryWorkbook(file: File): Promise<CopyLibraryImportPreview> {
  return parseCopyLibraryWorkbook(await readSpreadsheetWorkbook(file), file.name);
}

export function parseCopyLibraryWorkbook(sheets: SpreadsheetData[], sourceName = "导入工作簿"): CopyLibraryImportPreview {
  const errors: string[] = [];
  const warnings: string[] = [];
  const byName = new Map(sheets.map((sheet) => [sheet.sheetName.trim(), sheet]));
  const facts = rowsFor(byName, FACT_SHEET, ["key", "label", "value", "copy_text", "source", "status"], errors);
  const fragments = rowsFor(byName, FRAGMENT_SHEET, ["key", "name", "creative_types", "role", "text", "tags", "status"], errors);
  const recipes = rowsFor(byName, RECIPE_SHEET, ["key", "name", "creative_type", "description", "match_tags", "status"], errors);
  const rules = rowsFor(byName, RULE_SHEET, ["key", "name", "expression", "input_fact_keys", "output_fact_key", "source", "status"], errors);
  const settings = settingsFor(byName.get(SETTINGS_SHEET), warnings);
  if (settings.schema_version && settings.schema_version !== COPY_LIBRARY_WORKBOOK_VERSION) {
    errors.push(`工作簿版本为 ${settings.schema_version}，当前仅支持 v${COPY_LIBRARY_WORKBOOK_VERSION} 模板`);
  } else if (!settings.schema_version) {
    warnings.push(`“库设置”未标注 schema_version，按 v${COPY_LIBRARY_WORKBOOK_VERSION} 结构解析；请导出最新模板后再维护`);
  }

  const productFacts = facts.map((row) => parseFact(row, errors, warnings)).filter((row): row is CreativeProductFact => row !== null);
  const copyFragments = fragments.map((row) => parseFragment(row, errors, warnings)).filter((row): row is CreativeCopyFragment => row !== null);
  const copyRecipes = recipes.map((row) => parseRecipe(row, errors, warnings)).filter((row): row is CreativeCopyRecipe => row !== null);
  const calculationRules = rules.map((row) => parseRule(row, errors, warnings)).filter((row): row is CreativeCopyCalculationRule => row !== null);

  duplicateKeys(productFacts, "产品事实", errors);
  duplicateKeys(copyFragments, "文案片段", errors);
  duplicateKeys(copyRecipes, "组合方案", errors);
  duplicateKeys(calculationRules, "计算规则", errors);
  validateReferences(copyRecipes, copyFragments, errors);
  for (const fact of productFacts) {
    if (fact.status === "approved" && (!fact.copy_text || !fact.source)) {
      errors.push(`产品事实“${fact.key}”标记为已审核，但缺少印尼语展示或来源依据`);
    }
  }

  const config: CreativeCopyLibraryConfig = {
    schema_version: 2,
    market: settings.market ?? "Indonesia",
    locale: settings.locale ?? "id-ID",
    source: {
      name: settings.source_name ?? sourceName,
      url: settings.source_url ?? "",
      sync_status: settings.source_sync_status === "synced" || settings.source_sync_status === "failed" ? settings.source_sync_status : "pending",
      note: settings.source_note ?? "",
    },
    product_facts: productFacts,
    fragments: copyFragments,
    recipes: copyRecipes,
    calculation_rules: calculationRules,
    recommendation_policy: {
      type_weight: numberValue(settings.type_weight, 1000),
      tag_weight: numberValue(settings.tag_weight, 80),
      concise_weight: numberValue(settings.concise_weight, 1),
      default_creative_type: creativeType(settings.default_creative_type) ?? "num",
    },
  };

  warnings.push("计算规则只导入到“计算规则”列表；不会自动生成产品事实，也不会把未审核规则当作金融事实。只有“产品事实”表中状态为“已审核”的条目可在发布后进入文案推荐。");
  return {
    config,
    errors: unique(errors),
    warnings: unique(warnings),
    sourceName,
    counts: { productFacts: productFacts.length, fragments: copyFragments.length, recipes: copyRecipes.length, calculationRules: calculationRules.length },
  };
}

export function createCopyLibraryWorkbook(config: CreativeCopyLibraryConfig): Uint8Array {
  const sheets = copyLibraryWorkbookSheets(config);
  const files: Record<string, Uint8Array> = {
    "[Content_Types].xml": textFile(contentTypes(sheets.length)),
    "_rels/.rels": textFile(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`),
    "xl/workbook.xml": textFile(workbookXml(sheets)),
    "xl/_rels/workbook.xml.rels": textFile(workbookRelationships(sheets.length)),
  };
  sheets.forEach((sheet, index) => { files[`xl/worksheets/sheet${index + 1}.xml`] = textFile(worksheetXml(sheet)); });
  return zipSync(files, { level: 6 });
}

export function downloadCopyLibraryWorkbook(config: CreativeCopyLibraryConfig, filename: string): void {
  const bytes = createCopyLibraryWorkbook(config);
  const buffer = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(buffer).set(bytes);
  const blob = new Blob([buffer], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${safeFilename(filename || "文案库")}-v2.xlsx`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export function copyLibraryWorkbookSheets(config: CreativeCopyLibraryConfig): WorkbookSheet[] {
  return [
    {
      name: SETTINGS_SHEET,
      headers: ["key", "value", "说明"],
      rows: [
        ["schema_version", COPY_LIBRARY_WORKBOOK_VERSION, "固定为 2"],
        ["market", config.market, "市场"],
        ["locale", config.locale, "语言，例如 id-ID"],
        ["source_name", config.source.name, "资料来源名称"],
        ["source_url", config.source.url, "来源链接"],
        ["source_sync_status", config.source.sync_status, "pending / synced / failed"],
        ["source_note", config.source.note, "审核或同步备注"],
        ["type_weight", String(config.recommendation_policy.type_weight), "推荐类型匹配权重"],
        ["tag_weight", String(config.recommendation_policy.tag_weight), "推荐标签权重"],
        ["concise_weight", String(config.recommendation_policy.concise_weight), "推荐简洁度权重"],
        ["default_creative_type", config.recommendation_policy.default_creative_type, "num / repayment_plan"],
      ],
    },
    {
      name: FACT_SHEET,
      headers: ["id", "key", "label", "value", "copy_text", "source", "status"],
      rows: config.product_facts.map((fact) => [fact.id, fact.key, fact.label, fact.value, fact.copy_text, fact.source, fact.status]),
    },
    {
      name: FRAGMENT_SHEET,
      headers: ["id", "key", "name", "creative_types", "role", "text", "tags", "status"],
      rows: config.fragments.map((fragment) => [fragment.id, fragment.key, fragment.name, fragment.creative_types.join(","), fragment.role, fragment.text, fragment.tags.join(","), fragment.status]),
    },
    {
      name: RECIPE_SHEET,
      headers: ["id", "key", "name", "creative_type", "description", "match_tags", "status", ...ROLES.map((role) => `${role}_fragment_ids`)],
      rows: config.recipes.map((recipe) => [
        recipe.id, recipe.key, recipe.name, recipe.creative_type, recipe.description, recipe.match_tags.join(","), recipe.status,
        ...ROLES.map((role) => (recipe.fragment_ids[role] ?? []).join(",")),
      ]),
    },
    {
      name: RULE_SHEET,
      headers: ["id", "key", "name", "expression", "input_fact_keys", "output_fact_key", "source", "status"],
      rows: config.calculation_rules.map((rule) => [rule.id, rule.key, rule.name, rule.expression, rule.input_fact_keys.join(","), rule.output_fact_key, rule.source, rule.status]),
    },
  ];
}

type Row = { index: number; get: (header: string) => string };

function rowsFor(sheets: Map<string, SpreadsheetData>, name: string, requiredHeaders: string[], errors: string[]): Row[] {
  const sheet = sheets.get(name);
  if (!sheet) {
    errors.push(`缺少必填工作表“${name}”`);
    return [];
  }
  const headers = new Map(sheet.headers.map((header, index) => [header.trim(), index]));
  for (const header of requiredHeaders) if (!headers.has(header)) errors.push(`工作表“${name}”缺少列“${header}”`);
  return sheet.rows.filter((cells) => cells.some((cell) => cell.trim())).map((cells, index) => ({
    index: index + 2,
    get: (header) => cells[headers.get(header) ?? -1]?.trim() ?? "",
  }));
}

function settingsFor(sheet: SpreadsheetData | undefined, warnings: string[]): Record<string, string> {
  if (!sheet) {
    warnings.push("未提供“库设置”工作表，已使用默认市场、语言和推荐权重");
    return {};
  }
  const keyColumn = sheet.headers.findIndex((header) => header.trim() === "key");
  const valueColumn = sheet.headers.findIndex((header) => header.trim() === "value");
  if (keyColumn < 0 || valueColumn < 0) {
    warnings.push("“库设置”工作表未包含 key/value 列，已使用默认设置");
    return {};
  }
  return Object.fromEntries(sheet.rows.map((row) => [row[keyColumn]?.trim() ?? "", row[valueColumn]?.trim() ?? ""]).filter(([key]) => key));
}

function parseFact(row: Row, errors: string[], warnings: string[]): CreativeProductFact | null {
  const key = required(row, "key", "产品事实", errors);
  if (!key) return null;
  return { id: identifier(row, key, warnings), key, label: row.get("label"), value: row.get("value"), copy_text: row.get("copy_text"), source: row.get("source"), status: copyStatus(row, "产品事实", errors) };
}

function parseFragment(row: Row, errors: string[], warnings: string[]): CreativeCopyFragment | null {
  const key = required(row, "key", "文案片段", errors);
  const role = fragmentRole(row.get("role"));
  if (!key || !role) {
    if (!role) errors.push(`文案片段第 ${row.index} 行的 role 必须是 ${ROLES.join(" / ")}`);
    return null;
  }
  const types = splitList(row.get("creative_types")).map(creativeType).filter((value): value is CreativeType => value !== null);
  if (types.length === 0) errors.push(`文案片段“${key}”至少需要一个 creative_types（num 或 repayment_plan）`);
  return { id: identifier(row, key, warnings), key, name: row.get("name"), creative_types: types, role, text: row.get("text"), tags: splitList(row.get("tags")), status: copyStatus(row, "文案片段", errors) };
}

function parseRecipe(row: Row, errors: string[], warnings: string[]): CreativeCopyRecipe | null {
  const key = required(row, "key", "组合方案", errors);
  const type = creativeType(row.get("creative_type"));
  if (!key || !type) {
    if (!type) errors.push(`组合方案第 ${row.index} 行的 creative_type 必须是 num 或 repayment_plan`);
    return null;
  }
  const fragmentIds: Partial<Record<CreativeCopyFragmentRole, string[]>> = {};
  for (const role of ROLES) {
    const ids = splitList(row.get(`${role}_fragment_ids`));
    if (ids.length > 0) fragmentIds[role] = ids;
  }
  return {
    id: identifier(row, key, warnings), key, name: row.get("name"), creative_type: type, description: row.get("description"), match_tags: splitList(row.get("match_tags")), status: copyStatus(row, "组合方案", errors),
    fragment_ids: fragmentIds,
  };
}

function parseRule(row: Row, errors: string[], warnings: string[]): CreativeCopyCalculationRule | null {
  const key = required(row, "key", "计算规则", errors);
  if (!key) return null;
  return { id: identifier(row, key, warnings), key, name: row.get("name"), expression: row.get("expression"), input_fact_keys: splitList(row.get("input_fact_keys")), output_fact_key: row.get("output_fact_key"), source: row.get("source"), status: copyStatus(row, "计算规则", errors) };
}

function identifier(row: Row, key: string, warnings: string[]): string {
  const id = row.get("id");
  if (id) return id;
  warnings.push(`第 ${row.index} 行未提供 id，已暂以 key “${key}”作为本次草稿标识`);
  return key;
}

function required(row: Row, header: string, sheet: string, errors: string[]): string {
  const value = row.get(header);
  if (!value) errors.push(`${sheet}第 ${row.index} 行缺少 ${header}`);
  return value;
}

function copyStatus(row: Row, sheet: string, errors: string[]): CreativeCopyStatus {
  const value = row.get("status").toLowerCase();
  if (!value || value === "draft" || value === "草稿") return "draft";
  if (value === "approved" || value === "已审核") return "approved";
  if (value === "disabled" || value === "停用") return "disabled";
  errors.push(`${sheet}第 ${row.index} 行的 status 必须是 draft、approved 或 disabled`);
  return "draft";
}

function duplicateKeys(values: Array<{ key: string }>, sheet: string, errors: string[]): void {
  const seen = new Set<string>();
  for (const value of values) {
    if (seen.has(value.key)) errors.push(`${sheet}存在重复 key：“${value.key}”`);
    seen.add(value.key);
  }
}

function validateReferences(recipes: CreativeCopyRecipe[], fragments: CreativeCopyFragment[], errors: string[]): void {
  const ids = new Set(fragments.map((fragment) => fragment.id));
  for (const recipe of recipes) for (const [role, fragmentIds] of Object.entries(recipe.fragment_ids)) {
    for (const id of fragmentIds ?? []) if (!ids.has(id)) errors.push(`组合方案“${recipe.key}”的 ${role} 引用了不存在的片段 id：“${id}”`);
  }
}

function splitList(value: string): string[] { return [...new Set(value.split(/[、,，\n]/).map((item) => item.trim()).filter(Boolean))]; }
function creativeType(value: string | undefined): CreativeType | null { return value === "num" ? "num" : value === "repayment_plan" ? "repayment_plan" : null; }
function fragmentRole(value: string): CreativeCopyFragmentRole | null { return ROLES.includes(value as CreativeCopyFragmentRole) ? value as CreativeCopyFragmentRole : null; }
function numberValue(value: string | undefined, fallback: number): number { const number = Number(value); return Number.isFinite(number) ? number : fallback; }
function unique(values: string[]): string[] { return [...new Set(values)]; }

function contentTypes(sheetCount: number): string {
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>${Array.from({ length: sheetCount }, (_, index) => `<Override PartName="/xl/worksheets/sheet${index + 1}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`).join("")}</Types>`;
}

function workbookXml(sheets: WorkbookSheet[]): string {
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>${sheets.map((sheet, index) => `<sheet name="${escapeXml(sheet.name)}" sheetId="${index + 1}" r:id="rId${index + 1}"/>`).join("")}</sheets></workbook>`;
}

function workbookRelationships(sheetCount: number): string {
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${Array.from({ length: sheetCount }, (_, index) => `<Relationship Id="rId${index + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet${index + 1}.xml"/>`).join("")}</Relationships>`;
}

function worksheetXml(sheet: WorkbookSheet): string {
  const rows = [sheet.headers, ...sheet.rows];
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${rows.map((row, rowIndex) => `<row r="${rowIndex + 1}">${row.map((value, columnIndex) => `<c r="${columnName(columnIndex)}${rowIndex + 1}" t="inlineStr"><is><t xml:space="preserve">${escapeXml(value)}</t></is></c>`).join("")}</row>`).join("")}</sheetData></worksheet>`;
}

function columnName(index: number): string {
  let value = index + 1;
  let name = "";
  while (value > 0) { const remainder = (value - 1) % 26; name = String.fromCharCode(65 + remainder) + name; value = Math.floor((value - 1) / 26); }
  return name;
}

function safeFilename(value: string): string { return value.replace(/[\\/:*?"<>|]/g, "-").trim() || "文案库"; }
function escapeXml(value: string): string { return String(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&apos;"); }
function textFile(value: string): Uint8Array { return strToU8(value); }
