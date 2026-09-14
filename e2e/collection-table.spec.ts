import "./env";
import {
  expect,
  test,
  type BrowserContext,
  type Locator,
  type Page,
  type Request,
} from "@playwright/test";
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

type PatchTarget = {
  collectionId: string;
  recordId: string;
  fieldId: string;
  op: "set" | "clear";
};

type CollectionPatchBody = {
  expected_revision?: number;
  change?: {
    field_id?: string;
    op?: "set" | "clear";
    value?: unknown;
  };
};

function requestBody(request: Request): CollectionPatchBody | null {
  try {
    return request.postDataJSON() as CollectionPatchBody;
  } catch {
    return null;
  }
}

function isCollectionRecordPatch(request: Request, target: PatchTarget): boolean {
  const body = requestBody(request);
  return (
    request.method() === "PATCH" &&
    apiPath(request) ===
      `/api/collections/${target.collectionId}/records/${target.recordId}` &&
    body?.change?.field_id === target.fieldId &&
    body?.change?.op === target.op
  );
}

function isCollectionRecordQuery(request: Request, collectionId: string): boolean {
  return (
    request.method() === "POST" &&
    apiPath(request) === `/api/collections/${collectionId}/records/query`
  );
}

function collectionQueryPage(request: Request) {
  try {
    return (
      request.postDataJSON() as {
        page?: { limit?: number; cursor?: string | null };
      }
    ).page;
  } catch {
    return undefined;
  }
}

function isTaskSideEffectRequest(request: Request): boolean {
  if (["GET", "HEAD", "OPTIONS"].includes(request.method())) return false;
  const path = apiPath(request);
  return (
    /^\/api\/(?:issues|inbox|agents|tasks)(?:\/|$)/.test(path) ||
    /\/(?:quick-actions|run-confirm)(?:\/|$)/.test(path) ||
    /^\/tasks(?:\/|$)/.test(path)
  );
}

function assertOwnField(
  record: TestCollectionRecord,
  fieldId: string,
  value: unknown,
) {
  expect(Object.prototype.hasOwnProperty.call(record.fields, fieldId)).toBe(true);
  expect(record.fields[fieldId]).toEqual(value);
}

