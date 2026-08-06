import type { CreativeCopyLibraryConfig, CreativeProductFact, CreativeType } from "@multica/core/types";

const FACT_TOKEN = /\{\{fact\.([a-z0-9_]+)\.(copy_text|value)\}\}/gi;

export function appendCopyFactToken(value: string, factKey: string): string {
  const token = `{{fact.${factKey}.copy_text}}`;
  if (!value.trim()) return token;
  return `${value.replace(/\s+$/, "")} ${token}`;
}

export function copyTemplateFactKeys(value: string): string[] {
  return [...new Set([...value.matchAll(FACT_TOKEN)].map((match) => match[1]).filter((key): key is string => Boolean(key)))];
}

export function resolveCopyTemplateForBusiness(value: string, facts: CreativeProductFact[]): {
  text: string;
  usedFacts: CreativeProductFact[];
  missingFactKeys: string[];
} {
  const factsByKey = new Map(facts.map((fact) => [fact.key, fact]));
  const usedFacts = new Map<string, CreativeProductFact>();
  const missingFactKeys = new Set<string>();
  const text = value.replace(FACT_TOKEN, (_, key: string, field: "copy_text" | "value") => {
    const fact = factsByKey.get(key);
    if (!fact || fact.status !== "approved" || !fact[field].trim()) {
      missingFactKeys.add(key);
      return `[缺少事实：${key}]`;
    }
    usedFacts.set(key, fact);
    return fact[field];
  });
  return { text, usedFacts: [...usedFacts.values()], missingFactKeys: [...missingFactKeys] };
}

export function copyLibraryDraftError(value: CreativeCopyLibraryConfig): string {
  const invalidFact = value.product_facts.find((fact) => fact.status === "approved" && (!fact.key.trim() || !fact.copy_text.trim() || !fact.source.trim()));
  if (invalidFact) return `已审核产品事实“${invalidFact.label || invalidFact.key || "未命名"}”缺少事实键、印尼语展示或来源依据`;
  const approvedFacts = new Set(value.product_facts.filter((fact) => fact.status === "approved").map((fact) => fact.key));
  for (const fragment of value.fragments.filter((item) => item.status === "approved")) {
    const missing = copyTemplateFactKeys(fragment.text).find((key) => !approvedFacts.has(key));
    if (missing) return `已审核文案片段“${fragment.name || fragment.key}”缺少已审核产品事实“${missing}”`;
  }
  for (const creativeType of ["num", "repayment_plan"] as CreativeType[]) {
    const approved = value.fragments.filter((fragment) => fragment.status === "approved" && fragment.creative_types.includes(creativeType));
    if (!approved.some((fragment) => fragment.role === "headline")) return `${creativeType === "num" ? "NUM 数字利益点" : "还款计划"}至少需要一个已审核主标题原子`;
    if (!approved.some((fragment) => fragment.role === "benefit")) return `${creativeType === "num" ? "NUM 数字利益点" : "还款计划"}至少需要一个已审核利益点原子`;
  }
  return "";
}
