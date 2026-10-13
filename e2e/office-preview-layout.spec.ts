import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.use({ channel: "chromium" });

test.describe("Office previews and compact navigation", () => {
  test.setTimeout(180000);
  let api: TestApiClient;
  test.beforeEach(async () => { api = await createTestApi(); });
  test.afterEach(async () => { await api.cleanup(); });

  test("deep document paths stay clear of actions and both navigation columns collapse", async ({ page }, info) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    let parent: string | undefined;
    for (let i = 0; i < 9; i++) {
      const doc = await api.createIssue(`Level ${i} — a very long document title to exercise breadcrumb overflow`, { kind: "doc", parent_issue_id: parent });
      expect(doc.id).toBeTruthy();
      parent = doc.id;
    }
    const collection = await api.createCollection("Compact table navigation");
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/documents/${parent}`);
    const trail = page.getByRole("navigation", { name: "Document path" });
    await expect(trail.getByRole("link").last()).toContainText("Level 8");
    await expect(trail.getByRole("link")).toHaveCount(2);
    const navBounds = await trail.boundingBox();
    const parentBounds = await trail.locator("..").boundingBox();
    const actions = await trail.locator("xpath=following-sibling::*[1]").boundingBox();
    expect(navBounds!.x + navBounds!.width).toBeLessThanOrEqual(actions!.x);
    expect(navBounds!.x + navBounds!.width).toBeLessThanOrEqual(parentBounds!.x + parentBounds!.width);
    await page.getByRole("button", { name: "Show parent documents" }).click();
    await expect(page.getByRole("menuitem")).toHaveCount(8);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("menu")).toBeHidden();
    await page.screenshot({ animations: "disabled", path: info.outputPath("navigation-expanded.png") });
    await page.getByRole("button", { name: "Collapse panel", exact: true }).click();
    await page.getByRole("button", { name: "Collapse navigation", exact: true }).click();
    const rail = page.locator('[data-collapsible="icon"]');
    await expect(rail).toBeVisible();
    const docs = rail.getByRole("link", { name: "Documents", exact: true });
    await expect(docs).toBeVisible();
    await expect.poll(async () => (await rail.boundingBox())!.width).toBeLessThan(90);
    await page.evaluate(() => Promise.all(document.getAnimations().filter((a) => a.effect?.getTiming().iterations !== Infinity).map((a) => a.finished.catch(() => undefined))));
    await page.screenshot({ animations: "disabled", path: info.outputPath("navigation-collapsed.png") });
    await page.reload();
    await expect(page.getByRole("button", { name: "Expand panel", exact: true })).toBeVisible();
    await page.goto(`/${slug}/collections/${collection.id}`);
    await page.getByRole("button", { name: "Collapse panel", exact: true }).click();
    await expect(page.getByRole("button", { name: "Expand panel", exact: true })).toBeVisible();
    await page.reload();
    await expect(page.getByRole("button", { name: "Expand panel", exact: true })).toBeVisible();
  });

  test("Word and PowerPoint attachments open as authenticated PDF previews", async ({ page }, info) => {
    test.skip(!process.env.OFFICE_PREVIEW_URL, "Office conversion requires the private Gotenberg service");
    const issue = await api.createIssue("Office attachment preview", { kind: "doc" });
    const slug = await loginAsDefault(page);
    for (const ext of ["doc", "docx", "ppt", "pptx"]) {
      const attachment = await api.uploadOfficeAttachment(issue.id, `sample.${ext}`, readFileSync(`e2e/fixtures/office/sample.${ext}`));
      const response = page.waitForResponse((r) => r.request().method() === "GET" && r.status() === 200 && r.url().includes(`/api/attachments/${attachment.id}/preview`));
      await page.goto(`/${slug}/attachments/${attachment.id}/preview`);
      const pdf = await response;
      expect(pdf.status()).toBe(200);
      const frame = page.locator(`iframe[title="sample.${ext}"]`);
      await expect(frame).toHaveAttribute("src", /^blob:/);
      const signature = await frame.evaluate(async (element) => {
        const response = await fetch((element as HTMLIFrameElement).src);
        return (await response.blob()).slice(0, 5).text();
      });
      expect(signature).toBe("%PDF-");
      await expect.poll(() => page.frames().some((f) => f.url().startsWith("chrome-extension://mhjfbmdgcfjbbpaeojofohoefgiehjai/"))).toBe(true);
      const viewer = page.frames().find((f) => f.url().startsWith("chrome-extension://mhjfbmdgcfjbbpaeojofohoefgiehjai/"))!;
      await expect(viewer.locator("viewer-thumbnail canvas").first()).toBeVisible();
      await page.screenshot({ animations: "disabled", path: info.outputPath(`preview-${ext}.png`) });
    }
  });
});
