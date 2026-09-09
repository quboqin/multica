"use client";

import { useId } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { creativeKeys, creativeRetrySettingsOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { Switch } from "@multica/ui/components/ui/switch";
import { toast } from "sonner";
import { useT } from "../../i18n";

export function CreativeRetryToggle() {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const id = useId();
  const queryClient = useQueryClient();
  const settings = useQuery(creativeRetrySettingsOptions(wsId));
  const queryKey = creativeKeys.retrySettings(wsId);
  const mutation = useMutation({
    mutationFn: (enabled: boolean) => api.updateCreativeRetrySettings(enabled),
    onMutate: async (enabled) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData(queryKey);
      queryClient.setQueryData(queryKey, { ...settings.data, automatic_retry_enabled: enabled });
      return { previous };
    },
    onError: (_error, _enabled, context) => {
      queryClient.setQueryData(queryKey, context?.previous);
      toast.error(t(($) => $.automaticRetry.updateFailed));
    },
    onSettled: () => { void queryClient.invalidateQueries({ queryKey }); },
  });
  return <div className="flex shrink-0 items-center gap-2" title={t(($) => $.automaticRetry.description)}>
    <label htmlFor={id} className="text-xs">{t(($) => $.automaticRetry.label)}</label>
    <Switch id={id} size="sm" checked={settings.data?.automatic_retry_enabled ?? false}
      disabled={!settings.data?.can_manage || mutation.isPending || settings.isError}
      onCheckedChange={(checked) => mutation.mutate(checked)} />
    {settings.data && !settings.data.automatic_retry_enabled && <span className="text-xs text-muted-foreground">{t(($) => $.automaticRetry.paused)}</span>}
  </div>;
}
