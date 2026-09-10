import type {
  CreativeCopyFragment,
  CreativeCopyContentGroup,
  CreativeCopyFragmentRole,
  CreativeCopyLibraryConfig,
  CreativeCopyRecipe,
  CreativeCopySnapshot,
  CreativeRepaymentPlanEntry,
  CreativeResource,
  CreativeType,
} from "../types/creative";

export interface CreativeRepaymentPlanSelection {
  planKey: string;
  principal: number;
  tenorMonths: number;
  totalInterest: number;
  totalRepayment: number;
  monthlyInstallment: number;
  display: {
    principal: string;
    tenor: string;
    totalInterest: string;
    totalRepayment: string;
    monthlyInstallment: string;
  };
}

export interface CustomCopyFinancialFactValidation {
  allowed: boolean;
  unapproved: string[];
  unapprovedFacts: Array<{ normalized: string; kind: string; value: string; display: string }>;
  message: string;
}

const ROLES: CreativeCopyFragmentRole[] = ["headline", "subheadline", "benefit", "supporting", "cta", "legal"];

const CONTENT_GROUPS: CreativeCopyContentGroup[] = [
  "standard_headline", "core_benefit", "other_benefit", "call_to_action",
  "repayment_headline",
];

export function creativeCopyContentGroupLabel(value: CreativeCopyContentGroup): string {
  return ({
    standard_headline: "标题文案",
    core_benefit: "核心卖点",
    other_benefit: "通用补位卖点",
    call_to_action: "引导点击文案",
    repayment_headline: "分期还款标题",
  })[value];
}

export function creativeCopyContentGroupForFragment(fragment: CreativeCopyFragment): CreativeCopyContentGroup {
  if (fragment.content_group) return fragment.content_group;
  if (fragment.creative_types.length === 1 && fragment.creative_types[0] === "repayment_plan") {
    return "repayment_headline";
  }
  if (fragment.role === "cta") return "call_to_action";
  if (fragment.role === "supporting") return "other_benefit";
  if (fragment.role === "benefit") return "core_benefit";
  return "standard_headline";
}

export function parseCreativeCopyLibraryConfig(value: Record<string, unknown>): CreativeCopyLibraryConfig {
  const source = record(value.source);
  const fragments = array(value.fragments)
    .map(parseFragment)
    .filter((item): item is CreativeCopyFragment => item !== null)
    .map((fragment) => ({ ...fragment, content_group: creativeCopyContentGroupForFragment(fragment) }));
  const repaymentPlan = parseRepaymentPlan(value.repayment_plan);
  return {
    schema_version: 4,
    market: stringValue(value.market),
    locale: stringValue(value.locale) || "id-ID",
    source: {
      name: stringValue(source.name),
      url: stringValue(source.url),
      sync_status: source.sync_status === "synced" || source.sync_status === "failed" ? source.sync_status : "pending",
      note: stringValue(source.note),
    },
    fragments,
    recipes: array(value.recipes).map(parseRecipe).filter((item): item is CreativeCopyRecipe => item !== null),
    repayment_plan: repaymentPlan,
  };
}

export function creativeTypeLabel(value: CreativeType): string {
  return value === "repayment_plan" ? "还款计划" : "NUM 数字利益点";
}

/** Looks up the approved business result for the selected amount and tenor. */
export function selectCreativeRepaymentPlan(
  config: Pick<CreativeCopyLibraryConfig, "repayment_plan">,
  input: { principal: number; tenorMonths: number },
): CreativeRepaymentPlanSelection | null {
  const entry = approvedRepaymentPlans(config.repayment_plan.entries).find((item) => (
    item.principal === input.principal && item.tenor_months === input.tenorMonths
  ));
  if (!entry) return null;
  return {
    planKey: entry.key,
    principal: entry.principal,
    tenorMonths: entry.tenor_months,
    totalInterest: entry.total_interest,
    totalRepayment: entry.total_repayment,
    monthlyInstallment: entry.monthly_installment,
    display: {
      principal: formatIDR(entry.principal),
      tenor: `${entry.tenor_months} Bulan`,
      totalInterest: formatIDR(entry.total_interest),
      totalRepayment: formatIDR(entry.total_repayment),
      monthlyInstallment: formatIDR(entry.monthly_installment),
    },
  };
}

function approvedRepaymentPlans(entries: CreativeRepaymentPlanEntry[]): CreativeRepaymentPlanEntry[] {
  return entries.filter((entry) => entry.status === "approved"
    && entry.key.trim() !== ""
    && entry.source.trim() !== ""
    && Number.isInteger(entry.principal) && entry.principal > 0
    && Number.isInteger(entry.tenor_months) && entry.tenor_months > 0
    && Number.isInteger(entry.monthly_installment) && entry.monthly_installment > 0
    && Number.isInteger(entry.total_interest) && entry.total_interest >= 0
    && Number.isInteger(entry.total_repayment) && entry.total_repayment > 0);
}

