import test from "node:test";
import assert from "node:assert/strict";

import {
  appGrowingAppMaterialListVariables,
  appGrowingBrandFromStrategyMemory,
  appGrowingBrowserFallbackCompetitors,
  appGrowingCompetitorDiagnostics,
  appGrowingGraphQLDateWindow,
  appGrowingGraphQLRequest,
  appGrowingMaterialDedupeKey,
  appGrowingMaterialURL,
  appGrowingSearchAppVariables,
  appGrowingSelectionMixSummary,
  appGrowingShouldUseAdaptiveBrowserFallback,
  captureAppGrowingMaterialPage,
  connectorForID,
  connectorGraphQLHeaders,
  extractAppGrowingMaterials,
  isBrowserPageCrashError,
  selectAppGrowingMaterials,
  shouldBlockAppGrowingCrawlResource,
  shouldUseAppGrowingBrowserFallback,
} from "./index.mjs";

test("uses the asset path instead of temporary auth parameters for material identity", () => {
  const first = appGrowingMaterialDedupeKey({
    material_id: "material-1",
    resource_url: "https://cdn.example.com/a.jpg?auth_key=first",
  });
  const second = appGrowingMaterialDedupeKey({
    material_id: "material-1",
    resource_url: "https://cdn.example.com/a.jpg?auth_key=second",
  });
  const sibling = appGrowingMaterialDedupeKey({
    material_id: "material-1",
    resource_url: "https://cdn.example.com/b.jpg?auth_key=first",
  });

  assert.equal(first, second);
  assert.notEqual(first, sibling);
});

test("filters previously seen assets before filling the requested material limit", () => {
  const rules = {
    new_materials: { ratio: 0.4, duration_days_lt: 7, impression_gt: 1000 },
    volume_materials: { ratio: 0.6, duration_days_gt: 30, impression_gte: 10_000_000 },
  };
  const materials = [
    { resource_url: "https://cdn.example.com/seen.jpg", duration_days: 2, impression_estimate: 20_000 },
    { resource_url: "https://cdn.example.com/new.jpg", duration_days: 3, impression_estimate: 30_000 },
    { resource_url: "https://cdn.example.com/volume.jpg", duration_days: 90, impression_estimate: 20_000_000 },
  ];
  const excludedKeys = new Set([appGrowingMaterialDedupeKey(materials[0])]);

  const selection = selectAppGrowingMaterials(materials, rules, 2, { excludedKeys });

  assert.equal(selection.excludedCount, 1);
  assert.deepEqual(selection.selected.map((item) => item.resource_url), [
    "https://cdn.example.com/new.jpg",
    "https://cdn.example.com/volume.jpg",
  ]);
});

test("reports the actual selected material mix instead of the configured target", () => {
  const selected = [
    ...Array.from({ length: 20 }, () => ({ bucket: "new" })),
    ...Array.from({ length: 5 }, () => ({ bucket: "volume" })),
  ];

  assert.deepEqual(
    appGrowingSelectionMixSummary(
      selected,
      {
        new_materials: { ratio: 0.4 },
        volume_materials: { ratio: 0.6 },
      },
      25,
    ),
    {
      target: { new_materials: 10, volume_materials: 15 },
      actual: { new_materials: 20, volume_materials: 5, other_materials: 0 },
      actual_ratio: { new_materials: 0.8, volume_materials: 0.2, other_materials: 0 },
      shortfall: { new_materials: 0, volume_materials: 10 },
      selected: 25,
      requested_limit: 25,
      ratio_target_met: false,
    },
  );
});

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

test("uses AppGrowing accepted GraphQL language header", () => {
  const headers = connectorGraphQLHeaders(connectorForID("appgrowing"), "searchApp");

  assert.equal(headers["accept-language"], "en");
});

test("classifies AppGrowing GraphQL HTTP rejections as capture errors", async () => {
  const connector = connectorForID("appgrowing");
  const context = {
    request: {
      async post(url, options) {
        assert.equal(url, connector.graphQLURL);
        assert.equal(options.headers["accept-language"], "en");
        return {
          status: () => 406,
          async text() {
            return "The Language: [en-US,en;q=0.9] is no acceptable";
          },
        };
      },
    },
  };

  const result = await appGrowingGraphQLRequest(context, connector, "searchApp", "query SearchApp { searchAppBrand { data } }", {});

  assert.equal(result.status, 406);
  assert.match(result.error, /^appgrowing_graphql_http_406/);
  assert.match(result.error, /Language/);
});

test("uses learned AppGrowing brand ids from adaptive strategy memory", () => {
  const brand = appGrowingBrandFromStrategyMemory({
    _adaptive_strategy_memory: {
      enabled: true,
      memories: {
        easycash: {
          brand_id: "brand-memory-1",
          brand_name: "Easycash",
        },
      },
    },
  }, "Easycash");

  assert.deepEqual(brand, {
    id: "brand-memory-1",
    name: "Easycash",
    source: "strategy_memory",
  });
});

