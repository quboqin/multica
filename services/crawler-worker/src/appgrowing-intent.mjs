export class ConnectorInputError extends Error {
  constructor(message, statusCode = 400) {
    super(message);
    this.name = "ConnectorInputError";
    this.statusCode = statusCode;
  }
}

// Legacy AutoPilots predate structured selection_rules. Keep the established
// weekly split at the connector boundary so they remain executable, while new
// callers receive the same explicit structure in their normalized params.
export const DEFAULT_MATERIAL_SELECTION_RULES = Object.freeze({
  new_materials: Object.freeze({
    ratio: 0.4,
    duration_days_lt: 7,
    impression_gt: 1_000,
  }),
  volume_materials: Object.freeze({
    ratio: 0.6,
    duration_days_gt: 30,
    impression_gte: 10_000_000,
  }),
});

export const DEFAULT_MATERIAL_SEARCH_LIMIT = 5;

export function normalizeAppGrowingMaterialSearchParams(params = {}) {
  if (!params || typeof params !== "object" || Array.isArray(params)) {
    return {};
  }
  const intent = typeof params.intent === "string" ? params.intent.trim() : "";
  if (!intent) {
    return normalizeStructuredMaterialSearchFilters(withDefaultMaterialSelectionRules({ ...params }));
  }
  const explicit = { ...params };
  delete explicit.intent;
  return normalizeStructuredMaterialSearchFilters(
    withDefaultMaterialSelectionRules(mergeMaterialSearchParams(parseAppGrowingMaterialSearchIntent(intent), explicit)),
  );
}

export function parseAppGrowingMaterialSearchIntent(intent) {
  const text = normalizeIntentText(intent);
  if (!text) {
    throw new ConnectorInputError("material_search intent must not be empty");
  }
  const parsed = {};
  const competitors = extractCompetitors(text);
  if (competitors.length > 0) {
    parsed.competitors = competitors;
  }
  const competitorURLs = extractCompetitorURLs(text, competitors);
  if (Object.keys(competitorURLs).length > 0) {
    parsed.competitor_urls = competitorURLs;
  }
  const priorityCompetitors = extractPriorityCompetitors(text);
  if (priorityCompetitors.length > 0) {
    parsed.priority_competitors = priorityCompetitors;
  }
  const rules = extractMaterialRules(text);
  if (Object.keys(rules).length > 0) {
    parsed.selection_rules = rules;
  }
  Object.assign(parsed, extractMaterialFilters(text));
  const limit = extractInteger(text, /(?:limit|数量|素材数|最多|选取|输出)\D{0,12}(\d{1,3})/i);
  if (limit > 0) {
    parsed.limit = limit;
  }
  const pageLimit = extractInteger(text, /(?:pages_per_competitor|每个竞品页数|每家页数|普通页数)\D{0,12}(\d{1,2})/i);
  if (pageLimit > 0) {
    parsed.pages_per_competitor = pageLimit;
  }
  const priorityPageLimit = extractInteger(text, /(?:priority_pages_per_competitor|重点页数|优先页数)\D{0,12}(\d{1,2})/i);
  if (priorityPageLimit > 0) {
    parsed.priority_pages_per_competitor = priorityPageLimit;
  }
  const captureTimeoutMS = extractDurationMS(text, /(?:capture_timeout_ms|抓取超时|采集超时)\D{0,12}(\d+(?:\.\d+)?)\s*(ms|毫秒|s|秒|m|分钟)?/i);
  if (captureTimeoutMS > 0) {
    parsed.capture_timeout_ms = captureTimeoutMS;
  }
  const dateRange = text.match(/(?:date_range|daterange|日期范围|时间范围)\s*[:：=]?\s*(-?\d+\s*,\s*-?\d+)/i);
  if (dateRange) {
    parsed.date_range = dateRange[1].replace(/\s+/g, "");
  } else {
    const recentDays = extractRecentDaysRange(text);
    if (recentDays) {
      parsed.date_range = `-${recentDays - 1},0`;
    }
  }
  const purpose = extractInteger(text, /(?:purpose|用途)\s*[:：=]?\s*(\d+)/i);
  if (purpose > 0) {
    parsed.purpose = purpose;
  }
  if (/(兜底|fallback|没有命中.*热门|没命中.*热门|top\s*素材)/i.test(text)) {
    parsed.fallback_to_top_materials = true;
  }
  return parsed;
}

