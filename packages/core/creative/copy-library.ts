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
  theme: string;
  theme_elements: string[];
  primary_benefit: string;
  secondary_benefits: string[];
  benefit_value: string;
}

export interface CreativeCopyRecommendation {
  recipe: CreativeCopyRecipe;
  snapshot: CreativeCopySnapshot;
  score: number;
  reasons: string[];
  matchedSignals: string[];
  exactTypeMatch: boolean;
}

export interface CustomCopyFinancialFactValidation {
  allowed: boolean;
  unapproved: string[];
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
  const inferredType = inferCreativeType(signals.join(" ")) ?? config.recommendation_policy.default_creative_type;
  const facts = new Map(config.product_facts.filter((fact) => fact.status === "approved").map((fact) => [fact.key, fact]));
  const fragments = new Map(config.fragments.filter((fragment) => fragment.status === "approved").map((fragment) => [fragment.id, fragment]));
  return config.recipes
    .filter((recipe) => recipe.status === "approved")
    .flatMap((recipe) => {
      const exactTypeMatch = recipe.creative_type === inferredType;
      const matchedSignals = recipe.match_tags.filter((tag) => signalMatches(signals, tag));
      const resolved = resolveRecipe(recipe, fragments, facts);
      if (resolved.missingFactKeys.length > 0) return [];
      const concise = Math.max(0, 240 - resolved.visibleLength);
      const score = (exactTypeMatch ? config.recommendation_policy.type_weight : 0)
        + matchedSignals.length * config.recommendation_policy.tag_weight
        + concise * config.recommendation_policy.concise_weight;
      const reasons = [
        ...(exactTypeMatch ? [`创意类型：${creativeTypeLabel(recipe.creative_type)}`] : []),
        ...matchedSignals.slice(0, 3).map((signal) => `匹配信号：${signal}`),
        ...(resolved.factKeys.length > 0 ? [`产品事实：${resolved.factKeys.join("、")}`] : []),
      ];
      const libraryVersion = resource.published_version || resource.version;
      const snapshot: CreativeCopySnapshot = {
        schema_version: 2,
        id: recipe.id,
        library_id: resource.id,
        library_version: libraryVersion,
        recipe_id: recipe.id,
        recipe_key: recipe.key,
        creative_type: recipe.creative_type,
        headline: resolved.byRole.headline,
        subheadline: resolved.byRole.subheadline,
        benefit: resolved.byRole.benefit,
        supporting: resolved.byRole.supporting,
        cta: resolved.byRole.cta,
        legal_text: resolved.byRole.legal,
        fragments: resolved.fragments,
        product_facts: resolved.factKeys.map((key) => facts.get(key)).filter((fact): fact is CreativeProductFact => fact !== undefined)
          .map(({ key, label, value, copy_text, source }) => ({ key, label, value, copy_text, source })),
        recommendation: { score, reasons, matched_signals: matchedSignals },
        status: "approved",
      };
      return [{ recipe, snapshot, score, reasons, matchedSignals, exactTypeMatch }];
    })
    .sort((left, right) => Number(right.exactTypeMatch) - Number(left.exactTypeMatch) || right.score - left.score || left.recipe.key.localeCompare(right.recipe.key));
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
  if (observedTokens.length === 0) return { allowed: true, unapproved: [], message: "" };
  if (!resource || resource.published_version < 1) {
    return {
      allowed: false,
      unapproved: [],
      message: "当前市场规则未绑定已发布文案库，不能验证自定义文案中的金融事实。",
    };
  }
  const config = parseCreativeCopyLibraryConfig(resource.published_config ?? {});
  const allowedTokens = new Set(
    config.product_facts
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
  );
  const unapproved = unique(observedTokens
    .filter((token) => !allowedTokens.has(token.normalized))
    .map((token) => token.display));
  return {
    allowed: unapproved.length === 0,
    unapproved,
    message: unapproved.length === 0 ? "" : `自定义文案包含未审核金融事实：${unapproved.join("、")}。请改用已审核产品事实，或在文案库中审核后再发布。`,
  };
}

