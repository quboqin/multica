import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { CreateCreativeFeedbackResponse, CreativeFeedbackEventListResponse, CreativeOrderVariant } from "../types";
import { creativeKeys } from "./queries";

type GalleryEvent = Pick<CreateCreativeFeedbackResponse, "id" | "undo_of_id" | "subject_type" | "subject_id" | "event_type" | "decision">;

export interface CreativeGalleryDeliverySelection {
  revision: number;
  expectedSizes: string[];
  assetIds?: string[];
}

export function creativeGalleryDeliverySelection(variant: CreativeOrderVariant, event: Pick<CreateCreativeFeedbackResponse, "context_snapshot">): CreativeGalleryDeliverySelection {
  const snapshot = event.context_snapshot ?? {};
  const revision = typeof snapshot.revision === "number" && Number.isInteger(snapshot.revision) && snapshot.revision > 0 ? snapshot.revision : 0;
  const target = variant.revisions?.find((entry) => entry.revision === revision);
  const expectedSizes = Array.isArray(snapshot.delivery_package_sizes) ? snapshot.delivery_package_sizes : target?.expected_sizes ?? [];
  return {
    revision,
    expectedSizes: expectedSizes.every((size): size is string => typeof size === "string") ? expectedSizes : [],
    ...(snapshot.delivery_asset_ids !== undefined ? { assetIds: Array.isArray(snapshot.delivery_asset_ids) && snapshot.delivery_asset_ids.every((id): id is string => typeof id === "string") ? snapshot.delivery_asset_ids : [] } : {}),
  };
}

export function creativeGalleryEvents<T extends GalleryEvent>(events: T[]): Map<string, T> {
  const undone = new Set(events.map((event) => event.undo_of_id).filter(Boolean));
  const active = new Map<string, T>();
  const seen = new Set<string>();
  for (const event of events) {
    if (event.subject_type === "variant" && event.event_type === "decision" && event.decision === "accepted" && !seen.has(event.subject_id)) {
      seen.add(event.subject_id);
      if (!undone.has(event.id)) active.set(event.subject_id, event);
    }
  }
  return active;
}

export function creativeGalleryVariantIds(events: GalleryEvent[]): Set<string> {
  return new Set(creativeGalleryEvents(events).keys());
}

export function creativeGalleryAddKey(variantId: string, events: CreateCreativeFeedbackResponse[]): string {
  const latestUndo = events.filter((event) => event.subject_id === variantId && event.event_type === "undo")
    .sort((left, right) => right.created_at.localeCompare(left.created_at) || right.id.localeCompare(left.id))[0];
  return `gallery:${variantId}${latestUndo ? `:${latestUndo.id}` : ""}`;
}

export function useCreativeGalleryMutation(wsId: string) {
  const queryClient = useQueryClient();
  const queryKey = creativeKeys.feedback(wsId, "variant", "");
  return useMutation({
    mutationFn: async (input: { variantId: string; orderId: string; itemId: string; issueId: string; variantKey: string; revision: number; removeEventId?: string; qcRiskAcknowledged?: boolean; qcRiskReason?: string }) => {
      if (input.removeEventId) return api.undoCreativeFeedback(input.removeEventId);
      const events = queryClient.getQueryData<CreativeFeedbackEventListResponse>(queryKey)?.events ?? [];
      return api.confirmCreativeGalleryDelivery({
        idempotency_key: creativeGalleryAddKey(input.variantId, events),
        variant_id: input.variantId, revision: input.revision,
        qc_risk_acknowledged: input.qcRiskAcknowledged === true, qc_risk_reason: input.qcRiskReason?.trim() ?? "",
      });
    },
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<CreativeFeedbackEventListResponse>(queryKey);
      const original = previous?.events.find((event) => event.id === input.removeEventId);
      const optimisticUndoId = original ? `pending-gallery-remove:${original.id}` : "";
      if (original && previous) {
        queryClient.setQueryData(queryKey, { events: [{ ...original, id: optimisticUndoId, event_type: "undo", decision: "", undo_of_id: original.id }, ...previous.events] });
      }
      return { optimisticUndoId };
    },
    onSuccess: (event) => {
      if (!event.id) throw new Error("Gallery update returned no feedback event");
      queryClient.setQueryData<CreativeFeedbackEventListResponse>(queryKey, (current) => ({ events: [event, ...(current?.events ?? []).filter((entry) => entry.id !== event.id && entry.id !== `pending-gallery-remove:${event.undo_of_id}`)] }));
    },
    onError: (_error, _input, context) => {
      if (context?.optimisticUndoId) queryClient.setQueryData<CreativeFeedbackEventListResponse>(queryKey, (current) => ({ events: (current?.events ?? []).filter((event) => event.id !== context.optimisticUndoId) }));
    },
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey });
      await queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      void queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}
