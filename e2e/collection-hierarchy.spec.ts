import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Collection hierarchy and contact fields", () => {
  test.describe.configure({ timeout: 120000 });
  let api: TestApiClient;
  test.beforeEach(async () => { api = await createTestApi(); });
  test.afterEach(async () => { await api.cleanup(); });

  test("pins selection first, creates nested rows and persists Email and URL values", async ({ page }, info) => {
    const table = await api.createCollection("Hierarchy acceptance");
    const create = async (path: string, body: unknown) => {
      const result = await api.cortexRequest(`/api/collections/${table.id}/${path}`, "POST", body);
      expect(result.status).toBe(201);
      return result.body;
    };
    const email = await create("fields", { name: "Email", type: "email" });
    const website = await create("fields", { name: "Website", type: "url" });
    const parent = await create("records", { title: "Oracle" });
    await create("records", { title: "Cloud", parent_id: parent.id });
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/collections/${table.id}`, { waitUntil: "domcontentloaded" });
    const headers = page.locator("thead th");
    await expect(headers.first()).toHaveAttribute("data-column-id", "__select");
    await expect(headers.nth(1)).toHaveAttribute("data-column-id", "title");
    await expect(page.getByRole("button", { name: "Rename Cloud", exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "Expand children of Oracle" }).click();
    await expect(page.getByRole("button", { name: "Rename Cloud", exact: true })).toBeVisible();
    let rejected = false;
    await page.route(/\/api\/collections\/[^/]+\/records$/, async (route) => {
      if (!rejected && route.request().method() === "POST") {
        rejected = true;
        await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Could not save child" }) });
      } else await route.continue();
    });
    await page.getByRole("button", { name: "Add child row to Cloud" }).click();
    await page.getByLabel("Record title", { exact: true }).fill("Database");
    await page.getByRole("button", { name: "Add child row", exact: true }).click();
    await expect(page.getByRole("dialog").getByRole("alert")).toContainText("Could not save child");
    await expect(page.getByLabel("Record title", { exact: true })).toHaveValue("Database");
    await page.getByRole("button", { name: "Add child row", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Rename Database", exact: true })).toBeVisible();
    for (const [name, value] of [["Email", "cloud@example.com"], ["Website", "https://example.com/cloud"]]) {
      await page.getByRole("button", { name: `${name}: Cloud`, exact: true }).click();
      await page.getByLabel(name!, { exact: true }).fill(value!);
      await page.getByLabel(name!, { exact: true }).press("Enter");
      await expect(page.getByRole("button", { name: `${name}: Cloud`, exact: true })).toContainText(value!);
    }
    await expect.poll(async () => {
      const result = await api.cortexRequest(`/api/collections/${table.id}/records?search=Cloud`);
      return result.body.records[0]?.fields;
    }).toMatchObject({ [email.id]: "cloud@example.com", [website.id]: "https://example.com/cloud" });
    await page.screenshot({ path: info.outputPath("collection-hierarchy.png"), fullPage: true });
    await page.getByRole("button", { name: "Collapse children of Oracle" }).click();
    await expect(page.getByRole("button", { name: "Rename Database", exact: true })).toHaveCount(0);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "Expand children of Oracle" }).click();
    await page.getByRole("button", { name: "Expand children of Cloud" }).click();
    await expect(page.getByRole("button", { name: "Rename Database", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Email: Cloud", exact: true })).toContainText("cloud@example.com");
  });
});
