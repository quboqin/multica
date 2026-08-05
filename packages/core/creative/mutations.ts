import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { creativeKeys } from "./queries";

export interface AdoptCreativeOrderVariantVariables {
  itemId: string;
  variantId: string;
}

export function useAdoptCreativeOrderVariant(wsId: string, orderId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId, variantId }: AdoptCreativeOrderVariantVariables) =>
      api.adoptCreativeOrderVariant(orderId, itemId, { variant_id: variantId }),
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
