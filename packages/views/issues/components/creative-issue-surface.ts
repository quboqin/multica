import type { Issue } from "@multica/core/types";

export type CreativeIssueSurface =
  | { kind: "order"; orderId: string }
  | { kind: "migration" }
  | null;

export function getCreativeIssueSurface(issue: Pick<Issue, "metadata" | "parent_issue_id">): CreativeIssueSurface {
  if (issue.parent_issue_id) return null;

  const workflow = issue.metadata?.workflow;
  const orderId = issue.metadata?.creative_order_id;
  if ((workflow === "creative_order" || workflow === "creative_direct_edit") && typeof orderId === "string" && orderId) {
    return { kind: "order", orderId };
  }
  if (workflow === "creative_material") return { kind: "migration" };
  return null;
}
