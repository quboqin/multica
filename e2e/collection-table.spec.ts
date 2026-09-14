import "./env";
import { expect, test, type Locator, type Page, type Request } from "@playwright/test";
import { createTestApi, loginAsDefault, waitForPageText } from "./helpers";
import { TestApiClient } from "./fixtures";
import type {
  TestCollection,
  TestCollectionField,
  TestCollectionRecord,
} from "./fixtures";

const API_BASE = (
  process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`
).replace(/\/$/, "");

const COLLECTION_FIELDS = [
  { name: "Note", type: "text" as const },
  { name: "Quantity", type: "number" as const },
  { name: "Checked", type: "checkbox" as const },
];

type CollectionCreateResponse = {
  collection: TestCollection;
  fields: TestCollectionField[];
  replayed: boolean;
};

type RecordCreateResponse = {
  record: TestCollectionRecord;
  replayed: boolean;
};

function apiPath(request: Request): string {
  return new URL(request.url()).pathname;
}

function isCollectionRecordPatch(request: Request): boolean {
  return (
    request.method() === "PATCH" &&
    /^\/api\/collections\/[^/]+\/records\/[^/]+$/.test(apiPath(request))
  );
}

async function openCollections(page: Page, workspaceSlug: string) {
  await page.goto(`/${workspaceSlug}/collections`, {
    waitUntil: "domcontentloaded",
  });
  await expect(
    page.getByRole("heading", { name: "Collections", exact: true }),
  ).toBeVisible();
}

async function submitCollection(
  page: Page,
  name: string,
): Promise<CollectionCreateResponse> {
  const nameInput = page.getByRole("textbox", { name: "Collection name" });
  await expect(nameInput).toBeVisible();
  await nameInput.fill(name);
  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      apiPath(response.request()) === "/api/collections",
  );
  await page.getByRole("button", { name: "Create collection", exact: true }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(201);
  const body = (await response.json()) as CollectionCreateResponse;
  expect(body.replayed).toBe(false);
  expect(body.collection.id).toBeTruthy();
  expect(body.fields.map((field) => field.name)).toEqual(
    COLLECTION_FIELDS.map((field) => field.name),
  );
  await expect(page).toHaveURL(new RegExp(`/collections/${body.collection.id}$`));
  return body;
}

async function openCollectionDetail(
  page: Page,
  workspaceSlug: string,
  collection: TestCollection,
) {
  await page.goto(`/${workspaceSlug}/collections/${collection.id}`, {
    waitUntil: "domcontentloaded",
  });
  await expect(
    page.getByRole("heading", { name: collection.name, exact: true }),
  ).toBeVisible();
  for (const field of ["Title", "Note", "Quantity", "Checked"]) {
    await expect(
      page.getByRole("columnheader", { name: field, exact: true }),
    ).toBeVisible();
  }
}

async function addRecordInBrowser(
  page: Page,
  title: string,
): Promise<RecordCreateResponse> {
  const titleInput = page.getByRole("textbox", { name: "Record title" });
  await titleInput.fill(title);
  const responsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/api\/collections\/[^/]+\/records$/.test(apiPath(response.request())),
  );
  await page.getByRole("button", { name: "Add record", exact: true }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(201);
  const body = (await response.json()) as RecordCreateResponse;
  expect(body.replayed).toBe(false);
  expect(body.record.title).toBe(title);
  return body;
}

async function rowByTitle(page: Page, title: string): Promise<Locator> {
  const titleInputs = page.locator('tbody tr input[aria-label="Title"]');
  await expect
    .poll(
      async () => {
        const count = await titleInputs.count();
        for (let index = 0; index < count; index += 1) {
          if ((await titleInputs.nth(index).inputValue()) === title) return true;
        }
        return false;
      },
      { timeout: 30000, intervals: [100, 250, 500] },
    )
    .toBe(true);

  const count = await titleInputs.count();
  for (let index = 0; index < count; index += 1) {
    const input = titleInputs.nth(index);
    if ((await input.inputValue()) === title) {
      return input.locator("xpath=ancestor::tr");
    }
  }
  throw new Error(`Could not resolve rendered collection row ${title}`);
}

async function patchResponse(page: Page, action: () => Promise<void>, status: number) {
  const responsePromise = page.waitForResponse(
    (response) => isCollectionRecordPatch(response.request()),
  );
  await action();
  const response = await responsePromise;
  expect(response.status()).toBe(status);
  return response;
}

async function saveTextOrNumber(
  page: Page,
  row: Locator,
  fieldName: string,
  value: string,
  role: "textbox" | "spinbutton" = "textbox",
) {
  const input = row.getByRole(role, { name: fieldName });
  await input.fill(value);
  const form = input.locator("xpath=ancestor::form");
  await patchResponse(
    page,
    () => form.getByRole("button", { name: "Save", exact: true }).click(),
    200,
  );
  await expect(input).toHaveValue(value);
}

async function clearTextOrNumber(
  page: Page,
  row: Locator,
  fieldName: string,
  role: "textbox" | "spinbutton" = "textbox",
) {
  const input = row.getByRole(role, { name: fieldName });
  const form = input.locator("xpath=ancestor::form");
  await patchResponse(
    page,
    () => form.getByRole("button", { name: "Clear", exact: true }).click(),
    200,
  );
  await expect(input).toHaveValue("");
}

async function scrollTableToBottom(page: Page) {
  const scrollContainer = page.locator("table").locator("xpath=..");
  await scrollContainer.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
}

async function settlePage(page: Page) {
  await page.waitForLoadState("networkidle");
}

test.describe("T2 collection shared TableView", () => {
  test.describe.configure({ timeout: 120000 });

  let api: TestApiClient;
  let memberApi: TestApiClient | undefined;
  let workspaceSlug: string;

  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    workspaceSlug = await loginAsDefault(page);
  });

  test.afterEach(async () => {
    await memberApi?.cleanup();
    await api?.cleanup();
  });

  test("creates a collection and record, persists all scalar set/clear paths, and has zero task side effects", async ({
    page,
  }) => {
    await openCollections(page, workspaceSlug);
    await settlePage(page);
    const before = await api.readCollectionSideEffectCounts();
    const unexpectedRequests: string[] = [];
    const requestListener = (request: Request) => {
      const path = apiPath(request);
      if (
        path.startsWith("/api/issues") ||
        path.startsWith("/api/inbox") ||
        path.startsWith("/api/agents") ||
        path.includes("/task")
      ) {
        unexpectedRequests.push(`${request.method()} ${path}`);
      }
    };
    page.on("request", requestListener);

    const collectionName = `E2E Scalars ${Date.now()}`;
    const createdCollection = await submitCollection(page, collectionName);
    api.trackCollection(createdCollection.collection.id);
    await openCollectionDetail(page, workspaceSlug, createdCollection.collection);
    const recordTitle = `E2E Record ${Date.now()}`;
    const createdRecord = await addRecordInBrowser(page, recordTitle);
    const row = await rowByTitle(page, recordTitle);

    await saveTextOrNumber(page, row, "Note", "browser text");
    await clearTextOrNumber(page, row, "Note");
    await saveTextOrNumber(page, row, "Quantity", "7", "spinbutton");
    await clearTextOrNumber(page, row, "Quantity", "spinbutton");

    const checked = row.getByRole("checkbox", { name: "Checked" });
    await patchResponse(page, () => checked.check(), 200);
    await expect(checked).toBeChecked();
    const checkboxCell = checked.locator("xpath=..").locator("xpath=..");
    await patchResponse(
      page,
      () => checkboxCell.getByRole("button", { name: "Clear", exact: true }).click(),
      200,
    );
    await expect(checked).not.toBeChecked();

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(
      page.getByRole("heading", { name: collectionName, exact: true }),
    ).toBeVisible();
    const refreshedRow = await rowByTitle(page, recordTitle);
    await expect(refreshedRow.getByRole("textbox", { name: "Note" })).toHaveValue("");
    await expect(
      refreshedRow.getByRole("spinbutton", { name: "Quantity" }),
    ).toHaveValue("");
    await expect(
      refreshedRow.getByRole("checkbox", { name: "Checked" }),
    ).not.toBeChecked();

    page.off("request", requestListener);
    expect(unexpectedRequests).toEqual([]);
    expect(createdRecord.record.collection_id).toBe(createdCollection.collection.id);
    expect(await api.readCollectionSideEffectCounts()).toEqual(before);
  });

  test("loads the 201st row and recovers a tail-row revision conflict on explicit retry", async ({
    page,
  }) => {
    const collection = await api.createCollection(
      `E2E Pagination ${Date.now()}`,
      COLLECTION_FIELDS,
    );
    api.trackCollection(collection.collection.id);
    const records = await api.seedCollectionRecords(
      collection.collection.id,
      Array.from({ length: 201 }, (_, index) => ({
        title: `E2E Tail ${index.toString().padStart(3, "0")}`,
      })),
    );
    const tail = records.find((record) => record.title === "E2E Tail 200");
    const noteField = collection.fields.find(
      (field: TestCollectionField) => field.name === "Note",
    );
    if (!tail || !noteField) throw new Error("Pagination fixture is incomplete");

    await openCollectionDetail(page, workspaceSlug, collection.collection);
    await expect(
      page.getByRole("heading", { name: collection.collection.name, exact: true }).locator(".."),
    ).toContainText("201");
    await expect(await rowByTitle(page, "E2E Tail 000")).toBeVisible();

    await scrollTableToBottom(page);
    const loadMore = page.getByRole("button", { name: "Load more", exact: true });
    await expect(loadMore).toBeVisible();
    const pageResponsePromise = page.waitForResponse((response) => {
      if (
        response.request().method() !== "POST" ||
        !response.url().includes(`/api/collections/${collection.collection.id}/records/query`) ||
        response.status() !== 200
      ) {
        return false;
      }
      const body = response.request().postDataJSON() as {
        page?: { limit?: number; cursor?: string | null };
      };
      return body.page?.limit === 200 && Boolean(body.page.cursor);
    });
    await loadMore.click();
    const pageResponse = await pageResponsePromise;
    expect(pageResponse.status()).toBe(200);
    await scrollTableToBottom(page);
    const tailRow = await rowByTitle(page, tail.title);
    const noteInput = tailRow.getByRole("textbox", { name: "Note" });
    await noteInput.fill("browser stale value");

    const externalUpdate = await api.updateCollectionRecord(
      collection.collection.id,
      tail.id,
      tail.revision,
      { field_id: noteField.id, op: "set", value: "external value" },
    );
    expect(externalUpdate.response.status).toBe(200);

    const noteForm = noteInput.locator("xpath=ancestor::form");
    await patchResponse(
      page,
      () => noteForm.getByRole("button", { name: "Save", exact: true }).click(),
      409,
    );
    const conflictedRow = await rowByTitle(page, tail.title);
    await expect(
      conflictedRow.getByRole("textbox", { name: "Note" }),
    ).toHaveValue("browser stale value");
    await expect(
      conflictedRow.getByRole("button", { name: "Retry", exact: true }),
    ).toBeVisible();

    await patchResponse(
      page,
      () => conflictedRow.getByRole("button", { name: "Retry", exact: true }).click(),
      200,
    );
    await expect(
      (await rowByTitle(page, tail.title)).getByRole("textbox", { name: "Note" }),
    ).toHaveValue("browser stale value");
    const authoritative = await api.getCollectionRecord(
      collection.collection.id,
      tail.id,
    );
    expect(authoritative.revision).toBe(tail.revision + 2);
    expect(authoritative.fields[noteField.id]).toBe("browser stale value");
  });

  test("enforces member collection permissions while retaining member record access", async ({
    page,
  }) => {
    const collection = await api.createCollection(
      `E2E Permissions ${Date.now()}`,
      COLLECTION_FIELDS,
    );
    api.trackCollection(collection.collection.id);
    const existingRecord = await api.createCollectionRecord(
      collection.collection.id,
      `Owner record ${Date.now()}`,
    );
    const memberEmail = `collection-member-${Date.now()}@multica.ai`;
    await api.createWorkspaceMember(memberEmail, "Collection Member");
    memberApi = new TestApiClient();
    await memberApi.login(memberEmail, "Collection Member");
    memberApi.setWorkspace(api.getWorkspace());
    await memberApi.markUserOnboarded();
    const memberToken = memberApi.getToken();
    if (!memberToken) throw new Error("Member login did not return a token");
    await page.addInitScript((token) => {
      localStorage.setItem("multica_token", token);
      localStorage.setItem("multica:chat:isOpen", "false");
    }, memberToken);

    await page.goto(`/${workspaceSlug}/collections`, {
      waitUntil: "domcontentloaded",
    });
    await waitForPageText(page, collection.collection.name);
    await expect(
      page.getByRole("textbox", { name: "Collection name" }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Create collection", exact: true }),
    ).toHaveCount(0);
    await page.getByRole("link", { name: collection.collection.name }).click();
    await openCollectionDetail(page, workspaceSlug, collection.collection);
    const existingRow = await rowByTitle(page, existingRecord.title);

    const memberRecordTitle = `Member record ${Date.now()}`;
    const memberRecord = await addRecordInBrowser(page, memberRecordTitle);
    await expect(await rowByTitle(page, memberRecordTitle)).toBeVisible();
    const note = existingRow.getByRole("textbox", { name: "Note" });
    await note.fill("member edit");
    await patchResponse(
      page,
      () =>
        note
          .locator("xpath=ancestor::form")
          .getByRole("button", { name: "Save", exact: true })
          .click(),
      200,
    );
    await expect(note).toHaveValue("member edit");

    const deniedCreate = await page.evaluate(
      async ({ apiBase, slug }) => {
        const token = localStorage.getItem("multica_token");
        const response = await fetch(`${apiBase}/api/collections`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token ?? ""}`,
            "X-Workspace-Slug": slug,
          },
          body: JSON.stringify({
            client_request_id: crypto.randomUUID(),
            name: "Member must not create",
            fields: [{ name: "Note", type: "text" }],
          }),
        });
        return { status: response.status, body: await response.text() };
      },
      { apiBase: API_BASE, slug: workspaceSlug },
    );
    expect(deniedCreate.status).toBe(403);
    expect(deniedCreate.body).toContain("insufficient permissions");
  });
});
