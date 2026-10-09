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

function retainAssets(f, predicate) {
  f.release.assets = f.release.assets.filter(({ name }) => predicate(name));
  for (const name of Object.keys(f.contents)) {
    if (!predicate(name)) delete f.contents[name];
  }
}

const isMacDesktop = (name) => name.includes("-mac-") || /^latest(?:-x64)?-mac\.yml$/.test(name);

test("required publication succeeds without macOS desktop but still requires every CLI target", () => {
  const f = fixture();
  retainAssets(f, (name) => !isMacDesktop(name));
  verifyReleaseAssets(f.release, f.tag, f.contents, "required");
  assert.throws(() => verifyReleaseAssets(f.release, f.tag, f.contents), /Missing or incomplete asset/);
  assert.equal(expectedAssets(f.tag, "required").cli.length, 12);
  assert.ok(!expectedAssets(f.tag, "required").all.some(isMacDesktop));
  f.release.assets = f.release.assets.filter(({ name }) => name !== "multica-cli-1.2.3-darwin-arm64.tar.gz");
  assert.throws(() => verifyReleaseAssets(f.release, f.tag, f.contents, "required"), /Missing or incomplete asset/);
});

test("required publication still rejects missing Windows/Linux assets and broken checksums or feeds", () => {
  for (const mutate of [
    (f) => { f.release.assets = f.release.assets.filter(({ name }) => name !== "multica-desktop-1.2.3-windows-x64.exe"); },
    (f) => { f.release.assets = f.release.assets.filter(({ name }) => name !== "multica-desktop-1.2.3-linux-arm64.deb"); },
    (f) => { delete f.contents["checksums.txt"]; },
    (f) => { f.contents["latest-linux-arm64.yml"] = f.contents["latest-linux.yml"]; },
  ]) {
    const f = fixture();
    retainAssets(f, (name) => !isMacDesktop(name));
    mutate(f);
    assert.throws(() => verifyReleaseAssets(f.release, f.tag, f.contents, "required"));
  }
});

test("macOS publication independently requires both architectures and their updater references", () => {
  const f = fixture();
  retainAssets(f, isMacDesktop);
  verifyReleaseAssets(f.release, f.tag, f.contents, "mac");
  assert.equal(expectedAssets(f.tag, "mac").all.length, 10);
  for (const name of ["multica-desktop-1.2.3-mac-x64.dmg", "multica-desktop-1.2.3-mac-arm64.zip.blockmap"]) {
    const missing = { ...f.release, assets: f.release.assets.filter((asset) => asset.name !== name) };
    assert.throws(() => verifyReleaseAssets(missing, f.tag, f.contents, "mac"), /Missing or incomplete asset/);
  }
  f.contents["latest-x64-mac.yml"] = f.contents["latest-mac.yml"];
  assert.throws(() => verifyReleaseAssets(f.release, f.tag, f.contents, "mac"), /Wrong architecture/);
});

test("unknown verification scopes fail closed", () => {
  assert.throws(() => expectedAssets("v1.2.3", "typo"), /scope/);
});
