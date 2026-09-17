import { test, expect } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";

test("document creation, title/body save and refresh use the shared table", async ({
  page,
}) => {
  test.setTimeout(120000);
  const api = await createTestApi();
  let documentId: string | undefined;
  try {
    const title = `Document ${Date.now()}`;
    const task = await api.createIssue(`Task ${Date.now()}`);
    const configResponse = page.waitForResponse(
      (response) => new URL(response.url()).pathname === "/api/config",
    );
    const slug = await loginAsDefault(page);
    const config = await configResponse;
    expect(config.status()).toBe(200);
    expect(
      (await config.json()).feature_flags?.cortex_docs,
      "The browser's backend must enable FF_CORTEX_DOCS=true. Run pnpm test:e2e:documents; check local env overrides if this fails.",
    ).toBe(true);
    await page.goto(`/${slug}/docs`);
    await page
      .getByRole("button", { name: "New document", exact: true })
      .click();
    await page.getByRole("textbox", { name: "Title", exact: true }).fill(title);
    await page
      .locator('form [contenteditable="true"]')
      .fill("First document body");
    const created = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/issues") &&
        response.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Save document", exact: true })
      .click();
    const response = await created;
    expect(response.status()).toBe(201);
    const document = await response.json();
    documentId = document.id;
    expect(document.kind).toBe("doc");
    await expect(
      page.getByRole("status").filter({ hasText: "Saved" }),
    ).toBeVisible();
    await page
      .getByRole("textbox", { name: "Title", exact: true })
      .fill(`${title} edited`);
    await page
      .locator('form [contenteditable="true"]')
      .fill("Persisted document body");
    const updated = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/issues/${documentId}`) &&
        response.request().method() === "PUT",
    );
    await page
      .getByRole("button", { name: "Save document", exact: true })
      .click();
    expect((await updated).status()).toBe(200);
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await page.reload();
    await expect(
      page.getByRole("button", { name: `${title} edited`, exact: true }),
    ).toBeVisible();
    await expect(page.getByText(task.title, { exact: true })).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Load more", exact: true }),
    ).toHaveCount(0);
    await page.screenshot({
      path: "test-results/document-table.png",
      fullPage: true,
    });
    await page
      .getByRole("button", { name: `${title} edited`, exact: true })
      .click();
    await expect(page.locator('form [contenteditable="true"]')).toHaveText(
      "Persisted document body",
    );
    await page.screenshot({
      path: "test-results/document-editor.png",
      fullPage: true,
    });
    await page.goto(`/${slug}/issues`);
    const layout = page
      .getByRole("button", { name: /^(Board|List|Table)$/ })
      .first();
    if ((await layout.textContent())?.trim() !== "Table") {
      await layout.click();
      await page
        .getByRole("menuitemradio", { name: "Table", exact: true })
        .click();
    }
    await expect(
      page.getByText(task.title, { exact: true }).first(),
    ).toBeVisible();
    await expect(
      page.getByText(`${title} edited`, { exact: true }),
    ).toHaveCount(0);
  } finally {
    if (documentId) await api.deleteIssue(documentId);
    await api.cleanup();
  }
});
