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
    },
  });
}
