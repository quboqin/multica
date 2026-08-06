import type {
  CreativeCopyFragment,
  CreativeCopyFragmentRole,
  CreativeCopyLibraryConfig,
  CreativeCopyRecipe,
  CreativeCopySnapshot,
  CreativeMaterialCandidate,
  CreativeProductFact,
  CreativeResource,
  CreativeType,
} from "../types/creative";

export interface CreativeCopyRecommendationBrief {
  creative_type_hint?: CreativeType;
  copy_slots?: CreativeCopyFragmentRole[];
  theme: string;
  theme_elements: string[];
  primary_benefit: string;
  secondary_benefits: string[];
  benefit_value: string;
  source_semantics?: string;
  information_mechanism?: string;
  has_repayment_table?: boolean;
  detected_text?: string;
}

export interface CreativeCopyRecommendation {
  composition: { id: string; key: string; name: string };
  snapshot: CreativeCopySnapshot;
  score: number;
  reasons: string[];
  matchedSignals: string[];
  exactTypeMatch: boolean;
  approvedPrincipalDistance: number | null;
}

export interface CustomCopyFinancialFactValidation {
  allowed: boolean;
  unapproved: string[];
  unapprovedFacts: Array<{ normalized: string; kind: string; value: string; display: string }>;
  message: string;
}

const EMPTY_POLICY: CreativeCopyLibraryConfig["recommendation_policy"] = {
  type_weight: 1000,
  tag_weight: 80,
  concise_weight: 1,
  default_creative_type: "num",
};

const ROLES: CreativeCopyFragmentRole[] = ["headline", "subheadline", "benefit", "supporting", "cta", "legal"];

export function parseCreativeCopyLibraryConfig(value: Record<string, unknown>): CreativeCopyLibraryConfig {
  const source = record(value.source);
  const policy = record(value.recommendation_policy);
  return {
    schema_version: 2,
    market: stringValue(value.market),
    locale: stringValue(value.locale) || "id-ID",
    source: {
      name: stringValue(source.name),
      url: stringValue(source.url),
      sync_status: source.sync_status === "synced" || source.sync_status === "failed" ? source.sync_status : "pending",
      note: stringValue(source.note),
    },
    fragments: array(value.fragments).map(parseFragment).filter((item): item is CreativeCopyFragment => item !== null),
    recipes: array(value.recipes).map(parseRecipe).filter((item): item is CreativeCopyRecipe => item !== null),
    product_facts: array(value.product_facts).map(parseFact).filter((item): item is CreativeProductFact => item !== null),
    calculation_rules: array(value.calculation_rules).map((item) => {
      const row = record(item);
      return {
        id: stringValue(row.id), key: stringValue(row.key), name: stringValue(row.name), expression: stringValue(row.expression),
        input_fact_keys: stringArray(row.input_fact_keys), output_fact_key: stringValue(row.output_fact_key), source: stringValue(row.source),
        status: copyStatus(row.status),
      };
    }).filter((item) => item.id && item.key),
    recommendation_policy: {
      type_weight: numberValue(policy.type_weight, EMPTY_POLICY.type_weight),
      tag_weight: numberValue(policy.tag_weight, EMPTY_POLICY.tag_weight),
      concise_weight: numberValue(policy.concise_weight, EMPTY_POLICY.concise_weight),
      default_creative_type: creativeType(policy.default_creative_type) ?? EMPTY_POLICY.default_creative_type,
    },
  };
}

