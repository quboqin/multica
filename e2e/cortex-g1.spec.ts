import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Cortex G1 P0", () => {
  test.describe.configure({ timeout: 120000 });
  let api: TestApiClient;
  let slug: string;
  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    slug = await loginAsDefault(page);
  });
  test.afterEach(async () => {
    await api.cleanup();
  });

  test("document body saves, conflicts preserve a draft, review and publication persist", async ({
    page,
  }) => {
    const doc = await api.createIssue(`G1 document ${Date.now()}`, {
      kind: "doc",
      description: "Original paragraph",
    });
    await page.goto(`/${slug}/documents/${doc.id}`);
    await expect(
      page.getByRole("navigation", { name: "Document path" }),
    ).toContainText(doc.title);
    const editor = page
      .locator("[data-comment-content]")
      .first()
      .locator('[contenteditable="true"]');
    await expect(editor).toContainText("Original paragraph");
    const save = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(`/api/issues/${doc.id}`),
    );
    await editor.fill("Saved in the browser");
    await editor.press("Tab");
    expect((await save).status()).toBe(200);
    await expect(page.getByText("Saved", { exact: true })).toBeVisible();
    const current = (await api.cortexRequest(`/api/issues/${doc.id}`)).body;
    expect(current.document_revision).toBe(2);
    await page.reload();
    await expect(editor).toContainText("Saved in the browser");
    await page
      .getByRole("button", { name: "Submit for review", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Publish", exact: true }),
    ).toBeEnabled();
    await page.getByRole("button", { name: "Publish", exact: true }).click();
    await expect(
      page.getByText("published", { exact: true }).first(),
    ).toBeVisible();
    const legacy = await api.cortexRequest(`/api/issues/${doc.id}`, "PUT", {
      description: "unversioned overwrite",
    });
    expect(legacy.status).toBe(409);
    await page.route(`**/api/issues/${doc.id}`, async (route) => {
      if (route.request().method() === "PUT") {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({
            error: "Another editor saved a newer version",
            code: "document_conflict",
          }),
        });
      } else await route.continue();
    });
    await editor.fill("My draft must survive");
    await editor.press("Tab");
    await expect(
      page.getByRole("region", { name: "Resolve document conflict" }),
    ).toBeVisible();
    await expect(
      page.getByRole("textbox", { name: "Your draft — edit to merge" }),
    ).toHaveValue("My draft must survive");
    await page.reload();
    await expect(editor).toContainText("My draft must survive");
  });

  test("opening rich document content does not create an unsaved draft", async ({
    page,
  }) => {
    const collection = await api.createCollection(`Embedded ${Date.now()}`);
    const savedView = await api.cortexRequest("/api/issue-views", "POST", {
      name: "Embedded table",
      collection_id: collection.id,
      scope_type: "workspace",
      visibility: "private",
      query: {},
      display: { layout: "table" },
    });
    expect(savedView.status).toBe(201);
    const body = [
      "# Rich document",
      ":::multica-view " + savedView.body.id,
      "```mermaid\ngraph LR\n A --> B\n```",
      "$$\nx^2\n$$",
    ].join("\n\n");
    const doc = await api.createIssue(`Rich document ${Date.now()}`, {
      kind: "doc",
      description: body,
    });
    await page.goto(`/${slug}/documents/${doc.id}`);
    await expect(
      page.getByRole("heading", { name: collection.name, exact: true }),
    ).toBeVisible({ timeout: 30000 });
    await expect(page.getByText("Saved", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Submit for review", exact: true }),
    ).toBeEnabled();
    expect(
      (await api.cortexRequest(`/api/issues/${doc.id}`)).body.document_revision,
    ).toBe(1);
  });

  test("collection shared table, calendar date write, gallery and saved view", async ({
    page,
  }) => {
    const collection = await api.createCollection(
      `G1 collection ${Date.now()}`,
    );
    const field = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/fields`,
        "POST",
        { name: "Schedule", type: "date" },
      )
    ).body;
    const record = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/records`,
        "POST",
        { title: "Calendar record" },
      )
    ).body;
    const date = new Date();
    const day = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
    expect(
      (
        await api.cortexRequest(
          `/api/collections/${collection.id}/records/${record.id}/fields/${field.id}`,
          "PUT",
          { value: day, expected_value: null },
        )
      ).status,
    ).toBe(200);
    await page.goto(`/${slug}/collections/${collection.id}`);
    await expect(
      page.getByRole("textbox", {
        name: "Record title: Calendar record",
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.locator("[data-source-identity]")).toHaveAttribute(
      "data-source-identity",
      /collection/,
    );
    await page.getByRole("button", { name: "Calendar", exact: true }).click();
    const dateInput = page.getByLabel("Date field: Calendar record", {
      exact: true,
    });
    await expect(dateInput).toHaveValue(day);
    const next = new Date(date);
    next.setDate(date.getDate() + 1);
    const nextDay = `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, "0")}-${String(next.getDate()).padStart(2, "0")}`;
    const save = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(`/fields/${field.id}`),
    );
    await dateInput
      .locator("..")
      .dragTo(page.getByRole("region", { name: nextDay, exact: true }));
    expect((await save).status()).toBe(200);
    await page.reload();
    await expect(dateInput).toHaveValue(nextDay);
    await page.getByRole("button", { name: "Gallery", exact: true }).click();
    await expect(page.getByRole("article")).toContainText("Calendar record");
    await page
      .getByRole("textbox", { name: "View name", exact: true })
      .fill("My gallery");
    await page.getByRole("button", { name: "Save view", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "My gallery", exact: true }),
    ).toBeVisible();
  });

  test("a filtered-out edited record keeps its draft and rereads the current conflict value", async ({
    page,
  }) => {
    const collection = await api.createCollection(
      `Focused record ${Date.now()}`,
    );
    const field = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/fields`,
        "POST",
        { name: "Value", type: "text" },
      )
    ).body;
    const record = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/records`,
        "POST",
        { title: "Find this row" },
      )
    ).body;
    const fieldPath = `/api/collections/${collection.id}/records/${record.id}/fields/${field.id}`;
    expect(
      (await api.cortexRequest(fieldPath, "PUT", { value: "Before" })).status,
    ).toBe(200);
    await page.goto(`/${slug}/collections/${collection.id}`);
    await page
      .getByRole("textbox", { name: "Search records", exact: true })
      .fill("Find this row");
    const input = page.getByRole("textbox", { name: /Value:/ });
    await expect(input).toHaveValue("Before");
    await input.fill("Local draft");
    expect(
      (
        await api.cortexRequest(
          `/api/collections/${collection.id}/records/${record.id}`,
          "PUT",
          { title: "Moved outside filter", title_base: "Find this row" },
        )
      ).status,
    ).toBe(200);
    expect(
      (
        await api.cortexRequest(fieldPath, "PUT", {
          value: "Remote value",
          expected_value: "Before",
        })
      ).status,
    ).toBe(200);
    await expect(input).toHaveValue("Local draft");
    const conflict = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(fieldPath),
    );
    await page
      .getByRole("heading", { name: collection.name, exact: true })
      .click();
    expect((await conflict).status()).toBe(409);
    await expect(
      page.getByText("Current value: Remote value", { exact: true }),
    ).toBeVisible();
    const retry = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(fieldPath),
    );
    await page
      .getByRole("button", {
        name: "Save draft over current value",
        exact: true,
      })
      .click();
    expect((await retry).status()).toBe(200);
    const current = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/records?record_id=${record.id}`,
      )
    ).body.records[0];
    expect(current.fields[field.id]).toBe("Local draft");
  });

  test("task calendar persists a changed date and gallery opens the same task", async ({
    page,
  }) => {
    const now = new Date();
    const day = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
    const issue = await api.createIssue(`G1 scheduled ${Date.now()}`, {
      due_date: day,
    });
    await page.reload();
    await page.getByRole("button", { name: "Board", exact: true }).click();
    await page
      .getByRole("menuitemradio", { name: "Calendar", exact: true })
      .click();
    const input = page.getByLabel(`Date field: ${issue.title}`, {
      exact: true,
    });
    await expect(input).toHaveValue(day);
    const destination = new Date(now);
    destination.setDate(now.getDate() + 1);
    const movedDay = `${destination.getFullYear()}-${String(destination.getMonth() + 1).padStart(2, "0")}-${String(destination.getDate()).padStart(2, "0")}`;
    const saved = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(`/api/issues/${issue.id}`),
    );
    await input
      .locator("..")
      .dragTo(page.getByRole("region", { name: movedDay, exact: true }));
    expect((await saved).status()).toBe(200);
    await page.reload();
    await expect(input).toHaveValue(movedDay);
    await page.locator('input[aria-label="Date field"]').fill(movedDay);
    await page.getByRole("button", { name: "Week", exact: true }).click();
    await expect(input).toBeVisible();
    await page.getByRole("button", { name: "Calendar", exact: true }).click();
    await page
      .getByRole("menuitemradio", { name: "Gallery", exact: true })
      .click();
    await page
      .getByRole("article")
      .getByRole("button", { name: issue.title, exact: true })
      .click();
    await expect(page).toHaveURL(new RegExp(`/issues/${issue.identifier}$`), {
      timeout: 30000,
    });
  });
  test("document tree reparenting preserves breadcrumbs, favorites and recent access", async ({
    page,
  }) => {
    const suffix = Date.now();
    const a = await api.createIssue(`Tree A ${suffix}`, { kind: "doc" });
    const b = await api.createIssue(`Tree B ${suffix}`, { kind: "doc" });
    const child = await api.createIssue(`Child ${suffix}`, {
      kind: "doc",
      parent_issue_id: a.id,
    });
    const leaf = await api.createIssue(`Leaf ${suffix}`, {
      kind: "doc",
      parent_issue_id: child.id,
    });
    await page.goto(`/${slug}/documents/${leaf.id}`);
    const path = page.getByRole("navigation", { name: "Document path" });
    await expect(path).toContainText(a.title);
    await expect(path).toContainText(child.title);
    const library = page.getByRole("complementary", { name: "Documents" });
    const from = library.locator('[draggable="true"]').filter({
      has: page.getByRole("link", { name: child.title, exact: true }),
    });
    const to = library
      .locator('[draggable="true"]')
      .filter({ has: page.getByRole("link", { name: b.title, exact: true }) });
    const moved = page.waitForResponse((response) =>
      response.url().endsWith(`/documents/${child.id}/move`),
    );
    await from.dragTo(to);
    expect((await moved).status()).toBe(200);
    await expect(path).toContainText(b.title);
    await expect(path).not.toContainText(a.title);
    await page.reload();
    await expect(path).toContainText(b.title);
    await library
      .locator('[draggable="true"]')
      .filter({
        has: page.getByRole("link", { name: leaf.title, exact: true }),
      })
      .getByRole("button", { name: "Favorite", exact: true })
      .click();
    await library
      .getByRole("button", { name: "Favorites", exact: true })
      .click();
    await expect(
      library.getByRole("link", { name: leaf.title, exact: true }),
    ).toBeVisible();
    await library.getByRole("button", { name: "Recent", exact: true }).click();
    await expect(
      library.getByRole("link", { name: leaf.title, exact: true }),
    ).toBeVisible();
  });
});
