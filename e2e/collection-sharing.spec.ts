import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import { TestApiClient } from "./fixtures";

test("shares a collection with read/edit roles, revokes access, and pins it personally", async ({
  page,
  browser,
}, testInfo) => {
  test.setTimeout(150000);
  const owner = await createTestApi();
  const reader = new TestApiClient();
  const readerContext = await browser.newContext();
  try {
    const slug = await loginAsDefault(page);
    const ws = (await owner.getWorkspaces()).find((w) => w.slug === slug)!;
    const email = `table-reader-${Date.now()}@multica.ai`;
    const identity = await reader.login(email, "Table Reader");
    await owner.addTestWorkspaceMember(ws.id, identity.user.id);
    reader.setWorkspaceId(ws.id);
    reader.setWorkspaceSlug(slug);
    await reader.markUserOnboarded();
    const table = await owner.createCollection(
      `Sharing acceptance ${Date.now()}`,
    );
    const root = `/api/collections/${table.id}`;
    const field = (
      await owner.cortexRequest(`${root}/fields`, "POST", {
        name: "Notes",
        type: "text",
      })
    ).body;
    await owner.cortexRequest(`${root}/records`, "POST", {
      title: "Shared row",
      fields: { [field.id]: "Original note" },
    });
    expect((await reader.cortexRequest(root)).status).toBe(404);
    await page.goto(`/${slug}/collections/${table.id}`);
    await page
      .getByRole("button", { name: "Pin to sidebar", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Unpin from sidebar", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      page.getByRole("button", { name: table.name, exact: true }).first(),
    ).toBeVisible();
    await page.getByRole("button", { name: "Share", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Share table" });
    await dialog.getByRole("button", { name: new RegExp(email) }).click();
    await page.screenshot({
      path: testInfo.outputPath("collection-share.png"),
    });
    await dialog.getByRole("button", { name: "Save sharing" }).click();
    await expect(dialog).toBeHidden();
    expect((await reader.cortexRequest(root)).body.access).toMatchObject({
      can_edit: false,
      can_manage: false,
    });
    await readerContext.addInitScript((token) => {
      localStorage.setItem("multica_token", token);
      localStorage.setItem("multica:chat:isOpen", "false");
    }, reader.getToken()!);
    const readerPage = await readerContext.newPage();
    await readerPage.goto(
      new URL(`/${slug}/collections/${table.id}`, page.url()).toString(),
    );
    await expect(
      readerPage.getByText("Original note", { exact: true }),
    ).toBeVisible();
    await expect(
      readerPage.getByRole("button", { name: "New row", exact: true }),
    ).toBeDisabled();
    await expect(
      readerPage.getByRole("button", { name: "Share", exact: true }),
    ).toHaveCount(0);
    expect(
      (
        await reader.cortexRequest(`${root}/records`, "POST", {
          title: "Forbidden",
        })
      ).status,
    ).toBe(403);
    await readerPage
      .getByRole("button", { name: "Pin to sidebar", exact: true })
      .click();
    expect(
      (await reader.cortexRequest("/api/pins?include=collection")).body,
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ item_type: "collection", item_id: table.id }),
      ]),
    );
    await page.getByRole("button", { name: "Share", exact: true }).click();
    await dialog
      .getByRole("combobox", { name: "Permission", exact: true })
      .click();
    await page.getByRole("option", { name: "Can edit", exact: true }).click();
    await dialog.getByRole("button", { name: "Save sharing" }).click();
    await expect(dialog).toBeHidden();
    await expect(
      readerPage.getByRole("button", { name: "New row", exact: true }).first(),
    ).toBeEnabled();
    await readerPage
      .getByRole("button", { name: "Notes: Shared row", exact: true })
      .click();
    const input = readerPage
      .getByRole("textbox")
      .filter({ visible: true })
      .last();
    await input.fill("Edited by collaborator");
    await input.press("Enter");
    await expect(
      readerPage.getByText("Edited by collaborator", { exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Share", exact: true }).click();
    await dialog.getByRole("button", { name: "Remove", exact: true }).click();
    await dialog.getByRole("button", { name: "Save sharing" }).click();
    await expect(dialog).toBeHidden();
    await expect(
      readerPage.getByText("Edited by collaborator", { exact: true }),
    ).toHaveCount(0);
    expect((await reader.cortexRequest(root)).status).toBe(404);
    expect(
      (await reader.cortexRequest("/api/pins?include=collection")).body,
    ).toEqual([]);
    // Wider audiences use the same read/edit control as named collaborators.
    const project = await owner.createProject("Shared table project");
    await page.getByRole("button", { name: "Share", exact: true }).click();
    await dialog.getByRole("combobox", { name: "Share with" }).click();
    await page.getByRole("option", { name: "Project", exact: true }).click();
    await dialog.getByRole("combobox", { name: "Select project" }).click();
    await page.getByRole("option", { name: project.title, exact: true }).click();
    await expect(dialog.getByText(/All workspace members can currently access projects/)).toBeVisible();
    await dialog.getByRole("button", { name: "Save sharing" }).click();
    await expect(dialog).toBeHidden();
    expect((await reader.cortexRequest(root)).body.access).toMatchObject({scope:"project",project_id:project.id,can_edit:false});
    await page.getByRole("button", { name: "Share", exact: true }).click();
    await dialog.getByRole("combobox", { name: "Share with" }).click();
    await page.getByRole("option", { name: "Workspace", exact: true }).click();
    await dialog.getByRole("combobox", { name: "Permission", exact: true }).click();
    await page.getByRole("option", { name: "Can edit", exact: true }).click();
    await dialog.getByRole("button", { name: "Save sharing" }).click();
    await expect(dialog).toBeHidden();
    expect((await reader.cortexRequest(root)).body.access).toMatchObject({scope:"workspace",can_edit:true});
    await page.screenshot({
      path: testInfo.outputPath("collection-pinned.png"),
    });
  } finally {
    await readerContext.close();
    await owner.cleanup();
    await reader.cleanup();
  }
});