export function recommendCreativeCopy(
  candidate: CreativeMaterialCandidate,
  resource: Pick<CreativeResource, "id" | "published_version" | "version" | "published_config">,
  brief: CreativeCopyRecommendationBrief,
): CreativeCopyRecommendation[] {
  const config = parseCreativeCopyLibraryConfig(resource.published_config ?? {});
  const signals = recommendationSignals(candidate, brief);
  const inference = brief.creative_type_hint
    ? { type: brief.creative_type_hint, reason: `参考分析识别为${creativeTypeLabel(brief.creative_type_hint)}` }
    : inferCreativeType(signals.map((signal) => signal.value).join(" "), brief.has_repayment_table)
    ?? { type: config.recommendation_policy.default_creative_type, reason: "未识别月供表，使用文案库默认 NUM 类型" };
  const facts = new Map(config.product_facts.filter((fact) => fact.status === "approved").map((fact) => [fact.key, fact]));
  const observedAmounts = financialAmounts(signals.map((signal) => signal.value).join(" "));
  const desiredRoles = desiredCopyRoles(brief);
  const desiredBenefitGroups = inferBenefitGroups(signals.map((signal) => signal.value).join(" "));
  const available = config.fragments
    .filter((fragment) => fragment.status === "approved" && fragment.creative_types.includes(inference.type))
    .map((fragment) => {
      const resolved = resolveFragment(fragment, facts);
      const matches = unique(fragment.tags)
        .map((tag) => ({ tag, weight: signalMatchWeight(signals, tag) }))
        .filter((match) => match.weight > 0);
      const amountMatch = inference.type === "repayment_plan"
        ? closestApprovedPrincipal(resolved.factKeys, facts, observedAmounts)
        : null;
      return {
        fragment,
        resolved,
        matches,
        score: matches.reduce((total, match) => total + match.weight * config.recommendation_policy.tag_weight, 0)
          + (amountMatch?.score ?? 0)
          + (fragment.usage === "required" ? 10_000 : fragment.usage === "fallback" ? -100 : 0),
        amountMatch,
      };
    })
    .filter((item) => item.resolved.missingFactKeys.length === 0);
  const byRole = new Map(ROLES.map((role) => [role, available
    .filter((item) => item.fragment.role === role)
    .sort((left, right) => right.score - left.score || left.fragment.key.localeCompare(right.fragment.key))]));
  const libraryVersion = resource.published_version || resource.version;
  const compositions: CreativeCopyRecommendation[] = [];
  const seen = new Set<string>();
  for (let rank = 0; rank < 6 && compositions.length < 3; rank += 1) {
    const selected = selectDynamicFragments(byRole, rank, desiredRoles, desiredBenefitGroups);
    if (selected.length === 0) continue;
    const identity = selected.map((item) => item.fragment.id).join("|");
    if (seen.has(identity)) continue;
    seen.add(identity);
    const assembled = assembleDynamicFragments(selected, facts);
    const compositionID = stableCompositionUUID([
      resource.id, String(libraryVersion), candidate.id, inference.type,
      JSON.stringify(brief), identity,
    ].join("|"));
    const compositionKey = `dynamic-${inference.type}-${compositionID.slice(0, 8)}`;
    const matchedSignals = unique(selected.flatMap((item) => item.matches.map((match) => match.tag)));
    const fallbackRoles = unique(selected.filter((item) => item.fragment.usage === "fallback").map((item) => roleLabel(item.fragment.role)));
    const score = selected.reduce((total, item) => total + item.score, config.recommendation_policy.type_weight);
    const reasons = [
      inference.reason,
      ...matchedSignals.filter(isExplainableSignal).slice(0, 2).map((signal) => `贴近原图语义：${signal}`),
      ...(fallbackRoles.length > 0 ? [`通用文案补位：${fallbackRoles.join("、")}`] : []),
    ];
    const snapshot: CreativeCopySnapshot = {
      schema_version: 2,
      id: compositionID,
      library_id: resource.id,
      library_version: libraryVersion,
      composition_id: compositionID,
      composition_key: compositionKey,
      composition_engine_version: "atomic-v1",
      creative_type: inference.type,
      headline: assembled.byRole.headline,
      subheadline: assembled.byRole.subheadline,
      benefit: assembled.byRole.benefit,
      supporting: assembled.byRole.supporting,
      cta: assembled.byRole.cta,
      legal_text: assembled.byRole.legal,
      fragments: assembled.fragments,
      product_facts: assembled.factKeys.map((key) => facts.get(key)).filter((fact): fact is CreativeProductFact => fact !== undefined)
        .map(({ key, label, value, copy_text, source }) => ({ key, label, value, copy_text, source })),
      recommendation: { score, reasons, matched_signals: matchedSignals },
      status: "approved",
    };
    compositions.push({
      composition: { id: compositionID, key: compositionKey, name: `推荐文案 ${compositions.length + 1}` },
      snapshot, score, reasons, matchedSignals, exactTypeMatch: true,
      approvedPrincipalDistance: selected.map((item) => item.amountMatch?.distance).find((value): value is number => value !== undefined) ?? null,
    });
  }
  return compositions;
}

