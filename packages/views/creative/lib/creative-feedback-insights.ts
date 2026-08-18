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

function ratioInsight(key: string, label: string, value: number, total: number): CreativeFeedbackInsight {
  return { key, label, value, total, rate: total > 0 ? Math.round((value / total) * 100) : null };
}
