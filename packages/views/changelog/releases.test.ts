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

const RELEASE_FIELDS = ["summary", "change_1", "change_2", "change_3", "change_4"] as const;
const LOCALE_RESOURCES = [
  ["en", en],
  ["zh-Hans", zhHans],
  ["ko", ko],
  ["ja", ja],
] as const;

describe("platform release resources", () => {
  it("lists the current release first while preserving earlier releases", () => {
    expect(PLATFORM_RELEASE_ID).toBe("v0_3_51");
    expect(PLATFORM_VERSION).toBe("0.3.51");
    expect(PLATFORM_RELEASE_IDS).toEqual(["v0_3_51", "v0_3_50", "v0_3_49", "v0_3_48", "v0_3_47", "v0_3_23"]);
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
      }
    }

    expect(en.releases[PLATFORM_RELEASE_ID].version).toBe(PLATFORM_VERSION);
  });
});
