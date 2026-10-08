import { expect, test } from "@playwright/test";
import path from "node:path";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Create a Collection from Excel", () => {
  test.describe.configure({ timeout: 120000 });
  let api: TestApiClient;
  test.beforeEach(async () => {
    api = await createTestApi();
  });
  test.afterEach(async () => {
    await api.cleanup();
  });

  test("selects a worksheet, infers fields and creates a persisted table", async ({
    page,
  }) => {
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/collections`);
    await page
      .getByRole("button", { name: "Import Excel / CSV", exact: true })
      .first()
      .click();
    const dialog = page.getByRole("dialog");
    await dialog
      .locator('input[type="file"]')
      .setInputFiles(
        path.join(process.cwd(), "e2e/fixtures/cortex-customers.xlsx"),
      );
    // The first worksheet is empty; users must still be able to pick another.
    await expect(
      dialog.getByRole("button", { name: "Create table", exact: true }),
    ).toBeDisabled();
    await dialog
      .getByRole("combobox", { name: "Worksheet", exact: true })
      .selectOption("Customers");
    await expect(dialog.getByRole("status")).toHaveText(
      "Rows: 3 · Fields: 7 + title",
    );
    await expect(
      dialog.getByRole("combobox", { name: "Type for Seats" }),
    ).toHaveValue("number");
    await expect(
      dialog.getByRole("combobox", { name: "Type for Renewal" }),
    ).toHaveValue("date");
    await expect(
      dialog.getByRole("combobox", { name: "Type for Signed" }),
    ).toHaveValue("checkbox");
    await expect(
      dialog.getByRole("combobox", { name: "Type for Code" }),
    ).toHaveValue("text");
    await dialog
      .getByRole("combobox", { name: "Type for Stage" })
      .selectOption("select");
    await dialog
      .getByRole("checkbox", { name: "Import Ignore", exact: true })
      .uncheck();
    const tableName = `Excel customers ${Date.now()}`;
    await dialog
      .getByRole("textbox", { name: "Table name", exact: true })
      .fill(tableName);
    const submitted = page.waitForResponse(
      (r) => r.url().endsWith("/api/collections/import") && r.status() === 201,
    );
    await dialog
      .getByRole("button", { name: "Create table", exact: true })
      .click();
    const result = await (await submitted).json();
    api.trackCollection(result.collection.id);
    await expect(page).toHaveURL(
      new RegExp(`/collections/${result.collection.id}$`),
    );
    await expect(
      page.getByRole("link", { name: new RegExp(tableName) }),
    ).toBeVisible();
    await expect(page.getByText("Acme", { exact: true }).first()).toBeVisible();
    await page.reload();
    await expect(page.getByText("Acme", { exact: true }).first()).toBeVisible();
    const detail = (
      await api.cortexRequest(`/api/collections/${result.collection.id}`)
    ).body;
    expect(detail.fields).toHaveLength(6);
    const code = detail.fields.find((f: { name: string }) => f.name === "Code");
    const rows = (
      await api.cortexRequest(
        `/api/collections/${result.collection.id}/records`,
      )
    ).body;
    expect(rows.total).toBe(3);
    expect(
      rows.records.every(
        (r: { fields: Record<string, unknown> }) =>
          r.fields[code.id] === "000123",
      ),
    ).toBe(true);
  });

  test("keeps the preview after invalid conversion and imports CSV without a template", async ({
    page,
  }) => {
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/collections`);
    await page
      .getByRole("button", { name: "Import Excel / CSV", exact: true })
      .first()
      .click();
    const dialog = page.getByRole("dialog");
    await dialog
      .locator('input[type="file"]')
      .setInputFiles({
        name: "customers.csv",
        mimeType: "text/csv",
        buffer: Buffer.from("Customer,Code\nAcme,000123\n"),
      });
    await expect(dialog.getByRole("status")).toHaveText(
      "Rows: 1 · Fields: 1 + title",
    );
    await dialog
      .getByRole("combobox", { name: "Type for Code" })
      .selectOption("number");
    await dialog
      .getByRole("button", { name: "Create table", exact: true })
      .click();
    await expect(dialog.getByRole("alert")).toContainText("use text");
    await expect(
      dialog.getByRole("textbox", { name: "Table name", exact: true }),
    ).toHaveValue("customers");
    await dialog
      .getByRole("combobox", { name: "Type for Code" })
      .selectOption("text");
    const submitted = page.waitForResponse(
      (r) => r.url().endsWith("/api/collections/import") && r.status() === 201,
    );
    await dialog
      .getByRole("button", { name: "Create table", exact: true })
      .click();
    const result = await (await submitted).json();
    api.trackCollection(result.collection.id);
    await expect(page).toHaveURL(
      new RegExp(`/collections/${result.collection.id}$`),
    );
  });
});
