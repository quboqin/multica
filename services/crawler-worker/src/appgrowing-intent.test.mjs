import test from "node:test";
import assert from "node:assert/strict";

import {
  normalizeAppGrowingMaterialSearchParams,
  normalizeMaterialRules,
  parseAppGrowingMaterialSearchIntent,
} from "./appgrowing-intent.mjs";

test("parses weekly AppGrowing material search intent", () => {
  const intent = `
每周抓取 AppGrowing 印尼现金贷竞品素材。

竞品：
Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO

Easycash、Kredit Pintar、Adapundi 优先。
Easycash 品牌页：https://appgrowing-global.youcloud.com/appBrand/Vh1n0S5DbMuwFVmDX6dlGg==/leaflet?purpose=2&isAllDate=1

操作路径：
创意 -> 素材搜索 -> 输入竞品名称 -> 筛选素材

筛选：
- 新素材：占比 40%，投放天数 < 7 天，曝光估算 > 1K
- 跑量素材：占比 60%，投放天数 > 30 天，曝光估算 >= 10M
媒体：Meta Ads、AdMob、Google Ads
投放地区：印尼
语言：印尼语
设备：Android、iOS

抓取最近 30 天的数据，最多输出 25 条素材。
如果没有命中，说明原因即可。
`;

  const parsed = parseAppGrowingMaterialSearchIntent(intent);

  assert.deepEqual(parsed.competitors, [
    "Easycash",
    "Kredit Pintar",
    "Adapundi",
    "BantuSaku",
    "Rupiah Cepat",
    "UATAS",
    "JULO",
  ]);
  assert.deepEqual(parsed.priority_competitors, ["Easycash", "Kredit Pintar", "Adapundi"]);
  assert.equal(
    parsed.competitor_urls.Easycash,
    "https://appgrowing-global.youcloud.com/appBrand/Vh1n0S5DbMuwFVmDX6dlGg==/leaflet?purpose=2&isAllDate=1",
  );
  assert.equal(parsed.selection_rules.new_materials.ratio, 0.4);
  assert.equal(parsed.selection_rules.new_materials.duration_days_lt, 7);
  assert.equal(parsed.selection_rules.new_materials.impression_gt, 1000);
  assert.equal(parsed.selection_rules.volume_materials.ratio, 0.6);
  assert.equal(parsed.selection_rules.volume_materials.duration_days_gt, 30);
  assert.equal(parsed.selection_rules.volume_materials.impression_gte, 10_000_000);
  assert.equal(parsed.limit, 25);
  assert.equal(parsed.date_range, "-29,0");
  assert.deepEqual(parsed.media, [1, 2, 10, 16, 4]);
  assert.deepEqual(parsed.media_names, ["Google Ads"]);
  assert.deepEqual(parsed.area, ["ID"]);
  assert.deepEqual(parsed.language, ["id"]);
  assert.deepEqual(parsed.platform, [1, 2]);
});

test("parses competitor URLs without treating query commas as competitors", () => {
  const parsed = parseAppGrowingMaterialSearchIntent(`
竞品：Adapundi
Adapundi 搜索页：https://appgrowing-global.youcloud.com/leaflet?purpose=2&keyword=adapundi&daterange=-29,0&order=cnt_dt_desc&page=1
新素材：占比40%，投放天数<7天，曝光估算>1K
跑量素材：占比60%，投放天数>30天，曝光估算>=10M
`);

  assert.deepEqual(parsed.competitors, ["Adapundi"]);
  assert.equal(
    parsed.competitor_urls.Adapundi,
    "https://appgrowing-global.youcloud.com/leaflet?purpose=2&keyword=adapundi&daterange=-29,0&order=cnt_dt_desc&page=1",
  );
});

test("parses explicit English and Chinese market and language filters", () => {
  const parsed = parseAppGrowingMaterialSearchIntent(`
每周抓取 AppGrowing 现金贷/金融竞品素材。
竞品：Easycash
投放地区：Malaysia、印尼
语言：English、马来语、印尼语
新素材：占比40%，投放天数<7天，曝光估算>1K
跑量素材：占比60%，投放天数>30天，曝光估算>=10M
`);

  assert.deepEqual(parsed.area, ["MY", "ID"]);
  assert.deepEqual(parsed.language, ["en", "ms", "id"]);
});

test("explicit params override parsed intent params", () => {
  const normalized = normalizeAppGrowingMaterialSearchParams({
    intent: "竞品：Easycash\n新素材：占比40%，投放天数<7天，曝光估算>1K\n跑量素材：占比60%，投放天数>30天，曝光估算>=10M\n输出最多25条",
    limit: 10,
    selection_rules: {
      volume_materials: {
        impression_gt: 20_000_000,
      },
    },
  });

  assert.equal(normalized.limit, 10);
  assert.equal(normalized.selection_rules.new_materials.impression_gt, 1000);
  assert.equal(normalized.selection_rules.volume_materials.impression_gt, 20_000_000);
  assert.equal(normalized.selection_rules.volume_materials.impression_gte, 10_000_000);
});

test("normalizes inclusive threshold rules", () => {
  const rules = normalizeMaterialRules({
    new_materials: {
      ratio: 0.4,
      duration_days_lte: 7,
      impression_gte: 1000,
    },
    volume_materials: {
      ratio: 0.6,
      duration_days_gte: 30,
      impression_gte: 10_000_000,
    },
  });

  assert.equal(rules.new_materials.duration_days_lte, 7);
  assert.equal(rules.new_materials.impression_gte, 1000);
  assert.equal(rules.volume_materials.duration_days_gte, 30);
  assert.equal(rules.volume_materials.impression_gte, 10_000_000);
});
