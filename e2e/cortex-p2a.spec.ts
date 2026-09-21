import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Cortex P2a", () => {
  test.describe.configure({ timeout: 180000 });
  let api: TestApiClient;
  let slug: string;
  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    slug = await loginAsDefault(page);
  });
  test.afterEach(async () => {
    await api.cleanup();
  });
  test("imports 10000 rows, edits 500 atomically and restores deleted rows", async ({
    page,
  }) => {
    const c = await api.createCollection(`CSV acceptance ${Date.now()}`);
    const field = (
      await api.cortexRequest(`/api/collections/${c.id}/fields`, "POST", {
        name: "Score",
        type: "number",
      })
    ).body;
    await page.goto(`/${slug}/collections/${c.id}`);
    await page.getByRole("button", { name: "Append CSV rows", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog
      .locator('input[type="file"]')
      .setInputFiles({
        name: "rows.csv",
        mimeType: "text/csv",
        buffer: Buffer.from(
          "title,Score\n" +
            Array.from(
              { length: 10000 },
              (_, i) => `Imported ${String(i).padStart(5, "0")},${i}\n`,
            ).join(""),
        ),
      });
    await dialog.getByRole("button", { name: "Validate", exact: true }).click();
    await expect(dialog.getByRole("status")).toContainText("10000", {
      timeout: 30000,
    });
    await dialog
      .getByRole("button", { name: "Append CSV rows", exact: true })
      .click();
    await expect(dialog).not.toBeVisible({ timeout: 30000 });
    await expect(
      page.getByRole("button", { name: "Select up to 500 matching rows" }),
    ).toBeEnabled();
    await page
      .getByRole("button", { name: "Select up to 500 matching rows" })
      .click();
    await expect(page.getByText("500 selected", { exact: true })).toBeVisible({
      timeout: 30000,
    });
    await page
      .getByRole("combobox", { name: "Field to change" })
      .selectOption(field.id);
    // Clear the numeric value for the selected rows, then verify all 500 on the API.
    const change = page.waitForResponse(
      (r) =>
        r.url().endsWith("/records/batch") && r.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Apply to selected", exact: true })
      .click();
    expect((await change).status()).toBe(200);
    const cleared = await api.cortexRequest(
      `/api/collections/${c.id}/records?properties=${encodeURIComponent(JSON.stringify({ [field.id]: ["__none__"] }))}`,
    );
    expect(cleared.body.total).toBe(500);
    await page
      .getByRole("button", { name: "Select up to 500 matching rows" })
      .click();
    await expect(page.getByText("500 selected", { exact: true })).toBeVisible();
    await page
      .getByRole("button", { name: "Move to trash", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Move to trash", exact: true })
      .click();
    await expect(page.getByRole("dialog")).not.toBeVisible();
    expect(
      (await api.cortexRequest(`/api/collections/${c.id}/trash`)).body.total,
    ).toBe(500);
    await page.getByRole("button", { name: /Trash 500/ }).click();
    await page
      .getByRole("button", { name: "Restore 200 displayed records" })
      .click();
    await expect
      .poll(
        async () =>
          (await api.cortexRequest(`/api/collections/${c.id}/trash`)).body
            .total,
      )
      .toBe(300);
    await page
      .getByRole("button", { name: "Restore 200 displayed records" })
      .click();
    await expect
      .poll(
        async () =>
          (await api.cortexRequest(`/api/collections/${c.id}/trash`)).body
            .total,
      )
      .toBe(100);
    await page
      .getByRole("button", { name: "Restore 100 displayed records" })
      .click();
    await expect
      .poll(
        async () =>
          (await api.cortexRequest(`/api/collections/${c.id}/trash`)).body
            .total,
      )
      .toBe(0);
    await page.reload();
    expect(
      (await api.cortexRequest(`/api/collections/${c.id}/records`)).body.total,
    ).toBe(10000);
  });
  test("creates a project table with metadata and exposes it in the navigator", async ({
    page,
  }) => {
    const project = await api.createProject(`P2a project ${Date.now()}`);
    await page.goto(`/${slug}/projects/${project.id}`);
    await page.getByRole("button", { name: "New table", exact: true }).click();
    const title = `Project table ${Date.now()}`;
    await page
      .getByRole("textbox", { name: "Table name", exact: true })
      .fill(title);
    await expect(
      page.getByRole("combobox", { name: "Project", exact: true }),
    ).toHaveValue(project.id);
    await page
      .getByRole("textbox", { name: "Icon (emoji)", exact: true })
      .fill("📋");
    await page
      .getByRole("textbox", { name: "Description", exact: true })
      .fill("Project materials");
    const created = page.waitForResponse(
      (r) =>
        r.url().endsWith("/api/collections") && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Create", exact: true }).click();
    const body = await (await created).json();
    api.trackCollection(body.id);
    expect(body.project_id).toBe(project.id);
    expect(body.icon).toBe("📋");
    await expect(
      page.getByRole("link", { name: new RegExp(title) }),
    ).toBeVisible();
    await expect(
      page.getByText("Project materials", { exact: true }),
    ).toBeVisible();
  });
  test("edits metadata and restores archived fields and tables", async ({
    page,
  }) => {
    const c = await api.createCollection(`Restore acceptance ${Date.now()}`);
    const f = (
      await api.cortexRequest(`/api/collections/${c.id}/fields`, "POST", {
        name: "Retained",
        type: "text",
      })
    ).body;
    await api.cortexRequest(
      `/api/collections/${c.id}/fields/${f.id}`,
      "PATCH",
      { archived: true },
    );
    await page.goto(`/${slug}/collections/${c.id}`);
    await page
      .getByRole("button", { name: "Table settings", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Icon (emoji)").fill("📋");
    await dialog
      .getByLabel("Description", { exact: true })
      .fill("P2a browser acceptance");
    await dialog.getByRole("button", { name: "Restore", exact: true }).click();
    await expect(
      dialog.getByText("Retained", { exact: true }),
    ).not.toBeVisible();
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await expect(
      page.getByText("P2a browser acceptance", { exact: true }),
    ).toBeVisible();
    await api.cortexRequest(`/api/collections/${c.id}`, "PATCH", {
      archived: true,
    });
    await page.goto(`/${slug}/collections`);
    await page.getByRole("checkbox", { name: "Archived tables" }).check();
    const row = page.getByRole("listitem").filter({ hasText: c.name });
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "Restore", exact: true }).click();
    await expect(row).not.toBeVisible();
    await page.getByRole("checkbox", { name: "Archived tables" }).uncheck();
    await page.getByRole("link", { name: new RegExp(c.name) }).click();
    await expect(
      page.getByText("P2a browser acceptance", { exact: true }),
    ).toBeVisible();
  });
});
