import type { CreativeCopyEntry, CreativeMaterialCandidate } from "@multica/core/types";

export interface CopyRecommendationBrief {
  theme: string;
  theme_elements: string[];
  primary_benefit: string;
  secondary_benefits: string[];
  benefit_value: string;
}

const EMPTY_BRIEF: CopyRecommendationBrief = {
  theme: "",
  theme_elements: [],
  primary_benefit: "",
  secondary_benefits: [],
  benefit_value: "",
};

const COPY_INTENT_RULES = [
  { key: "repayment_plan", label: "还款/分期", pattern: /还款|分期|期限|cicilan|angsuran|tenor|repayment|pelunasan/i },
  { key: "rate_down", label: "利率", pattern: /低利率|降息|利率|bunga|suku bunga|interest rate|rate down|0[,.]0\d\s*%/i },
  { key: "interest_free", label: "免息", pattern: /免息|0\s*%\s*(?:bunga|interest)|bebas bunga|interest free/i },
  { key: "fee_reduction", label: "费用减免", pattern: /降费|费用减免|减免|biaya|uang muka|fee|potongan/i },
  { key: "limit_amount", label: "额度", pattern: /额度|limit|jumlah pinjaman|pencairan|dana|rp\s*\d|juta/i },
  { key: "fast_disbursement", label: "快速放款", pattern: /快速放款|到账|秒批|cepat cair|pencairan cepat|cair dalam|menit/i },
  { key: "easy_application", label: "低门槛", pattern: /低门槛|易申请|mudah|tanpa jaminan|cukup ktp|syarat/i },
  { key: "early_repayment", label: "提前还款", pattern: /提前还款|early repayment|pelunasan lebih awal/i },
  { key: "app_interface", label: "App 界面", pattern: /app|phone|screen|interface|halaman|beranda|whatsapp/i },
  { key: "comparison", label: "对比", pattern: /comparison|perbandingan|bandingkan/i },
  { key: "lifestyle_scenario", label: "生活场景", pattern: /生活场景|消费场景|lifestyle|consumption scenario|kebutuhan sehari/i },
] as const;

const COPY_THEME_RULES = [
  { key: "football", label: "足球赛事", pattern: /世界杯|足球|球场|world cup|piala dunia|sepak bola|lapangan|\bbola\b|\bgol\b/i },
  { key: "ramadan", label: "斋月", pattern: /斋月|ramadan|ramadhan|puasa/i },
  { key: "eid", label: "开斋节", pattern: /开斋节|lebaran|idul fitri|\beid\b/i },
  { key: "payday", label: "发薪日", pattern: /发薪日|payday|gajian|tanggal gajian/i },
  { key: "school", label: "开学季", pattern: /开学|学校|sekolah|school|tahun ajaran/i },
  { key: "year_end", label: "年末", pattern: /年末|年底|akhir tahun|year end/i },
] as const;

const CONTENT_KEYWORD_INTENTS: Record<string, string> = {
  "REPAYMENT PLAN": "repayment_plan",
  "RATE DOWN": "rate_down",
  "30D INTEREST FREE": "interest_free",
  "7D INTEREST FREE": "interest_free",
  "INTEREST FREE": "interest_free",
  "0% UANG MUKA": "fee_reduction",
  NUM: "limit_amount",
  "NUM GROWTH": "limit_amount",
  "EARLY REPAYMENT": "early_repayment",
  "PHONE TYPE": "app_interface",
  PHONE: "app_interface",
  "APP STORE PAGES": "app_interface",
  "PROGRESS BAR": "app_interface",
  "USER INFORMATION": "app_interface",
  CALCULATOR: "app_interface",
  WHATSAPP: "app_interface",
  "E-WALLET": "app_interface",
  COMPARISON: "comparison",
  "CONSUMPTION SCENARIOS": "lifestyle_scenario",
};

