import type { CreativeFeedbackDashboard } from "@multica/core/types";

export type CreativeFeedbackInsight = {
  key: string;
  label: string;
  value: number;
  total: number;
  rate: number | null;
};

export function creativeWorkflowInsights(workflow: CreativeFeedbackDashboard["workflow"]): CreativeFeedbackInsight[] {
  return [
    ratioInsight("image-generation", "图片生成成功率", workflow.image_generation_success, workflow.image_generation_total),
    ratioInsight("candidate", "素材采用率", workflow.candidate_selected, workflow.candidate_selected + workflow.candidate_rejected),
    ratioInsight("copy", "推荐文案采用率", workflow.copy_accepted, workflow.copy_accepted + workflow.copy_replaced),
    ratioInsight("production-adoption", "产线采用率", workflow.production_adopted, workflow.production_adoption_eligible),
  ];
}

export function formatCreativeDuration(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return "-";
  const roundedSeconds = Math.round(seconds);
  if (roundedSeconds < 60) return `${roundedSeconds} 秒`;
  const minutes = Math.floor(roundedSeconds / 60);
  const remainingSeconds = roundedSeconds % 60;
  if (minutes < 60) return remainingSeconds > 0 ? `${minutes} 分 ${remainingSeconds} 秒` : `${minutes} 分`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours} 小时 ${remainingMinutes} 分` : `${hours} 小时`;
}

function ratioInsight(key: string, label: string, value: number, total: number): CreativeFeedbackInsight {
  return { key, label, value, total, rate: total > 0 ? Math.round((value / total) * 100) : null };
}
