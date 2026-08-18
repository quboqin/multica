"use client";

import { Blocks } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  workspaceCapabilityKeys,
  workspaceCapabilitiesOptions,
} from "@multica/core/workspace/queries";
import { useUpdateWorkspaceCapability } from "@multica/core/workspace/mutations";
import { useT } from "../../i18n";

export function FeaturesTab() {
  const { t } = useT("settings");
  const workspaceId = useWorkspaceId();
  const capabilities = useQuery(workspaceCapabilitiesOptions(workspaceId));
  const update = useUpdateWorkspaceCapability();
  const creative = capabilities.data?.items.find(
    (item) => item.key === workspaceCapabilityKeys.creativeFactory,
  );
  const canManage = capabilities.data?.can_manage === true;

  const handleChange = (enabled: boolean) => {
    if (!workspaceId || update.isPending) return;
    update.mutate(
      { workspaceId, key: workspaceCapabilityKeys.creativeFactory, enabled },
      {
        onSuccess: (_, { enabled: wasEnabled }) => {
          if (wasEnabled) {
            toast.success(t(($) => $.features.toast_enabled));
          }
        },
        onError: (error) => {
          toast.error(error instanceof Error ? error.message : t(($) => $.features.toast_failed));
        },
      },
    );
  };

  return (
    <div className="space-y-8">
      <section className="space-y-1">
        <p className="text-sm text-muted-foreground">{t(($) => $.features.page_description)}</p>
      </section>

      <section className="space-y-3">
        <Card>
          <CardContent>
            <div className="flex items-start justify-between gap-4">
              <div className="flex min-w-0 items-start gap-3">
                <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
                  <Blocks className="h-4 w-4" />
                </div>
                <div className="min-w-0 space-y-1">
                  <Label htmlFor="creative-factory-capability" className="text-sm font-medium">
                    {t(($) => $.features.creative_factory_title)}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {t(($) => $.features.creative_factory_description)}
                  </p>
                  {capabilities.isError ? (
                    <p className="text-xs text-destructive">{t(($) => $.features.load_failed)}</p>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      {creative?.enabled
                        ? t(($) => $.features.creative_factory_enabled)
                        : t(($) => $.features.creative_factory_disabled)}
                    </p>
                  )}
                </div>
              </div>
              <Switch
                id="creative-factory-capability"
                checked={creative?.enabled === true}
                onCheckedChange={handleChange}
                disabled={!creative || !canManage || capabilities.isError || update.isPending}
              />
            </div>
          </CardContent>
        </Card>
      </section>

      {!canManage && !capabilities.isError && (
        <p className="text-xs text-muted-foreground">{t(($) => $.features.manage_hint)}</p>
      )}
    </div>
  );
}
