import type { CreativeMaterialCandidate, CreativeSourceAnalysis } from "@multica/core/types";

export type MaterialLibraryFilter = "available" | "analyze" | "generated" | "rejected" | "all";

export type MaterialAnalysisState = { ready: boolean; status: string; error: string; version: number };

export type CreativeMaterialProductionStatus =
  | "available"
  | "analyzing"
  | "failed"
  | "manual_required"
  | "unsupported"
  | "generated"
  | "rejected";

export type CreativeMaterialAnalysisReadiness = {
  ready: boolean;
  active: boolean;
  failed: boolean;
  unavailable?: boolean;
  manualRequired?: boolean;
  reason?: string;
};

export type CreativeMaterialProductionState = {
  status: CreativeMaterialProductionStatus;
  label: string;
  selectable: boolean;
  active: boolean;
  reason?: string;
};

export function latestCompletedAnalyses(analyses: CreativeSourceAnalysis[]): Map<string, CreativeSourceAnalysis> {
  const latest = latestAnalyses(analyses);
  return new Map([...latest.entries()].filter(([, analysis]) => analysis.status === "completed"));
}

export function latestAnalyses(analyses: CreativeSourceAnalysis[]): Map<string, CreativeSourceAnalysis> {
  const latest = new Map<string, CreativeSourceAnalysis>();
  for (const analysis of analyses) {
    const current = latest.get(analysis.candidate_id);
    if (!current || analysis.analysis_version > current.analysis_version || (analysis.analysis_version === current.analysis_version && compareNewest(analysis, current) > 0)) {
      latest.set(analysis.candidate_id, analysis);
    }
  }
  return latest;
}

export function materialAnalysisState(candidate: Pick<CreativeMaterialCandidate, "id" | "analysis_status" | "analysis_error">, analyses: CreativeSourceAnalysis[]): MaterialAnalysisState {
  const latest = latestAnalyses(analyses).get(candidate.id);
  if (latest?.status === "completed") return { ready: true, status: "completed", error: "", version: latest.analysis_version };
  if (latest) return { ready: false, status: latest.status || "pending", error: latest.error_message || "", version: latest.analysis_version };

  const fallbackStatus = candidate.analysis_status || "pending";
  return { ready: fallbackStatus === "completed", status: fallbackStatus, error: candidate.analysis_error || "", version: 0 };
}

export function effectiveMaterialCandidateStatus(candidate: Pick<CreativeMaterialCandidate, "status">, decision?: string): string {
  return decision === "selected" || decision === "rejected" ? decision : candidate.status;
}

export function creativeMaterialProductionState(
  candidate: Pick<CreativeMaterialCandidate, "asset_type" | "archived_url" | "archive_status" | "status">,
  analysisState: MaterialAnalysisState,
  decision: string | undefined,
  generated: boolean,
  analysisReadiness: CreativeMaterialAnalysisReadiness,
): CreativeMaterialProductionState {
  const effectiveStatus = effectiveMaterialCandidateStatus(candidate, decision);
  if (effectiveStatus === "rejected") return { status: "rejected", label: "已拒绝", selectable: false, active: false };
  if (generated) return { status: "generated", label: "已生成", selectable: true, active: false };
  if (candidate.asset_type !== "image") return { status: "unsupported", label: "非图片素材", selectable: false, active: false };
  if (!candidate.archived_url) {
    return candidate.archive_status === "failed"
      ? { status: "failed", label: "处理失败", selectable: false, active: false, reason: "归档失败" }
      : { status: "analyzing", label: "分析中", selectable: false, active: true };
  }
  if (!analysisReadiness.ready) {
    if (analysisReadiness.unavailable) {
      return {
        status: "unsupported",
        label: "无可配置文案",
        selectable: false,
        active: false,
        reason: analysisReadiness.reason,
      };
    }
    if (analysisReadiness.manualRequired) {
      return {
        status: "manual_required",
        label: "待人工确认",
        selectable: false,
        active: false,
        reason: analysisReadiness.reason,
      };
    }
    if (analysisReadiness.failed || analysisState.status === "failed") {
      return {
        status: "failed",
        label: analysisState.ready && analysisReadiness.failed ? "适配失败" : "处理失败",
        selectable: false,
        active: false,
        reason: analysisReadiness.reason || analysisState.error,
      };
    }
    return { status: "analyzing", label: "分析中", selectable: false, active: true };
  }
  return { status: "available", label: "可用", selectable: true, active: false };
}

export function canRetryMaterialAnalysis(
  candidate: Pick<CreativeMaterialCandidate, "asset_type" | "archived_url">,
  state: Pick<CreativeMaterialProductionState, "status">,
): boolean {
  return candidate.asset_type === "image"
    && candidate.archived_url.trim() !== ""
    && state.status !== "rejected"
    && state.status !== "unsupported";
}

export function creativeMaterialSelectionActionLabel(
  selected: boolean,
  state: Pick<CreativeMaterialProductionState, "label" | "selectable" | "status">,
): string {
  if (selected) return "取消选择";
  if (state.selectable) return "选择出图素材";
  return state.status === "analyzing" ? "等待分析完成" : state.label;
}

export function creativeMaterialMatchesFilter(state: CreativeMaterialProductionState, filter: MaterialLibraryFilter): boolean {
  switch (filter) {
    case "available":
      return state.status === "available";
    case "analyze":
      return state.status === "analyzing" || state.status === "failed" || state.status === "manual_required";
    case "generated":
      return state.status === "generated";
    case "rejected":
      return state.status === "rejected";
    case "all":
    default:
      return true;
  }
}

export function materialReferenceAnalysisBadgeLabel(analysisState: MaterialAnalysisState): string {
  if (analysisState.ready) return analysisState.version > 0 ? `参考分析 v${analysisState.version}` : "参考分析完成";
  return materialReferenceAnalysisLabel(analysisState);
}

export function materialReferenceAnalysisLabel(analysisState: Pick<MaterialAnalysisState, "status">): string {
  if (analysisState.status === "failed") return "参考分析失败";
  if (analysisState.status === "running") return "参考分析中";
  return "等待参考分析";
}

function compareNewest(left: { id: string; created_at?: string; completed_at?: string }, right: { id: string; created_at?: string; completed_at?: string }): number {
  const leftTime = Date.parse(left.completed_at || left.created_at || "") || 0;
  const rightTime = Date.parse(right.completed_at || right.created_at || "") || 0;
  if (leftTime !== rightTime) return leftTime - rightTime;
  return left.id.localeCompare(right.id);
}
