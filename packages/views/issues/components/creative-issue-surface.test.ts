import { describe, expect, it } from "vitest";
import { getCreativeIssueSurface } from "./creative-issue-surface";

describe("getCreativeIssueSurface", () => {
  it("keeps creative orders on the order summary only", () => {
    expect(getCreativeIssueSurface({ parent_issue_id: null, metadata: {
      workflow: "creative_order",
      creative_order_id: "order-1",
    } })).toEqual({ kind: "order", orderId: "order-1" });
  });

  it("shows the same order summary for direct image edit issues", () => {
    expect(getCreativeIssueSurface({ parent_issue_id: null, metadata: {
      workflow: "creative_direct_edit",
      creative_order_id: "direct-order-1",
    } })).toEqual({ kind: "order", orderId: "direct-order-1" });
  });

  it("moves historical material workflow issues to the read-only migration entry", () => {
    expect(getCreativeIssueSurface({ parent_issue_id: null, metadata: {
      workflow: "creative_material",
    } })).toEqual({ kind: "migration" });
  });

  it("does not expose either creative surface on child or unrelated issues", () => {
    expect(getCreativeIssueSurface({ parent_issue_id: "parent-1", metadata: { workflow: "creative_material" } })).toBeNull();
    expect(getCreativeIssueSurface({ parent_issue_id: null, metadata: { workflow: "creative_collection" } })).toBeNull();
    expect(getCreativeIssueSurface({ parent_issue_id: null, metadata: { workflow: "creative_order" } })).toBeNull();
    expect(getCreativeIssueSurface({ parent_issue_id: null, metadata: { workflow: "creative_direct_edit" } })).toBeNull();
  });
});
