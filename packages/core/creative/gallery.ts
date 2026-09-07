import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { CreateCreativeFeedbackResponse, CreativeFeedbackEventListResponse } from "../types";
import { creativeKeys } from "./queries";

type GalleryEvent = Pick<CreateCreativeFeedbackResponse, "id" | "undo_of_id" | "subject_type" | "subject_id" | "event_type" | "decision">;

export function creativeGalleryEvents<T extends GalleryEvent>(events: T[]): Map<string, T> {
  const undone = new Set(events.map((event) => event.undo_of_id).filter(Boolean));
  const active = new Map<string, T>();
  for (const event of events) {
    if (event.subject_type === "variant" && event.event_type === "decision" && event.decision === "accepted" && !undone.has(event.id) && !active.has(event.subject_id)) {
      active.set(event.subject_id, event);
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
    mutationFn: async (input: { variantId: string; orderId: string; itemId: string; issueId: string; variantKey: string; revision: number; removeEventId?: string }) => {
      if (input.removeEventId) return api.undoCreativeFeedback(input.removeEventId);
      const events = queryClient.getQueryData<CreativeFeedbackEventListResponse>(queryKey)?.events ?? [];
      const existing = creativeGalleryEvents(events).get(input.variantId);
      if (existing) return existing;
      return api.createCreativeFeedback({
        idempotency_key: creativeGalleryAddKey(input.variantId, events),
        issue_id: input.issueId, subject_type: "variant", subject_id: input.variantId,
        event_type: "decision", decision: "accepted",
        context_snapshot: { action: "add_to_gallery", order_id: input.orderId, item_id: input.itemId, variant_key: input.variantKey, revision: input.revision },
      });
    },
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<CreativeFeedbackEventListResponse>(queryKey);
      if (input.removeEventId && previous) {
        queryClient.setQueryData(queryKey, { events: previous.events.filter((event) => event.id !== input.removeEventId) });
      }
      return { previous };
    },
    onSuccess: (event) => {
      if (!event.id) throw new Error("Gallery update returned no feedback event");
      queryClient.setQueryData<CreativeFeedbackEventListResponse>(queryKey, (current) => ({ events: [event, ...(current?.events ?? []).filter((entry) => entry.id !== event.id)] }));
    },
    onError: (_error, _input, context) => {
      if (context?.previous) queryClient.setQueryData(queryKey, context.previous);
    },
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey });
      void queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}
