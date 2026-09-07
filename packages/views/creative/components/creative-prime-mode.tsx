import { creativeOrderPrimeConfig, type CreativePrimeMode } from "@multica/core/creative";
import type { CreativeOrder } from "@multica/core/types";
import { useT } from "../../i18n";

type CreativeT = ReturnType<typeof useT<"creative">>["t"];

export function creativePrimeModeLabel(t: CreativeT, mode: CreativePrimeMode): string {
  if (mode === "model_integrated") return t(($) => $.primeMode.modelIntegrated);
  if (mode === "deterministic") return t(($) => $.primeMode.deterministic);
  return t(($) => $.primeMode.unknown);
}

export function creativePrimeFamilyLabel(t: CreativeT, family: string): string {
  if (family === "light_background") return t(($) => $.resourceFiles.family.lightTitle);
  if (family === "dark_background") return t(($) => $.resourceFiles.family.darkTitle);
  return family;
}

export function CreativeOrderPrimeSummary({ order }: { order: Pick<CreativeOrder, "input_snapshot"> }) {
  const { t } = useT("creative");
  const config = creativeOrderPrimeConfig(order);
  return <p className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground" data-testid="creative-order-prime-mode">
    <span>{t(($) => $.primeMode.orderMode)}: <span className="text-foreground">{creativePrimeModeLabel(t, config.mode)}</span></span>
    {config.templateFamilyId && <span>{t(($) => $.primeMode.templateFamily)}: {creativePrimeFamilyLabel(t, config.templateFamilyId)}</span>}
  </p>;
}