export function creativeTypeLabel(value: CreativeType): string {
  return value === "repayment_plan" ? "还款计划" : "NUM 数字利益点";
}

export function validateCustomCopyFinancialFacts(
  snapshot: Pick<CreativeCopySnapshot, "headline" | "subheadline" | "benefit" | "supporting" | "cta" | "legal_text">,
  resource: Pick<CreativeResource, "published_version" | "published_config"> | undefined,
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
  const facts = new Map(config.product_facts.filter((fact) => fact.status === "approved").map((fact) => [fact.key, fact]));
  const allowedTokens = new Set([
    ...config.product_facts
      .filter((fact) => fact.status === "approved")
      .flatMap((fact) => {
        const tokens = financialFactTokens(`${fact.copy_text}\n${fact.value}`);
        return [
          ...tokens.map((token) => token.normalized),
          ...tokens.filter((token) => token.normalized.startsWith("currency:"))
            .map((token) => `financial_number:${token.normalized.slice("currency:".length)}`),
          ...(fact.value.match(/^\d+(?:[.,]\d+)*$/) ? [`financial_number:${fact.value.replace(/\D/g, "")}`] : []),
        ];
      }),
    ...config.fragments
      .filter((fragment) => fragment.status === "approved")
      .flatMap((fragment) => financialFactTokens(resolveFragment(fragment, facts).text).map((token) => token.normalized)),
  ]);
  const unapprovedFacts = observedTokens.filter((token) => !allowedTokens.has(token.normalized));
  const unapproved = unique(unapprovedFacts.map((token) => token.display));
  return {
    allowed: unapproved.length === 0,
    unapproved,
    unapprovedFacts: uniqueBy(unapprovedFacts, (token) => token.normalized)
      .map(({ normalized, kind, value, display }) => ({ normalized, kind, value, display })),
    message: unapproved.length === 0 ? "" : `自定义文案包含未审核金融事实：${unapproved.join("、")}。请改用已审核产品事实，或在文案库中审核后再发布。`,
  };
}

type ResolvedFragment = {
  text: string;
  factKeys: string[];
  missingFactKeys: string[];
};

type DynamicFragmentCandidate = {
  fragment: CreativeCopyFragment;
  resolved: ResolvedFragment;
  matches: Array<{ tag: string; weight: number }>;
  score: number;
  amountMatch: { score: number; copyText: string; distance: number } | null;
};

function resolveFragment(fragment: CreativeCopyFragment, facts: Map<string, CreativeProductFact>): ResolvedFragment {
  const factKeys = new Set<string>();
  const missingFactKeys = new Set<string>();
  return {
    text: interpolateFacts(fragment.text, facts, factKeys, missingFactKeys),
    factKeys: [...factKeys],
    missingFactKeys: [...missingFactKeys],
  };
}

