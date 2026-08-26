"use client";

import { useState } from "react";
import { Blocks } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Switch } from "@multica/ui/components/ui/switch";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  workspaceCapabilityKeys,
  workspaceCapabilitiesOptions,
} from "@multica/core/workspace/queries";
import { useUpdateWorkspaceCapability } from "@multica/core/workspace/mutations";
import { useT } from "../../i18n";

type CreativeFactoryMarketKey = "Indonesia" | "Malaysia";

type CreativeFactoryInitDraft = {
  market: CreativeFactoryMarketKey;
  brand: string;
  competitors: string;
  priorityCompetitors: string;
};

const CREATIVE_FACTORY_MARKETS: Record<CreativeFactoryMarketKey, { label: string; locale: string; currency: string; competitors: string; priorityCompetitors: string }> = {
  Indonesia: {
    label: "印尼",
    locale: "id-ID",
    currency: "IDR",
    competitors: "Easycash / Kredit Pintar / Adapundi / BantuSaku / Rupiah Cepat / UATAS / JULO",
    priorityCompetitors: "Easycash / Kredit Pintar / Adapundi",
  },
  Malaysia: {
    label: "马来",
    locale: "ms-MY",
    currency: "MYR",
    competitors: "",
    priorityCompetitors: "",
  },
};

function creativeFactoryDefaultDraft(market: CreativeFactoryMarketKey = "Indonesia"): CreativeFactoryInitDraft {
  const preset = CREATIVE_FACTORY_MARKETS[market];
  return {
    market,
    brand: "AdaKami",
    competitors: preset.competitors,
    priorityCompetitors: preset.priorityCompetitors,
  };
}

function creativeFactoryListInput(value: string): string[] {
  return value.split(/[\n,，、/]+/).map((item) => item.trim()).filter(Boolean);
}

export function FeaturesTab() {
  const { t } = useT("settings");
  const workspaceId = useWorkspaceId();
  const capabilities = useQuery(workspaceCapabilitiesOptions(workspaceId));
  const update = useUpdateWorkspaceCapability();
  const [initOpen, setInitOpen] = useState(false);
  const [initDraft, setInitDraft] = useState<CreativeFactoryInitDraft>(() => creativeFactoryDefaultDraft());
  const creative = capabilities.data?.items.find(
    (item) => item.key === workspaceCapabilityKeys.creativeFactory,
  );
  const canManage = capabilities.data?.can_manage === true;

  const handleChange = (enabled: boolean) => {
    if (!workspaceId || update.isPending) return;
    if (enabled && creative?.enabled !== true) {
      setInitDraft(creativeFactoryDefaultDraft());
      setInitOpen(true);
      return;
    }
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

  const confirmEnableCreativeFactory = () => {
    if (!workspaceId || update.isPending) return;
    const preset = CREATIVE_FACTORY_MARKETS[initDraft.market];
    update.mutate(
      {
        workspaceId,
        key: workspaceCapabilityKeys.creativeFactory,
        enabled: true,
        creativeFactory: {
          brand: initDraft.brand.trim() || "AdaKami",
          market: initDraft.market,
          locale: preset.locale,
          currency: preset.currency,
          competitors: creativeFactoryListInput(initDraft.competitors),
          priority_competitors: creativeFactoryListInput(initDraft.priorityCompetitors),
        },
      },
      {
        onSuccess: () => {
          setInitOpen(false);
          toast.success(t(($) => $.features.toast_enabled));
        },
        onError: (error) => {
          toast.error(error instanceof Error ? error.message : t(($) => $.features.toast_failed));
        },
      },
    );
  };

  const changeInitMarket = (value: string) => {
    const market: CreativeFactoryMarketKey = value === "Malaysia" ? "Malaysia" : "Indonesia";
    setInitDraft((current) => ({ ...creativeFactoryDefaultDraft(market), brand: current.brand || "AdaKami" }));
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

      <Dialog open={initOpen} onOpenChange={setInitOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>初始化创意工厂</DialogTitle>
            <DialogDescription>选择市场后，系统会自动创建对应的市场规则、文案库、采集智能体和 AutoPilot。</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="creative-factory-market">目标市场</Label>
                <NativeSelect id="creative-factory-market" value={initDraft.market} onChange={(event) => changeInitMarket(event.target.value)}>
                  <NativeSelectOption value="Indonesia">印尼</NativeSelectOption>
                  <NativeSelectOption value="Malaysia">马来</NativeSelectOption>
                </NativeSelect>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="creative-factory-brand">品牌</Label>
                <Input id="creative-factory-brand" value={initDraft.brand} onChange={(event) => setInitDraft((current) => ({ ...current, brand: event.target.value }))} />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="creative-factory-competitors">竞品</Label>
              <Textarea id="creative-factory-competitors" rows={3} value={initDraft.competitors} onChange={(event) => setInitDraft((current) => ({ ...current, competitors: event.target.value }))} placeholder="Easycash / Kredit Pintar / ..." />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="creative-factory-priority-competitors">优先竞品</Label>
              <Input id="creative-factory-priority-competitors" value={initDraft.priorityCompetitors} onChange={(event) => setInitDraft((current) => ({ ...current, priorityCompetitors: event.target.value }))} placeholder="默认取竞品列表前 3 个" />
            </div>
            <div className="grid gap-2 border-t pt-3 text-xs text-muted-foreground sm:grid-cols-3">
              <span>市场：{CREATIVE_FACTORY_MARKETS[initDraft.market].label}</span>
              <span>语言：{CREATIVE_FACTORY_MARKETS[initDraft.market].locale}</span>
              <span>币种：{CREATIVE_FACTORY_MARKETS[initDraft.market].currency}</span>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setInitOpen(false)} disabled={update.isPending}>取消</Button>
            <Button onClick={confirmEnableCreativeFactory} disabled={update.isPending || !creative}>开启</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {!canManage && !capabilities.isError && (
        <p className="text-xs text-muted-foreground">{t(($) => $.features.manage_hint)}</p>
      )}
    </div>
  );
}
