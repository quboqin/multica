import { describe, expect, it } from "vitest";
import en from "../locales/en/changelog.json";
import zhHans from "../locales/zh-Hans/changelog.json";
import ko from "../locales/ko/changelog.json";
import ja from "../locales/ja/changelog.json";
import {
  PLATFORM_RELEASE_ID,
  PLATFORM_RELEASE_IDS,
  PLATFORM_VERSION,
  platformReleaseContent,
} from "./releases";

const RELEASE_FIELDS = ["version", "date", "title", "summary"] as const;
const LOCALE_RESOURCES = [
  ["en", en],
  ["zh-Hans", zhHans],
  ["ko", ko],
  ["ja", ja],
] as const;

describe("platform release resources", () => {
  it("lists the current release first while preserving earlier releases", () => {
    expect(PLATFORM_RELEASE_ID).toBe("v0_3_52");
    expect(PLATFORM_VERSION).toBe("0.3.52");
    expect(PLATFORM_RELEASE_IDS).toEqual(["v0_3_52", "v0_3_51", "v0_3_50", "v0_3_49", "v0_3_48", "v0_3_47", "v0_3_23"]);
  });

  it("contains every release in every supported locale", () => {
    for (const [locale, resources] of LOCALE_RESOURCES) {
      for (const releaseId of PLATFORM_RELEASE_IDS) {
        const release = resources.releases[releaseId];

        expect(release).toBeDefined();
        expect(platformReleaseContent(locale, releaseId)).toEqual(release);
        for (const field of RELEASE_FIELDS) {
          expect(release[field]).toBeTruthy();
        }
        expect(release.changes.length).toBeGreaterThan(0);
        expect(release.changes).toHaveLength(en.releases[releaseId].changes.length);
        expect(new Set(release.changes).size).toBe(release.changes.length);
        for (const change of release.changes) expect(change.trim()).not.toBe("");
        expect(release.version).toBe(en.releases[releaseId].version);
        expect(release.date).toBe(en.releases[releaseId].date);
      }
    }

    expect(en.releases[PLATFORM_RELEASE_ID].version).toBe(PLATFORM_VERSION);
  });

  it("keeps release notes focused on user-facing changes without filler slots", () => {
    expect(zhHans.releases.v0_3_48.changes).toHaveLength(1);
    expect(zhHans.releases.v0_3_51.changes).toHaveLength(2);
    for (const [, resources] of LOCALE_RESOURCES) {
      expect(JSON.stringify(resources)).not.toMatch(/bootstrap|DesignDNA|LayoutPlan|request_id|change_[1-4]/i);
    }
  });
});
