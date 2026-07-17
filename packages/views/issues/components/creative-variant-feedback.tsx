"use client";

import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, CircleX, RefreshCw, Send } from "lucide-react";
import type {
  CreateCreativeEditFeedbackRequest,
  CreativeEditFeedbackDecision,
  CreativeEditFeedbackReason,
  CreativeEditVariant,
} from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";

type DecisionOption = {
  value: CreativeEditFeedbackDecision;
  label: string;
  icon: typeof CheckCircle2;
};

type ReasonOption = {
  value: CreativeEditFeedbackReason;
  label: string;
};

const DECISIONS: DecisionOption[] = [
  { value: "accepted", label: "接受", icon: CheckCircle2 },
  { value: "needs_revision", label: "需要调整", icon: RefreshCw },
  { value: "rejected", label: "不采用", icon: CircleX },
];

const ACCEPTED_REASONS: ReasonOption[] = [
  { value: "ready_to_publish", label: "可直接投放" },
  { value: "copy_accurate", label: "文案准确" },
  { value: "benefit_clear", label: "利益点清晰" },
  { value: "layout_match", label: "构图合适" },
  { value: "brand_complete", label: "品牌元素完整" },
  { value: "other", label: "其他" },
];

const ISSUE_REASONS: ReasonOption[] = [
  { value: "copy_error", label: "文案或数字错误" },
  { value: "copy_too_long", label: "文案过长" },
  { value: "benefit_mismatch", label: "利益点不匹配" },
  { value: "layout_mismatch", label: "偏离参考构图" },
  { value: "missing_content", label: "元素缺失或空白" },
  { value: "brand_compliance", label: "品牌或合规问题" },
  { value: "visual_quality", label: "视觉质量问题" },
  { value: "other", label: "其他" },
];

const DECISION_LABELS: Record<string, string> = {
  accepted: "已接受",
  needs_revision: "需要调整",
  rejected: "不采用",
};

const REASON_LABELS = Object.fromEntries(
  [...ACCEPTED_REASONS, ...ISSUE_REASONS].map((reason) => [reason.value, reason.label]),
) as Record<string, string>;

