import type { Issue } from "@multica/core/types";

const WORKFLOW_STAGE_ORDER: Record<string, number> = {
  creative_plan: 10,
  creative_direct_edit: 15,
  creative_production: 20,
  creative_production_continuation: 20,
  creative_qc: 40,
  creative_delivery: 50,
};

function metadataNumber(issue: Issue, key: string): number | null {
  const value = issue.metadata[key];
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && /^\d+$/.test(value)) return Number(value);
  return null;
}

function variantOrder(issue: Issue): number {
  const variant = String(issue.metadata.variant ?? issue.metadata.creative_variant ?? "");
  const match = /^V(\d+)$/i.exec(variant);
  return match ? Number(match[1]) : 0;
}

/** Keeps workflow-created sub-issues in execution order even when they were created in parallel. */
export function sortWorkflowChildren(children: Issue[]): Issue[] {
  return [...children].sort((left, right) => {
    const leftWorkflow = String(left.metadata.workflow ?? "");
    const rightWorkflow = String(right.metadata.workflow ?? "");
    const leftExplicitStage = metadataNumber(left, "workflow_order") ?? WORKFLOW_STAGE_ORDER[leftWorkflow];
    const rightExplicitStage = metadataNumber(right, "workflow_order") ?? WORKFLOW_STAGE_ORDER[rightWorkflow];
    if (leftExplicitStage === undefined && rightExplicitStage === undefined) return 0;

    const leftStage = leftExplicitStage ?? Number.MAX_SAFE_INTEGER;
    const rightStage = rightExplicitStage ?? Number.MAX_SAFE_INTEGER;
    if (leftStage !== rightStage) return leftStage - rightStage;

    const leftVariant = variantOrder(left);
    const rightVariant = variantOrder(right);
    if (leftVariant !== rightVariant) return leftVariant - rightVariant;

    const leftRevision = metadataNumber(left, "revision") ?? metadataNumber(left, "creative_revision") ?? 0;
    const rightRevision = metadataNumber(right, "revision") ?? metadataNumber(right, "creative_revision") ?? 0;
    if (leftRevision !== rightRevision) return leftRevision - rightRevision;

    return left.identifier.localeCompare(right.identifier);
  });
}
