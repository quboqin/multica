"use client";

import { useQuery } from "@tanstack/react-query";
import { BarChart3, MessageSquareWarning, ScanSearch } from "lucide-react";
import { creativeFeedbackMetricsOptions, creativeFeedbackOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { Badge } from "@multica/ui/components/ui/badge";
import { creativeFeedbackInsights, creativeFeedbackReasonSummary } from "../lib/creative-feedback-insights";

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
  other: "其他",
};

export function CreativeFeedbackDashboard() {
  const wsId = useWorkspaceId();
  const metrics = useQuery(creativeFeedbackMetricsOptions(wsId));
  const feedback = useQuery(creativeFeedbackOptions(wsId));
  const insights = metrics.data ? creativeFeedbackInsights(metrics.data) : [];
  const reasons = creativeFeedbackReasonSummary(feedback.data?.events ?? []);
  const totalDecisions = insights.reduce((total, insight) => total + insight.total, 0);

  return <div className="mx-auto max-w-[1280px] border-x bg-background">
    <header className="flex flex-wrap items-end justify-between gap-4 border-y px-5 py-4">
      <div>
        <div className="flex items-center gap-2"><BarChart3 className="h-4 w-4" /><h2 className="text-base font-semibold">数据反馈</h2></div>
        <p className="mt-1 text-xs text-muted-foreground">最近累计的素材、文案、成图和 QC 决策</p>
      </div>
      <Badge variant="outline">{totalDecisions} 次有效判断</Badge>
    </header>

    <section className="grid border-b sm:grid-cols-2 xl:grid-cols-4" aria-label="反馈指标">
      {insights.map((insight, index) => <div key={insight.key} className={`min-w-0 px-5 py-5 ${index > 0 ? "border-t sm:border-l sm:border-t-0" : ""} ${index === 2 ? "sm:border-l-0 sm:border-t xl:border-l xl:border-t-0" : ""}`}>
        <p className="text-xs text-muted-foreground">{insight.label}</p>
        <div className="mt-2 flex items-baseline gap-2"><span className="text-2xl font-semibold tabular-nums">{insight.rate === null ? "-" : `${insight.rate}%`}</span><span className="text-xs text-muted-foreground">{insight.value}/{insight.total}</span></div>
        <div className="mt-3 h-1.5 overflow-hidden bg-muted"><div className="h-full bg-foreground transition-[width]" style={{ width: `${insight.rate ?? 0}%` }} /></div>
      </div>)}
      {!metrics.isLoading && insights.length === 0 && <p className="col-span-full px-5 py-10 text-center text-sm text-muted-foreground">暂无反馈数据</p>}
    </section>

    <section className="grid lg:grid-cols-[minmax(0,1.3fr)_minmax(280px,0.7fr)]">
      <div className="border-b p-5 lg:border-b-0 lg:border-r">
        <div className="flex items-center gap-2"><MessageSquareWarning className="h-4 w-4" /><h3 className="text-sm font-semibold">高频反馈原因</h3></div>
        <div className="mt-4 divide-y border-y">
          {reasons.map((reason) => <div key={reason.code} className="flex items-center justify-between gap-4 py-3 text-sm"><span>{REASON_LABELS[reason.code] ?? reason.code}</span><span className="tabular-nums text-muted-foreground">{reason.count} 次</span></div>)}
          {!feedback.isLoading && reasons.length === 0 && <p className="py-8 text-center text-sm text-muted-foreground">还没有记录拒绝或返工原因</p>}
        </div>
      </div>
      <div className="p-5">
        <div className="flex items-center gap-2"><ScanSearch className="h-4 w-4" /><h3 className="text-sm font-semibold">QC 反馈</h3></div>
        <dl className="mt-4 divide-y border-y text-sm">
          <MetricRow label="业务确认通过" value={metrics.data?.qc_accepted ?? 0} />
          <MetricRow label="发现漏检" value={metrics.data?.qc_missed_issue ?? 0} />
          <MetricRow label="确认误报" value={metrics.data?.qc_false_positive ?? 0} />
          <MetricRow label="成图问题反馈" value={metrics.data?.asset_reported ?? 0} />
        </dl>
      </div>
    </section>
  </div>;
}

function MetricRow({ label, value }: { label: string; value: number }) {
  return <div className="flex items-center justify-between gap-3 py-3"><dt className="text-muted-foreground">{label}</dt><dd className="font-medium tabular-nums">{value}</dd></div>;
}
