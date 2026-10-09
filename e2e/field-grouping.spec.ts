import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Field grouping", () => {
  test.describe.configure({ timeout: 120000 });
  let api: TestApiClient;
  const propertyIds: string[] = [];
  test.beforeEach(async () => {
    api = await createTestApi();
  });
  test.afterEach(async () => {
    await api.cleanup();
    for (const id of propertyIds.splice(0))
      await api.cortexRequest(`/api/properties/${id}`, "PATCH", {
        archived: true,
      });
  });

  test("issue board and list group by number beside Filter and retain the choice after reload", async ({
    page,
  }, info) => {
    const result = await api.cortexRequest("/api/properties", "POST", {
      name: "Estimate",
      type: "number",
    });
    expect(result.status).toBe(201);
    propertyIds.push(result.body.id);
    const issue = await api.createIssue("Zero estimate task", {
      status: "todo",
      properties: { [result.body.id]: 0 },
    });
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/issues`);
    const groupButton = page.getByRole("button", {
      name: "Group: Status",
      exact: true,
    });
    await expect(groupButton).toBeVisible();
    const filter = await page
      .getByRole("button", { name: "Filter", exact: true })
      .boundingBox();
    const group = await groupButton.boundingBox();
    expect(group!.x).toBeGreaterThan(filter!.x);
    await groupButton.click();
    await page
      .getByRole("menuitemradio", { name: "Estimate", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Group: Estimate", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText(issue.title, { exact: true }).first(),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("issue-number-grouping.png"),
    });
    await page.reload();
    await expect(
      page.getByRole("button", { name: "Group: Estimate", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText(issue.title, { exact: true }).first(),
    ).toBeVisible();
    await page.getByRole("button", { name: "Board", exact: true }).click();
    await page.getByRole("menuitemradio", { name: "List", exact: true }).click();
    await expect(page.getByRole("button", { name: "Group: Estimate", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Group: Estimate", exact: true }).click();
    await page.getByRole("menuitemradio", { name: "Status", exact: true }).click();
    await expect(page.getByRole("button", { name: "Group: Status", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Group: Status", exact: true }).click();
    await page.getByRole("menuitemradio", { name: "Estimate", exact: true }).click();
    const zeroGroup = page.getByRole("button", { name: "0 1", exact: true });
    await expect(zeroGroup).toBeVisible();
    await expect(page.getByText(issue.title, { exact: true }).first()).toBeVisible();
    await zeroGroup.click();
    await expect(zeroGroup).toHaveAttribute("aria-expanded", "false");
    await page.reload();
    await expect(page.getByRole("button", { name: "Group: Estimate", exact: true })).toBeVisible();
    await expect(zeroGroup).toHaveAttribute("aria-expanded", "false");
    await zeroGroup.click();
    await expect(page.getByText(issue.title, { exact: true }).first()).toBeVisible();
    await page.screenshot({ path: info.outputPath("issue-list-number-grouping.png") });
    await page.getByRole("button", { name: "List", exact: true }).click();
    await page
      .getByRole("menuitemradio", { name: "Table", exact: true })
      .click();
    await page.getByRole("button", { name: "Group", exact: true }).click();
    await page
      .getByRole("menuitemradio", { name: "Estimate", exact: true })
      .click();
    await expect(
      page.getByText(issue.title, { exact: true }).first(),
    ).toBeVisible();
  });

  test("collection table and board group by text, multi-select, formula and title", async ({
    page,
  }, info) => {
    const c = await api.createCollection("Grouping acceptance");
    const field = async (body: unknown) => {
      const result = await api.cortexRequest(
        `/api/collections/${c.id}/fields`,
        "POST",
        body,
      );
      expect(result.status).toBe(201);
      return result.body;
    };
    const text = await field({ name: "Region", type: "text" });
    const multi = await field({
      name: "Tags",
      type: "multi_select",
      config: {
        options: [
          { name: "Alpha", color: "#666666" },
          { name: "Beta", color: "#777777" },
        ],
      },
    });
    await field({
      name: "Title length",
      type: "formula",
      config: { formula: { expression: "LEN({title})" } },
    });
    for (const title of ["Record A", "Record B"]) {
      const result = await api.cortexRequest(
        `/api/collections/${c.id}/records`,
        "POST",
        {
          title,
          fields: {
            [text.id]: "North",
            [multi.id]: multi.config.options.map(
              (option: { id: string }) => option.id,
            ),
          },
        },
      );
      expect(result.status).toBe(201);
    }
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/collections/${c.id}`);
    await page.getByRole("button", { name: "Group", exact: true }).click();
    await page
      .getByRole("menuitemradio", { name: "Region", exact: true })
      .click();
    await page.keyboard.press("Escape");
    await expect(
      page.getByRole("heading", { name: "North 2", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Group Region", exact: true })
      .click();
    await page
      .getByRole("menuitemradio", { name: "Tags", exact: true })
      .click();
    await page.keyboard.press("Escape");
    await expect(
      page.getByRole("heading", { name: "Alpha, Beta 2", exact: true }),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("collection-multi-grouping.png"),
    });
    await page.getByRole("radio", { name: "Board", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "Alpha, Beta", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Record A", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Group Tags", exact: true }).click();
    await page
      .getByRole("menuitemradio", { name: "Title length", exact: true })
      .click();
    await page.keyboard.press("Escape");
    await expect(
      page.getByRole("heading", { name: "8", exact: true }),
    ).toBeVisible();
    await page.reload();
    await expect(
      page.getByRole("button", { name: "Group Title length", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Record B", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Group Title length", exact: true })
      .click();
    await page
      .getByRole("menuitemradio", { name: "Region", exact: true })
      .click();
    await page.keyboard.press("Escape");
    const card = page
      .locator("article")
      .filter({
        has: page.getByRole("button", { name: "Record A", exact: true }),
      });
    await card.dragTo(
      page.getByRole("region", { name: "No value", exact: true }),
    );
    await expect(
      page
        .getByRole("region", { name: "No value", exact: true })
        .getByRole("button", { name: "Record A", exact: true }),
    ).toBeVisible();
    await page.reload();
    await expect(
      page
        .getByRole("region", { name: "No value", exact: true })
        .getByRole("button", { name: "Record A", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Group Region", exact: true })
      .click();
    await page
      .getByRole("menuitemradio", { name: "Name", exact: true })
      .click();
    await page.keyboard.press("Escape");
    await expect(
      page.getByRole("heading", { name: "Record B", exact: true }),
    ).toBeVisible();
  });
});