function formatIDR(value: number): string {
  return `Rp${Math.round(value).toLocaleString("id-ID")}`;
}

export function validateCustomCopyFinancialFacts(
  snapshot: Pick<CreativeCopySnapshot, "headline" | "subheadline" | "benefit" | "supporting" | "cta" | "legal_text">,
  resource: Pick<CreativeResource, "published_version"> & { published_config?: Record<string, unknown> } | undefined,
): CustomCopyFinancialFactValidation {
  const text = [snapshot.headline, snapshot.subheadline, snapshot.benefit, snapshot.supporting, snapshot.cta, snapshot.legal_text].join("\n");
  const observedTokens = financialFactTokens(text);
  if (observedTokens.length === 0) return { allowed: true, unapproved: [], unapprovedFacts: [], message: "" };
  if (!resource || resource.published_version < 1) {
    return {
      allowed: false,
      unapproved: [],
      unapprovedFacts: [],
      message: "当前市场规则未绑定已发布文案库，不能验证自定义文案中的金融事实。",
    };
  }
  const config = parseCreativeCopyLibraryConfig(resource.published_config ?? {});
  const allowedTokens = new Set([
    ...config.fragments
      .filter((fragment) => fragment.status === "approved")
      .flatMap((fragment) => financialFactTokens(fragment.text).map((token) => token.normalized)),
    ...approvedRepaymentPlans(config.repayment_plan.entries).flatMap((entry) => financialFactTokens([
      formatIDR(entry.principal), `${entry.tenor_months} Bulan`, formatIDR(entry.monthly_installment),
      formatIDR(entry.total_interest), formatIDR(entry.total_repayment),
    ].join("\n")).map((token) => token.normalized)),
  ]);
  const unapprovedFacts = observedTokens.filter((token) => !allowedTokens.has(token.normalized));
  const unapproved = unique(unapprovedFacts.map((token) => token.display));
  return {
    allowed: unapproved.length === 0,
    unapproved,
    unapprovedFacts: uniqueBy(unapprovedFacts, (token) => token.normalized)
      .map(({ normalized, kind, value, display }) => ({ normalized, kind, value, display })),
    message: unapproved.length === 0 ? "" : `本次人工文案包含未审核金融数值：${unapproved.join("、")}。会按本次改写提交，不会自动写入文案库；生成前请确认业务口径。`,
  };
}

type FinancialFactToken = { normalized: string; kind: string; value: string; display: string; start: number; end: number };

function financialFactTokens(text: string): FinancialFactToken[] {
  const tokens: FinancialFactToken[] = [];
  const protectedRanges: Array<{ start: number; end: number }> = [];
  const add = (kind: string, value: string, display: string, start: number, end: number, protect = false) => {
    const normalizedValue = value.trim();
    if (normalizedValue) tokens.push({ normalized: `${kind}:${normalizedValue}`, kind, value: normalizedValue, display: display.trim(), start, end });
    if (protect) protectedRanges.push({ start, end });
  };
  for (const match of text.matchAll(/\bRp\s*\d{1,3}(?:[.,]\d{3})*(?:[.,]\d+)?/gi)) {
    add("currency", match[0].replace(/\D/g, ""), match[0], match.index, match.index + match[0].length, true);
  }
  for (const match of text.matchAll(/\b\d+(?:[.,]\d+)?\s*%/g)) {
    add("percent", match[0].replace(/\s/g, "").replace(",", "."), match[0], match.index, match.index + match[0].length, true);
  }
  for (const match of text.matchAll(/\b\d+(?:\s*-\s*\d+)?\s*(?:bulan|hari|tahun)\b/gi)) {
    add("term", match[0].replace(/\s/g, "").toLocaleLowerCase(), match[0], match.index, match.index + match[0].length, true);
  }
  for (const match of text.matchAll(/\b(?:limit|pinjaman|dana|jumlah|cicilan|angsuran|tenor|bunga|interest|biaya|fee)\D{0,24}(\d{1,3}(?:[.,]\d{3})+|\d+)(?:\s*(?:juta|ribu|miliar))?/gi)) {
    const start = match.index;
    const end = match.index + match[0].length;
    if (protectedRanges.some((range) => start < range.end && range.start < end)) continue;
    const numeric = match[1]?.replace(/\D/g, "") ?? "";
    add("financial_number", numeric, match[0], start, end);
  }
  return tokens;
}

