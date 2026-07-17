"use client";

import { ImageIcon, Maximize2 } from "lucide-react";
import { Badge } from "@multica/ui/components/ui/badge";
import { cn } from "@multica/ui/lib/utils";

type UnknownRecord = Record<string, unknown>;

export interface CreativeQcAttempt {
  key: string;
  conceptIndex: number;
  candidateIndex: number;
  size: string;
  requestedModelSize: string;
  exactOutput: string;
  providerReturnedSize: string;
  normalization: string;
  verdict: string;
  imageUrl: string;
  imageArtifact: string;
  qcArtifact: string;
  promptArtifact: string;
  issues: { type: string; message: string }[];
}

export interface CreativeQcAttemptRound {
  key: string;
  submissionIndex: number;
  submissionStatus: string;
  round: number;
  attempts: CreativeQcAttempt[];
}

type PreviewItem = {
  url: string;
  title: string;
  subtitle?: string;
  assetType?: string;
  openUrl?: string;
};

export function extractCreativeQcAttemptRounds(
  processData: Record<string, unknown>,
  candidateId: string,
): CreativeQcAttemptRound[] {
  const groups: CreativeQcAttemptRound[] = [];
  const root = asRecord(processData);
  const candidates = asArray(root?.candidates).map(asRecord).filter(isRecord);

  for (const candidate of candidates) {
    const currentCandidateId = asString(candidate.candidate_id);
    if (candidateId && currentCandidateId !== candidateId) continue;
    const submissions = asArray(candidate.submissions).map(asRecord).filter(isRecord);
    for (const [submissionOffset, submission] of submissions.entries()) {
      const submissionIndex = asPositiveInteger(submission.submission_index) ?? submissionOffset + 1;
      const submissionStatus = asString(submission.status) || "unknown";
      const process = asRecord(submission.process);
      const rounds = asArray(process?.rounds).map(asRecord).filter(isRecord);
      for (const [roundOffset, roundData] of rounds.entries()) {
        const round = asPositiveInteger(roundData.round) ?? roundOffset + 1;
        const records: UnknownRecord[] = [];
        collectAttemptRecords(roundData.verdicts, records);
        const attempts = records.map((record, attemptOffset) => {
          const artifacts = asRecord(record.artifacts);
          const candidateIndex = asPositiveInteger(record.candidate_index) ?? attemptOffset + 1;
          const conceptIndex = asPositiveInteger(record.concept_index) ?? candidateIndex;
          const size = asString(record.size) || asString(record.exact_output) || "主图";
          const imageArtifact = safeArtifactLabel(artifacts?.image);
          const imageUrl = safeAttemptImageUrl(artifacts?.image_url);
          const qcArtifact = safeArtifactLabel(artifacts?.qc);
          const promptArtifact = safeArtifactLabel(record.prompt_artifact)
            || safeArtifactLabel(artifacts?.prompt);
          return {
            key: `${submissionIndex}:${round}:${conceptIndex}:${size}:${attemptOffset}`,
            conceptIndex,
            candidateIndex,
            size,
            requestedModelSize: asString(record.requested_model_size),
            exactOutput: asString(record.exact_output),
            providerReturnedSize: asString(record.provider_returned_size),
            normalization: asString(record.normalization),
            verdict: asString(record.verdict) || "unknown",
            imageUrl,
            imageArtifact,
            qcArtifact,
            promptArtifact,
            issues: asArray(record.issues)
              .map(asRecord)
              .filter(isRecord)
              .map((issue) => ({
                type: asString(issue.type),
                message: asString(issue.message),
              }))
              .filter((issue) => issue.message),
          };
        }).sort(compareAttempts);
        if (attempts.length > 0) {
          groups.push({
            key: `${currentCandidateId}:${submissionIndex}:${round}`,
            submissionIndex,
            submissionStatus,
            round,
            attempts,
          });
        }
      }
    }
  }
  return groups;
}

