"use client";

import { useId } from "react";
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { creativeKeys, creativeRetrySettingsOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { Switch } from "@multica/ui/components/ui/switch";
import { toast } from "sonner";
import { useT } from "../../i18n";

export function CreativeRetryToggle() {
  return <div className="flex flex-wrap items-center gap-4"><CreativePolicyToggle kind="automatic_retry_enabled" /><CreativePolicyToggle kind="visual_rework_enabled" /></div>;
}

function CreativePolicyToggle({ kind }: { kind: "automatic_retry_enabled" | "visual_rework_enabled" }) {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const id = useId();
  const queryClient = useQueryClient();
  const settings = useQuery(creativeRetrySettingsOptions(wsId));
  const queryKey = creativeKeys.retrySettings(wsId);
  const saving = useIsMutating({ mutationKey: queryKey }) > 0;
  const mutation = useMutation({
    mutationKey: queryKey,
    mutationFn: (enabled: boolean) => kind === "automatic_retry_enabled" ? api.updateCreativeRetrySettings(enabled) : api.updateCreativeRetrySettings(enabled, kind),
    onMutate: async (enabled) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData(queryKey);
      queryClient.setQueryData(queryKey, { ...settings.data, [kind]: enabled });
      return { previous };
    },
    onError: (_error, _enabled, context) => {
      queryClient.setQueryData(queryKey, context?.previous);
      toast.error(t(($) => $.automaticRetry.updateFailed));
    },
    onSettled: () => { void queryClient.invalidateQueries({ queryKey }); },
  });
  return <div className="flex shrink-0 items-center gap-2" title={t(($) => kind === "automatic_retry_enabled" ? $.automaticRetry.description : $.automaticRetry.reworkDescription)}>
    <label htmlFor={id} className="text-xs">{t(($) => kind === "automatic_retry_enabled" ? $.automaticRetry.label : $.automaticRetry.reworkLabel)}</label>
    <Switch id={id} size="sm" checked={settings.data?.[kind] ?? false}
      disabled={!settings.data?.can_manage || saving || settings.isError}
      onCheckedChange={(checked) => mutation.mutate(checked)} />
    {settings.data && !settings.data[kind] && <span className="text-xs text-muted-foreground">{t(($) => $.automaticRetry.paused)}</span>}
  </div>;
}
