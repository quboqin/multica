import test from "node:test";
import assert from "node:assert/strict";

import {
  appGrowingMaterialURL,
  extractAppGrowingMaterials,
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