export function normalizeMaterialRules(value) {
  if (value === undefined) {
    value = defaultMaterialSelectionRules();
  }
  if (typeof value !== "object" || Array.isArray(value)) {
    throw new ConnectorInputError("material_search params.selection_rules is required");
  }
  const newRules = normalizeMaterialRuleAliases(value.new_materials, "new_materials");
  const volumeRules = normalizeMaterialRuleAliases(value.volume_materials, "volume_materials");
  const rules = {
    new_materials: {
      ratio: finiteNumberParam(newRules.ratio, NaN, 0, 1),
      duration_days_lt: optionalFiniteNumberParam(newRules.duration_days_lt, 0, 3650),
      duration_days_lte: optionalFiniteNumberParam(newRules.duration_days_lte, 0, 3650),
      impression_gt: optionalFiniteNumberParam(newRules.impression_gt, 0, 1_000_000_000_000),
      impression_gte: optionalFiniteNumberParam(newRules.impression_gte, 0, 1_000_000_000_000),
    },
    volume_materials: {
      ratio: finiteNumberParam(volumeRules.ratio, NaN, 0, 1),
      duration_days_gt: optionalFiniteNumberParam(volumeRules.duration_days_gt, 0, 3650),
      duration_days_gte: optionalFiniteNumberParam(volumeRules.duration_days_gte, 0, 3650),
      impression_gt: optionalFiniteNumberParam(volumeRules.impression_gt, 0, 1_000_000_000_000),
      impression_gte: optionalFiniteNumberParam(volumeRules.impression_gte, 0, 1_000_000_000_000),
    },
  };
  if (!Number.isFinite(rules.new_materials.ratio)
    || !hasUpperBound(rules.new_materials, "duration_days")
    || !hasLowerBound(rules.new_materials, "impression")
    || !Number.isFinite(rules.volume_materials.ratio)
    || !hasLowerBound(rules.volume_materials, "duration_days")
    || !hasLowerBound(rules.volume_materials, "impression")) {
    const missing = missingMaterialRuleFields(rules);
    throw new ConnectorInputError(
      `material_search selection_rules is incomplete: ${missing.join(", ")}`,
    );
  }
  return rules;
}

function withDefaultMaterialSelectionRules(params) {
  if (Object.prototype.hasOwnProperty.call(params, "selection_rules")
    || Object.prototype.hasOwnProperty.call(params, "rules")) {
    return params;
  }
  return {
    ...params,
    selection_rules: defaultMaterialSelectionRules(),
  };
}

function defaultMaterialSelectionRules() {
  return {
    new_materials: { ...DEFAULT_MATERIAL_SELECTION_RULES.new_materials },
    volume_materials: { ...DEFAULT_MATERIAL_SELECTION_RULES.volume_materials },
  };
}

function normalizeStructuredMaterialSearchFilters(params) {
  const out = { ...params };
  if (out.limit === undefined || out.limit === null || out.limit === "") {
    const limitAlias = firstDefined(
      out.max_results,
      out.max_materials,
      out.max_outputs,
      out.max_output,
      out.output_limit,
      out.target_count,
      out.material_limit,
    );
    if (limitAlias !== undefined) {
      out.limit = limitAlias;
    } else {
      delete out.limit;
    }
  }
  if (typeof out.daterange !== "string" || !out.daterange.trim()) {
    const days = recentDaysFromDateRange(out.date_range);
    if (days !== null) {
      out.daterange = `-${Math.max(0, days - 1)},0`;
    }
  }

  const areas = normalizeAreaFilters(structuredFilterValues(
    firstDefined(out.area, out.areas, out.regions, out.region),
  ));
  if (areas.length > 0) {
    out.area = areas;
  }
  const languages = normalizeLanguageFilters(structuredFilterValues(
    firstDefined(out.language, out.languages),
  ));
  if (languages.length > 0) {
    out.language = languages;
  }
  const platforms = normalizePlatformFilters(structuredFilterValues(
    firstDefined(out.platform, out.platforms, out.device, out.devices),
  ));
  if (platforms.length > 0) {
    out.platform = platforms;
  }
  return out;
}

function recentDaysFromDateRange(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  const type = String(value.type || "").trim().toLowerCase();
  const days = Number(value.days);
  if ((type && type !== "recent_days") || !Number.isInteger(days) || days < 1 || days > 3650) {
    return null;
  }
  return days;
}

