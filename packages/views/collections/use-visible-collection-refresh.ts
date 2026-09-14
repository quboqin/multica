"use client";

import { useEffect, useRef } from "react";
import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import {
  captureClientWorkspaceAccessGeneration,
  getCurrentSlug,
  getCurrentWsId,
  isClientWorkspaceAccessGenerationCurrent,
} from "@multica/core/platform";

export function useVisibleCollectionRefresh({
  enabled,
  workspaceId,
  workspaceSlug,
  queryKey,
}: {
  enabled: boolean;
  workspaceId: string;
  workspaceSlug: string;
  queryKey: QueryKey;
}) {
  const queryClient = useQueryClient();
  const queryKeyRef = useRef(queryKey);
  queryKeyRef.current = queryKey;

  useEffect(() => {
    if (!enabled || typeof window === "undefined") return;
    const workspaceAccessGeneration = captureClientWorkspaceAccessGeneration(
      queryClient,
      workspaceId,
    );
    const refresh = () => {
      if (
        getCurrentWsId() !== workspaceId ||
        getCurrentSlug() !== workspaceSlug ||
        !isClientWorkspaceAccessGenerationCurrent(
          queryClient,
          workspaceId,
          workspaceAccessGeneration,
        ) ||
        (typeof document !== "undefined" &&
          document.visibilityState !== "visible")
      ) {
        return;
      }
      void queryClient.invalidateQueries({ queryKey: queryKeyRef.current });
    };
    const handleVisibility = () => {
      if (document.visibilityState === "visible") refresh();
    };
    const interval = window.setInterval(refresh, 30_000);
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [enabled, queryClient, workspaceId, workspaceSlug]);
}
