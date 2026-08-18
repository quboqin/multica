"use client";

import { useMemo, type ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { createI18n } from "./create-i18n";
import type { LocaleResources, SupportedLocale } from "./types";

let nextI18nInstanceKey = 0;
const i18nInstanceKeys = new WeakMap<object, number>();

// Remount consumers when the resource snapshot changes so their translation hooks see it.
function i18nInstanceKey(instance: object): number {
  const existing = i18nInstanceKeys.get(instance);
  if (existing !== undefined) return existing;
  const key = ++nextI18nInstanceKey;
  i18nInstanceKeys.set(instance, key);
  return key;
}

export interface I18nProviderProps {
  locale: SupportedLocale;
  resources: Record<string, LocaleResources>;
  children: ReactNode;
}

export function I18nProvider({
  locale,
  resources,
  children,
}: I18nProviderProps) {
  const instance = useMemo(() => createI18n(locale, resources), [locale, resources]);
  return <I18nextProvider key={i18nInstanceKey(instance)} i18n={instance}>{children}</I18nextProvider>;
}
