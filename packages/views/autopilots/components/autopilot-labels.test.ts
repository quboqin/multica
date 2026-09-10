import { describe, expect, it } from "vitest";
import {
  autopilotExecutionModeLabelKey,
  autopilotRunSourceLabelKey,
  autopilotStatusLabelKey,
  autopilotTriggerKindLabelKey,
  webhookDeliveryStatusLabelKey,
  webhookSignatureStatusLabelKey,
} from "./autopilot-labels";

describe("autopilot label keys", () => {
  it("returns known i18n keys for current autopilot enums", () => {
    expect(autopilotStatusLabelKey("active")).toBe("active");
    expect(autopilotExecutionModeLabelKey("run_only")).toBe("run_only");
    expect(autopilotRunSourceLabelKey("webhook")).toBe("webhook");
    expect(autopilotTriggerKindLabelKey("schedule")).toBe("schedule");
    expect(webhookDeliveryStatusLabelKey("dispatched")).toBe("dispatched");
    expect(webhookSignatureStatusLabelKey("not_required")).toBe("not_required");
  });

  it("lets callers render future enum values as raw strings instead of crashing", () => {
    expect(autopilotStatusLabelKey("retired")).toBeNull();
    expect(autopilotExecutionModeLabelKey("dry_run")).toBeNull();
    expect(autopilotRunSourceLabelKey("creative_factory")).toBeNull();
    expect(autopilotTriggerKindLabelKey("event_bus")).toBeNull();
    expect(webhookDeliveryStatusLabelKey("rate_limited")).toBeNull();
    expect(webhookSignatureStatusLabelKey("expired")).toBeNull();
  });
});