function selectDynamicFragments(
  byRole: Map<CreativeCopyFragmentRole, DynamicFragmentCandidate[]>,
  rank: number,
  desiredRoles: Set<CreativeCopyFragmentRole>,
  desiredBenefitGroups: string[],
): DynamicFragmentCandidate[] {
  const selected: DynamicFragmentCandidate[] = [];
  for (const role of ROLES) {
    const pool = byRole.get(role) ?? [];
    const required = pool.filter((item) => item.fragment.usage === "required");
    const core = pool.filter((item) => item.fragment.usage === "core");
    const fallback = pool.filter((item) => item.fragment.usage === "fallback");
    selected.push(...required);
    if (role === "legal" || required.length > 0) continue;
    if (!desiredRoles.has(role)) continue;
    if (role === "benefit") {
      const grouped = desiredBenefitGroups.map((group) => {
        const candidates = core.filter((item) => item.fragment.semantic_group === group);
        return candidates[rank % Math.max(1, candidates.length)];
      }).filter((item): item is DynamicFragmentCandidate => item !== undefined);
      const benefits = grouped.length > 0 ? uniqueBy(grouped, (item) => item.fragment.id) : core.slice(0, 3);
      selected.push(...(benefits.length > 0 ? benefits : fallback.slice(rank % Math.max(1, fallback.length), rank % Math.max(1, fallback.length) + 1)));
      continue;
    }
    const matchedCore = core.filter((item) => item.matches.length > 0);
    const matchedFallback = fallback.filter((item) => item.matches.length > 0);
    const preferred = matchedCore.length > 0 ? matchedCore : core;
    const poolForRole = preferred.length > 0 ? preferred : matchedFallback.length > 0 ? matchedFallback : fallback;
    if (poolForRole.length > 0) selected.push(poolForRole[rank % poolForRole.length]!);
  }
  return selected;
}

function desiredCopyRoles(brief: CreativeCopyRecommendationBrief): Set<CreativeCopyFragmentRole> {
  if (brief.copy_slots?.length) return new Set(brief.copy_slots);
  const roles = new Set<CreativeCopyFragmentRole>(["headline", "benefit"]);
  const sourceText = `${brief.detected_text ?? ""}\n${brief.information_mechanism ?? ""}`;
  if (/\b(?:ajukan|ambil|download|apply|submit|daftar|mulai|sekarang)\b|按钮|行动文案|立即申请|马上申请/i.test(sourceText)) roles.add("cta");
  return roles;
}

function assembleDynamicFragments(selected: DynamicFragmentCandidate[], facts: Map<string, CreativeProductFact>) {
  const byRole = { headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal: "" };
  const fragments: CreativeCopySnapshot["fragments"] = [];
  const factKeys = new Set<string>();
  for (const role of ROLES) {
    const lines = selected.filter((item) => item.fragment.role === role).map((item) => {
      fragments.push({ id: item.fragment.id, key: item.fragment.key, role, text: item.resolved.text });
      for (const key of item.resolved.factKeys) if (facts.has(key)) factKeys.add(key);
      return item.resolved.text;
    }).filter(Boolean);
    byRole[role] = lines.join("\n");
  }
  return { byRole, fragments, factKeys: [...factKeys] };
}

function roleLabel(role: CreativeCopyFragmentRole): string {
  return ({ headline: "主标题", subheadline: "副标题", benefit: "利益点", supporting: "补充卖点", cta: "行动按钮", legal: "合规文字" })[role];
}

function stableCompositionUUID(value: string): string {
  const words = [0x811c9dc5, 0x9e3779b9, 0x85ebca6b, 0xc2b2ae35].map((seed, index) => {
    let hash = seed ^ (index * 0x27d4eb2d);
    for (let position = 0; position < value.length; position += 1) {
      hash ^= value.charCodeAt(position) + index * 17;
      hash = Math.imul(hash, 0x01000193);
    }
    return hash >>> 0;
  });
  const hex = words.map((word) => word.toString(16).padStart(8, "0")).join("").split("");
  hex[12] = "5";
  hex[16] = ((Number.parseInt(hex[16]!, 16) & 0x3) | 0x8).toString(16);
  const raw = hex.join("");
  return `${raw.slice(0, 8)}-${raw.slice(8, 12)}-${raw.slice(12, 16)}-${raw.slice(16, 20)}-${raw.slice(20, 32)}`;
}

