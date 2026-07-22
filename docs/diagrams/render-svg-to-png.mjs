import fs from "node:fs";
import path from "node:path";
import { chromium } from "playwright";

const [inputArg, outputArg] = process.argv.slice(2);

if (!inputArg || !outputArg) {
  console.error("Usage: node render-svg-to-png.mjs <input.svg> <output.png>");
  process.exit(1);
}

const inputPath = path.resolve(inputArg);
const outputPath = path.resolve(outputArg);
const svg = fs.readFileSync(inputPath, "utf8");
const viewBoxMatch = svg.match(/viewBox="0 0 ([0-9.]+) ([0-9.]+)"/);
const widthMatch = svg.match(/width="([0-9.]+)"/);
const heightMatch = svg.match(/height="([0-9.]+)"/);
const width = Math.round(Number(widthMatch?.[1] ?? viewBoxMatch?.[1] ?? 1280));
const height = Math.round(Number(heightMatch?.[1] ?? viewBoxMatch?.[2] ?? 820));

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({
  viewport: { width, height },
  deviceScaleFactor: 2,
});

await page.setContent(
  `<!doctype html>
  <html>
    <head>
      <meta charset="utf-8" />
      <style>
        html, body { margin: 0; width: ${width}px; height: ${height}px; background: #ffffff; overflow: hidden; }
        img { display: block; width: ${width}px; height: ${height}px; }
      </style>
    </head>
    <body>
      <img src="data:image/svg+xml;base64,${Buffer.from(svg).toString("base64")}" alt="" />
    </body>
  </html>`,
  { waitUntil: "networkidle" },
);

await page.screenshot({ path: outputPath, type: "png", fullPage: false });
await browser.close();
console.log(`Rendered ${outputPath} (${width}x${height} @2x)`);
