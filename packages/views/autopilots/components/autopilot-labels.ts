import type {
  AutopilotExecutionMode,
  AutopilotRunSource,
  AutopilotStatus,
  AutopilotTriggerKind,
  WebhookDeliveryStatus,
  WebhookSignatureStatus,
} from "@multica/core/types";

export function autopilotStatusLabelKey(status: string): AutopilotStatus | null {
  switch (status) {
    case "active":
    case "paused":
    case "archived":
      return status;
    default:
      return null;
  }
}

export function autopilotExecutionModeLabelKey(mode: string): AutopilotExecutionMode | null {
  switch (mode) {
    case "create_issue":
    case "run_only":
      return mode;
    default:
      return null;
  }
}

export function autopilotRunSourceLabelKey(source: string): AutopilotRunSource | null {
  switch (source) {
    case "schedule":
    case "manual":
    case "webhook":
    case "api":
      return source;
    default:
      return null;
  }
}

export function autopilotTriggerKindLabelKey(kind: string): AutopilotTriggerKind | null {
  switch (kind) {
    case "schedule":
    case "webhook":
    case "api":
      return kind;
    default:
      return null;
  }
}

export function webhookDeliveryStatusLabelKey(status: string): WebhookDeliveryStatus | null {
  switch (status) {
    case "queued":
    case "dispatched":
    case "rejected":
    case "ignored":
    case "failed":
      return status;
    default:
      return null;
  }
}

export function webhookSignatureStatusLabelKey(status: string): WebhookSignatureStatus | null {
  switch (status) {
    case "not_required":
    case "valid":
    case "invalid":
    case "missing":
      return status;
    default:
      return null;
  }
}
