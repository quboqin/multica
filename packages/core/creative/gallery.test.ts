import { describe, expect, it } from "vitest";
import type { CreateCreativeFeedbackResponse } from "../types";
import { creativeGalleryAddKey, creativeGalleryEvents, creativeGalleryDeliverySelection } from "./gallery";
import type { CreativeOrderVariant } from "../types";

const entry = (id: string, overrides: Partial<CreateCreativeFeedbackResponse> = {}): CreateCreativeFeedbackResponse => ({
  id, idempotency_key: "", workspace_id: "workspace", issue_id: "issue", actor_type: "member", actor_id: "member",
  subject_type: "variant", subject_id: "variant", event_type: "decision", decision: "accepted", reason_codes: [], comment: "", context_snapshot: {}, undo_of_id: "", created_at: "2026-09-07T01:00:00Z", ...overrides,
});

describe("creative gallery membership", () => {
  it("does not resurrect an older acceptance when the latest membership is removed", () => {
    const removed = entry("undo", { event_type: "undo", decision: "", undo_of_id: "latest" });
    expect(creativeGalleryEvents([removed, entry("latest"), entry("old")]).size).toBe(0);
  });
  it("resolves historical entries from their recorded revision rather than the active version", () => {
    const variant = { active_revision: 2, revisions: [{ revision: 1, expected_sizes: ["1080x1080"] }] } as CreativeOrderVariant;
    expect(creativeGalleryDeliverySelection(variant, { context_snapshot: { revision: 1 } })).toEqual({ revision: 1, expectedSizes: ["1080x1080"] });
    expect(creativeGalleryDeliverySelection(variant, { context_snapshot: {} })).toEqual({ revision: 0, expectedSizes: [] });
    expect(creativeGalleryDeliverySelection(variant, { context_snapshot: { revision: 1, delivery_package_sizes: ["800x1000"], delivery_asset_ids: ["saved-asset"] } })).toEqual({ revision: 1, expectedSizes: ["800x1000"], assetIds: ["saved-asset"] });
  });
  it("keeps independent variants and removes only the undone membership", () => {
    const events = [entry("removed", { event_type: "undo", decision: "", undo_of_id: "added" }), entry("added"), entry("other", { subject_id: "variant-2" })];
    expect([...creativeGalleryEvents(events).keys()]).toEqual(["variant-2"]);
  });
  it("uses a new shared idempotency key after removal and permits re-adding", () => {
    const accepted = entry("added");
    const undone = entry("undo-1", { event_type: "undo", decision: "", undo_of_id: accepted.id });
    expect(creativeGalleryAddKey("variant", [])).toBe("gallery:variant");
    expect(creativeGalleryAddKey("variant", [accepted, undone])).toBe("gallery:variant:undo-1");
    expect(creativeGalleryEvents([entry("added-again"), undone, accepted]).get("variant")?.id).toBe("added-again");
  });
});