function parseFragment(value: unknown): CreativeCopyFragment | null {
  const row = record(value);
  const role = ROLES.find((item) => item === row.role);
  if (!stringValue(row.id) || !stringValue(row.key) || !role) return null;
  return {
    id: stringValue(row.id), key: stringValue(row.key), name: stringValue(row.name), creative_types: stringArray(row.creative_types).map(creativeType).filter((item): item is CreativeType => item !== null),
    role, content_group: copyContentGroup(row.content_group) ?? undefined, semantic_group: stringValue(row.semantic_group) || undefined,
    text: stringValue(row.text), tags: stringArray(row.tags), usage: fragmentUsage(row.usage, role), status: copyStatus(row.status),
  };
}

function parseRecipe(value: unknown): CreativeCopyRecipe | null {
  const row = record(value);
  const type = creativeType(row.creative_type);
  if (!stringValue(row.id) || !stringValue(row.key) || !type) return null;
  const rawFragments = record(row.fragment_ids);
  const fragmentIds: CreativeCopyRecipe["fragment_ids"] = {};
  for (const role of ROLES) fragmentIds[role] = stringArray(rawFragments[role]);
  return {
    id: stringValue(row.id), key: stringValue(row.key), name: stringValue(row.name), creative_type: type,
    description: stringValue(row.description), fragment_ids: fragmentIds, match_tags: stringArray(row.match_tags), status: copyStatus(row.status),
  };
}

function parseRepaymentPlan(value: unknown): CreativeCopyLibraryConfig["repayment_plan"] {
  const row = record(value);
  const labels = record(row.labels);
  return {
    labels: {
      principal: stringValue(labels.principal), tenor: stringValue(labels.tenor), monthly_installment: stringValue(labels.monthly_installment),
      total_interest: stringValue(labels.total_interest), total_repayment: stringValue(labels.total_repayment),
    },
    entries: array(row.entries).map(parseRepaymentPlanEntry).filter((entry): entry is CreativeRepaymentPlanEntry => entry !== null),
  };
}

function parseRepaymentPlanEntry(value: unknown): CreativeRepaymentPlanEntry | null {
  const row = record(value);
  const principal = numberValue(row.principal, NaN);
  const tenorMonths = numberValue(row.tenor_months, NaN);
  const monthlyInstallment = numberValue(row.monthly_installment, NaN);
  const totalInterest = numberValue(row.total_interest, NaN);
  const totalRepayment = numberValue(row.total_repayment, NaN);
  if (!stringValue(row.id) || !stringValue(row.key)
    || !Number.isInteger(principal) || !Number.isInteger(tenorMonths)
    || !Number.isInteger(monthlyInstallment) || !Number.isInteger(totalInterest) || !Number.isInteger(totalRepayment)) return null;
  return {
    id: stringValue(row.id), key: stringValue(row.key), principal, tenor_months: tenorMonths,
    monthly_installment: monthlyInstallment, total_interest: totalInterest, total_repayment: totalRepayment,
    source: stringValue(row.source), status: copyStatus(row.status),
  };
}

function record(value: unknown): Record<string, unknown> { return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
function array(value: unknown): unknown[] { return Array.isArray(value) ? value : []; }
function stringValue(value: unknown): string { return typeof value === "string" ? value.trim() : ""; }
function stringArray(value: unknown): string[] { return array(value).filter((item): item is string => typeof item === "string").map((item) => item.trim()).filter(Boolean); }
function numberValue(value: unknown, fallback: number): number { return typeof value === "number" && Number.isFinite(value) ? value : fallback; }
function creativeType(value: unknown): CreativeType | null { return value === "num" || value === "repayment_plan" ? value : null; }
function copyContentGroup(value: unknown): CreativeCopyContentGroup | null { return typeof value === "string" && CONTENT_GROUPS.includes(value as CreativeCopyContentGroup) ? value as CreativeCopyContentGroup : null; }
function copyStatus(value: unknown): "draft" | "approved" | "disabled" { return value === "approved" || value === "disabled" ? value : "draft"; }
function fragmentUsage(value: unknown, role: CreativeCopyFragmentRole): CreativeCopyFragment["usage"] {
  if (value === "core" || value === "fallback" || value === "required") return value;
  if (role === "legal") return "required";
  if (role === "supporting" || role === "cta") return "fallback";
  return "core";
}
function unique(values: string[]): string[] { return [...new Set(values)]; }
function uniqueBy<T>(values: T[], key: (value: T) => string): T[] {
  const seen = new Set<string>();
  return values.filter((value) => {
    const identity = key(value);
    if (seen.has(identity)) return false;
    seen.add(identity);
    return true;
  });
}