function structuredFilterValues(value) {
  if (Array.isArray(value)) {
    return value.flatMap(structuredFilterValues);
  }
  return typeof value === "string" ? value.split(/[、,，;；]/) : [value];
}

function missingMaterialRuleFields(rules) {
  const missing = [];
  if (!Number.isFinite(rules.new_materials.ratio)) {
    missing.push("new_materials.ratio");
  }
  if (!hasUpperBound(rules.new_materials, "duration_days")) {
    missing.push("new_materials.duration_days_lt|duration_days_lte");
  }
  if (!hasLowerBound(rules.new_materials, "impression")) {
    missing.push("new_materials.impression_gt|impression_gte");
  }
  if (!Number.isFinite(rules.volume_materials.ratio)) {
    missing.push("volume_materials.ratio");
  }
  if (!hasLowerBound(rules.volume_materials, "duration_days")) {
    missing.push("volume_materials.duration_days_gt|duration_days_gte");
  }
  if (!hasLowerBound(rules.volume_materials, "impression")) {
    missing.push("volume_materials.impression_gt|impression_gte");
  }
  return missing;
}

function normalizeMaterialRuleAliases(value, segment) {
  const source = objectOrEmpty(value);
  const out = { ...source };
  const isNewMaterials = segment === "new_materials";

  setIfMissing(out, "ratio", normalizedRatio(firstDefined(
    source.ratio,
    source.allocation_ratio,
    source.ratio_pct,
    source.share_pct,
    source.ratio_percent,
    source.percentage,
    source.share,
  ), ratioAliasUsesPercent(source)));

  if (isNewMaterials && !hasUpperBound(out, "duration_days")) {
    setIfMissing(out, "duration_days_lt", firstDefined(
      source.duration_lt_days,
      source.duration_max_days,
      source.max_duration_days,
      objectValue(source.duration_days, "lt", "max"),
      objectValue(source.ad_days, "lt", "max"),
    ));
  } else if (!isNewMaterials && !hasLowerBound(out, "duration_days")) {
    setIfMissing(out, "duration_days_gt", firstDefined(
      source.duration_gt_days,
      source.duration_min_days,
      source.min_duration_days,
      objectValue(source.duration_days, "gt", "min"),
      objectValue(source.ad_days, "gt", "min"),
    ));
  }

  const impression = firstDefined(
    source.impressions_gt,
    source.impressions_gte,
    source.estimated_impressions_gt,
    source.estimated_impressions_gte,
    source.impressions_min,
    source.min_estimated_impressions,
    objectValue(source.estimated_impressions, "gt", "gte", "min"),
    objectValue(source.impression_threshold, "gt", "gte", "min"),
  );
  if (isNewMaterials && !hasLowerBound(out, "impression")) {
    setIfMissing(out, "impression_gt", impression);
  } else if (!isNewMaterials && !hasLowerBound(out, "impression")) {
    setIfMissing(out, "impression_gte", impression);
  }
  return out;
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function objectValue(value, ...keys) {
  const source = objectOrEmpty(value);
  return firstDefined(...keys.map((key) => source[key]));
}

function setIfMissing(target, key, value) {
  if (target[key] === undefined && value !== undefined) {
    target[key] = value;
  }
}

function ratioAliasUsesPercent(source) {
  return source.ratio_pct !== undefined
    || source.share_pct !== undefined
    || source.ratio_percent !== undefined
    || source.percentage !== undefined
    || (source.share !== undefined && Number(source.share) > 1);
}

function normalizedRatio(value, isPercent) {
  if (!isPercent || value === undefined || value === null) {
    return value;
  }
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric / 100 : value;
}

function mergeMaterialSearchParams(parsed, explicit) {
  const merged = { ...parsed, ...explicit };
  const parsedRules = objectOrEmpty(parsed.selection_rules || parsed.rules);
  const explicitRules = objectOrEmpty(explicit.selection_rules || explicit.rules);
  if (Object.keys(parsedRules).length > 0 || Object.keys(explicitRules).length > 0) {
    merged.selection_rules = {
      ...parsedRules,
      ...explicitRules,
      new_materials: {
        ...objectOrEmpty(parsedRules.new_materials),
        ...objectOrEmpty(explicitRules.new_materials),
      },
      volume_materials: {
        ...objectOrEmpty(parsedRules.volume_materials),
        ...objectOrEmpty(explicitRules.volume_materials),
      },
    };
    delete merged.rules;
  }
  return merged;
}

function objectOrEmpty(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function normalizeIntentText(intent) {
  return String(intent || "")
    .replace(/\r\n/g, "\n")
    .replace(/\u00a0/g, " ")
    .trim();
}

function extractCompetitors(text) {
  return uniqueNames(extractListAfterLabels(text, ["直接竞品", "竞品池", "竞品", "competitors"]));
}

function extractPriorityCompetitors(text) {
  return uniqueNames(extractListAfterLabels(text, ["重点竞品", "优先竞品", "priority_competitors", "priority competitors", "优先", "重点"]));
}

function extractCompetitorURLs(text, competitors) {
  const out = {};
  const known = new Map((competitors || []).map((name) => [name.toLowerCase(), name]));
  for (const line of text.split("\n")) {
    const match = line.match(/https?:\/\/[^\s。)）]+/i);
    if (!match) {
      continue;
    }
    const rawURL = match[0].replace(/[）)。,，]+$/, "");
    const before = line.slice(0, match.index).replace(/(?:品牌页|搜索页|页面|链接|url|URL)\s*[:：=]?\s*$/i, "");
    let name = cleanName(before);
    if (!name || !/[A-Za-z0-9]/.test(name)) {
      name = competitorNameInText(before, known);
    }
    if (!name) {
      name = competitorNameInText(line, known);
    }
    if (name && /appgrowing-global\.youcloud\.com/i.test(rawURL)) {
      out[name] = rawURL;
    }
  }
  return out;
}

