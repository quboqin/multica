import assert from "node:assert/strict";
import test from "node:test";
import { expectedAssets, verifyReleaseAssets } from "./verify-release-assets.mjs";

function fixture() {
  const tag = "v1.2.3";
  const expected = expectedAssets(tag);
  return {
    tag,
    release: { tag_name: tag, draft: false, prerelease: false, assets: expected.all.map((name) => ({
      name, state: "uploaded", size: 10, browser_download_url: `https://github.com/quboqin/multica/releases/download/${tag}/${name}`,
    })) },
    contents: {
      "checksums.txt": expected.cli.map((name) => `${"a".repeat(64)}  ${name}`).join("\n"),
      ...Object.fromEntries(Object.entries(expected.feeds).map(([feed, installer]) => [feed, `version: 1.2.3\nfiles:\n  - url: ${installer}\npath: ${installer}\n`])),
    },
  };
}
test("complete release covers all six CLI targets and desktop architecture feeds", () => {
  const f = fixture(); verifyReleaseAssets(f.release, f.tag, f.contents);
});
test("partial, foreign, draft and mismatched architecture releases fail closed", () => {
  for (const mutate of [
    (f) => f.release.assets.pop(),
    (f) => { f.release.assets[0].size = 0; },
    (f) => { f.release.assets[0].state = "new"; },
    (f) => { f.release.assets[0].browser_download_url = "https://github.com/multica-ai/multica"; },
    (f) => { f.release.draft = true; },
    (f) => { f.release.prerelease = true; },
    (f) => { f.contents["checksums.txt"] = f.contents["checksums.txt"].split("\n").slice(1).join("\n"); },
    (f) => { f.contents["latest-arm64.yml"] = f.contents["latest.yml"]; },
    (f) => { f.contents["latest.yml"] += "  - url: https://untrusted.example/app.exe\n"; },
    (f) => { f.contents["latest-mac.yml"] = f.contents["latest-mac.yml"].replace("1.2.3", "1.2.2"); },
  ]) {
    const f = fixture(); mutate(f);
    assert.throws(() => verifyReleaseAssets(f.release, f.tag, f.contents));
  }
});
