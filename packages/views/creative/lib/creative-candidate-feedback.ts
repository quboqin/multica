import type { CreateCreativeFeedbackRequest, CreativeMaterialCandidate } from "@multica/core/types";

export function candidateDecisionFeedbackInput(
  candidate: Pick<CreativeMaterialCandidate, "id" | "source_run_id" | "analysis_status">,
  decision: "selected" | "rejected",
  reasonCodes?: string[],
  comment?: string,
  idempotencyKey?: string,
): CreateCreativeFeedbackRequest {
  return {
    idempotency_key: idempotencyKey,
    issue_id: "",
    subject_type: "candidate",
    subject_id: candidate.id,
    event_type: "decision",
    decision,
    reason_codes: reasonCodes,
    comment,
    context_snapshot: { crawl_run_id: candidate.source_run_id, analysis_status: candidate.analysis_status },
  };
}

export function latestCandidateFeedback(events: { id: string; subject_id: string; event_type: string; decision: string; undo_of_id: string; created_at?: string }[]) {
  const undone = new Set(events.filter((event) => event.event_type === "undo" && event.undo_of_id).map((event) => event.undo_of_id));
  const latest = new Map<string, { id: string; decision: string; created_at: string }>();
  for (const event of events.filter((event) => event.event_type === "decision" && !undone.has(event.id))) {
    const current = latest.get(event.subject_id);
    if (!current || compareNewest(event, current) > 0) {
      latest.set(event.subject_id, { id: event.id, decision: event.decision, created_at: event.created_at ?? "" });
    }
  }
  return latest;
}

function compareNewest(left: { id: string; created_at?: string }, right: { id: string; created_at?: string }): number {
  const leftTime = Date.parse(left.created_at || "") || 0;
  const rightTime = Date.parse(right.created_at || "") || 0;
  if (leftTime !== rightTime) return leftTime - rightTime;
  return left.id.localeCompare(right.id);
}
