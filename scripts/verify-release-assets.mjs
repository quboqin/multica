#!/usr/bin/env node
import { pathToFileURL } from "node:url";

export function expectedAssets(tag) {
  if (!/^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(tag)) throw new Error("Invalid stable release tag");
  const version = tag.slice(1);
  const cli = [];
  for (const os of ["darwin", "linux", "windows"]) {
    for (const arch of ["amd64", "arm64"]) {
      const ext = os === "windows" ? "zip" : "tar.gz";
      cli.push(`multica-cli-${version}-${os}-${arch}.${ext}`, `multica_${os}_${arch}.${ext}`);
    }
  }
  const desktop = [];
  for (const arch of ["x64", "arm64"]) {
    for (const ext of ["dmg", "zip"]) {
      const name = `multica-desktop-${version}-mac-${arch}.${ext}`;
      desktop.push(name, `${name}.blockmap`);
    }
    const win = `multica-desktop-${version}-windows-${arch}.exe`;
    desktop.push(win, `${win}.blockmap`);
  }
  for (const [ext, arches] of Object.entries({ AppImage: ["x86_64", "arm64"], deb: ["amd64", "arm64"], rpm: ["x86_64", "aarch64"] })) {
    for (const arch of arches) desktop.push(`multica-desktop-${version}-linux-${arch}.${ext}`);
  }
  const feeds = {
    "latest.yml": `multica-desktop-${version}-windows-x64.exe`,
    "latest-arm64.yml": `multica-desktop-${version}-windows-arm64.exe`,
    "latest-mac.yml": `multica-desktop-${version}-mac-arm64.zip`,
    "latest-x64-mac.yml": `multica-desktop-${version}-mac-x64.zip`,
    "latest-linux.yml": `multica-desktop-${version}-linux-x86_64.AppImage`,
    "latest-linux-arm64.yml": `multica-desktop-${version}-linux-arm64.AppImage`,
  };
  return { cli, feeds, all: [...cli, ...desktop, ...Object.keys(feeds), "checksums.txt"] };
}

export function verifyReleaseAssets(release, tag, contents) {
  if (release.tag_name !== tag || release.draft || release.prerelease) throw new Error("Expected the published stable release");
  const expected = expectedAssets(tag);
  const assets = new Map(release.assets.map((asset) => [asset.name, asset]));
  for (const name of expected.all) {
    const asset = assets.get(name);
    if (!asset || asset.size <= 0 || asset.state !== "uploaded") throw new Error(`Missing or incomplete asset: ${name}`);
    if (asset.browser_download_url !== `https://github.com/quboqin/multica/releases/download/${tag}/${name}`) {
      throw new Error(`Unexpected download source: ${name}`);
    }
  }
  const checksums = new Map((contents["checksums.txt"] ?? "").trim().split(/\r?\n/).map((line) => {
    const match = /^([a-f0-9]{64})\s+\*?(\S+)$/.exec(line);
    if (!match) throw new Error("Malformed checksums.txt");
    return [match[2], match[1]];
  }));
  for (const name of expected.cli) {
    if (!checksums.has(name)) throw new Error(`Missing CLI checksum: ${name}`);
  }
  // Validate the references in electron-builder's generated YAML. Do not
  // interpret or execute arbitrary YAML tags. Packaging owns YAML generation.
  for (const [feed, installer] of Object.entries(expected.feeds)) {
    const text = contents[feed] ?? "";
    if (!text.split(/\r?\n/).includes(`version: ${tag.slice(1)}`)) throw new Error(`Wrong version in ${feed}`);
    const refs = [...text.matchAll(/^\s*(?:-\s*)?(?:url|path):\s*(\S+)\s*$/gm)].map((match) => match[1]);
    if (!refs.includes(installer)) throw new Error(`Wrong architecture or installer in ${feed}`);
    for (const ref of refs) {
      if (!assets.has(ref) || !expected.all.includes(ref)) throw new Error(`Unknown asset reference in ${feed}: ${ref}`);
    }
  }
}

async function main() {
  const tag = process.argv[2];
  const { feeds } = expectedAssets(tag);
  const response = await fetch(`https://api.github.com/repos/quboqin/multica/releases/tags/${tag}`, {
    headers: { Authorization: `Bearer ${process.env.GH_TOKEN}`, Accept: "application/vnd.github+json" },
    signal: AbortSignal.timeout(30_000),
  });
  if (!response.ok) throw new Error(`Release API: HTTP ${response.status}`);
  const release = await response.json();
  const contents = {};
  for (const name of ["checksums.txt", ...Object.keys(feeds)]) {
    // Public release URLs: never forward the Actions credential to redirects.
    const download = await fetch(`https://github.com/quboqin/multica/releases/download/${tag}/${name}`, { signal: AbortSignal.timeout(30_000) });
    if (!download.ok) throw new Error(`Cannot download ${name}: HTTP ${download.status}`);
    contents[name] = await download.text();
  }
  verifyReleaseAssets(release, tag, contents);
  console.log(`All CLI/Desktop assets and update references are present for ${tag}.`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => { console.error(error.message); process.exitCode = 1; });
}