function competitorNameInText(text, known) {
  const lower = String(text || "").toLowerCase();
  for (const [key, name] of known.entries()) {
    if (lower.includes(key)) {
      return name;
    }
  }
  return "";
}

function extractListAfterLabels(text, labels) {
  const lines = text.split("\n").map((line) => line.trim()).filter(Boolean);
  const names = [];
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const match = findLabel(line, labels);
    if (!match) {
      continue;
    }
    let content = listContentAroundLabel(line, match);
    if (!content || splitNameList(content).length === 0) {
      const following = [];
      for (let next = index + 1; next < lines.length && following.length < 4; next += 1) {
        if (isMaterialSearchHeading(lines[next])) {
          break;
        }
        following.push(lines[next]);
      }
      content = following.join("\n");
    }
    names.push(...splitNameList(content));
  }
  return names;
}

function findLabel(line, labels) {
  const lower = line.toLowerCase();
  for (const label of labels) {
    const at = lower.indexOf(label.toLowerCase());
    if (at >= 0) {
      return { label, at };
    }
  }
  return null;
}

function listContentAroundLabel(line, match) {
  const before = line.slice(0, match.at).trim();
  const after = line.slice(match.at + match.label.length).trim();
  const afterDelimiter = after.replace(/^[\s:：=，,、-]+/, "").trim();
  if (afterDelimiter && /[A-Za-z0-9]/.test(afterDelimiter)) {
    return afterDelimiter;
  }
  if (/[、,，;；]/.test(before)) {
    return before;
  }
  return "";
}

function isMaterialSearchHeading(line) {
  return /^(筛选|规则|输出|日期|时间|新素材|跑量素材|purpose|limit|数量|下载|素材|媒体|投放地区|地区|国家|语言|设备|平台|media|area|region|country|language|platform|device)/i.test(line.trim());
}

function splitNameList(content) {
  return String(content || "")
    .replace(/https?:\/\/[^\s。)）]+/gi, "")
    .split(/[、,，;；\n]+|\s+(?:and|or)\s+/i)
    .map(cleanName)
    .filter((name) => /[A-Za-z0-9]/.test(name) && name.length <= 80);
}

function cleanName(value) {
  return String(value || "")
    .replace(/^[\s\-*•·:：=]+/, "")
    .replace(/^(包括|为|是|其次|以及|和)\s*/i, "")
    .replace(/\s*(?:是)?(?:重点|优先|直接)?竞品.*$/i, "")
    .replace(/\s*(?:优先|重点|为主|更新很快|素材不错).*$/i, "")
    .replace(/[\s:：=]+$/i, "")
    .replace(/\s*(?:品牌页|搜索页|页面|链接|url|URL)$/i, "")
    .trim();
}

