import type { CreateCreativeFeedbackResponse, CreativeFeedbackMetrics } from "@multica/core/types";

export type CreativeFeedbackInsight = {
  key: string;
  label: string;
  value: number;
  total: number;
  rate: number | null;
};

export type CreativeFeedbackReasonSummary = {
  code: string;
  count: number;
};

export function creativeFeedbackInsights(metrics: CreativeFeedbackMetrics): CreativeFeedbackInsight[] {
  return [
    ratioInsight("candidate", "素材采用率", metrics.candidate_selected, metrics.candidate_selected + metrics.candidate_rejected),
    ratioInsight("copy", "推荐文案采用率", metrics.copy_accepted, metrics.copy_accepted + metrics.copy_replaced),
    ratioInsight("variant", "一次通过率", metrics.variant_accepted, metrics.variant_accepted + metrics.variant_needs_revision),
    ratioInsight("qc", "QC 反馈通过率", metrics.qc_accepted, metrics.qc_accepted + metrics.qc_missed_issue + metrics.qc_false_positive),
  ];
}

export function creativeFeedbackReasonSummary(events: CreateCreativeFeedbackResponse[], limit = 6): CreativeFeedbackReasonSummary[] {
  const undone = new Set(events.filter((event) => event.event_type === "undo" && event.undo_of_id).map((event) => event.undo_of_id));
  const counts = new Map<string, number>();
  for (const event of events) {
    if (event.event_type === "undo" || undone.has(event.id)) continue;
    for (const code of event.reason_codes) counts.set(code, (counts.get(code) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([code, count]) => ({ code, count }))
    .sort((left, right) => right.count - left.count || left.code.localeCompare(right.code))
    .slice(0, limit);
}

function ratioInsight(key: string, label: string, value: number, total: number): CreativeFeedbackInsight {
  return { key, label, value, total, rate: total > 0 ? Math.round((value / total) * 100) : null };
}