function assertClearedField(record: TestCollectionRecord, fieldId: string) {
  expect(Object.prototype.hasOwnProperty.call(record.fields, fieldId)).toBe(false);
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

async function patchResponse(
  page: Page,
  target: PatchTarget,
  action: () => Promise<void>,
  status: number,
) {
  const responsePromise = page.waitForResponse(
    (response) => isCollectionRecordPatch(response.request(), target),
  );
  await action();
  const response = await responsePromise;
  expect(response.status()).toBe(status);
  return {
    response,
    request: requestBody(response.request()),
  };
}

async function saveTextOrNumber(
  page: Page,
  row: Locator,
  target: PatchTarget,
  fieldName: string,
  value: string,
  role: "textbox" | "spinbutton" = "textbox",
  submit: "click" | "enter" = "click",
) {
  const input = row.getByRole(role, { name: fieldName });
  // Force an input event when proving set("") after a prior clear; filling the
  // same empty value can otherwise leave the editor clean in some browsers.
  if (value === "") await input.fill("temporary non-empty draft");
  await input.fill(value);
  const form = input.locator("xpath=ancestor::form");
  const result = await patchResponse(page, target, () =>
    submit === "enter"
      ? input.press("Enter")
      : form.getByRole("button", { name: "Save", exact: true }).click(),
  200);
  await expect(input).toHaveValue(value);
  return result;
}

async function clearTextOrNumber(
  page: Page,
  row: Locator,
  target: PatchTarget,
  fieldName: string,
  role: "textbox" | "spinbutton" = "textbox",
) {
  const input = row.getByRole(role, { name: fieldName });
  const form = input.locator("xpath=ancestor::form");
  const result = await patchResponse(page, target, () =>
    form.getByRole("button", { name: "Clear", exact: true }).click(),
  200);
  await expect(input).toHaveValue("");
  return result;
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

async function readMemberIdentity(
  page: Page,
  workspaceId: string,
  workspaceSlug: string,
) {
  return page.evaluate(
    async ({ apiBase, workspaceId: id, workspaceSlug: slug }) => {
      const token = localStorage.getItem("multica_token");
      const headers = {
        Authorization: `Bearer ${token ?? ""}`,
        "X-Workspace-Slug": slug,
      };
      const [meResponse, membersResponse] = await Promise.all([
        fetch(`${apiBase}/api/me`, { headers }),
        fetch(`${apiBase}/api/workspaces/${id}/members`, { headers }),
      ]);
      return {
        meStatus: meResponse.status,
        me: (await meResponse.json()) as { id?: string; email?: string },
        membersStatus: membersResponse.status,
        members: (await membersResponse.json()) as Array<{
          user_id?: string;
          workspace_id?: string;
          role?: string;
          email?: string;
        }>,
      };
    },
    { apiBase: API_BASE, workspaceId, workspaceSlug },
  );
}

test.describe("T2 collection shared TableView", () => {
  test.describe.configure({ timeout: 120000 });

  let api: TestApiClient;
  let memberApi: TestApiClient | undefined;
  let foreignApi: TestApiClient | undefined;
  let memberContext: BrowserContext | undefined;
  let workspaceSlug: string;

  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    workspaceSlug = await loginAsDefault(page);
  });

  test.afterEach(async () => {
    await memberContext?.close();
    await memberApi?.cleanup();
    await foreignApi?.cleanup();
    await api?.cleanup();
  });

  test("creates isolated collections, persists scalar set/clear paths, and has zero task side effects", async ({
    page,
  }) => {
    await openCollections(page, workspaceSlug);
    await settlePage(page);
    const before = await api.readCollectionSideEffectCounts();
    const taskSideEffectRequests: string[] = [];
    const requestListener = (request: Request) => {
      if (isTaskSideEffectRequest(request)) {
        taskSideEffectRequests.push(`${request.method()} ${apiPath(request)}`);
      }
    };

    const collectionName = `E2E Scalars ${Date.now()}`;
    const createdCollection = await submitCollection(page, collectionName);
    api.trackCollection(createdCollection.collection.id);
    const isolated = await api.createCollection(
      `E2E Isolated ${Date.now()}`,
      COLLECTION_FIELDS,
    );
    const isolatedNote = isolated.fields.find((field) => field.name === "Note");
    if (!isolatedNote) throw new Error("Isolated collection fixture is incomplete");
    const isolatedRecord = await api.createCollectionRecord(
      isolated.collection.id,
      `E2E Isolated Record ${Date.now()}`,
      { [isolatedNote.id]: "second collection only" },
    );
    await openCollections(page, workspaceSlug);
    await settlePage(page);
    await expect(
      page.getByRole("link", { name: collectionName, exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: isolated.collection.name, exact: true }),
    ).toBeVisible();
    await page.getByRole("link", { name: collectionName, exact: true }).click();
    await openCollectionDetail(page, workspaceSlug, createdCollection.collection);
    await settlePage(page);
    // Observe only after the dashboard shell and collection source have
    // initialized. Read-only shell prefetches (notably GET /api/agents) are
    // not task side effects and are intentionally outside this assertion.
    page.on("request", requestListener);
    const recordTitle = `E2E Record ${Date.now()}`;
    const createdRecord = await addRecordInBrowser(page, recordTitle);
    const row = await rowByTitle(page, recordTitle);
    const noteField = createdCollection.fields.find((field) => field.name === "Note");
    const quantityField = createdCollection.fields.find(
      (field) => field.name === "Quantity",
    );
    const checkedField = createdCollection.fields.find(
      (field) => field.name === "Checked",
    );
    if (!noteField || !quantityField || !checkedField) {
      throw new Error("Scalar collection fixture is incomplete");
    }
    const noteTarget = {
      collectionId: createdCollection.collection.id,
      recordId: createdRecord.record.id,
      fieldId: noteField.id,
      op: "set" as const,
    };
    const noteClearTarget = { ...noteTarget, op: "clear" as const };
    const quantityTarget = {
      ...noteTarget,
      fieldId: quantityField.id,
      op: "set" as const,
    };
    const quantityClearTarget = { ...quantityTarget, op: "clear" as const };
    const checkedTarget = {
      ...noteTarget,
      fieldId: checkedField.id,
      op: "set" as const,
    };
    const checkedClearTarget = { ...checkedTarget, op: "clear" as const };

    await saveTextOrNumber(page, row, noteTarget, "Note", "browser text");
    let authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, noteField.id, "browser text");
    await clearTextOrNumber(page, row, noteClearTarget, "Note");
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, noteField.id);

    // An empty string is a legal set, distinct from clear/delete.
    await saveTextOrNumber(page, row, noteTarget, "Note", "");
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, noteField.id, "");
    await clearTextOrNumber(page, row, noteClearTarget, "Note");
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, noteField.id);

    await saveTextOrNumber(
      page,
      row,
      quantityTarget,
      "Quantity",
      "7",
      "spinbutton",
    );
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, quantityField.id, 7);
    await clearTextOrNumber(
      page,
      row,
      quantityClearTarget,
      "Quantity",
      "spinbutton",
    );
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, quantityField.id);

    await saveTextOrNumber(
      page,
      row,
      quantityTarget,
      "Quantity",
      "-1.5",
      "spinbutton",
      "enter",
    );
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, quantityField.id, -1.5);
    await clearTextOrNumber(
      page,
      row,
      quantityClearTarget,
      "Quantity",
      "spinbutton",
    );
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, quantityField.id);

    await saveTextOrNumber(
      page,
      row,
      quantityTarget,
      "Quantity",
      "0",
      "spinbutton",
    );
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, quantityField.id, 0);
    await clearTextOrNumber(
      page,
      row,
      quantityClearTarget,
      "Quantity",
      "spinbutton",
    );
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, quantityField.id);

    const checked = row.getByRole("checkbox", { name: "Checked" });
    await patchResponse(page, checkedTarget, () => checked.check(), 200);
    await expect(checked).toBeChecked();
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, checkedField.id, true);
    const checkboxCell = checked.locator("xpath=..").locator("xpath=..");
    await patchResponse(page, checkedClearTarget, () =>
      checkboxCell.getByRole("button", { name: "Clear", exact: true }).click(),
    200);
    await expect(checked).not.toBeChecked();
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, checkedField.id);

    // A false set must remain a present JSON field; it is not equivalent to
    // clear. Exercise that wire value through the real authenticated API.
    const falseSet = await api.updateCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
      authoritative.revision,
      { field_id: checkedField.id, op: "set", value: false },
    );
    expect(falseSet.response.status).toBe(200);
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertOwnField(authoritative, checkedField.id, false);
    const falseClear = await api.updateCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
      authoritative.revision,
      { field_id: checkedField.id, op: "clear" },
    );
    expect(falseClear.response.status).toBe(200);
    authoritative = await api.getCollectionRecord(
      createdCollection.collection.id,
      createdRecord.record.id,
    );
    assertClearedField(authoritative, checkedField.id);

    // Refresh is its own phase: it may issue dashboard GETs, but must not
    // invoke any task write/run-confirm endpoint.
    page.off("request", requestListener);
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

    expect(taskSideEffectRequests).toEqual([]);
    expect(createdRecord.record.collection_id).toBe(createdCollection.collection.id);
    expect(await api.readCollectionSideEffectCounts()).toEqual(before);

    // The second collection has a separate source identity and must not expose
    // the first collection's record or field value.
    await openCollectionDetail(page, workspaceSlug, isolated.collection);
    await settlePage(page);
    const isolatedRow = await rowByTitle(page, isolatedRecord.title);
    await expect(
      isolatedRow.getByRole("textbox", { name: "Note" }),
    ).toHaveValue("second collection only");
    const renderedTitles = await page
      .locator('tbody tr input[aria-label="Title"]')
      .evaluateAll((inputs) => inputs.map((input) => (input as HTMLInputElement).value));
    expect(renderedTitles).toContain(isolatedRecord.title);
    expect(renderedTitles).not.toContain(recordTitle);
    const isolatedAuthoritative = await api.getCollectionRecord(
      isolated.collection.id,
      isolatedRecord.id,
    );
    assertOwnField(isolatedAuthoritative, isolatedNote.id, "second collection only");
  });

  test("loads the 201st row and recovers a tail-row revision conflict on explicit Save retry", async ({
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

    const firstPageResponsePromise = page.waitForResponse(
      (response) =>
        response.status() === 200 &&
        isCollectionRecordQuery(response.request(), collection.collection.id) &&
        collectionQueryPage(response.request())?.limit === 200 &&
        collectionQueryPage(response.request())?.cursor == null,
    );
    await openCollectionDetail(page, workspaceSlug, collection.collection);
    const firstPageResponse = await firstPageResponsePromise;
    const firstPage = (await firstPageResponse.json()) as {
      records: TestCollectionRecord[];
      total: number;
      next_cursor: string | null;
    };
    expect(firstPage.records).toHaveLength(200);
    expect(firstPage.total).toBe(201);
    expect(firstPage.next_cursor).toBeTruthy();
    expect(new Set(firstPage.records.map((record) => record.id)).size).toBe(200);
    expect(firstPage.records.some((record) => record.id === tail.id)).toBe(false);
    await expect(await rowByTitle(page, "E2E Tail 000")).toBeVisible();

    await scrollTableToBottom(page);
    const loadMore = page.getByRole("button", { name: "Load more", exact: true });
    await expect(loadMore).toBeVisible();
    const pageResponsePromise = page.waitForResponse((response) => {
      if (
        response.request().method() !== "POST" ||
        !isCollectionRecordQuery(response.request(), collection.collection.id) ||
        response.status() !== 200
      ) {
        return false;
      }
      const body = collectionQueryPage(response.request());
      return (
        body?.limit === 200 &&
        body.cursor === firstPage.next_cursor
      );
    });
    await loadMore.click();
    const pageResponse = await pageResponsePromise;
    expect(pageResponse.status()).toBe(200);
    const lastPage = (await pageResponse.json()) as {
      records: TestCollectionRecord[];
      total: number;
      next_cursor: string | null;
    };
    expect(lastPage.records).toHaveLength(1);
    expect(lastPage.total).toBe(201);
    expect(lastPage.next_cursor).toBeNull();
    expect(firstPage.records.some((record) => record.id === lastPage.records[0]?.id)).toBe(
      false,
    );
    expect(lastPage.records[0]?.id).toBe(tail.id);
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
    const noteTarget = {
      collectionId: collection.collection.id,
      recordId: tail.id,
      fieldId: noteField.id,
      op: "set" as const,
    };
    const recoveryResponsePromise = page.waitForResponse(
      (response) =>
        response.status() === 200 &&
        response.request().method() === "GET" &&
        apiPath(response.request()) ===
          `/api/collections/${collection.collection.id}/records/${tail.id}`,
    );
    const firstSave = await patchResponse(
      page,
      noteTarget,
      () => noteForm.getByRole("button", { name: "Save", exact: true }).click(),
      409,
    );
    expect(firstSave.request?.expected_revision).toBe(tail.revision);
    const recoveryResponse = await recoveryResponsePromise;
    const recoveryRecord = (await recoveryResponse.json()) as {
      record: TestCollectionRecord;
    };
    expect(recoveryRecord.record.id).toBe(tail.id);
    expect(recoveryRecord.record.revision).toBe(tail.revision + 1);
    const conflictedRow = await rowByTitle(page, tail.title);
    await expect(
      conflictedRow.getByRole("textbox", { name: "Note" }),
    ).toHaveValue("browser stale value");
    await expect(conflictedRow.getByRole("button", { name: "Save", exact: true })).toBeVisible();

    const secondSave = await patchResponse(
      page,
      noteTarget,
      () => conflictedRow.getByRole("button", { name: "Save", exact: true }).click(),
      200,
    );
    expect(secondSave.request?.expected_revision).toBe(tail.revision + 1);
    await expect(
      (await rowByTitle(page, tail.title)).getByRole("textbox", { name: "Note" }),
    ).toHaveValue("browser stale value");
    const authoritative = await api.getCollectionRecord(
      collection.collection.id,
      tail.id,
    );
    expect(authoritative.revision).toBe(tail.revision + 2);
    assertOwnField(authoritative, noteField.id, "browser stale value");
  });

  test("enforces member collection permissions while retaining member record access", async ({
    page,
    browser,
    baseURL,
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
    foreignApi = new TestApiClient();
    await foreignApi.login(
      `collection-foreign-owner-${Date.now()}@multica.ai`,
      "Foreign Collection Owner",
    );
    const foreignWorkspace = await foreignApi.ensureWorkspace(
      `E2E Foreign Workspace ${Date.now()}`,
      `e2e-foreign-${Date.now()}`,
    );
    await foreignApi.markUserOnboarded();
    const foreignCollection = await foreignApi.createCollection(
      `E2E Foreign Collection ${Date.now()}`,
      COLLECTION_FIELDS,
    );
    const memberEmail = `collection-member-${Date.now()}@multica.ai`;
    await api.createWorkspaceMember(memberEmail, "Collection Member");
    memberApi = new TestApiClient();
    await memberApi.login(memberEmail, "Collection Member");
    memberApi.setWorkspace(api.getWorkspace());
    await memberApi.markUserOnboarded();
    const memberToken = memberApi.getToken();
    if (!memberToken) throw new Error("Member login did not return a token");
    // Do not add a member token to the owner page. helpers.loginAsDefault has
    // already registered the owner init script there, and Playwright does not
    // guarantee ordering for multiple init scripts. A fresh context gives the
    // permission assertion one deterministic member-only browser session.
    memberContext = await browser.newContext({
      baseURL: baseURL ?? "http://localhost:3000",
    });
    await memberContext.addInitScript((token) => {
      localStorage.setItem("multica_token", token);
      localStorage.setItem("multica:chat:isOpen", "false");
    }, memberToken);
    const memberPage = await memberContext.newPage();

    await memberPage.goto(`/${workspaceSlug}/collections`, {
      waitUntil: "domcontentloaded",
    });
    await waitForPageText(memberPage, collection.collection.name);
    const identity = await readMemberIdentity(
      memberPage,
      api.getWorkspace().id,
      workspaceSlug,
    );
    expect(identity.meStatus).toBe(200);
    expect(identity.membersStatus).toBe(200);
    expect(identity.me.email).toBe(memberEmail);
    const membership = identity.members.find(
      (entry) => entry.user_id === identity.me.id,
    );
    expect(membership).toMatchObject({
      user_id: identity.me.id,
      workspace_id: api.getWorkspace().id,
      role: "member",
      email: memberEmail,
    });
    await expect(
      memberPage.getByRole("textbox", { name: "Collection name" }),
    ).toHaveCount(0);
    await expect(
      memberPage.getByRole("button", { name: "Create collection", exact: true }),
    ).toHaveCount(0);
    await memberPage
      .getByRole("link", { name: collection.collection.name })
      .click();
    await openCollectionDetail(memberPage, workspaceSlug, collection.collection);
    const existingRow = await rowByTitle(memberPage, existingRecord.title);

    const memberRecordTitle = `Member record ${Date.now()}`;
    const memberRecord = await addRecordInBrowser(memberPage, memberRecordTitle);
    await expect(await rowByTitle(memberPage, memberRecordTitle)).toBeVisible();
    const note = existingRow.getByRole("textbox", { name: "Note" });
    await note.fill("member edit");
    const noteField = collection.fields.find((field) => field.name === "Note");
    if (!noteField) throw new Error("Permission collection fixture is incomplete");
    await patchResponse(
      memberPage,
      {
        collectionId: collection.collection.id,
        recordId: existingRecord.id,
        fieldId: noteField.id,
        op: "set",
      },
      () =>
        note
          .locator("xpath=ancestor::form")
          .getByRole("button", { name: "Save", exact: true })
          .click(),
      200,
    );
    await expect(note).toHaveValue("member edit");

    const deniedCreate = await memberPage.evaluate(
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

    // A member session must not cross the workspace boundary even when it
    // knows a foreign collection UUID. Test both the foreign workspace route
    // and the current-workspace resource lookup.
    const deniedForeignWorkspace = await memberPage.evaluate(
      async ({ apiBase, slug, collectionId }) => {
        const token = localStorage.getItem("multica_token");
        const response = await fetch(
          `${apiBase}/api/collections/${collectionId}/records/query`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${token ?? ""}`,
              "X-Workspace-Slug": slug,
            },
            body: JSON.stringify({ page: { limit: 200, cursor: null } }),
          },
        );
        return { status: response.status, body: await response.text() };
      },
      {
        apiBase: API_BASE,
        slug: foreignWorkspace.slug,
        collectionId: foreignCollection.collection.id,
      },
    );
    expect(deniedForeignWorkspace.status).toBe(403);

    const deniedForeignResource = await memberPage.evaluate(
      async ({ apiBase, slug, collectionId }) => {
        const token = localStorage.getItem("multica_token");
        const response = await fetch(
          `${apiBase}/api/collections/${collectionId}`,
          {
            headers: {
              Authorization: `Bearer ${token ?? ""}`,
              "X-Workspace-Slug": slug,
            },
          },
        );
        return { status: response.status, body: await response.text() };
      },
      {
        apiBase: API_BASE,
        slug: workspaceSlug,
        collectionId: foreignCollection.collection.id,
      },
    );
    expect(deniedForeignResource.status).toBe(404);
  });

  test("creates 1,000 records through the API without task side effects", async () => {
    test.setTimeout(180000);
    const collection = await api.createCollection(
      `E2E API Bulk ${Date.now()}`,
      COLLECTION_FIELDS,
    );
    const before = await api.readCollectionSideEffectCounts();
    const records = await api.createCollectionRecords(
      collection.collection.id,
      `E2E API Record ${Date.now()}`,
      1000,
    );
    expect(records).toHaveLength(1000);
    expect(new Set(records.map((record) => record.id)).size).toBe(1000);
    expect(records.every((record) => record.collection_id === collection.collection.id)).toBe(
      true,
    );
    const after = await api.readCollectionSideEffectCounts();
    expect(after).toEqual(before);
  });
});
