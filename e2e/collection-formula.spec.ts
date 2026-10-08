import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Collection formulas", () => {
  test.describe.configure({ timeout: 120000 });
  let api: TestApiClient;
  test.beforeEach(async () => {
    api = await createTestApi();
  });
  test.afterEach(async () => {
    await api.cleanup();
  });

  test("creates a formula, recalculates edited rows and recovers from formula errors", async ({
    page,
  }, testInfo) => {
    const slug = await loginAsDefault(page);
    const c = await api.createCollection(`Formula acceptance ${Date.now()}`);
    const quantity = (
      await api.cortexRequest(`/api/collections/${c.id}/fields`, "POST", {
        name: "Quantity",
        type: "number",
      })
    ).body;
    const price = (
      await api.cortexRequest(`/api/collections/${c.id}/fields`, "POST", {
        name: "Price",
        type: "number",
      })
    ).body;
    await api.cortexRequest(`/api/collections/${c.id}/records`, "POST", {
      title: "Order A",
      fields: { [quantity.id]: 3, [price.id]: 12.5 },
    });
    await page.goto(`/${slug}/collections/${c.id}`);
    await page
      .locator("thead")
      .getByRole("button", { name: "New field", exact: true })
      .click();
    const panel = page.getByRole("dialog", { name: "New field", exact: true });
    await panel.getByLabel("Name", { exact: true }).fill("Total");
    await panel.getByRole("combobox", { name: "Field type" }).click();
    await page.getByRole("option", { name: "Formula", exact: true }).click();
    await expect(
      panel.getByRole("button", { name: "Create field" }),
    ).toBeDisabled();
    await panel
      .getByLabel("Expression", { exact: true })
      .fill("{Quantity} * {Price}");
    await page.screenshot({ path: testInfo.outputPath("formula-editor.png") });
    await panel.getByRole("button", { name: "Create field" }).click();
    await expect(panel).toBeHidden();
    const result = page.getByLabel("Total: Order A", { exact: true });
    await expect(result).toHaveText("37.5");
    await expect(
      page.getByRole("button", { name: "Total: Order A", exact: true }),
    ).toHaveCount(0);
    await page
      .getByRole("button", { name: "Quantity: Order A", exact: true })
      .click();
    const input = page.getByRole("spinbutton");
    await input.fill("4");
    await input.press("Enter");
    await expect(result).toHaveText("50");
    await page.reload();
    await expect(result).toHaveText("50");
    await page.getByRole("button", { name: "Total", exact: true }).click();
    await expect(
      page.getByRole("menuitem", { name: /Sort ascending/ }),
    ).toHaveCount(0);
    await page.getByRole("menuitem", { name: /Edit field/ }).click();
    const edit = page.getByRole("dialog", { name: "Edit field", exact: true });
    await edit.getByLabel("Expression", { exact: true }).fill("{Total} + 1");
    await edit.getByRole("button", { name: "Save", exact: true }).click();
    await expect(edit.getByRole("alert")).toContainText("circular reference");
    await edit.getByLabel("Expression", { exact: true }).fill("{Quantity} / 0");
    await edit.getByRole("button", { name: "Save", exact: true }).click();
    await expect(edit).toBeHidden();
    await expect(result).toHaveText("#DIV/0!");
    await page.getByRole("button", { name: "Total", exact: true }).click();
    await page.getByRole("menuitem", { name: /Edit field/ }).click();
    await edit
      .getByLabel("Expression", { exact: true })
      .fill("IFERROR({Quantity} / 0, 0)");
    await edit.getByRole("button", { name: "Save", exact: true }).click();
    await expect(result).toHaveText("0");
  });
});