function interpolateFacts(template: string, facts: Map<string, CreativeProductFact>, used: Set<string>, missing: Set<string>): string {
  return template.replace(/\{\{fact\.([a-z0-9_]+)\.(copy_text|value)\}\}/gi, (_, key: string, field: "copy_text" | "value") => {
    const fact = facts.get(key);
    if (!fact) {
      missing.add(key);
      return "";
    }
    used.add(key);
    return fact[field];
  });
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

function inferBenefitGroups(text: string): string[] {
  const groups: string[] = [];
  if (/额度|最高|limit|jumlah|pinjaman|dana|rp\s*[\d.,]+/i.test(text)) groups.push("limit");
  if (/期限|分期|tenor|bulan|term/i.test(text)) groups.push("tenor");
  if (/利率|费率|bunga|interest|\d+(?:[.,]\d+)?\s*%/i.test(text)) groups.push("interest_rate");
  if (groups.length === 0 && /月供|还款|cicilan|angsuran|repayment/i.test(text)) groups.push("repayment_example");
  return groups;
}

type RecommendationSignal = { value: string; weight: number };

function recommendationSignals(candidate: CreativeMaterialCandidate, brief: CreativeCopyRecommendationBrief): RecommendationSignal[] {
  return [
    { value: brief.primary_benefit, weight: 4 },
    { value: brief.benefit_value, weight: 3 },
    { value: brief.information_mechanism ?? "", weight: 3 },
    { value: brief.detected_text ?? "", weight: 3 },
    ...brief.secondary_benefits.map((value) => ({ value, weight: 2 })),
    { value: brief.source_semantics ?? "", weight: 2 },
    { value: brief.theme, weight: 1 },
    ...brief.theme_elements.map((value) => ({ value, weight: 1 })),
    { value: candidate.title, weight: 1 },
    ...candidate.tags.map((value) => ({ value, weight: 1 })),
  ].map((signal) => ({ ...signal, value: signal.value.trim() })).filter((signal) => signal.value !== "");
}

function inferCreativeType(text: string, hasRepaymentTable?: boolean): { type: CreativeType; reason: string } | null {
  if (hasRepaymentTable === true || /月供|还款(?:计划|对照|明细|表格|方案)|分期(?:对照|明细|表格|方案|计划)|tabel\s+(?:cicilan|angsuran)|(?:cicilan|angsuran)\s+(?:per\s+bulan|bulanan)|simulasi\s+(?:cicilan|angsuran)|rincian\s+(?:cicilan|angsuran)|repayment\s+(?:plan|schedule|table|comparison)|payment\s+schedule/i.test(text)) {
    return { type: "repayment_plan", reason: "识别到月供表或还款明细结构" };
  }
  if (/额度|金额|数字|limit|jumlah|pinjaman|pencairan|dana|rp\s*[\d.,]+|juta|\bnum\b|快速|申请|放款|cepat|ajukan|mudah|tenor|期限|bunga|interest|免息|0\s*%/i.test(text)) {
    return { type: "num", reason: "未识别月供表，按数字利益点推荐" };
  }
  return null;
}

function signalMatchWeight(signals: RecommendationSignal[], tag: string): number {
  const normalized = tag.trim().toLocaleLowerCase();
  if (normalized === "") return 0;
  return signals.reduce((best, signal) => signal.value.toLocaleLowerCase().includes(normalized) ? Math.max(best, signal.weight) : best, 0);
}

function isExplainableSignal(tag: string): boolean {
  const normalized = tag.trim().toLocaleLowerCase();
  return normalized !== "rp" && normalized !== "num" && !/^\d+m$/.test(normalized) && !/^rp\s*[\d.,]+$/.test(normalized);
}

function financialAmounts(text: string): number[] {
  const amounts: number[] = [];
  for (const match of text.matchAll(/\brp\s*([\d.,]+)/gi)) {
    const amount = Number((match[1] ?? "").replace(/\D/g, ""));
    if (Number.isFinite(amount) && amount >= 1_000_000) amounts.push(amount);
  }
  for (const match of text.matchAll(/\b(\d+(?:[.,]\d+)?)\s*juta\b/gi)) {
    const amount = Number((match[1] ?? "").replace(",", ".")) * 1_000_000;
    if (Number.isFinite(amount) && amount >= 1_000_000) amounts.push(amount);
  }
  return uniqueNumbers(amounts);
}

function closestApprovedPrincipal(
  factKeys: string[],
  facts: Map<string, CreativeProductFact>,
  observedAmounts: number[],
): { score: number; copyText: string; distance: number } | null {
  if (observedAmounts.length === 0) return null;
  const principal = factKeys.map((key) => facts.get(key)).find((fact) => fact && /^principal_/i.test(fact.key));
  if (!principal) return null;
  const value = Number(principal.value.replace(/\D/g, ""));
  if (!Number.isFinite(value) || value <= 0) return null;
  const distance = Math.min(...observedAmounts.map((amount) => Math.abs(Math.log(amount / value))));
  return { score: Math.round(160 / (1 + distance)), copyText: principal.copy_text, distance };
}

function parseFragment(value: unknown): CreativeCopyFragment | null {
  const row = record(value);
  const role = ROLES.find((item) => item === row.role);
  if (!stringValue(row.id) || !stringValue(row.key) || !role) return null;
  return {
    id: stringValue(row.id), key: stringValue(row.key), name: stringValue(row.name), creative_types: stringArray(row.creative_types).map(creativeType).filter((item): item is CreativeType => item !== null),
    role, semantic_group: stringValue(row.semantic_group) || undefined, text: stringValue(row.text), tags: stringArray(row.tags), usage: fragmentUsage(row.usage, role), status: copyStatus(row.status),
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

function parseFact(value: unknown): CreativeProductFact | null {
  const row = record(value);
  if (!stringValue(row.id) || !stringValue(row.key)) return null;
  return { id: stringValue(row.id), key: stringValue(row.key), label: stringValue(row.label), value: stringValue(row.value), copy_text: stringValue(row.copy_text), source: stringValue(row.source), status: copyStatus(row.status) };
}

function record(value: unknown): Record<string, unknown> { return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
function array(value: unknown): unknown[] { return Array.isArray(value) ? value : []; }
function stringValue(value: unknown): string { return typeof value === "string" ? value.trim() : ""; }
function stringArray(value: unknown): string[] { return array(value).filter((item): item is string => typeof item === "string").map((item) => item.trim()).filter(Boolean); }
function numberValue(value: unknown, fallback: number): number { return typeof value === "number" && Number.isFinite(value) ? value : fallback; }
function creativeType(value: unknown): CreativeType | null { return value === "num" || value === "repayment_plan" ? value : null; }
function copyStatus(value: unknown): "draft" | "approved" | "disabled" { return value === "approved" || value === "disabled" ? value : "draft"; }
function fragmentUsage(value: unknown, role: CreativeCopyFragmentRole): CreativeCopyFragment["usage"] {
  if (value === "core" || value === "fallback" || value === "required") return value;
  if (role === "legal") return "required";
  if (role === "supporting" || role === "cta") return "fallback";
  return "core";
}
function unique(values: string[]): string[] { return [...new Set(values)]; }
function uniqueNumbers(values: number[]): number[] { return [...new Set(values)]; }
function uniqueBy<T>(values: T[], key: (value: T) => string): T[] {
  const seen = new Set<string>();
  return values.filter((value) => {
    const identity = key(value);
    if (seen.has(identity)) return false;
    seen.add(identity);
    return true;
  });
}
