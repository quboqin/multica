import { describe, expect, it } from "vitest";
import type { CreateCreativeFeedbackResponse } from "../types";
import { creativeGalleryAddKey, creativeGalleryEvents } from "./gallery";

const entry = (id: string, overrides: Partial<CreateCreativeFeedbackResponse> = {}): CreateCreativeFeedbackResponse => ({
  id, idempotency_key: "", workspace_id: "workspace", issue_id: "issue", actor_type: "member", actor_id: "member",
  subject_type: "variant", subject_id: "variant", event_type: "decision", decision: "accepted", reason_codes: [], comment: "", context_snapshot: {}, undo_of_id: "", created_at: "2026-09-07T01:00:00Z", ...overrides,
});

describe("creative gallery membership", () => {
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