export function CreativeVariantFeedback({
  variant,
  submitting,
  onSubmit,
}: {
  variant: CreativeEditVariant;
  submitting: boolean;
  onSubmit: (input: CreateCreativeEditFeedbackRequest) => Promise<void>;
}) {
  const [decision, setDecision] = useState<CreativeEditFeedbackDecision | "">("");
  const [reasonCodes, setReasonCodes] = useState<CreativeEditFeedbackReason[]>([]);
  const [suggestion, setSuggestion] = useState("");
  const reasons = decision === "accepted" ? ACCEPTED_REASONS : ISSUE_REASONS;
  const feedback = variant.feedback ?? [];
  const latest = feedback[0];

  useEffect(() => {
    setDecision("");
    setReasonCodes([]);
    setSuggestion("");
  }, [variant.id]);

  const validReasonCodes = useMemo(
    () => new Set(reasons.map((reason) => reason.value)),
    [reasons],
  );

  const chooseDecision = (next: CreativeEditFeedbackDecision) => {
    setDecision(next);
    const nextReasons = next === "accepted" ? ACCEPTED_REASONS : ISSUE_REASONS;
    const allowed = new Set(nextReasons.map((reason) => reason.value));
    setReasonCodes((current) => current.filter((reason) => allowed.has(reason)));
  };

  const toggleReason = (reason: CreativeEditFeedbackReason) => {
    if (!validReasonCodes.has(reason)) return;
    setReasonCodes((current) => current.includes(reason)
      ? current.filter((item) => item !== reason)
      : [...current, reason]);
  };

  const submit = async () => {
    if (!decision || reasonCodes.length === 0 || submitting) return;
    try {
      await onSubmit({
        decision,
        reason_codes: reasonCodes,
        suggestion: suggestion.trim(),
      });
    } catch {
      return;
    }
    setDecision("");
    setReasonCodes([]);
    setSuggestion("");
  };

  return (
    <section className="border-t pt-4" aria-label="成图反馈">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h4 className="text-sm font-semibold">成图反馈</h4>
          <p className="mt-1 text-xs text-muted-foreground">
            变体 {variant.variant_index} · QC {variant.qc_status || "待处理"}
          </p>
        </div>
        {latest && (
          <div className="flex items-center gap-2 text-xs">
            <span className="text-muted-foreground">最近确认</span>
            <DecisionBadge decision={latest.decision} />
          </div>
        )}
      </div>

      <div className="mt-3 grid grid-cols-3 overflow-hidden rounded-md border">
        {DECISIONS.map((option) => {
          const Icon = option.icon;
          const active = decision === option.value;
          return (
            <button
              key={option.value}
              type="button"
              aria-pressed={active}
              className={cn(
                "flex min-h-9 items-center justify-center gap-1.5 border-r px-2 text-xs font-medium transition-colors last:border-r-0",
                active ? "bg-primary text-primary-foreground" : "bg-background hover:bg-muted",
              )}
              onClick={() => chooseDecision(option.value)}
            >
              <Icon className="h-3.5 w-3.5" />
              {option.label}
            </button>
          );
        })}
      </div>

      {decision && (
        <div className="mt-3">
          <div className="text-xs font-medium">
            {decision === "accepted" ? "确认亮点" : "问题分类"}
          </div>
          <div className="mt-2 flex flex-wrap gap-2">
            {reasons.map((reason) => {
              const selected = reasonCodes.includes(reason.value);
              return (
                <button
                  key={reason.value}
                  type="button"
                  aria-pressed={selected}
                  className={cn(
                    "min-h-8 rounded-md border px-2.5 text-xs transition-colors",
                    selected
                      ? "border-primary bg-primary/10 text-foreground"
                      : "bg-background text-muted-foreground hover:text-foreground",
                  )}
                  onClick={() => toggleReason(reason.value)}
                >
                  {reason.label}
                </button>
              );
            })}
          </div>
        </div>
      )}

      <div className="mt-3">
        <Textarea
          value={suggestion}
          onChange={(event) => setSuggestion(event.target.value)}
          maxLength={2000}
          placeholder="补充具体调整建议，例如：保留原图 Up to，只替换金额..."
          className="min-h-20 resize-y text-sm"
        />
        <div className="mt-2 flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">{suggestion.length} / 2000</span>
          <Button
            size="sm"
            disabled={!decision || reasonCodes.length === 0 || submitting}
            onClick={() => void submit()}
          >
            <Send className="h-4 w-4" />
            {submitting ? "提交中" : "提交反馈"}
          </Button>
        </div>
      </div>

      {feedback.length > 0 && (
        <div className="mt-4 border-t pt-3">
          <div className="flex items-center gap-2">
            <h5 className="text-xs font-semibold">反馈历史</h5>
            <Badge variant="outline">{feedback.length} 条</Badge>
          </div>
          <div className="mt-3 space-y-3">
            {feedback.map((item) => {
              const snapshot = item.process_snapshot ?? {};
              const progress = typeof snapshot.job_progress === "number"
                ? `${snapshot.job_progress}%`
                : "";
              return (
                <article key={item.id} className="border-l-2 border-border pl-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <DecisionBadge decision={item.decision} />
                    <span className="text-xs font-medium">{item.created_by_name || "成员"}</span>
                    <time className="text-xs text-muted-foreground" dateTime={item.created_at}>
                      {formatFeedbackTime(item.created_at)}
                    </time>
                  </div>
                  <div className="mt-1 flex flex-wrap gap-x-2 gap-y-1 text-xs text-muted-foreground">
                    {item.reason_codes.map((reason) => (
                      <span key={reason}>{REASON_LABELS[reason] || reason}</span>
                    ))}
                  </div>
                  {item.suggestion && (
                    <p className="mt-2 whitespace-pre-wrap text-sm leading-5">{item.suggestion}</p>
                  )}
                  <p className="mt-2 text-[11px] text-muted-foreground">
                    {[
                      snapshot.job_stage || snapshot.job_status,
                      snapshot.variant_qc_status ? `QC ${snapshot.variant_qc_status}` : "",
                      progress,
                    ].filter(Boolean).join(" · ") || "流程数据已记录"}
                  </p>
                </article>
              );
            })}
          </div>
        </div>
      )}
    </section>
  );
}

function DecisionBadge({ decision }: { decision: string }) {
  const variant = decision === "accepted"
    ? "default"
    : decision === "rejected"
      ? "destructive"
      : "outline";
  return <Badge variant={variant}>{DECISION_LABELS[decision] || decision || "未确认"}</Badge>;
}

function formatFeedbackTime(value: string) {
  if (!value) return "刚刚";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