export function CreativeQcAttemptHistory({
  processData,
  candidateId,
  candidateName,
  onPreview,
}: {
  processData: Record<string, unknown>;
  candidateId: string;
  candidateName?: string;
  onPreview: (item: PreviewItem) => void;
}) {
  const rounds = extractCreativeQcAttemptRounds(processData, candidateId);
  if (rounds.length === 0) return null;
  const attempts = rounds.flatMap((round) => round.attempts);
  const failed = attempts.filter((attempt) => attempt.verdict !== "pass").length;

  return (
    <section className="space-y-3 border-t pt-3" aria-label="QC 每轮记录">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 className="text-sm font-semibold">QC 每轮记录</h4>
          <p className="mt-1 text-xs text-muted-foreground">
            通过和未通过的尝试图都会保留，方便比较每次修改是否有效。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline">共 {attempts.length} 张</Badge>
          {failed > 0 && <Badge variant="outline">未通过 {failed} 张</Badge>}
        </div>
      </div>

      {rounds.map((round) => {
        const passed = round.attempts.filter((attempt) => attempt.verdict === "pass").length;
        return (
          <div key={round.key} className="overflow-hidden rounded-md border bg-background">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/30 px-3 py-2">
              <div className="text-xs font-medium">
                批次 {round.submissionIndex} · 第 {round.round} 轮
              </div>
              <div className="text-xs text-muted-foreground">
                通过 {passed} / {round.attempts.length}
              </div>
            </div>
            <div className="grid gap-3 p-3 md:grid-cols-2 xl:grid-cols-3">
              {round.attempts.map((attempt) => (
                <article key={attempt.key} className="overflow-hidden rounded-md border">
                  <div className="relative aspect-[4/3] bg-muted">
                    {attempt.imageUrl ? (
                      <button
                        type="button"
                        className="group h-full w-full cursor-zoom-in outline-none ring-inset focus-visible:ring-2 focus-visible:ring-primary"
                        aria-label={`预览第 ${round.round} 轮概念 ${attempt.conceptIndex} ${attempt.size} 尝试图`}
                        onClick={() => onPreview({
                          url: attempt.imageUrl,
                          openUrl: attempt.imageUrl,
                          title: `第 ${round.round} 轮 · 概念 ${attempt.conceptIndex} · ${attempt.size}`,
                          subtitle: candidateName,
                          assetType: "image",
                        })}
                      >
                        <img
                          src={attempt.imageUrl}
                          alt={`第 ${round.round} 轮概念 ${attempt.conceptIndex} ${attempt.size} 尝试图`}
                          className="h-full w-full object-contain"
                        />
                        <span className="absolute bottom-2 right-2 inline-flex rounded-md bg-background/90 p-1.5 opacity-0 shadow-sm ring-1 ring-border transition group-hover:opacity-100">
                          <Maximize2 className="h-3.5 w-3.5" />
                        </span>
                      </button>
                    ) : (
                      <div className="flex h-full flex-col items-center justify-center gap-2 px-3 text-center text-xs text-muted-foreground">
                        <ImageIcon className="h-7 w-7" />
                        {attempt.verdict === "generation_error" ? "图片生成失败" : "尝试图不可用"}
                      </div>
                    )}
                  </div>
                  <div className="space-y-2 p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="text-xs font-semibold">
                        概念 {attempt.conceptIndex} · {attempt.size}
                      </span>
                      <Badge
                        variant="outline"
                        className={cn(
                          attempt.verdict === "pass"
                            ? "border-emerald-500/40 text-emerald-700 dark:text-emerald-300"
                            : "border-destructive/40 text-destructive",
                        )}
                      >
                        {qcVerdictLabel(attempt.verdict)}
                      </Badge>
                    </div>
                    {(attempt.providerReturnedSize || attempt.requestedModelSize || attempt.exactOutput) && (
                      <div className="flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
                        {attempt.providerReturnedSize && <span>模型返回 {attempt.providerReturnedSize}</span>}
                        {attempt.requestedModelSize && <span>要求模型尺寸 {attempt.requestedModelSize}</span>}
                        {attempt.exactOutput && <span>交付尺寸 {attempt.exactOutput}</span>}
                      </div>
                    )}
                    {attempt.issues.length > 0 ? (
                      <ul className="space-y-1.5 text-xs text-muted-foreground">
                        {attempt.issues.map((issue, index) => (
                          <li key={`${attempt.key}:issue:${index}`} className="leading-5">
                            <span className="font-medium text-foreground">{qcIssueTypeLabel(issue.type)}：</span>
                            {issue.message}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <p className="text-xs text-muted-foreground">
                        {attempt.verdict === "pass" ? "QC 未发现阻断问题。" : "QC 未返回具体问题。"}
                      </p>
                    )}
                    {(attempt.imageArtifact || attempt.qcArtifact || attempt.promptArtifact) && (
                      <div className="space-y-0.5 border-t pt-2 text-[11px] text-muted-foreground">
                        {attempt.imageArtifact && <div>尝试图：{attempt.imageArtifact}</div>}
                        {attempt.qcArtifact && <div>QC 记录：{attempt.qcArtifact}</div>}
                        {attempt.promptArtifact && <div>本轮提示词：{attempt.promptArtifact}</div>}
                      </div>
                    )}
                  </div>
                </article>
              ))}
            </div>
          </div>
        );
      })}
    </section>
  );
}

function collectAttemptRecords(value: unknown, output: UnknownRecord[], depth = 0) {
  if (depth > 8 || value == null) return;
  if (Array.isArray(value)) {
    for (const item of value) collectAttemptRecords(item, output, depth + 1);
    return;
  }
  const record = asRecord(value);
  if (!record) return;
  if (typeof record.verdict === "string" || asRecord(record.artifacts)?.image) {
    output.push(record);
    return;
  }
  for (const item of Object.values(record)) collectAttemptRecords(item, output, depth + 1);
}

function safeAttemptImageUrl(value: unknown) {
  const raw = asString(value);
  if (!raw || raw.length > 2000) return "";
  try {
    const url = new URL(raw);
    if (!new Set(["http:", "https:"]).has(url.protocol)
      || url.username
      || url.password
      || url.search
      || url.hash) return "";
    const segments = url.pathname.split("/").filter(Boolean);
    if (segments.length < 3 || segments.at(-3) !== "files") return "";
    const fileName = decodeURIComponent(segments.at(-1) ?? "");
    if (!/^[a-z0-9][a-z0-9._-]*\.(?:png|jpe?g|webp|gif|avif|bmp|tiff?)$/i.test(fileName)) return "";
    return raw;
  } catch {
    return "";
  }
}

function safeArtifactLabel(value: unknown) {
  const text = asString(value);
  if (!text || text.length > 255 || text.includes("/") || text.includes("\\")) return "";
  return /^[a-z0-9][a-z0-9._-]*$/i.test(text) ? text : "";
}

function compareAttempts(left: CreativeQcAttempt, right: CreativeQcAttempt) {
  if (left.conceptIndex !== right.conceptIndex) return left.conceptIndex - right.conceptIndex;
  const sizeOrder = ["1080x1080", "800x1000", "1200x628", "主图"];
  const leftIndex = sizeOrder.indexOf(left.size);
  const rightIndex = sizeOrder.indexOf(right.size);
  return (leftIndex < 0 ? sizeOrder.length : leftIndex)
    - (rightIndex < 0 ? sizeOrder.length : rightIndex);
}

function qcVerdictLabel(verdict: string) {
  switch (verdict) {
    case "pass": return "通过";
    case "fail": return "未通过";
    case "generation_error": return "生成失败";
    default: return verdict || "待判定";
  }
}

function qcIssueTypeLabel(type: string) {
  switch (type) {
    case "financial": return "文案或金融事实";
    case "hard_visual": return "严格视觉规则";
    case "warning": return "QC 提示";
    case "generation": return "生成异常";
    default: return type || "QC 问题";
  }
}

function asRecord(value: unknown): UnknownRecord | null {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? value as UnknownRecord
    : null;
}

function isRecord(value: UnknownRecord | null): value is UnknownRecord {
  return value !== null;
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function asString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function asPositiveInteger(value: unknown) {
  const parsed = typeof value === "number" ? value : Number.parseInt(asString(value), 10);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
}
