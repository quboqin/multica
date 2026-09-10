import { expect, test, type Page, type Route } from "@playwright/test";

type AcceptanceContext = {
  apiBase: string;
  token: string;
  workspaceSlug: string;
};

type CreativeCandidate = {
  id: string;
  analysis_status?: string;
};

type SourceAnalysis = {
  id: string;
  candidate_id: string;
  analysis_version: number;
  status: string;
};

type FeedbackEvent = Record<string, unknown> & {
  id: string;
  subject_type: string;
  subject_id: string;
  event_type: string;
  decision: string;
};

function acceptanceContext(): AcceptanceContext {
  const token = process.env.MULTICA_TEST_TOKEN ?? process.env.CREATIVE_ACCEPTANCE_TOKEN ?? "";
  if (!token) throw new Error("MULTICA_TEST_TOKEN or CREATIVE_ACCEPTANCE_TOKEN is required for read-only acceptance");
  return {
    apiBase: (process.env.CREATIVE_ACCEPTANCE_API_URL || process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000").replace(/\/+$/, ""),
    token,
    workspaceSlug: process.env.CREATIVE_ACCEPTANCE_WORKSPACE_SLUG || "ad-creative-direct-pilot",
  };
}

async function apiJSON<T>(context: AcceptanceContext, path: string): Promise<T> {
  const requestURL = `${context.apiBase}${path}`;
  const response = await fetch(requestURL, {
    headers: {
      Authorization: `Bearer ${context.token}`,
      "X-Workspace-Slug": context.workspaceSlug,
    },
  });
  const body = await response.text();
  if (!response.ok) throw new Error(`${requestURL} returned HTTP ${response.status}: ${body.slice(0, 500)}`);
  return JSON.parse(body) as T;
}

function syntheticFeedback(payload: Record<string, unknown>, id: string): FeedbackEvent {
  return {
    id,
    idempotency_key: typeof payload.idempotency_key === "string" ? payload.idempotency_key : "",
    workspace_id: "browser-only",
    actor_type: "member",
    actor_id: "browser-only",
    issue_id: typeof payload.issue_id === "string" ? payload.issue_id : "",
    subject_type: typeof payload.subject_type === "string" ? payload.subject_type : "",
    subject_id: typeof payload.subject_id === "string" ? payload.subject_id : "",
    event_type: typeof payload.event_type === "string" ? payload.event_type : "",
    decision: typeof payload.decision === "string" ? payload.decision : "",
    reason_codes: Array.isArray(payload.reason_codes) ? payload.reason_codes : [],
    comment: typeof payload.comment === "string" ? payload.comment : "",
    context_snapshot: payload.context_snapshot && typeof payload.context_snapshot === "object" ? payload.context_snapshot : {},
    undo_of_id: "",
    created_at: new Date().toISOString(),
  };
}

async function installReadOnlyFeedbackSimulation(page: Page, targetCandidateId: string) {
  const syntheticEvents: FeedbackEvent[] = [];
  let interceptedPosts = 0;
  await page.route("**/api/creative-feedback-events*", async (route: Route) => {
    const request = route.request();
    if (request.method() === "POST") {
      interceptedPosts += 1;
      const payload = request.postDataJSON() as Record<string, unknown>;
      const event = syntheticFeedback(payload, `browser-only-${interceptedPosts}`);
      syntheticEvents.push(event);
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(event) });
      return;
    }
    if (request.method() === "GET") {
      const response = await route.fetch();
      const body = await response.json() as { events?: FeedbackEvent[] };
      const realEvents = (body.events ?? []).filter((event) => event.subject_id !== targetCandidateId);
      await route.fulfill({ response, contentType: "application/json", body: JSON.stringify({ ...body, events: [...realEvents, ...syntheticEvents] }) });
      return;
    }
    await route.fallback();
  });
  return { syntheticEvents, interceptedPostCount: () => interceptedPosts };
}

