import { expect, test } from "@playwright/test";
import { loginAsDefault } from "./helpers";

test("creative factory presents a business-first workspace", async ({ page }, testInfo) => {
  test.setTimeout(120_000);
  const workspaceSlug = await loginAsDefault(page);

  await page.goto(`/${workspaceSlug}/creative`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "创意工作台" })).toBeVisible({ timeout: 60_000 });
  await expect(page.getByRole("tab", { name: "工作台" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("region", { name: "现在需要处理" })).toBeVisible();
  await expect(page.getByText("Issue 用于")).toHaveCount(0);
  await expect(page.getByText("WorkUnit")).toHaveCount(0);
  await expect(page.getByText("Task Batch")).toHaveCount(0);

  await page.getByRole("tab", { name: "素材库" }).click();
  await expect(page.getByRole("heading", { name: "素材库", exact: true })).toBeVisible({ timeout: 30_000 });
  await expect(page).toHaveURL(/tab=materials/);

  await page.getByRole("tab", { name: "创意订单" }).click();
  await expect(page.getByRole("heading", { name: "创意订单", exact: true })).toBeVisible({ timeout: 30_000 });
  await expect(page).toHaveURL(/tab=orders/);

  await page.getByRole("tab", { name: "品牌与市场规则" }).click();
  await expect(page.getByRole("heading", { name: "品牌与市场规则" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("button", { name: "市场规则" })).toBeVisible();
  await expect(page.getByRole("button", { name: "文案库" })).toBeVisible();
  await expect(page).toHaveURL(/tab=resources/);

  const metricsResponse = page.waitForResponse((response) =>
    new URL(response.url()).pathname === "/api/creative-feedback-events/metrics",
  );
  await page.getByRole("tab", { name: "数据反馈" }).click();
  expect((await metricsResponse).status()).toBe(200);
  await expect(page.getByRole("heading", { name: "数据反馈" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("region", { name: "反馈指标" })).toBeVisible();
  await expect(page).toHaveURL(/tab=feedback/);

  await testInfo.attach("creative-business-workspace.png", {
    body: await page.screenshot({ fullPage: true }),
    contentType: "image/png",
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/${workspaceSlug}/creative`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "创意工作台" })).toBeVisible({ timeout: 30_000 });
  const hasPageOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(hasPageOverflow).toBe(false);
  await testInfo.attach("creative-business-workspace-mobile.png", {
    body: await page.screenshot({ fullPage: true }),
    contentType: "image/png",
  });
});
