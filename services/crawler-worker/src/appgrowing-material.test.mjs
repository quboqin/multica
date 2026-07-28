import test from "node:test";
import assert from "node:assert/strict";

import {
  appGrowingAppMaterialListVariables,
  appGrowingGraphQLDateWindow,
  appGrowingMaterialURL,
  appGrowingSearchAppVariables,
  captureAppGrowingMaterialPage,
  extractAppGrowingMaterials,
  isBrowserPageCrashError,
  shouldBlockAppGrowingCrawlResource,
  shouldUseAppGrowingBrowserFallback,
} from "./index.mjs";

test("extracts AppGrowing material resources from nested GraphQL list rows", () => {
  const materials = extractAppGrowingMaterials({
    data: {
      materialList: {
        data: [
          {
            material: {
              id: "easycash-material-1",
              duration: 2,
              impression_inc_2y: "20.1K",
              creative: {
                slogan: "Bunga tetap 0,1%",
                resource: [
                  {
                    id: "resource-1",
                    path: "//cdn.example.com/easycash.jpg",
                    poster: "https://cdn.example.com/easycash-poster.jpg",
                    width: 1080,
                    height: 1080,
                    format: "image",
                  },
                ],
              },
              campaign: {
                name: "Easycash - Pinjaman Daring",
              },
              media: [
                { id: 4, name: "AdMob" },
              ],
              platform: [
                { id: 1, name: "Android" },
              ],
              area: [
                { cc: "ID", name: "印度尼西亚" },
              ],
              language: [
                { code: "id", name: "印尼语" },
              ],
              landingPage: {
                link: "https://example.com/landing",
              },
            },
            highlight: {},
          },
        ],
      },
    },
  });

  assert.equal(materials.length, 1);
  assert.deepEqual(materials[0], {
    material_id: "easycash-material-1",
    title: "Bunga tetap 0,1%",
    duration_days: 2,
    impression_estimate: 20_100,
    asset_type: "image",
    preview_url: "https://cdn.example.com/easycash.jpg",
    resource_url: "https://cdn.example.com/easycash.jpg",
    poster_url: "https://cdn.example.com/easycash-poster.jpg",
    resource_format: "image",
    width: 1080,
    height: 1080,
    media_ids: [4],
    media_names: ["AdMob"],
    platform_ids: [1],
    platform_names: ["Android"],
    area_codes: ["ID"],
    area_names: ["印度尼西亚"],
    language_codes: ["id"],
    language_names: ["印尼语"],
    landing_url: "https://example.com/landing",
  });
});

test("uses AppGrowing material-search defaults that match the visible UI path", () => {
  const url = appGrowingMaterialURL(
    { probeURL: "https://appgrowing-global.youcloud.com/leaflet" },
    {
      competitor: "Easycash",
      pageNumber: 1,
      params: {
        date_range: "-29,0",
        media: [1, 2, 10, 16],
        area: ["ID"],
        language: ["id"],
        platform: [1],
      },
    },
  );

  const parsed = new URL(url);
  assert.equal(parsed.searchParams.get("keyword"), "Easycash");
  assert.equal(parsed.searchParams.get("order"), "_score_desc");
  assert.equal(parsed.searchParams.get("isSearchAiScene"), "0");
  assert.equal(parsed.searchParams.get("daterange"), "-29,0");
  assert.equal(parsed.searchParams.get("media"), "1,2,10,16");
  assert.equal(parsed.searchParams.get("area"), "ID");
  assert.equal(parsed.searchParams.get("language"), "id");
  assert.equal(parsed.searchParams.get("platform"), "1");
});

test("applies material-search filters on competitor URLs", () => {
  const url = appGrowingMaterialURL(
    { probeURL: "https://appgrowing-global.youcloud.com/leaflet" },
    {
      competitor: "Easycash",
      pageNumber: 2,
      params: {
        competitor_urls: {
          Easycash: "https://appgrowing-global.youcloud.com/appBrand/Vh1n0S5DbMuwFVmDX6dlGg==/leaflet?purpose=2&isAllDate=1",
        },
        area: ["ID"],
        platform: [2],
      },
    },
  );

  const parsed = new URL(url);
  assert.equal(parsed.pathname, "/appBrand/Vh1n0S5DbMuwFVmDX6dlGg==/leaflet");
  assert.equal(parsed.searchParams.get("page"), "2");
  assert.equal(parsed.searchParams.get("area"), "ID");
  assert.equal(parsed.searchParams.get("platform"), "2");
});


test("classifies AppGrowing browser page crashes", () => {
  assert.equal(isBrowserPageCrashError(new Error("page.goto: Page crashed")), true);
  assert.equal(isBrowserPageCrashError(new Error("Target page, context or browser has been closed")), true);
  assert.equal(isBrowserPageCrashError(new Error("net::ERR_ABORTED")), false);
});