test("creative material selection form is usable without writing business data", async ({ page }, testInfo) => {
  test.setTimeout(120_000);
  const context = acceptanceContext();
  const [library, analysisResponse, feedbackBefore] = await Promise.all([
    apiJSON<{ candidates: CreativeCandidate[] }>(context, "/api/creative/materials"),
    apiJSON<{ analyses: SourceAnalysis[] }>(context, "/api/creative/source-analyses"),
    apiJSON<{ events: FeedbackEvent[] }>(context, "/api/creative-feedback-events?subject_type=candidate"),
  ]);

  const completedByCandidate = new Map<string, SourceAnalysis>();
  for (const analysis of analysisResponse.analyses) {
    if (analysis.status !== "completed") continue;
    const current = completedByCandidate.get(analysis.candidate_id);
    if (!current || analysis.analysis_version > current.analysis_version) completedByCandidate.set(analysis.candidate_id, analysis);
  }
  const candidatePrefix = process.env.CREATIVE_ACCEPTANCE_CANDIDATE_ID || "00c0b54c";
  const candidate = library.candidates.find((item) => item.id.startsWith(candidatePrefix));
  if (!candidate) throw new Error(`Candidate ${candidatePrefix} was not returned by ${context.workspaceSlug}`);
  if (candidate.analysis_status === "completed" || !completedByCandidate.has(candidate.id)) {
    throw new Error(`Candidate ${candidate.id} no longer reproduces completed Source Analysis + non-completed candidate status`);
  }
  const analysis = completedByCandidate.get(candidate.id)!;
  const targetFeedbackBefore = feedbackBefore.events.filter((event) => event.subject_id === candidate.id).map((event) => event.id).sort();

  const simulation = await installReadOnlyFeedbackSimulation(page, candidate.id);
  const forbiddenWrites: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "POST") return;
    const path = new URL(request.url()).pathname;
    if (path === "/api/issues" || path === "/api/creative/orders" || path === "/api/creative/direct-edits") forbiddenWrites.push(`${request.method()} ${path}`);
  });
  await page.addInitScript((token) => {
    localStorage.setItem("multica_token", token);
    localStorage.setItem("multica:chat:isOpen", "false");
  }, context.token);

  await page.goto(`/${context.workspaceSlug}/creative`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "创意工作台" })).toBeVisible({ timeout: 60_000 });
  await page.getByRole("tab", { name: "素材库" }).click();
  await expect(page.getByRole("heading", { name: "工作区素材库" })).toBeVisible({ timeout: 60_000 });
  const tile = page.locator(`[data-testid="creative-material-tile"][data-candidate-id="${candidate.id}"]`);
  await expect(tile).toBeVisible({ timeout: 60_000 });
  await expect(tile.getByText(`素材 ID ${candidate.id.slice(0, 8)}`, { exact: true })).toBeVisible();
  await expect(tile.getByText("等待分析", { exact: true })).toHaveCount(0);
  await expect(tile.getByText("分析中", { exact: true })).toHaveCount(0);
  const selectButton = tile.getByRole("button", { name: "选择素材" });
  await expect(selectButton).toBeEnabled();

  await selectButton.click();
  const draft = page.getByRole("region", { name: "配置创意订单" });
  await expect(draft).toBeVisible({ timeout: 30_000 });
  await expect(draft.getByLabel("市场规则")).toBeVisible();
  await draft.getByText(/高级设置/).click();
  await expect(draft.getByLabel("生成服务")).toBeVisible();
  const item = draft.locator("article").filter({ hasText: `素材 ID ${candidate.id.slice(0, 8)}` });
  await expect(item).toBeVisible();
  await expect(item.getByLabel("选用文案")).toBeVisible();
  expect(forbiddenWrites).toEqual([]);

  const feedbackAfter = await apiJSON<{ events: FeedbackEvent[] }>(context, "/api/creative-feedback-events?subject_type=candidate");
  const targetFeedbackAfter = feedbackAfter.events.filter((event) => event.subject_id === candidate.id).map((event) => event.id).sort();
  expect(targetFeedbackAfter).toEqual(targetFeedbackBefore);
  expect(feedbackAfter.events.some((event) => event.id.startsWith("browser-only-"))).toBe(false);
  expect(simulation.interceptedPostCount()).toBeGreaterThan(0);

  await testInfo.attach("creative-readonly-selection.png", { body: await page.screenshot({ fullPage: true }), contentType: "image/png" });
  console.log(JSON.stringify({
    candidateId: candidate.id,
    candidateAnalysisStatus: candidate.analysis_status,
    completedAnalysisId: analysis.id,
    completedAnalysisVersion: analysis.analysis_version,
    interceptedFeedbackPosts: simulation.interceptedPostCount(),
    persistedTargetFeedbackEvents: targetFeedbackAfter.length,
    labelsVerified: ["市场规则", "生成服务", "选用文案"],
    pageURL: page.url(),
  }));
});