function uniqueNames(values) {
  const seen = new Set();
  const out = [];
  for (const value of values) {
    const key = value.toLowerCase();
    if (!key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(value);
  }
  return out;
}

function extractMaterialFilters(text) {
  const out = {};
  const media = normalizeMediaFilters(extractFilterValuesAfterLabels(text, ["媒体", "投放媒体", "广告媒体", "media"]));
  if (media.ids.length > 0) {
    out.media = media.ids;
  }
  if (media.names.length > 0) {
    out.media_names = media.names;
  }

  const areas = normalizeAreaFilters(extractFilterValuesAfterLabels(text, ["投放地区", "地区", "国家", "area", "region", "country"]));
  if (areas.length > 0) {
    out.area = areas;
  }

  const languages = normalizeLanguageFilters(extractFilterValuesAfterLabels(text, ["投放语言", "语言", "language"]));
  if (languages.length > 0) {
    out.language = languages;
  }

  const platforms = normalizePlatformFilters(extractFilterValuesAfterLabels(text, ["设备类型", "投放设备", "设备", "platform", "device"]));
  if (platforms.length > 0) {
    out.platform = platforms;
  }

  return out;
}

function extractFilterValuesAfterLabels(text, labels) {
  const lines = text.split("\n").map((line) => line.trim()).filter(Boolean);
  const values = [];
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const match = findLabel(line, labels);
    if (!match) {
      continue;
    }
    let content = filterContentAroundLabel(line, match);
    if (!content || splitFilterValueList(content).length === 0) {
      const following = [];
      for (let next = index + 1; next < lines.length && following.length < 4; next += 1) {
        if (isMaterialSearchHeading(lines[next])) {
          break;
        }
        following.push(lines[next]);
      }
      content = following.join("\n");
    }
    values.push(...splitFilterValueList(content));
  }
  return values;
}

function filterContentAroundLabel(line, match) {
  const before = line.slice(0, match.at).trim();
  const after = line.slice(match.at + match.label.length).trim();
  const afterDelimiter = after.replace(/^[\s:：=，,、-]+/, "").trim();
  if (afterDelimiter) {
    return afterDelimiter;
  }
  if (/[、,，;；]/.test(before)) {
    return before;
  }
  return "";
}

function splitFilterValueList(content) {
  return String(content || "")
    .replace(/https?:\/\/[^\s。)）]+/gi, "")
    .split(/[、,，;；\n]+|\s+(?:and|or)\s+/i)
    .map(cleanFilterValue)
    .filter(Boolean);
}

function cleanFilterValue(value) {
  return String(value || "")
    .replace(/^[\s\-*•·:：=]+/, "")
    .replace(/[\s。；;，,、]+$/g, "")
    .trim();
}

function normalizeMediaFilters(values) {
  const ids = [];
  const names = [];
  for (const value of values) {
    const raw = String(value || "").trim();
    if (!raw) {
      continue;
    }
    if (/^\d+$/.test(raw)) {
      ids.push(Number.parseInt(raw, 10));
      continue;
    }
    const mapped = appGrowingMediaIDs(raw);
    if (mapped.length > 0) {
      ids.push(...mapped);
    } else {
      names.push(raw);
    }
  }
  return {
    ids: uniqueNumbers(ids),
    names: uniqueNames(names),
  };
}

function appGrowingMediaIDs(value) {
  const normalized = normalizeAlias(value);
  const known = [
    [["facebook"], [2]],
    [["instagram"], [1]],
    [["facebookfan", "fan", "facebookaudiencenetwork"], [10]],
    [["admob"], [4]],
    [["applovin"], [8]],
    [["mintegral"], [22]],
    [["messenger"], [16]],
    [["meta", "metaads"], [1, 2, 10, 16]],
  ];
  for (const [aliases, ids] of known) {
    if (aliases.includes(normalized)) {
      return ids;
    }
  }
  return [];
}

