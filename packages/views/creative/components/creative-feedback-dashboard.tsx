"use client";

import { useQuery } from "@tanstack/react-query";
import { BarChart3, ImageIcon, MessageSquareWarning } from "lucide-react";
import { creativeFeedbackDashboardOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { creativeWorkflowInsights, formatCreativeDuration } from "../lib/creative-feedback-insights";

const REASON_LABELS: Record<string, string> = {
  duplicate: "重复素材",
  irrelevant: "内容不相关",
  low_quality: "素材质量不足",
  composition_unsuitable: "构图不适合",
  copy_unsuitable: "原文案不适合",
  competitor_hard_to_replace: "竞品元素难替换",
  app_ui_unsuitable: "App UI 不适合",
  benefit_mismatch: "利益点不匹配",
  facts_inapplicable: "事实不适用",
  unnatural: "表达不自然",
  tone_mismatch: "语气不匹配",
  too_long: "文案过长",
  compliance_risk: "合规风险",
  translation: "翻译问题",
  visual_direction_mismatch: "视觉方向不匹配",
  layout_mismatch: "版式不匹配",
  brand_issue: "品牌元素问题",
  copy_error: "成图文案错误",
  theme_mismatch: "主题偏离",
  subject_mismatch: "主体偏离",
  size_inconsistency: "尺寸内容不一致",
  brand_or_prime: "Prime 或品牌问题",
  broken_image: "图片质量问题",
  competitor_residue: "竞品残留",
  missed_issue: "QC 漏检",
  false_positive: "QC 误报",
  warning_accepted: "接受 QC 提示",
  other: "其他",
};

export function CreativeFeedbackDashboard() {
  const wsId = useWorkspaceId();
  const dashboard = useQuery(creativeFeedbackDashboardOptions(wsId));
  const workflow = dashboard.data?.workflow;
  const insights = workflow ? creativeWorkflowInsights(workflow) : [];

  return <div className="mx-auto max-w-[1280px] border-x bg-background">
    <header className="border-y px-5 py-4">
      <div>
        <div className="flex items-center gap-2"><BarChart3 className="h-4 w-4" /><h2 className="text-base font-semibold">流程改进</h2></div>
        <p className="mt-1 text-xs text-muted-foreground">聚焦素材、文案、出图与采用，帮助下一批更稳定。</p>
      </div>
    </header>

    {dashboard.isError ? <p className="px-5 py-10 text-center text-sm text-muted-foreground">流程数据暂时不可用</p> : <>
      <section className="grid border-b sm:grid-cols-2 xl:grid-cols-4" aria-label="流程改进指标">
        {insights.map((insight, index) => <div key={insight.key} className={`min-w-0 px-5 py-5 ${index > 0 ? "border-t sm:border-l sm:border-t-0" : ""} ${index > 0 && index % 2 === 0 ? "sm:border-l-0 sm:border-t xl:border-l xl:border-t-0" : ""}`}>
          <p className="text-xs text-muted-foreground">{insight.label}</p>
          <div className="mt-2 flex items-baseline gap-2"><span className="text-2xl font-semibold tabular-nums">{insight.rate === null ? "-" : `${insight.rate}%`}</span><span className="text-xs text-muted-foreground">{insight.value}/{insight.total}</span></div>
          <div className="mt-3 h-1.5 overflow-hidden bg-muted"><div className="h-full bg-foreground transition-[width]" style={{ width: `${insight.rate ?? 0}%` }} /></div>
        </div>)}
        {!dashboard.isLoading && insights.length === 0 && <p className="col-span-full px-5 py-10 text-center text-sm text-muted-foreground">暂无流程数据</p>}
      </section>
      <section className="grid lg:grid-cols-[minmax(0,1.3fr)_minmax(280px,0.7fr)]">
        <div className="border-b p-5 lg:border-b-0 lg:border-r">
          <div className="flex items-center gap-2"><MessageSquareWarning className="h-4 w-4" /><h3 className="text-sm font-semibold">问题改进优先级</h3></div>
          <div className="mt-4 divide-y border-y">
            {workflow?.feedback_reasons.map((reason) => <div key={reason.code} className="flex items-center justify-between gap-4 py-3 text-sm"><span>{REASON_LABELS[reason.code] ?? reason.code}</span><span className="tabular-nums text-muted-foreground">{reason.count} 次</span></div>)}
            {!dashboard.isLoading && (workflow?.feedback_reasons.length ?? 0) === 0 && <p className="py-8 text-center text-sm text-muted-foreground">暂无可归因的反馈问题</p>}
          </div>
        </div>
        <div className="p-5">
          <div className="flex items-center gap-2"><ImageIcon className="h-4 w-4" /><h3 className="text-sm font-semibold">出图状态</h3></div>
          <dl className="mt-4 divide-y border-y text-sm">
            <MetricRow label="出图失败" value={workflow?.image_generation_failed ?? 0} />
            <MetricRow label="仍在生成" value={workflow?.image_generation_in_progress ?? 0} />
            <MetricRow label="成图问题反馈" value={workflow?.asset_reported ?? 0} />
            <MetricRow label="整套出图平均耗时" value={formatCreativeDuration(workflow?.image_generation_duration_seconds)} />
          </dl>
          {(workflow?.image_generation_duration_package_count ?? 0) > 0 && <p className="mt-2 text-[11px] text-muted-foreground">基于 {workflow?.image_generation_duration_package_count} 套完整出图</p>}
        </div>
      </section>
    </>}
  </div>;
}

function MetricRow({ label, value }: { label: string; value: number | string }) {
  return <div className="flex items-center justify-between gap-3 py-3"><dt className="text-muted-foreground">{label}</dt><dd className="font-medium tabular-nums">{value}</dd></div>;
}
