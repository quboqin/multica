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
    // The top bar's contextual action disappears once the doc is published,
    // and the status chip / meta line switch to the published label.
    await expect(
      page.getByRole("button", { name: "Publish", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByText("Published", { exact: true }).first(),
    ).toBeVisible();
    expect((await api.cortexRequest(`/api/issues/${doc.id}`)).body.status).toBe(
      "published",
    );
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
    // The embed header names the saved view; the live table underneath
    // renders the collection through the shared data view.
    const embed = page.locator('[data-type="saved-view"]');
    await expect(embed).toContainText("Embedded table", { timeout: 30000 });
    await expect(embed.locator("[data-source-identity]").first()).toBeVisible({
      timeout: 30000,
    });
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
      page.getByRole("button", { name: "Rename Calendar record", exact: true }),
    ).toBeVisible();
    // Tables open a column of their own, without the document tree.
    await expect(
      page
        .getByRole("complementary", { name: "Tables", exact: true })
        .getByRole("link", { name: collection.name }),
    ).toHaveAttribute("aria-current", "page");
    await expect(
      page.getByRole("complementary", { name: "Documents", exact: true }),
    ).toHaveCount(0);
    await expect(page.locator("[data-source-identity]")).toHaveAttribute(
      "data-source-identity",
      /collection/,
    );
    await page.getByRole("radio", { name: "Calendar", exact: true }).click();
    const pill = page.locator(`[data-row-id="${record.id}"]`);
    await expect(
      page.getByRole("region", { name: day, exact: true }).locator(pill),
    ).toBeVisible();
    const next = new Date(date);
    next.setDate(date.getDate() + 1);
    const nextDay = `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, "0")}-${String(next.getDate()).padStart(2, "0")}`;
    const save = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(`/fields/${field.id}`),
    );
    await pill.dragTo(page.getByRole("region", { name: nextDay, exact: true }));
    expect((await save).status()).toBe(200);
    await page.reload();
    await expect(
      page.getByRole("region", { name: nextDay, exact: true }).locator(pill),
    ).toBeVisible();
    await page.getByRole("radio", { name: "Gallery", exact: true }).click();
    await expect(page.getByRole("article")).toContainText("Calendar record");
    await page.getByRole("button", { name: "Save view", exact: true }).click();
    await page.getByPlaceholder("View name").fill("My gallery");
    await page.getByPlaceholder("View name").press("Enter");
    await expect(
      page.getByRole("tab", { name: "My gallery", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
  });

  test("a new table gets its fields from the header plus and edits them from the column menu", async ({
    page,
  }) => {
    const collection = await api.createCollection(
      `G1 fields ${Date.now()}`,
    );
    await page.goto(`/${slug}/collections/${collection.id}`);
    // With no fields yet the "+" cell spells the action out, right after Name.
    const add = page
      .locator("thead")
      .getByRole("button", { name: "New field", exact: true });
    await expect(add).toHaveText("New field");
    await add.click();
    const panel = page.getByRole("dialog", { name: "New field", exact: true });
    await panel.getByLabel("Name", { exact: true }).fill("Stage");
    await panel.getByRole("combobox", { name: "Field type" }).click();
    await page.getByRole("option", { name: "Select", exact: true }).click();
    await panel.getByLabel("Option 1", { exact: true }).fill("Backlog");
    await panel.getByLabel("Option 1", { exact: true }).press("Enter");
    await panel.getByLabel("Option 2", { exact: true }).fill("Doing");
    const created = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/api/collections/${collection.id}/fields`),
    );
    await panel.getByRole("button", { name: "Create field", exact: true }).click();
    expect((await created).status()).toBe(201);
    await expect(panel).toBeHidden();
    await expect(
      page.getByRole("button", { name: "Stage", exact: true }),
    ).toBeVisible();
    // Once the table has a field the cell shrinks to a plus.
    await expect(add).toHaveText("");

    // A duplicate name keeps the panel open with the server's answer.
    await add.click();
    await panel.getByLabel("Name", { exact: true }).fill("Stage");
    await panel.getByLabel("Name", { exact: true }).press("Enter");
    await expect(panel.getByRole("alert")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(panel).toBeHidden();

    await page.getByRole("button", { name: "Stage", exact: true }).click();
    await page.getByRole("menuitem", { name: /Edit field/ }).click();
    const edit = page.getByRole("dialog", { name: "Edit field", exact: true });
    await expect(edit.getByLabel("Name", { exact: true })).toHaveValue("Stage");
    // An existing field only converts where its values survive.
    await edit.getByRole("combobox", { name: "Field type" }).click();
    await expect(page.getByRole("option")).toHaveText([
      "Select",
      "Multi-select",
    ]);
    await page.keyboard.press("Escape");
    await edit.getByLabel("Name", { exact: true }).fill("Phase");
    await edit
      .getByRole("button", { name: "Remove Backlog", exact: true })
      .click();
    const updated = page.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        response.url().includes(`/api/collections/${collection.id}/fields/`),
    );
    await edit.getByRole("button", { name: "Save", exact: true }).click();
    expect((await updated).status()).toBe(200);
    await expect(edit).toBeHidden();
    await expect(
      page.getByRole("button", { name: "Phase", exact: true }),
    ).toBeVisible();

    const detail = (
      await api.cortexRequest(`/api/collections/${collection.id}`)
    ).body;
    expect(
      detail.fields.map(
        (field: { name: string; type: string; config: { options: { name: string }[] } }) => ({
          name: field.name,
          type: field.type,
          options: field.config.options.map((option) => option.name),
        }),
      ),
    ).toEqual([{ name: "Phase", type: "select", options: ["Doing"] }]);

    // The first column is the record title: it can be renamed, nothing else.
    // Its menu opens from anywhere in the header cell, not only from the
    // short label at the left of a wide column.
    const titleHeader = page.locator("thead th").first();
    const titleBox = (await titleHeader.boundingBox())!;
    await titleHeader.click({
      position: { x: titleBox.width - 40, y: titleBox.height / 2 },
    });
    await page.getByRole("menuitem", { name: /Edit field/ }).click();
    const title = page.getByRole("dialog", { name: "Edit field", exact: true });
    await expect(title.getByLabel("Name", { exact: true })).toHaveValue("Name");
    await expect(
      title.getByRole("combobox", { name: "Field type" }),
    ).toBeDisabled();
    await title.getByLabel("Name", { exact: true }).fill("Customer");
    const renamed = page.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        response.url().endsWith(`/api/collections/${collection.id}`),
    );
    await title.getByRole("button", { name: "Save", exact: true }).click();
    expect((await renamed).status()).toBe(200);
    await expect(
      page.getByRole("button", { name: "Customer", exact: true }),
    ).toBeVisible();
    expect(
      (await api.cortexRequest(`/api/collections/${collection.id}`)).body
        .collection.title_name,
    ).toBe("Customer");
  });

  test("a table left on the board layout still gets and edits its fields", async ({
    page,
  }) => {
    const collection = await api.createCollection(
      `G1 board fields ${Date.now()}`,
    );
    await page.goto(`/${slug}/collections/${collection.id}`);
    await page.getByRole("radio", { name: "Board", exact: true }).click();
    // A board has no column headers, so its empty state offers the field it
    // is missing.
    const main = page.locator("main main");
    await main.getByRole("button", { name: "New field", exact: true }).click();
    const panel = page.getByRole("dialog", { name: "New field", exact: true });
    await expect(
      panel.getByRole("combobox", { name: "Field type" }),
    ).toContainText("Select");
    await panel.getByLabel("Name", { exact: true }).fill("Stage");
    await panel.getByLabel("Option 1", { exact: true }).fill("Todo");
    await panel.getByRole("button", { name: "Create field", exact: true }).click();
    await expect(panel).toBeHidden();
    await expect(page.getByRole("heading", { name: "Todo", exact: true })).toBeVisible();

    // From here on fields are managed from Display.
    await page.getByRole("button", { name: /^Display/ }).click();
    await page
      .getByRole("button", { name: "Edit field Stage", exact: true })
      .click();
    const edit = page.getByRole("dialog", { name: "Edit field", exact: true });
    await edit.getByLabel("Name", { exact: true }).fill("Phase");
    await edit.getByRole("button", { name: "Save", exact: true }).click();
    await expect(edit).toBeHidden();
    await expect(
      page.getByRole("button", { name: /^Group\s*Phase/ }),
    ).toBeVisible();
  });

  test("documents and tables are deleted from their list rows", async ({
    page,
  }) => {
    const suffix = Date.now();
    const parent = await api.createIssue(`Delete parent ${suffix}`, {
      kind: "doc",
    });
    const child = await api.createIssue(`Delete child ${suffix}`, {
      kind: "doc",
      parent_issue_id: parent.id,
    });
    const collection = await api.createCollection(`G1 delete ${suffix}`);

    await page.goto(`/${slug}/documents/${parent.id}`);
    const library = page.getByRole("complementary", {
      name: "Documents",
      exact: true,
    });
    const parentRow = library.locator('[draggable="true"]').filter({
      has: page.getByRole("link", { name: parent.title, exact: true }),
    });
    await parentRow.hover();
    await parentRow
      .getByRole("button", { name: `Actions for ${parent.title}`, exact: true })
      .click();
    await page
      .getByRole("menuitem", { name: "Delete document", exact: true })
      .click();
    const confirm = page.getByRole("alertdialog");
    await expect(confirm).toContainText(parent.title);
    await expect(confirm).toContainText("1 child page moves to the top level");
    const deleted = page.waitForResponse(
      (response) =>
        response.request().method() === "DELETE" &&
        response.url().endsWith(`/api/issues/${parent.id}`),
    );
    await confirm.getByRole("button", { name: "Delete", exact: true }).click();
    expect((await deleted).status()).toBe(204);
    // Deleting the open document leaves its page once the server confirms.
    await expect(page).toHaveURL(new RegExp(`/${slug}/documents$`));
    await expect(
      library.getByRole("link", { name: parent.title, exact: true }),
    ).toHaveCount(0);
    await expect(
      library.getByRole("link", { name: child.title, exact: true }),
    ).toBeVisible();
    expect(
      (await api.cortexRequest(`/api/issues/${child.id}`)).body.parent_issue_id,
    ).toBeNull();

    await page.goto(`/${slug}/collections/${collection.id}`);
    const tables = page.getByRole("complementary", {
      name: "Tables",
      exact: true,
    });
    const tableRow = tables.getByRole("listitem").filter({
      has: page.getByRole("link", { name: new RegExp(collection.name) }),
    });
    await tableRow.hover();
    await tableRow
      .getByRole("button", { name: `Actions for ${collection.name}`, exact: true })
      .click();
    await page
      .getByRole("menuitem", { name: "Delete table", exact: true })
      .click();
    await expect(confirm).toContainText(collection.name);
    const archived = page.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        response.url().endsWith(`/api/collections/${collection.id}`),
    );
    await confirm.getByRole("button", { name: "Delete", exact: true }).click();
    expect((await archived).status()).toBe(200);
    await expect(page).toHaveURL(new RegExp(`/${slug}/collections$`));
    await expect(
      tables.getByRole("link", { name: new RegExp(collection.name) }),
    ).toHaveCount(0);
    expect(
      (await api.cortexRequest(`/api/collections/${collection.id}`)).status,
    ).toBe(404);
  });

  // FR-028 / AC-8: a row links to the issue that implements it, the issue
  // lists the row, and deleting the issue leaves a link that says so.
  test("a relation links a record to issues, the issue links back, and a deleted issue stays visible", async ({
    page,
  }) => {
    const suffix = Date.now();
    const collection = await api.createCollection(`G1 relations ${suffix}`);
    const recordTitle = `Embed live views ${suffix}`;
    const record = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/records`,
        "POST",
        { title: recordTitle },
      )
    ).body;
    const existing = await api.createIssue(`Build the embed block ${suffix}`);
    await page.goto(`/${slug}/collections/${collection.id}`);

    // A relation names what it links to when it is created.
    await page
      .locator("thead")
      .getByRole("button", { name: "New field", exact: true })
      .click();
    const fieldPanel = page.getByRole("dialog", { name: "New field", exact: true });
    await fieldPanel.getByLabel("Name", { exact: true }).fill("Implementation");
    await fieldPanel.getByRole("combobox", { name: "Field type" }).click();
    await page.getByRole("option", { name: "Relation", exact: true }).click();
    await expect(
      fieldPanel.getByRole("combobox", { name: "Link to" }),
    ).toContainText("Issues");
    const fieldCreated = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/api/collections/${collection.id}/fields`),
    );
    await fieldPanel.getByRole("button", { name: "Create field", exact: true }).click();
    const field = await (await fieldCreated).json();
    expect(field.config.relation).toEqual({ to_type: "issue" });
    await expect(fieldPanel).toBeHidden();

    // Link an existing issue from the cell.
    const cell = page.getByRole("button", {
      name: `Implementation: ${recordTitle}`,
      exact: true,
    });
    await cell.click();
    await page.getByPlaceholder("Search Issues…").fill(`embed block ${suffix}`);
    const linked = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/records/${record.id}/links`),
    );
    await page
      .getByRole("button", { name: new RegExp(`Build the embed block ${suffix}`) })
      .click();
    expect((await linked).status()).toBe(201);
    await page.keyboard.press("Escape");
    await expect(cell).toContainText(existing.identifier);

    // The record's panel is the way out to discussion: convert it to an issue.
    await page.getByRole("row", { name: new RegExp(recordTitle) }).hover();
    await page
      .getByRole("button", { name: `Open ${recordTitle}`, exact: true })
      .click();
    const panel = page.getByRole("complementary", { name: "Record details" });
    const relation = panel.getByRole("region", { name: "Implementation" });
    await expect(
      relation.getByRole("link", { name: new RegExp(existing.identifier) }),
    ).toBeVisible();
    await panel.getByRole("button", { name: "Convert to issue" }).click();
    const dialog = page.getByRole("dialog", { name: "Convert to issue" });
    await expect(dialog.getByLabel("Issue title")).toHaveValue(recordTitle);
    const issueCreated = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith("/api/issues"),
    );
    await dialog.getByRole("button", { name: "Create issue", exact: true }).click();
    const converted = await (await issueCreated).json();
    await expect(dialog).toBeHidden();
    await expect(
      relation.getByRole("link", { name: new RegExp(converted.identifier) }),
    ).toBeVisible();

    try {
      // The issue side lists the record, and the link opens it.
      await relation
        .getByRole("link", { name: new RegExp(converted.identifier) })
        .click();
      await expect(page).toHaveURL(new RegExp(`/${slug}/issues/`));
      const back = page.getByRole("link", { name: new RegExp(recordTitle) });
      await expect(back).toBeVisible();
      await back.click();
      await expect(page).toHaveURL(
        new RegExp(`/collections/${collection.id}\\?record=${record.id}`),
      );
      await expect(
        page
          .getByRole("complementary", { name: "Record details" })
          .getByLabel("Record title"),
      ).toHaveValue(recordTitle);

      // Deleting a linked issue says which records point at it first.
      await page.goto(`/${slug}/issues/${existing.id}`);
      await expect(
        page.getByRole("button", { name: /Linked records/ }),
      ).toBeVisible();
      const links = await api.cortexRequest(
        `/api/issues/${existing.id}/record-links`,
      );
      expect(links.body.links).toHaveLength(1);
      expect(links.body.links[0].record_id).toBe(record.id);
    } finally {
      await api.deleteIssue(converted.id);
    }

    // The edge survives its issue and the cell says what happened.
    await api.deleteIssue(existing.id);
    await page.goto(`/${slug}/collections/${collection.id}`);
    await expect(cell).toContainText("Deleted");
    const after = (
      await api.cortexRequest(
        `/api/collections/${collection.id}/records?record_id=${record.id}`,
      )
    ).body.records[0].links[field.id];
    expect(after).toHaveLength(2);
    expect(after.every((link: { missing: boolean }) => link.missing)).toBe(true);
  });

  test("a cell edit that loses a conflict keeps the user's value until they choose to overwrite", async ({
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
    const cell = page.getByRole("button", {
      name: "Value: Find this row",
      exact: true,
    });
    await expect(cell).toContainText("Before");
    await cell.click();
    const input = page.getByPlaceholder("Enter value…");
    await expect(input).toHaveValue("Before");
    await input.fill("Local draft");
    // Another writer renames the row out of the search and changes the cell.
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
    const conflict = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(fieldPath),
    );
    await input.press("Enter");
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
      .getByRole("button", { name: "Use mine: Local draft", exact: true })
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
    const pill = page.locator(`[data-row-id="${issue.id}"]`);
    await expect(
      page.getByRole("region", { name: day, exact: true }).locator(pill),
    ).toBeVisible();
    const destination = new Date(now);
    destination.setDate(now.getDate() + 1);
    const movedDay = `${destination.getFullYear()}-${String(destination.getMonth() + 1).padStart(2, "0")}-${String(destination.getDate()).padStart(2, "0")}`;
    const saved = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(`/api/issues/${issue.id}`),
    );
    await pill.dragTo(
      page.getByRole("region", { name: movedDay, exact: true }),
    );
    expect((await saved).status()).toBe(200);
    await page.reload();
    await expect(
      page.getByRole("region", { name: movedDay, exact: true }).locator(pill),
    ).toBeVisible();
    await page.getByRole("button", { name: "Week", exact: true }).click();
    await expect(
      page.getByRole("region", { name: day, exact: true }),
    ).toBeVisible();
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
    const library = page.getByRole("complementary", {
      name: "Documents",
      exact: true,
    });
    // Documents and tables each open a column of their own.
    await expect(
      page.getByRole("complementary", { name: "Tables", exact: true }),
    ).toHaveCount(0);
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
    // Row actions take no room until their row is hovered.
    const leafRow = library.locator('[draggable="true"]').filter({
      has: page.getByRole("link", { name: leaf.title, exact: true }),
    });
    await leafRow.hover();
    await leafRow
      .getByRole("button", { name: "Add to favorites", exact: true })
      .click();
    // Tree filters live in the section's filter menu.
    await library.getByRole("button", { name: "Filter documents" }).click();
    await page.getByRole("menuitemradio", { name: "Favorites" }).click();
    await page.keyboard.press("Escape");
    await expect(
      library.getByRole("link", { name: leaf.title, exact: true }),
    ).toBeVisible();
    await expect(
      library.getByRole("link", { name: b.title, exact: true }),
    ).toHaveCount(0);
    await library.getByRole("button", { name: "Filter documents" }).click();
    await page.getByRole("menuitemradio", { name: "Recent" }).click();
    await page.keyboard.press("Escape");
    await expect(
      library.getByRole("link", { name: leaf.title, exact: true }),
    ).toBeVisible();
    await expect(
      library.getByRole("link", { name: a.title, exact: true }),
    ).toHaveCount(0);
    await library.getByRole("button", { name: "Clear filter Recent" }).click();
  });
});
