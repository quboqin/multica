import changelog from "../locales/en/changelog.json";
import jaChangelog from "../locales/ja/changelog.json";
import koChangelog from "../locales/ko/changelog.json";
import zhHansChangelog from "../locales/zh-Hans/changelog.json";
import type { SupportedLocale } from "@multica/core/i18n";

export const PLATFORM_VERSION = "0.3.49";

export const PLATFORM_RELEASE_ID = "v0_3_49" as const;

type ChangelogResources = typeof changelog;
export type PlatformReleaseId = keyof ChangelogResources["releases"];

type PlatformRelease = {
  version: string;
  date: string;
  title: string;
  summary: string;
  change_1: string;
  change_2: string;
  change_3: string;
  change_4: string;
};

type PlatformChangelog = {
  releases: Record<PlatformReleaseId, PlatformRelease>;
};

const localizedChangelogs = {
  en: changelog,
  "zh-Hans": zhHansChangelog,
  ko: koChangelog,
  ja: jaChangelog,
} satisfies Record<SupportedLocale, PlatformChangelog>;

export const PLATFORM_RELEASE_IDS = Object.keys(changelog.releases) as PlatformReleaseId[];

export function platformReleaseContent(
  locale: string,
  releaseId: PlatformReleaseId,
): PlatformRelease {
  const changelog = localizedChangelogs[locale as SupportedLocale] ?? localizedChangelogs.en;
  return changelog.releases[releaseId];
}
