"use client";

import { useQuery } from "@tanstack/react-query";
import { BarChart3, ImageIcon, MessageSquareWarning } from "lucide-react";
import { creativeFeedbackDashboardOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { creativeWorkflowInsights, formatCreativeDuration } from "../lib/creative-feedback-insights";
import { useT } from "../../i18n";

export function CreativeFeedbackDashboard() {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const dashboard = useQuery(creativeFeedbackDashboardOptions(wsId));
  const workflow = dashboard.data?.workflow;
  const insights = workflow ? creativeWorkflowInsights(workflow) : [];

  return <div className="mx-auto max-w-[1280px] border-x bg-background">
    <header className="border-y px-5 py-4">
      <div>
        <div className="flex items-center gap-2"><BarChart3 className="h-4 w-4" /><h2 className="text-base font-semibold">{t(($) => $.feedback.title)}</h2></div>
        <p className="mt-1 text-xs text-muted-foreground">{t(($) => $.feedback.subtitle)}</p>
      </div>
    </header>

    {dashboard.isError ? <p className="px-5 py-10 text-center text-sm text-muted-foreground">{t(($) => $.feedback.unavailable)}</p> : <>
      <section className="grid border-b sm:grid-cols-2 xl:grid-cols-4" aria-label={t(($) => $.feedback.metrics)}>
        {insights.map((insight, index) => <div key={insight.key} className={`min-w-0 px-5 py-5 ${index > 0 ? "border-t sm:border-l sm:border-t-0" : ""} ${index > 0 && index % 2 === 0 ? "sm:border-l-0 sm:border-t xl:border-l xl:border-t-0" : ""}`}>
          <p className="text-xs text-muted-foreground">{insightLabel(t, insight.key)}</p>
          <div className="mt-2 flex items-baseline gap-2"><span className="text-2xl font-semibold tabular-nums">{insight.rate === null ? "-" : `${insight.rate}%`}</span><span className="text-xs text-muted-foreground">{insight.value}/{insight.total}</span></div>
          <div className="mt-3 h-1.5 overflow-hidden bg-muted"><div className="h-full bg-foreground transition-[width]" style={{ width: `${insight.rate ?? 0}%` }} /></div>
        </div>)}
        {!dashboard.isLoading && insights.length === 0 && <p className="col-span-full px-5 py-10 text-center text-sm text-muted-foreground">{t(($) => $.feedback.noData)}</p>}
      </section>
      <section className="grid lg:grid-cols-[minmax(0,1.3fr)_minmax(280px,0.7fr)]">
        <div className="border-b p-5 lg:border-b-0 lg:border-r">
          <div className="flex items-center gap-2"><MessageSquareWarning className="h-4 w-4" /><h3 className="text-sm font-semibold">{t(($) => $.feedback.priority)}</h3></div>
          <div className="mt-4 divide-y border-y">
            {workflow?.feedback_reasons.map((reason) => <div key={reason.code} className="flex items-center justify-between gap-4 py-3 text-sm"><span>{feedbackReasonLabel(t, reason.code)}</span><span className="tabular-nums text-muted-foreground">{t(($) => $.feedback.times, { count: reason.count })}</span></div>)}
            {!dashboard.isLoading && (workflow?.feedback_reasons.length ?? 0) === 0 && <p className="py-8 text-center text-sm text-muted-foreground">{t(($) => $.feedback.noReasons)}</p>}
          </div>
        </div>
        <div className="p-5">
          <div className="flex items-center gap-2"><ImageIcon className="h-4 w-4" /><h3 className="text-sm font-semibold">{t(($) => $.feedback.imageStatus)}</h3></div>
          <dl className="mt-4 divide-y border-y text-sm">
            <MetricRow label={t(($) => $.feedback.generationFailed)} value={workflow?.image_generation_failed ?? 0} />
            <MetricRow label={t(($) => $.feedback.generationInProgress)} value={workflow?.image_generation_in_progress ?? 0} />
            <MetricRow label={t(($) => $.feedback.assetReported)} value={workflow?.asset_reported ?? 0} />
            <MetricRow label={t(($) => $.feedback.averageDuration)} value={formatCreativeDuration(workflow?.image_generation_duration_seconds)} />
          </dl>
          {(workflow?.image_generation_duration_package_count ?? 0) > 0 && <p className="mt-2 text-[11px] text-muted-foreground">{t(($) => $.feedback.basedOnPackages, { count: workflow?.image_generation_duration_package_count })}</p>}
        </div>
      </section>
    </>}
  </div>;
}

function insightLabel(t: ReturnType<typeof useT<"creative">>["t"], key: string): string {
  if (key === "image-generation") return t(($) => $.feedback.insights["image-generation"]);
  if (key === "candidate") return t(($) => $.feedback.insights.candidate);
  if (key === "copy") return t(($) => $.feedback.insights.copy);
  if (key === "production-adoption") return t(($) => $.feedback.insights["production-adoption"]);
  return key;
}

function feedbackReasonLabel(t: ReturnType<typeof useT<"creative">>["t"], code: string): string {
  const labels: Record<string, string> = {
    duplicate: t(($) => $.feedback.reasons.duplicate), irrelevant: t(($) => $.feedback.reasons.irrelevant), low_quality: t(($) => $.feedback.reasons.low_quality), composition_unsuitable: t(($) => $.feedback.reasons.composition_unsuitable), copy_unsuitable: t(($) => $.feedback.reasons.copy_unsuitable), competitor_hard_to_replace: t(($) => $.feedback.reasons.competitor_hard_to_replace), app_ui_unsuitable: t(($) => $.feedback.reasons.app_ui_unsuitable), benefit_mismatch: t(($) => $.feedback.reasons.benefit_mismatch), facts_inapplicable: t(($) => $.feedback.reasons.facts_inapplicable), unnatural: t(($) => $.feedback.reasons.unnatural), tone_mismatch: t(($) => $.feedback.reasons.tone_mismatch), too_long: t(($) => $.feedback.reasons.too_long), compliance_risk: t(($) => $.feedback.reasons.compliance_risk), translation: t(($) => $.feedback.reasons.translation), visual_direction_mismatch: t(($) => $.feedback.reasons.visual_direction_mismatch), layout_mismatch: t(($) => $.feedback.reasons.layout_mismatch), brand_issue: t(($) => $.feedback.reasons.brand_issue), copy_error: t(($) => $.feedback.reasons.copy_error), theme_mismatch: t(($) => $.feedback.reasons.theme_mismatch), subject_mismatch: t(($) => $.feedback.reasons.subject_mismatch), size_inconsistency: t(($) => $.feedback.reasons.size_inconsistency), brand_or_prime: t(($) => $.feedback.reasons.brand_or_prime), broken_image: t(($) => $.feedback.reasons.broken_image), competitor_residue: t(($) => $.feedback.reasons.competitor_residue), missed_issue: t(($) => $.feedback.reasons.missed_issue), false_positive: t(($) => $.feedback.reasons.false_positive), warning_accepted: t(($) => $.feedback.reasons.warning_accepted), other: t(($) => $.feedback.reasons.other),
  };
  return labels[code] ?? code;
}

function MetricRow({ label, value }: { label: string; value: number | string }) {
  return <div className="flex items-center justify-between gap-3 py-3"><dt className="text-muted-foreground">{label}</dt><dd className="font-medium tabular-nums">{value}</dd></div>;
}