test("blocks heavyweight AppGrowing fallback resources", () => {
  const request = (resourceType, url) => ({
    resourceType: () => resourceType,
    url: () => url,
  });

  assert.equal(shouldBlockAppGrowingCrawlResource(request("image", "https://cdn.example.com/ad.jpg")), true);
  assert.equal(shouldBlockAppGrowingCrawlResource(request("media", "https://cdn.example.com/ad.mp4")), true);
  assert.equal(shouldBlockAppGrowingCrawlResource(request("script", "https://www.googletagmanager.com/gtm.js")), true);
  assert.equal(shouldBlockAppGrowingCrawlResource(request("xhr", "https://api-appgrowing-global.youcloud.com/graphql")), false);
});

test("disables AppGrowing browser fallback by default for bulk material searches", () => {
  assert.equal(shouldUseAppGrowingBrowserFallback({}, 1), true);
  assert.equal(shouldUseAppGrowingBrowserFallback({}, 2), false);
  assert.equal(shouldUseAppGrowingBrowserFallback({ browser_capture_fallback: true }, 10), true);
  assert.equal(shouldUseAppGrowingBrowserFallback({ browser_capture_fallback: false }, 1), false);
});

test("captures AppGrowing browser page crashes as page-level errors", async () => {
  const handlers = new Map();
  let routeRegistered = false;
  let routeRemoved = false;
  const page = {
    route: async () => {
      routeRegistered = true;
    },
    unroute: async () => {
      routeRemoved = true;
    },
    on: (event, handler) => {
      handlers.set(event, handler);
    },
    off: (event, handler) => {
      if (handlers.get(event) === handler) {
        handlers.delete(event);
      }
    },
    goto: async () => {
      throw new Error("page.goto: Page crashed");
    },
    waitForLoadState: async () => null,
    mouse: { wheel: async () => null },
    keyboard: { press: async () => null },
    locator: () => ({ innerText: async () => "" }),
    title: async () => "",
    url: () => "about:blank",
  };

  const capture = await captureAppGrowingMaterialPage(
    page,
    {
      graphQLURL: "https://api-appgrowing-global.youcloud.com/graphql",
      probeURL: "https://appgrowing-global.youcloud.com/leaflet",
      anonymousTextPatterns: [],
    },
    {
      competitor: "Easycash",
      pageNumber: 2,
      params: { date_range: "-29,0" },
      captureTimeoutMS: 3000,
    },
  );

  assert.equal(routeRegistered, true);
  assert.equal(routeRemoved, true);
  assert.equal(handlers.has("response"), false);
  assert.equal(capture.pageCrashed, true);
  assert.match(capture.error, /Page crashed/);
  assert.equal(capture.materials.length, 0);
  assert.equal(capture.url.includes("page=2"), true);
});


test("builds AppGrowing GraphQL material-list variables from relative date ranges", () => {
  const now = new Date("2026-07-22T12:34:56Z");

  assert.deepEqual(appGrowingGraphQLDateWindow({ date_range: "-29,0" }, now), {
    startDate: "2026-06-23",
    endDate: "2026-07-22",
  });

  const variables = appGrowingAppMaterialListVariables(
    { purpose: 2, date_range: "-29,0", order: "_score_desc" },
    "brand-123",
    2,
    now,
  );

  assert.deepEqual(variables, {
    purpose: 2,
    startDate: "2026-06-23",
    endDate: "2026-07-22",
    field: "all",
    order: "impression_inc_2y_desc",
    page: 2,
    accurateSearch: 1,
    appBrand: "brand-123",
  });
});

test("builds AppGrowing searchApp variables for competitor brand lookup", () => {
  assert.deepEqual(appGrowingSearchAppVariables("Kredit Pintar", { purpose: 2 }), {
    purpose: 2,
    keyword: "Kredit Pintar",
    accurateSearch: 1,
    page: 1,
    hadAdvert: 1,
  });
});

test("extracts AppGrowing materials from detailed appMaterialList GraphQL results", () => {
  const materials = extractAppGrowingMaterials({
    data: {
      materialList: {
        total: 1,
        limit: 20,
        data: [
          {
            material: {
              id: "material-detail-1",
              duration: 35,
              impression_inc_2y: "12M",
              creative: {
                slogan: "Pinjaman cepat cair",
                resource: {
                  width: 720,
                  height: 1280,
                  format: "mp4",
                  path: "https://cdn.example.com/detail.mp4",
                  poster: "//cdn.example.com/detail-poster.jpg",
                },
              },
              campaign: {
                id: "brand-1",
                name: "Kredit Pintar",
              },
              platform: [
                { id: 2, name: "iOS" },
              ],
              area: [
                { cc: "ID", name: "印度尼西亚" },
              ],
            },
          },
        ],
      },
    },
  });

  assert.equal(materials.length, 1);
  assert.equal(materials[0].material_id, "material-detail-1");
  assert.equal(materials[0].title, "Pinjaman cepat cair");
  assert.equal(materials[0].duration_days, 35);
  assert.equal(materials[0].impression_estimate, 12_000_000);
  assert.equal(materials[0].asset_type, "video");
  assert.equal(materials[0].resource_url, "https://cdn.example.com/detail.mp4");
  assert.equal(materials[0].poster_url, "https://cdn.example.com/detail-poster.jpg");
});