test("enables adaptive browser fallback after GraphQL path failures", () => {
  assert.equal(appGrowingShouldUseAdaptiveBrowserFallback([
    { source: "graphql_api", error: "appgrowing_graphql_http_406" },
    { source: "graphql_api", error: "app_brand_not_found" },
  ], { memories: {} }), true);

  assert.equal(appGrowingShouldUseAdaptiveBrowserFallback([
    { source: "graphql_api", error: "" },
  ], { memories: {} }), false);

  assert.equal(appGrowingShouldUseAdaptiveBrowserFallback([], {
    memories: { easycash: { preferred_source: "browser_network" } },
  }), true);
});

test("falls back per competitor when only some GraphQL pages return material", () => {
  const competitors = ["Easycash", "Kredit Pintar", "Adapundi"];
  const captured = [
    { source: "graphql_api", competitor: "Easycash", page: 1, materials_found: 12, error: "" },
    { source: "graphql_api", competitor: "Kredit Pintar", page: 1, materials_found: 0, error: "appgrowing_graphql_http_406" },
    { source: "graphql_api", competitor: "Kredit Pintar", page: 2, materials_found: 0, error: "appgrowing_graphql_http_406" },
    { source: "graphql_api", competitor: "Adapundi", page: 1, materials_found: 0, error: "" },
  ];

  assert.deepEqual(appGrowingBrowserFallbackCompetitors(
    competitors,
    captured,
    {},
    { memories: {} },
    false,
    true,
  ), ["Kredit Pintar", "Adapundi"]);
  assert.deepEqual(appGrowingBrowserFallbackCompetitors(
    competitors,
    captured,
    { browser_capture_fallback: false },
    { memories: {} },
    false,
    true,
  ), []);
});

test("reports multi-page coverage and per-competitor browser fallback", () => {
  const diagnostics = appGrowingCompetitorDiagnostics(
    ["Easycash", "Kredit Pintar"],
    new Set(["easycash"]),
    3,
    5,
    [
      { source: "graphql_api", competitor: "Easycash", page: 1, materials_found: 3, error: "" },
      { source: "graphql_api", competitor: "Easycash", page: 2, materials_found: 2, error: "" },
      { source: "graphql_api", competitor: "Easycash", page: 3, materials_found: 0, error: "" },
      { source: "graphql_api", competitor: "Easycash", page: 4, materials_found: 1, error: "" },
      { source: "graphql_api", competitor: "Easycash", page: 5, materials_found: 0, error: "" },
      { source: "graphql_api", competitor: "Kredit Pintar", page: 1, materials_found: 0, error: "appgrowing_graphql_http_406" },
      { source: "browser_network", competitor: "Kredit Pintar", page: 1, materials_found: 4, error: "" },
      { source: "browser_network", competitor: "Kredit Pintar", page: 2, materials_found: 0, error: "" },
      { source: "browser_network", competitor: "Kredit Pintar", page: 3, materials_found: 0, error: "material_search_time_budget_exhausted", skipped: true },
    ],
    [
      { competitor: "Easycash" },
      { competitor: "Kredit Pintar" },
      { competitor: "Kredit Pintar" },
    ],
  );

  assert.deepEqual(diagnostics[0], {
    competitor: "Easycash",
    priority: true,
    requested_pages: 5,
    coverage_complete: true,
    status: "found",
    materials_found: 6,
    selected: 1,
    browser_fallback_used: false,
    errors: [],
    pages: [
      { page: 1, status: "found", materials_found: 3, attempts: [{ source: "graphql_api", materials_found: 3, error: "", needs_reauth: false, skipped: false }] },
      { page: 2, status: "found", materials_found: 2, attempts: [{ source: "graphql_api", materials_found: 2, error: "", needs_reauth: false, skipped: false }] },
      { page: 3, status: "no_match", materials_found: 0, attempts: [{ source: "graphql_api", materials_found: 0, error: "", needs_reauth: false, skipped: false }] },
      { page: 4, status: "found", materials_found: 1, attempts: [{ source: "graphql_api", materials_found: 1, error: "", needs_reauth: false, skipped: false }] },
      { page: 5, status: "no_match", materials_found: 0, attempts: [{ source: "graphql_api", materials_found: 0, error: "", needs_reauth: false, skipped: false }] },
    ],
  });
  assert.equal(diagnostics[1].requested_pages, 3);
  assert.equal(diagnostics[1].coverage_complete, false);
  assert.equal(diagnostics[1].status, "found_incomplete");
  assert.equal(diagnostics[1].browser_fallback_used, true);
  assert.equal(diagnostics[1].pages[2].status, "budget_exhausted");
  assert.equal(diagnostics[1].selected, 2);
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