export function recommendCopyEntries<T extends CopyRecommendationBrief = CopyRecommendationBrief>(
  candidate: CreativeMaterialCandidate,
  entries: CreativeCopyEntry[],
  brief: T = EMPTY_BRIEF as T,
) {
  const hasPrimaryBenefit = Boolean(brief.primary_benefit.trim());
  const primaryText = (brief.primary_benefit.trim() || brief.benefit_value).toLocaleLowerCase();
  const secondaryText = brief.secondary_benefits.join(" ").toLocaleLowerCase();
  const primaryIntents = COPY_INTENT_RULES.filter((rule) => rule.pattern.test(primaryText) || primaryText.includes(rule.key));
  const secondaryIntents = COPY_INTENT_RULES.filter((rule) => rule.pattern.test(secondaryText) || secondaryText.includes(rule.key));
  const benefitTokens = signalTokens([brief.primary_benefit, ...brief.secondary_benefits, brief.benefit_value].join(" "));
  const requestedThemes = COPY_THEME_RULES.filter((rule) => rule.pattern.test([brief.theme, ...brief.theme_elements].join(" ")));
  const fallbackText = [candidate.title, ...candidate.tags, ...candidate.media_names].join(" ").toLocaleLowerCase();
  const fallbackIntents = hasPrimaryBenefit ? [] : COPY_INTENT_RULES.filter((rule) => rule.pattern.test(fallbackText));

  return entries
    .filter((entry) => entry.status === "approved" && copyEntryBelongsToCandidate(entry, candidate.id))
    .map((entry) => {
      const identityText = [entry.headline, entry.subheadline, entry.copy_role, ...entry.tags].join(" ").toLocaleLowerCase();
      const entryText = [identityText, entry.benefit, entry.cta].join(" ").toLocaleLowerCase();
      const primaryIntent = copyEntryPrimaryIntent(entry, identityText);
      const entryIntents = new Set([primaryIntent, ...metadataStrings(entry.metadata, "secondary_intents")].filter(Boolean));
      const exactPrimaryMatch = primaryIntents.some((rule) => entryIntents.has(rule.key));
      const matchedSecondary = secondaryIntents.filter((rule) => entryIntents.has(rule.key));
      const matchedFallback = fallbackIntents.filter((rule) => entryIntents.has(rule.key));
      const entryThemes = new Set([
        ...metadataStrings(entry.metadata, "theme_tags"),
        ...COPY_THEME_RULES.filter((rule) => rule.pattern.test(identityText)).map((rule) => rule.key),
      ]);
      const matchedThemes = requestedThemes.filter((rule) => entryThemes.has(rule.key));
      const benefitOverlap = benefitTokens.filter((token) => entryText.includes(token)).slice(0, 3);
      const conciseScore = Math.max(0, 8 - Math.round([entry.headline, entry.subheadline, entry.benefit, entry.cta].join(" ").length / 50));
      const score = (exactPrimaryMatch ? 1000 : 0) + matchedSecondary.length * 180 + matchedThemes.length * 45 + benefitOverlap.length * 12 + matchedFallback.length * 30 + conciseScore;
      const reasons = hasPrimaryBenefit
        ? [
            ...(exactPrimaryMatch ? primaryIntents.filter((rule) => entryIntents.has(rule.key)).map((rule) => `主利益点：${rule.label}`) : []),
            ...matchedSecondary.map((rule) => `辅助利益点：${rule.label}`),
            ...matchedThemes.map((rule) => `主题：${rule.label}`),
            ...benefitOverlap.map((token) => `关键值：${token}`),
          ].slice(0, 4)
        : matchedFallback.map((rule) => `采集信息兜底：${rule.label}`).slice(0, 2);
      return { entry, score, reasons, exactPrimaryMatch };
    })
    .sort((left, right) => Number(right.exactPrimaryMatch) - Number(left.exactPrimaryMatch) || right.score - left.score || Date.parse(right.entry.updated_at) - Date.parse(left.entry.updated_at) || left.entry.external_key.localeCompare(right.entry.external_key));
}

function copyEntryBelongsToCandidate(entry: CreativeCopyEntry, candidateId: string) {
  if (entry.copy_role !== "issue_override") return true;
  return metadataString(entry.metadata, "candidate_id") === candidateId;
}

function copyEntryPrimaryIntent(entry: CreativeCopyEntry, identityText: string) {
  const contentKeyword = metadataString(entry.metadata, "content_keyword").toUpperCase();
  const keywordIntent = CONTENT_KEYWORD_INTENTS[contentKeyword];
  if (keywordIntent) return keywordIntent;
  const declared = normalizeCopyIntent(metadataString(entry.metadata, "primary_intent") || metadataString(entry.metadata, "suggested_copy_type"));
  if (declared) return declared;
  return COPY_INTENT_RULES.find((rule) => rule.pattern.test(identityText))?.key ?? "";
}

function normalizeCopyIntent(value: string) {
  const normalized = value.trim().toLocaleLowerCase().replaceAll(" ", "_");
  if (normalized === "limit_or_amount") return "limit_amount";
  if (normalized === "user_interface") return "app_interface";
  return normalized;
}

function metadataString(metadata: CreativeCopyEntry["metadata"], key: string) {
  const value = metadata && typeof metadata === "object" ? metadata[key] : undefined;
  return typeof value === "string" ? value.trim() : "";
}

function metadataStrings(metadata: CreativeCopyEntry["metadata"], key: string) {
  const value = metadata && typeof metadata === "object" ? metadata[key] : undefined;
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string").map(normalizeCopyIntent) : [];
}

function signalTokens(value: string) {
  const stop = new Set(["adakami", "easycash", "kredit", "pintar", "adapundi", "bantusaku", "rupiah", "cepat", "julo", "indonesia", "image", "video"]);
  return [...new Set(value.toLocaleLowerCase().split(/[^\p{L}\p{N}%]+/u).filter((token) => token.length >= 4 && !stop.has(token)))];
}
