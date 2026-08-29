import type { CreativeCopyLibraryConfig } from "@multica/core/types";

export function copyLibraryDraftError(value: CreativeCopyLibraryConfig): string {
  for (const fragment of value.fragments.filter((item) => item.status === "approved")) {
    if (!fragment.text.trim()) return `已审核文案“${fragment.name || fragment.key || "未命名"}”不能为空`;
    if (/\{\{[^}]+\}\}/.test(fragment.text)) return `已审核文案“${fragment.name || fragment.key}”不能使用变量，请填写最终可投放文案`;
  }
  if (Object.values(value.repayment_plan.labels).some((label) => !label.trim())) return "还款计划缺少表格字段名称";
  const approvedEntries = value.repayment_plan.entries.filter((entry) => entry.status === "approved");
  const seen = new Set<string>();
  for (const entry of approvedEntries) {
    if (!entry.id.trim() || !entry.key.trim() || !entry.source.trim()) return "已审核还款计划缺少标识或来源依据";
    if (!Number.isInteger(entry.principal) || entry.principal <= 0 || !Number.isInteger(entry.tenor_months) || entry.tenor_months <= 0) return `还款计划“${entry.key || "未命名"}”的金额或期限无效`;
    if (!Number.isInteger(entry.monthly_installment) || entry.monthly_installment <= 0 || !Number.isInteger(entry.total_interest) || entry.total_interest < 0 || !Number.isInteger(entry.total_repayment) || entry.total_repayment <= 0) return `还款计划“${entry.key}”缺少已审核还款结果`;
    const identity = `${entry.principal}:${entry.tenor_months}`;
    if (seen.has(identity)) return `还款计划重复了 ${entry.principal} / ${entry.tenor_months} 个月`;
    seen.add(identity);
  }
  const approved = value.fragments.filter((fragment) => fragment.status === "approved");
  if (!approved.some((fragment) => fragment.role === "benefit")) return "投放文案至少需要一个已审核核心卖点";
  if (approvedEntries.length === 0) return "还款计划至少需要一条已审核的金额和期限组合";
  return "";
}
