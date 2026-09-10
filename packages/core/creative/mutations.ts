import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { creativeKeys } from "./queries";

export interface AdoptCreativeOrderVariantVariables {
  itemId: string;
  variantId: string;
  qcRiskAcknowledged?: boolean;
  qcRiskReason?: string;
}

export function useAdoptCreativeOrderVariant(wsId: string, orderId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId, variantId, qcRiskAcknowledged, qcRiskReason }: AdoptCreativeOrderVariantVariables) =>
      api.adoptCreativeOrderVariant(orderId, itemId, {
        variant_id: variantId,
        ...(qcRiskAcknowledged ? { qc_risk_acknowledged: true, qc_risk_reason: qcRiskReason?.trim() } : {}),
      }),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}

export function useCancelCreativeOrder(wsId: string, orderId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.cancelCreativeOrder(orderId),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}

export function useDeleteCreativeOrder(wsId: string, orderId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.deleteCreativeOrder(orderId),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}

export function useUnadoptCreativeOrderVariant(wsId: string, orderId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId }: { itemId: string }) => api.unadoptCreativeOrderVariant(orderId, itemId),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}

export function useSelectCreativeOrderVariantRevision(wsId: string, orderId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ variantId, revision }: { variantId: string; revision: number }) =>
      api.selectCreativeOrderVariantRevision(orderId, variantId, revision),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.order(wsId, orderId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) });
      queryClient.invalidateQueries({ queryKey: creativeKeys.feedbackDashboard(wsId) });
    },
  });
}