function normalizeAreaFilters(values) {
  return uniqueStrings(values.map((value) => {
    const raw = String(value || "").trim();
    const normalized = normalizeAlias(raw);
    const known = {
      id: "ID",
      indonesia: "ID",
      indonesian: "ID",
      印尼: "ID",
      印度尼西亚: "ID",
      my: "MY",
      malaysia: "MY",
      malaysian: "MY",
      马来: "MY",
      马来西亚: "MY",
      ph: "PH",
      philippines: "PH",
      菲律宾: "PH",
      th: "TH",
      thailand: "TH",
      泰国: "TH",
      vn: "VN",
      vietnam: "VN",
      越南: "VN",
      sg: "SG",
      singapore: "SG",
      新加坡: "SG",
    };
    if (known[normalized]) {
      return known[normalized];
    }
    if (/^[a-zA-Z]{2}$/.test(raw)) {
      return raw.toUpperCase();
    }
    return "";
  }).filter(Boolean));
}

function normalizeLanguageFilters(values) {
  return uniqueStrings(values.map((value) => {
    const raw = String(value || "").trim();
    const normalized = normalizeAlias(raw);
    const known = {
      id: "id",
      indonesian: "id",
      bahasaindonesia: "id",
      印尼语: "id",
      印度尼西亚语: "id",
      en: "en",
      english: "en",
      英语: "en",
      ms: "ms",
      malay: "ms",
      马来语: "ms",
      zh: "zh",
      chinese: "zh",
      中文: "zh",
      ja: "ja",
      japanese: "ja",
      日语: "ja",
    };
    if (known[normalized]) {
      return known[normalized];
    }
    if (/^[a-zA-Z]{2,5}(?:-[a-zA-Z]{2,5})?$/.test(raw)) {
      return raw.toLowerCase();
    }
    return "";
  }).filter(Boolean));
}

function normalizePlatformFilters(values) {
  return uniqueNumbers(values.flatMap((value) => {
    const raw = String(value || "").trim();
    const normalized = normalizeAlias(raw);
    if (/^\d+$/.test(raw)) {
      return [Number.parseInt(raw, 10)];
    }
    if (["android", "安卓"].includes(normalized)) {
      return [1];
    }
    if (["ios", "iphone", "ipad", "苹果"].includes(normalized)) {
      return [2];
    }
    return [];
  }));
}

function normalizeAlias(value) {
  return String(value || "")
    .toLowerCase()
    .replace(/[()\s_\-+/&.]/g, "")
    .trim();
}