function resolveRecipe(recipe: CreativeCopyRecipe, fragments: Map<string, CreativeCopyFragment>, facts: Map<string, CreativeProductFact>) {
  const byRole = { headline: "", subheadline: "", benefit: "", supporting: "", cta: "", legal: "" };
  const used: CreativeCopySnapshot["fragments"] = [];
  const factKeys = new Set<string>();
  const missingFactKeys = new Set<string>();
  for (const role of ROLES) {
    const lines = (recipe.fragment_ids[role] ?? []).map((id) => fragments.get(id)).filter((item): item is CreativeCopyFragment => item !== undefined)
      .map((fragment) => {
        const text = interpolateFacts(fragment.text, facts, factKeys, missingFactKeys);
        used.push({ id: fragment.id, key: fragment.key, role: fragment.role, text });
        return text;
      }).filter(Boolean);
    byRole[role] = lines.join("\n");
  }
  return { byRole, fragments: used, factKeys: [...factKeys], missingFactKeys: [...missingFactKeys], visibleLength: Object.values(byRole).join("").length };
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

type FinancialFactToken = { normalized: string; display: string };

function financialFactTokens(text: string): FinancialFactToken[] {
  const tokens: FinancialFactToken[] = [];
  const add = (kind: string, value: string, display: string) => {
    const normalizedValue = value.trim();
    if (normalizedValue) tokens.push({ normalized: `${kind}:${normalizedValue}`, display: display.trim() });
  };
  for (const match of text.matchAll(/\bRp\s*\d{1,3}(?:[.,]\d{3})*(?:[.,]\d+)?/gi)) {
    add("currency", match[0].replace(/\D/g, ""), match[0]);
  }
  for (const match of text.matchAll(/\b\d+(?:[.,]\d+)?\s*%/g)) {
    add("percent", match[0].replace(/\s/g, "").replace(",", "."), match[0]);
  }
  for (const match of text.matchAll(/\b\d+(?:\s*-\s*\d+)?\s*(?:bulan|hari|tahun)\b/gi)) {
    add("term", match[0].replace(/\s/g, "").toLocaleLowerCase(), match[0]);
  }
  for (const match of text.matchAll(/\b(?:limit|pinjaman|dana|jumlah|cicilan|angsuran|tenor|bunga|interest|biaya|fee)\D{0,24}(\d{1,3}(?:[.,]\d{3})+|\d+)(?:\s*(?:juta|ribu|miliar))?/gi)) {
    const numeric = match[1]?.replace(/\D/g, "") ?? "";
    add("financial_number", numeric, match[0]);
  }
  return tokens;
}

function recommendationSignals(candidate: CreativeMaterialCandidate, brief: CreativeCopyRecommendationBrief): string[] {
  return [brief.primary_benefit, ...brief.secondary_benefits, brief.benefit_value, brief.theme, ...brief.theme_elements, candidate.title, ...candidate.tags]
    .map((value) => value.trim()).filter(Boolean);
}

function inferCreativeType(text: string): CreativeType | null {
  if (/还款|分期|期限|月供|cicilan|angsuran|tenor|repayment|pelunasan|bunga|interest/i.test(text)) return "repayment_plan";
  if (/额度|金额|数字|limit|jumlah|pinjaman|pencairan|dana|rp\s*[\d.,]+|juta|\bnum\b/i.test(text)) return "num";
  return null;
}

function signalMatches(signals: string[], tag: string): boolean {
  const normalized = tag.trim().toLocaleLowerCase();
  return normalized !== "" && signals.some((signal) => signal.toLocaleLowerCase().includes(normalized));
}

function parseFragment(value: unknown): CreativeCopyFragment | null {
  const row = record(value);
  const role = ROLES.find((item) => item === row.role);
  if (!stringValue(row.id) || !stringValue(row.key) || !role) return null;
  return {
    id: stringValue(row.id), key: stringValue(row.key), name: stringValue(row.name), creative_types: stringArray(row.creative_types).map(creativeType).filter((item): item is CreativeType => item !== null),
    role, text: stringValue(row.text), tags: stringArray(row.tags), status: copyStatus(row.status),
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
function unique(values: string[]): string[] { return [...new Set(values)]; }