function uniqueStrings(values) {
  const seen = new Set();
  const out = [];
  for (const value of values) {
    const key = String(value || "").trim();
    if (!key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(key);
  }
  return out;
}

function uniqueNumbers(values) {
  const seen = new Set();
  const out = [];
  for (const value of values) {
    const number = Number(value);
    if (!Number.isInteger(number) || seen.has(number)) {
      continue;
    }
    seen.add(number);
    out.push(number);
  }
  return out;
}

function extractMaterialRules(text) {
  const rules = {};
  const newRule = extractBucketRule(text, ["新素材", "new material", "new materials"], ["跑量素材", "volume material", "volume materials", "scale material"]);
  if (newRule) {
    rules.new_materials = newRule;
  }
  const volumeRule = extractBucketRule(text, ["跑量素材", "volume material", "volume materials", "scale material"], ["新素材", "new material", "new materials"]);
  if (volumeRule) {
    rules.volume_materials = volumeRule;
  }
  return rules;
}

function extractBucketRule(text, labels, otherLabels) {
  const section = sectionByLabels(text, labels, otherLabels);
  if (!section) {
    return null;
  }
  const rule = {};
  const ratio = extractRatio(section);
  if (Number.isFinite(ratio)) {
    rule.ratio = ratio;
  }
  const duration = extractBound(section, ["投放天数", "duration_days", "duration"]);
  if (duration && Number.isFinite(duration.value)) {
    rule[`duration_days_${duration.kind}`] = duration.value;
  }
  const impression = extractBound(section, ["曝光估算", "曝光", "impression_estimate", "impression"]);
  if (impression && Number.isFinite(impression.value)) {
    rule[`impression_${impression.kind}`] = impression.value;
  }
  return Object.keys(rule).length > 0 ? rule : null;
}

function sectionByLabels(text, labels, otherLabels) {
  const lower = text.toLowerCase();
  let start = -1;
  for (const label of labels) {
    const index = lower.indexOf(label.toLowerCase());
    if (index >= 0 && (start < 0 || index < start)) {
      start = index;
    }
  }
  if (start < 0) {
    return "";
  }
  let end = text.length;
  for (const label of otherLabels) {
    const index = lower.indexOf(label.toLowerCase(), start + 1);
    if (index > start && index < end) {
      end = index;
    }
  }
  return text.slice(start, end);
}

function extractRatio(section) {
  const match = section.match(/(?:占比|ratio)\s*[:：=]?\s*(\d+(?:\.\d+)?)\s*%/i)
    || section.match(/(\d+(?:\.\d+)?)\s*%/);
  if (!match) {
    return NaN;
  }
  return Number(match[1]) / 100;
}

function extractBound(section, labels) {
  for (const label of labels) {
    const escaped = escapeRegExp(label);
    const beforeValue = section.match(new RegExp(`${escaped}[^\\n\\d<>≥≤不大超小低少以+]{0,20}(>=|<=|>|<|≥|≤|不少于|不低于|大于|超过|以上|小于|低于|少于|以内)?\\s*(\\d+(?:\\.\\d+)?)\\s*([kKmM万亿]?)\\s*(天|日|days?|次|个|条)?\\s*(\\+|以上|以内|以下)?`, "i"));
    if (beforeValue) {
      const kind = boundKind(beforeValue[1] || beforeValue[5]);
      const value = parseUnitNumber(beforeValue[2], beforeValue[3]);
      if (kind && Number.isFinite(value)) {
        return { kind, value };
      }
    }
    const afterValue = section.match(new RegExp(`${escaped}[^\\n\\d]{0,20}(\\d+(?:\\.\\d+)?)\\s*([kKmM万亿]?)\\s*(天|日|days?|次|个|条)?\\s*(以上|以内|以下|\\+)`, "i"));
    if (afterValue) {
      const kind = boundKind(afterValue[4]);
      const value = parseUnitNumber(afterValue[1], afterValue[2]);
      if (kind && Number.isFinite(value)) {
        return { kind, value };
      }
    }
  }
  return null;
}

function boundKind(operator) {
  const op = String(operator || "").trim().toLowerCase();
  if (op === ">" || op === "大于" || op === "超过") {
    return "gt";
  }
  if (op === ">=" || op === "≥" || op === "不少于" || op === "不低于" || op === "以上" || op === "+") {
    return "gte";
  }
  if (op === "<" || op === "小于" || op === "低于" || op === "少于") {
    return "lt";
  }
  if (op === "<=" || op === "≤" || op === "以内" || op === "以下") {
    return "lte";
  }
  return "";
}

function parseUnitNumber(raw, unit) {
  const value = Number(raw);
  if (!Number.isFinite(value)) {
    return NaN;
  }
  switch (String(unit || "").toLowerCase()) {
    case "k":
      return value * 1_000;
    case "m":
      return value * 1_000_000;
    case "万":
      return value * 10_000;
    case "亿":
      return value * 100_000_000;
    default:
      return value;
  }
}

function extractInteger(text, pattern) {
  const match = text.match(pattern);
  return match ? Number.parseInt(match[1], 10) : 0;
}

function extractRecentDaysRange(text) {
  const match = text.match(/最近\s*(\d{1,3})\s*天/i)
    || text.match(/近\s*(\d{1,3})\s*天/i);
  if (!match) {
    return 0;
  }
  const days = Number.parseInt(match[1], 10);
  if (!Number.isInteger(days) || days <= 0 || days > 366) {
    return 0;
  }
  return days;
}

function extractDurationMS(text, pattern) {
  const match = text.match(pattern);
  if (!match) {
    return 0;
  }
  const value = Number(match[1]);
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  const unit = String(match[2] || "").toLowerCase();
  if (unit === "m" || unit === "分钟") {
    return Math.round(value * 60_000);
  }
  if (unit === "s" || unit === "秒") {
    return Math.round(value * 1_000);
  }
  return Math.round(value);
}

function hasUpperBound(rule, prefix) {
  return Number.isFinite(rule[`${prefix}_lt`]) || Number.isFinite(rule[`${prefix}_lte`]);
}

function hasLowerBound(rule, prefix) {
  return Number.isFinite(rule[`${prefix}_gt`]) || Number.isFinite(rule[`${prefix}_gte`]);
}

function finiteNumberParam(value, fallback, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return fallback;
  }
  return Math.max(min, Math.min(max, number));
}

function optionalFiniteNumberParam(value, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return NaN;
  }
  return Math.max(min, Math.min(max, number));
}

function escapeRegExp(value) {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
