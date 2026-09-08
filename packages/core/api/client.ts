import type {
  Issue,
  CreateIssueRequest,
  UpdateIssueRequest,
  GroupedIssuesResponse,
  ListIssuesResponse,
  SearchIssuesResponse,
  SearchProjectsResponse,
  UpdateMeRequest,
  CreateMemberRequest,
  UpdateMemberRequest,
  ListIssuesParams,
  ListGroupedIssuesParams,
  Agent,
  AgentTaskFanoutResponse,
  CreateAgentRequest,
  AgentTemplate,
  AgentTemplateSummary,
  CreateAgentFromTemplateRequest,
  CreateAgentFromTemplateResponse,
  UpdateAgentRequest,
  AgentEnvResponse,
  UpdateAgentEnvRequest,
  AgentTask,
  AgentActivityBucket,
  AgentRunCount,
  AgentRuntime,
  RuntimeProfile,
  CreateRuntimeProfileRequest,
  UpdateRuntimeProfileRequest,
  InboxItem,
  IssueSubscriber,
  Comment,
  CommentTriggerPreview,
  Reaction,
  IssueReaction,
  Workspace,
  WorkspaceRepo,
  CreativeFactoryInitializationRequest,
  WorkspaceCapability,
  WorkspaceCapabilitiesResponse,
  MemberWithUser,
  User,
  Skill,
  SkillSummary,
  CreateSkillRequest,
  UpdateSkillRequest,
  SetAgentSkillsRequest,
  PersonalAccessToken,
  CreatePersonalAccessTokenRequest,
  CreatePersonalAccessTokenResponse,
  RuntimeUsage,
  IssueUsageSummary,
  RuntimeHourlyActivity,
  RuntimeUsageByAgent,
  RuntimeUsageByHour,
  DashboardUsageDaily,
  DashboardUsageByAgent,
  DashboardUsageByUser,
  DashboardUsageByUserDaily,
  DashboardAgentRunTime,
  DashboardRunTimeDaily,
  RuntimeUpdate,
  RuntimeModelListRequest,
  RuntimeLocalSkillListRequest,
  CreateRuntimeLocalSkillImportRequest,
  RuntimeLocalSkillImportRequest,
  TimelineEntry,
  AssigneeFrequencyEntry,
  TaskMessagePayload,
  Attachment,
  Favorite,
  FavoriteCategory,
  FavoriteType,
  ChatSession,
  ChatMessage,
  ChatMessagesPage,
  ChatPendingTask,
  PendingChatTasksResponse,
  SendChatMessageResponse,
  CancelTaskResponse,
  Project,
  Milestone,
  CreateMilestoneRequest,
  UpdateMilestoneRequest,
  ListMilestonesResponse,
  KpiMetric,
  CreateKpiMetricRequest,
  UpdateKpiMetricRequest,
  ListKpiMetricsResponse,
  CreateProjectRequest,
  UpdateProjectRequest,
  ListProjectsResponse,
  ProjectResource,
  CreateProjectResourceRequest,
  UpdateProjectResourceRequest,
  ListProjectResourcesResponse,
  Label,
  LabelResourceType,
  CreateLabelRequest,
  UpdateLabelRequest,
  ListLabelsResponse,
  IssueLabelsResponse,
  ProjectLabelsResponse,
  AgentLabelsResponse,
  PinnedItem,
  CreatePinRequest,
  PinnedItemType,
  ReorderPinsRequest,
  Invitation,
  Autopilot,
  AutopilotTrigger,
  AutopilotRun,
  CreateAutopilotRequest,
  UpdateAutopilotRequest,
  CreateAutopilotTriggerRequest,
  UpdateAutopilotTriggerRequest,
  ListAutopilotsResponse,
  GetAutopilotResponse,
  ListAutopilotRunsResponse,
  ListWebhookDeliveriesResponse,
  WebhookDelivery,
  NotificationPreferenceResponse,
  NotificationPreferences,
  GitHubPullRequest,
  ListGitHubInstallationsResponse,
  GitHubConnectResponse,
  ListLarkInstallationsResponse,
  BeginLarkInstallResponse,
  LarkInstallStatusResponse,
  RedeemLarkBindingTokenResponse,
  ComposioToolkit,
  ComposioConnection,
  ComposioConnectInitResponse,
  Squad,
  SquadMember,
  SquadMemberStatusListResponse,
  BillingBalance,
  BillingTransactionsPage,
  BillingBatchesPage,
  BillingTopupsPage,
  BillingPriceTier,
  CreateBillingCheckoutSessionRequest,
  CreateBillingCheckoutSessionResponse,
  BillingCheckoutSessionStatus,
  CreateBillingPortalSessionResponse,
  CreativeImportSummary,
	CreativeAdjustmentRequest,
	CreateCreativeAdjustmentRequest,
	CreateCreativeFeedbackRequest,
	CreateCreativeFeedbackResponse,
	QueueCreativeOrderAdjustmentRequest,
	QueueCreativeOrderAdjustmentResponse,
	CreativeFeedbackEventListResponse,
	CreativeFeedbackDashboard,
	CreativeFeedbackMetrics,
	CreativeOrderListResponse,
	CreativeOrder,
	CreativeOrderWorkflowRetryResponse,
	CreativeOrderPrimeComposeResponse,
	CreativeOrderQCRetryResponse,
	CreativeOrderItem,
	CreativeOrderQCFinalizeResponse,
	AdoptCreativeOrderVariantRequest,
	CreateCreativeOrderRequest,
	CreateCreativeDirectEditRequest,
	CreativeDirectEditResponse,
	RegisterCreativeDeliveriesRequest,
	RegisterCreativeDeliveriesResponse,
  CreativeIssueContext,
  CreativeIssueItem,
  CreativeMaterialLibraryResponse,
  CreativeMaterialLibraryQuery,
  CreativeMaterialImportResult,
  CreativeMaterialsResponse,
  CreativeResource,
  CreativeResourceFile,
  CreativeResourceFileListResponse,
  CreativeResourceKind,
  CreativeResourceListResponse,
  CreateCreativeResourceRequest,
  CredentialCrawlResult,
  CredentialProfile,
  AddCredentialProfileManagerRequest,
  ImportCreativeMaterialsRequest,
  ImportCreativeMaterialLibraryRequest,
  ListCredentialConnectorsResponse,
  ListCredentialProfilesResponse,
  RunCredentialCrawlRequest,
  StartCredentialLoginSessionRequest,
  StartCredentialLoginSessionResponse,
  PutCreativeIssueContextRequest,
  UpdateCreativeResourceRequest,
  UpdateCreativeMaterialCandidateRequest,
  CreatePreviewSessionRequest,
  PreviewSession,
  PreviewSessionListResponse,
} from "../types";
import type { OnboardingCompletionPath } from "../onboarding/types";
import type {
  CloudRuntimeNode,
  CreateCloudRuntimeNodeRequest,
  ListCloudRuntimeNodesParams,
} from "../runtimes/cloud-runtime";
import { type Logger, noopLogger } from "../logger";
import { createRequestId } from "../utils";
import { getCurrentSlug } from "../platform/workspace-storage";
import { parseWithFallback } from "./schema";
import {
  AgentTemplateSchema,
  AgentTaskFanoutResponseSchema,
  AgentTemplateSummaryListSchema,
  FavoriteCategoryListSchema,
  FavoriteCategoryResponseSchema,
  AttachmentResponseSchema,
  FavoriteListSchema,
  FavoriteResponseSchema,
  CancelTaskResponseSchema,
  ChatMessageListSchema,
  ChatMessagesPageSchema,
  ChildIssuesResponseSchema,
  CommentsListSchema,
  CommentTriggerPreviewSchema,
  CloudRuntimeNodeListSchema,
  CloudRuntimeNodeSchema,
  CreateAgentFromTemplateResponseSchema,
  DashboardAgentRunTimeListSchema,
  DashboardRunTimeDailyListSchema,
  DashboardUsageByAgentListSchema,
  DashboardUsageByUserListSchema,
  DashboardUsageByUserDailyListSchema,
  DashboardUsageDailyListSchema,
  EMPTY_AGENT_TEMPLATE_DETAIL,
  EMPTY_AGENT_TASK_FANOUT_RESPONSE,
  EMPTY_AGENT_TEMPLATE_SUMMARY_LIST,
  EMPTY_APP_CONFIG,
  EMPTY_WORKSPACE_CAPABILITIES,
  EMPTY_WORKSPACE_CAPABILITY,
  EMPTY_ATTACHMENT,
  EMPTY_FAVORITE,
  EMPTY_FAVORITE_CATEGORIES,
  EMPTY_FAVORITE_CATEGORY,
  EMPTY_FAVORITES,
  EMPTY_CLOUD_RUNTIME_NODE,
  EMPTY_CLOUD_RUNTIME_NODE_LIST,
  EMPTY_CREATE_AGENT_FROM_TEMPLATE_RESPONSE,
  EMPTY_GROUPED_ISSUES_RESPONSE,
  EMPTY_LIST_ISSUES_RESPONSE,
  EMPTY_SQUAD,
  EMPTY_SQUAD_LIST,
  EMPTY_SQUAD_MEMBER_STATUS_LIST,
  EMPTY_TIMELINE_ENTRIES,
  EMPTY_USER,
  EMPTY_LIST_WEBHOOK_DELIVERIES_RESPONSE,
  EMPTY_WEBHOOK_DELIVERY,
  AppConfigSchema,
  WorkspaceCapabilitiesSchema,
  WorkspaceCapabilitySchema,
  type AppConfigResponse,
  GroupedIssuesResponseSchema,
  ListAutopilotsResponseSchema,
  EMPTY_LIST_AUTOPILOTS_RESPONSE,
  ListIssuesResponseSchema,
  ListWebhookDeliveriesResponseSchema,
  RuntimeHourlyActivityListSchema,
  RuntimeUsageByAgentListSchema,
  RuntimeUsageByHourListSchema,
  RuntimeUsageListSchema,
  SquadSchema,
  SquadListSchema,
  SquadMemberStatusListResponseSchema,
  SubscribersListSchema,
  TimelineEntriesSchema,
  UserSchema,
  WebhookDeliveryResponseSchema,
  BillingBalanceSchema,
  BillingTransactionsPageSchema,
  BillingBatchesPageSchema,
  BillingTopupsPageSchema,
  BillingPriceTierListSchema,
  CreateBillingCheckoutSessionResponseSchema,
  BillingCheckoutSessionStatusSchema,
  CreateBillingPortalSessionResponseSchema,
  EMPTY_BILLING_BALANCE,
  EMPTY_BILLING_TRANSACTIONS_PAGE,
  EMPTY_BILLING_BATCHES_PAGE,
  EMPTY_BILLING_TOPUPS_PAGE,
  EMPTY_BILLING_PRICE_TIER_LIST,
  EMPTY_CREATE_BILLING_CHECKOUT_SESSION_RESPONSE,
  EMPTY_BILLING_CHECKOUT_SESSION_STATUS,
  EMPTY_CREATE_BILLING_PORTAL_SESSION_RESPONSE,
  EMPTY_CANCEL_TASK_RESPONSE,
  EMPTY_CHAT_MESSAGE_LIST,
  EMPTY_CHAT_MESSAGES_PAGE,
  EMPTY_CREATIVE_IMPORT_SUMMARY,
  EMPTY_CREATIVE_ISSUE_CONTEXT,
  EMPTY_CREATIVE_ISSUE_ITEM,
  EMPTY_CREATIVE_MATERIAL_LIBRARY,
  EMPTY_CREATIVE_MATERIAL_IMPORT_RESULT,
  EMPTY_CREATIVE_MATERIALS_RESPONSE,
  EMPTY_CREATIVE_RESOURCE,
  EMPTY_CREATIVE_RESOURCE_FILE,
  EMPTY_CREATIVE_RESOURCE_FILE_LIST,
  EMPTY_CREATIVE_RESOURCE_LIST,
  EMPTY_CREDENTIAL_CRAWL_RESULT,
  EMPTY_LIST_CREDENTIAL_CONNECTORS_RESPONSE,
  EMPTY_LIST_CREDENTIAL_PROFILES_RESPONSE,
  EMPTY_START_CREDENTIAL_LOGIN_SESSION_RESPONSE,
  EMPTY_LARK_LOGIN_RESPONSE,
  EMPTY_LARK_LOGIN_STATE_RESPONSE,
  EMPTY_LOGIN_RESPONSE,
  CreativeImportSummarySchema,
  CreativeIssueContextSchema,
  CreativeIssueItemSchema,
	CreativeAdjustmentRequestSchema,
	EMPTY_CREATIVE_ADJUSTMENT_REQUEST,
	CreateCreativeFeedbackResponseSchema,
	QueueCreativeOrderAdjustmentResponseSchema,
	EMPTY_CREATIVE_FEEDBACK_RESPONSE,
	CreativeFeedbackEventListResponseSchema,
	EMPTY_CREATIVE_FEEDBACK_EVENT_LIST_RESPONSE,
	CreativeFeedbackMetricsSchema,
	EMPTY_CREATIVE_FEEDBACK_METRICS,
	CreativeFeedbackDashboardSchema,
	EMPTY_CREATIVE_FEEDBACK_DASHBOARD,
	CreativeOrderListResponseSchema,
	CreativeOrderSchema,
	CreativeOrderWorkflowRetryResponseSchema,
	CreativeOrderPrimeComposeResponseSchema,
	CreativeOrderQCRetryResponseSchema,
	CreativeOrderItemSchema,
	EMPTY_CREATIVE_ORDER_ITEM,
	CreativeOrderQCFinalizeResponseSchema,
	CreativeDirectEditResponseSchema,
	EMPTY_CREATIVE_DIRECT_EDIT_RESPONSE,
	EMPTY_CREATIVE_ORDER_QC_FINALIZE_RESPONSE,
	CreativeSourceAnalysisListResponseSchema,
	EMPTY_CREATIVE_SOURCE_ANALYSIS_LIST_RESPONSE,
	CreativePreAdaptationRetryResponseSchema,
	EMPTY_CREATIVE_PRE_ADAPTATION_RETRY_RESPONSE,
	EMPTY_CREATIVE_ORDER_LIST_RESPONSE,
	RegisterCreativeDeliveriesResponseSchema,
  CreativeMaterialLibrarySchema,
  CreativeMaterialImportResultSchema,
  CreativeMaterialsResponseSchema,
  CreativeResourceListSchema,
  CreativeResourceFileListSchema,
  CreativeResourceFileSchema,
  CreativeResourceSchema,
  CredentialCrawlResultSchema,
  CredentialProfileSchema,
  ListCredentialConnectorsResponseSchema,
  ListCredentialProfilesResponseSchema,
  LarkLoginResponseSchema,
  LarkLoginStateResponseSchema,
  LoginResponseSchema,
  StartCredentialLoginSessionResponseSchema,
  EMPTY_PREVIEW_SESSION,
  EMPTY_PREVIEW_SESSION_LIST_RESPONSE,
  PreviewSessionListResponseSchema,
  PreviewSessionSchema,
} from "./schemas";

/** Identifies the calling client to the server.
 *  Sent on every HTTP request as X-Client-Platform / X-Client-Version /
 *  X-Client-OS so the backend can log, gate, or split metrics by client.
 *  See server/internal/middleware/client.go for the receiving end. */
export interface ApiClientIdentity {
  /** Logical client kind. Server expects: "web" | "desktop" | "cli" | "daemon". */
  platform?: string;
  /** Client/app version string (e.g. "0.1.0", git tag, commit). */
  version?: string;
  /** Operating system the client is running on: "macos" | "windows" | "linux". */
  os?: string;
}

export interface ApiClientOptions {
  logger?: Logger;
  onUnauthorized?: () => void;
  /** Identifies the client to the server. Sent as X-Client-* headers. */
  identity?: ApiClientIdentity;
}

export interface LoginResponse {
  token: string;
  user: User;
}

export interface LarkLoginStateResponse {
  state: string;
  authorize_url?: string;
}

export interface LarkLoginResponse extends LoginResponse {
  workspace_id?: string;
  workspace_slug?: string;
  next?: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly statusText: string;
  // Raw decoded JSON body (when the server returned one). Carries structured
  // error fields like `code` so callers can branch on machine-readable
  // identifiers instead of pattern-matching the human-readable message.
  readonly body?: unknown;

  constructor(message: string, status: number, statusText: string, body?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}

function unsupportedCreativeBriefExtensionField(
  error: unknown,
): string | null {
  if (!(error instanceof ApiError) || error.status !== 400) return null;
  return error.message.match(/unknown field "(user_direction|app_ui_replacement_required|selected_app_ui_references)"/)?.[1] ?? null;
}

function creativeBriefHasExtensionContent(brief: CreativeIssueItem["creative_brief"]): boolean {
  return Boolean(brief.user_direction.trim())
    || brief.app_ui_replacement_required
    || brief.selected_app_ui_references.length > 0;
}

// Thrown by getAttachmentTextContent when the server refuses to inline a
// file because it exceeds the 2 MB cap. UI maps to a "too large, please
// download" affordance with the Download CTA still available.
export class PreviewTooLargeError extends Error {
  constructor() {
    super("attachment too large for inline preview");
    this.name = "PreviewTooLargeError";
  }
}

// Thrown by getAttachmentTextContent when the server's text whitelist
// rejects the content type. Normally the client's isPreviewable() guard
// catches this earlier, but the two whitelists can drift — surfacing the
// 415 as a typed error makes the drift visible.
export class PreviewUnsupportedError extends Error {
  constructor() {
    super("attachment type not supported for inline preview");
    this.name = "PreviewUnsupportedError";
  }
}

export class ApiClient {
  private baseUrl: string;
  private token: string | null = null;
  private logger: Logger;
  private options: ApiClientOptions;

  constructor(baseUrl: string, options?: ApiClientOptions) {
    this.baseUrl = baseUrl;
    this.options = options ?? {};
    this.logger = options?.logger ?? noopLogger;
  }

  getBaseUrl(): string {
    return this.baseUrl;
  }

  setToken(token: string | null) {
    this.token = token;
  }

  private readCsrfToken(): string | null {
    if (typeof document === "undefined") return null;
    const match = document.cookie
      .split("; ")
      .find((c) => c.startsWith("multica_csrf="));
    return match ? match.split("=")[1] ?? null : null;
  }

  private authHeaders(): Record<string, string> {
    const headers: Record<string, string> = {};
    if (this.token) headers["Authorization"] = `Bearer ${this.token}`;
    const slug = getCurrentSlug();
    if (slug) headers["X-Workspace-Slug"] = slug;
    const csrf = this.readCsrfToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
    const id = this.options.identity;
    if (id?.platform) headers["X-Client-Platform"] = id.platform;
    if (id?.version) headers["X-Client-Version"] = id.version;
    if (id?.os) headers["X-Client-OS"] = id.os;
    return headers;
  }

  private handleUnauthorized() {
    this.token = null;
    // Workspace id is owned by the URL-driven workspace-storage singleton
    // (set by [workspaceSlug]/layout.tsx). On 401, the auth flow navigates
    // to /login which leaves the workspace route, and the next workspace
    // entry will overwrite the id. No clear needed here.
    this.options.onUnauthorized?.();
  }

  private async parseErrorMessage(res: Response, fallback: string): Promise<string> {
    try {
      const data = await res.json() as { error?: string };
      if (typeof data.error === "string" && data.error) return data.error;
    } catch {
      // Ignore non-JSON error bodies.
    }
    return fallback;
  }

  // Reads the response body once for both human-readable error message and
  // structured fields. The Response stream can only be consumed once, so
  // both pieces have to come from a single read.
  private async parseErrorBody(res: Response, fallback: string): Promise<{ message: string; body: unknown }> {
    try {
      const data = await res.json() as { error?: string };
      const message = typeof data.error === "string" && data.error ? data.error : fallback;
      return { message, body: data };
    } catch {
      return { message: fallback, body: undefined };
    }
  }

  // Sends the request with the standard headers (auth, CSRF, request id,
  // client identity) and runs the shared error path (401 → handleUnauthorized,
  // structured ApiError, status-aware log level). Returns the raw Response so
  // callers can decide how to decode the body — JSON for the typed `fetch<T>`
  // path, plain text for the attachment-preview proxy, etc.
  private async fetchRaw(
    path: string,
    init?: RequestInit & { extraHeaders?: Record<string, string>; suppressErrorLog?: boolean },
  ): Promise<Response> {
    const rid = createRequestId();
    const start = Date.now();
    const method = init?.method ?? "GET";
    const { extraHeaders, suppressErrorLog, ...requestInit } = init ?? {};

    const headers: Record<string, string> = {
      "X-Request-ID": rid,
      ...this.authHeaders(),
      ...(extraHeaders ?? {}),
      ...((requestInit.headers as Record<string, string>) ?? {}),
    };

    this.logger.info(`→ ${method} ${path}`, { rid });

    const res = await fetch(`${this.baseUrl}${path}`, {
      ...requestInit,
      headers,
      credentials: "include",
    });

    if (!res.ok) {
      if (res.status === 401) this.handleUnauthorized();
      const { message, body } = await this.parseErrorBody(res, `API error: ${res.status} ${res.statusText}`);
      if (!suppressErrorLog) {
        const logLevel = res.status === 401 || res.status === 404 ? "warn" : "error";
        this.logger[logLevel](`← ${res.status} ${path}`, { rid, duration: `${Date.now() - start}ms`, error: message });
      }
      throw new ApiError(message, res.status, res.statusText, body);
    }

    this.logger.info(`← ${res.status} ${path}`, { rid, duration: `${Date.now() - start}ms` });
    return res;
  }

  private async fetch<T>(path: string, init?: RequestInit & { suppressErrorLog?: boolean }): Promise<T> {
    const res = await this.fetchRaw(path, {
      ...init,
      extraHeaders: { "Content-Type": "application/json" },
    });
    // Handle 204 No Content
    if (res.status === 204) {
      return undefined as T;
    }
    return res.json() as Promise<T>;
  }

  // Auth
  async sendCode(email: string): Promise<void> {
    await this.fetch("/auth/send-code", {
      method: "POST",
      body: JSON.stringify({ email }),
    });
  }

  async verifyCode(email: string, code: string): Promise<LoginResponse> {
    const raw = await this.fetch<unknown>("/auth/verify-code", {
      method: "POST",
      body: JSON.stringify({ email, code }),
    });
    return parseWithFallback(raw, LoginResponseSchema, EMPTY_LOGIN_RESPONSE as LoginResponse, {
      endpoint: "POST /auth/verify-code",
    });
  }

  async googleLogin(code: string, redirectUri: string): Promise<LoginResponse> {
    const raw = await this.fetch<unknown>("/auth/google", {
      method: "POST",
      body: JSON.stringify({ code, redirect_uri: redirectUri }),
    });
    return parseWithFallback(raw, LoginResponseSchema, EMPTY_LOGIN_RESPONSE as LoginResponse, {
      endpoint: "POST /auth/google",
    });
  }

  async createLarkLoginState(installationId: string, next?: string, redirectUri?: string): Promise<LarkLoginStateResponse> {
    const raw = await this.fetch<unknown>("/auth/lark/state", {
      method: "POST",
      body: JSON.stringify({ installation_id: installationId, next, redirect_uri: redirectUri }),
    });
    return parseWithFallback(raw, LarkLoginStateResponseSchema, EMPTY_LARK_LOGIN_STATE_RESPONSE as LarkLoginStateResponse, {
      endpoint: "POST /auth/lark/state",
    });
  }

  async larkLogin(code: string, state: string): Promise<LarkLoginResponse> {
    const raw = await this.fetch<unknown>("/auth/lark", {
      method: "POST",
      body: JSON.stringify({ code, state }),
    });
    return parseWithFallback(raw, LarkLoginResponseSchema, EMPTY_LARK_LOGIN_RESPONSE as LarkLoginResponse, {
      endpoint: "POST /auth/lark",
    });
  }

  async logout(): Promise<void> {
    await this.fetch("/auth/logout", { method: "POST" });
  }

  async issueCliToken(): Promise<{ token: string }> {
    return this.fetch("/api/cli-token", { method: "POST" });
  }

  async getMe(): Promise<User> {
    const raw = await this.fetch<unknown>("/api/me");
    return parseWithFallback(raw, UserSchema, EMPTY_USER, {
      endpoint: "GET /api/me",
    });
  }

  async markOnboardingComplete(payload?: {
    completion_path?: OnboardingCompletionPath;
    workspace_id?: string;
  }): Promise<User> {
    const raw = await this.fetch<unknown>("/api/me/onboarding/complete", {
      method: "POST",
      body: payload ? JSON.stringify(payload) : undefined,
    });
    return parseWithFallback(raw, UserSchema, EMPTY_USER, {
      endpoint: "POST /api/me/onboarding/complete",
    });
  }

  async joinCloudWaitlist(payload: {
    email: string;
    reason?: string;
  }): Promise<User> {
    const raw = await this.fetch<unknown>("/api/me/onboarding/cloud-waitlist", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    return parseWithFallback(raw, UserSchema, EMPTY_USER, {
      endpoint: "POST /api/me/onboarding/cloud-waitlist",
    });
  }

  async patchOnboarding(payload: {
    questionnaire?: Record<string, unknown>;
  }): Promise<User> {
    const raw = await this.fetch<unknown>("/api/me/onboarding", {
      method: "PATCH",
      body: JSON.stringify(payload),
    });
    return parseWithFallback(raw, UserSchema, EMPTY_USER, {
      endpoint: "PATCH /api/me/onboarding",
    });
  }

  async updateMe(data: UpdateMeRequest): Promise<User> {
    const raw = await this.fetch<unknown>("/api/me", {
      method: "PATCH",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, UserSchema, EMPTY_USER, {
      endpoint: "PATCH /api/me",
    });
  }

  // Issues
  async listIssues(params?: ListIssuesParams): Promise<ListIssuesResponse> {
    const search = new URLSearchParams();
    if (params?.limit) search.set("limit", String(params.limit));
    if (params?.offset) search.set("offset", String(params.offset));
    if (params?.workspace_id) search.set("workspace_id", params.workspace_id);
    if (params?.status) search.set("status", params.status);
    if (params?.priority) search.set("priority", params.priority);
    if (params?.assignee_id) search.set("assignee_id", params.assignee_id);
    if (params?.assignee_ids?.length) search.set("assignee_ids", params.assignee_ids.join(","));
    if (params?.creator_id) search.set("creator_id", params.creator_id);
    if (params?.project_id) search.set("project_id", params.project_id);
    if (params?.involves_user_id) search.set("involves_user_id", params.involves_user_id);
    if (params?.metadata && Object.keys(params.metadata).length > 0) {
      search.set("metadata", JSON.stringify(params.metadata));
    }
    if (params?.open_only) search.set("open_only", "true");
    if (params?.scheduled) search.set("scheduled", "true");
    if (params?.date_field) search.set("date_field", params.date_field);
    if (params?.date_start) search.set("date_start", params.date_start);
    if (params?.date_end) search.set("date_end", params.date_end);
    if (params?.sort_by) search.set("sort", params.sort_by);
    if (params?.sort_direction) search.set("direction", params.sort_direction);
    const path = `/api/issues?${search}`;
    const raw = await this.fetch<unknown>(path);
    return parseWithFallback(raw, ListIssuesResponseSchema, EMPTY_LIST_ISSUES_RESPONSE, {
      endpoint: "GET /api/issues",
    });
  }

  async listGroupedIssues(params: ListGroupedIssuesParams): Promise<GroupedIssuesResponse> {
    const search = new URLSearchParams({ group_by: params.group_by });
    if (params.limit) search.set("limit", String(params.limit));
    if (params.offset) search.set("offset", String(params.offset));
    if (params.workspace_id) search.set("workspace_id", params.workspace_id);
    if (params.statuses?.length) search.set("statuses", params.statuses.join(","));
    if (params.priorities?.length) search.set("priorities", params.priorities.join(","));
    if (params.assignee_types?.length) search.set("assignee_types", params.assignee_types.join(","));
    if (params.assignee_id) search.set("assignee_id", params.assignee_id);
    if (params.assignee_ids?.length) search.set("assignee_ids", params.assignee_ids.join(","));
    if (params.creator_id) search.set("creator_id", params.creator_id);
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.involves_user_id) search.set("involves_user_id", params.involves_user_id);
    if (params.metadata && Object.keys(params.metadata).length > 0) {
      search.set("metadata", JSON.stringify(params.metadata));
    }
    if (params.assignee_filters?.length) {
      search.set("assignee_filters", params.assignee_filters.map((f) => `${f.type}:${f.id}`).join(","));
    }
    if (params.include_no_assignee) search.set("include_no_assignee", "true");
    if (params.creator_filters?.length) {
      search.set("creator_filters", params.creator_filters.map((f) => `${f.type}:${f.id}`).join(","));
    }
    if (params.project_ids?.length) search.set("project_ids", params.project_ids.join(","));
    if (params.include_no_project) search.set("include_no_project", "true");
    if (params.label_ids?.length) search.set("label_ids", params.label_ids.join(","));
    if (params.group_assignee_type) search.set("group_assignee_type", params.group_assignee_type);
    if (params.group_assignee_id) search.set("group_assignee_id", params.group_assignee_id);
    if (params.date_field) search.set("date_field", params.date_field);
    if (params.date_start) search.set("date_start", params.date_start);
    if (params.date_end) search.set("date_end", params.date_end);
    if (params.sort_by) search.set("sort", params.sort_by);
    if (params.sort_direction) search.set("direction", params.sort_direction);
    const raw = await this.fetch<unknown>(`/api/issues/grouped?${search}`);
    return parseWithFallback(raw, GroupedIssuesResponseSchema, EMPTY_GROUPED_ISSUES_RESPONSE, {
      endpoint: "GET /api/issues/grouped",
    });
  }

  async searchIssues(params: { q: string; limit?: number; offset?: number; include_closed?: boolean; signal?: AbortSignal }): Promise<SearchIssuesResponse> {
    const search = new URLSearchParams({ q: params.q });
    if (params.limit !== undefined) search.set("limit", String(params.limit));
    if (params.offset !== undefined) search.set("offset", String(params.offset));
    if (params.include_closed) search.set("include_closed", "true");
    return this.fetch(`/api/issues/search?${search}`, params.signal ? { signal: params.signal } : undefined);
  }

  async searchProjects(params: { q: string; limit?: number; offset?: number; include_closed?: boolean; signal?: AbortSignal }): Promise<SearchProjectsResponse> {
    const search = new URLSearchParams({ q: params.q });
    if (params.limit !== undefined) search.set("limit", String(params.limit));
    if (params.offset !== undefined) search.set("offset", String(params.offset));
    if (params.include_closed) search.set("include_closed", "true");
    return this.fetch(`/api/projects/search?${search}`, params.signal ? { signal: params.signal } : undefined);
  }

  async getIssue(id: string): Promise<Issue> {
    return this.fetch(`/api/issues/${id}`);
  }

  async listPreviewSessions(issueId: string): Promise<PreviewSessionListResponse> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/preview-sessions`,
    );
    return parseWithFallback(
      raw,
      PreviewSessionListResponseSchema,
      EMPTY_PREVIEW_SESSION_LIST_RESPONSE,
      { endpoint: "GET /api/issues/:id/preview-sessions" },
    );
  }

  async createPreviewSession(
    issueId: string,
    data: CreatePreviewSessionRequest,
  ): Promise<PreviewSession> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/preview-sessions`,
      {
        method: "POST",
        body: JSON.stringify({
          ...(data.title !== undefined ? { title: data.title } : {}),
          preview_url: data.previewUrl,
          platform: data.platform ?? "web",
          provider: data.provider ?? "external_web",
          ...(data.expiresAt !== undefined
            ? { expires_at: data.expiresAt }
            : {}),
        }),
      },
    );
    return parseWithFallback(raw, PreviewSessionSchema, EMPTY_PREVIEW_SESSION, {
      endpoint: "POST /api/issues/:id/preview-sessions",
    });
  }

  async getPreviewSession(sessionId: string): Promise<PreviewSession> {
    const raw = await this.fetch<unknown>(
      `/api/preview-sessions/${sessionId}`,
    );
    return parseWithFallback(raw, PreviewSessionSchema, EMPTY_PREVIEW_SESSION, {
      endpoint: "GET /api/preview-sessions/:id",
    });
  }

  async touchPreviewSession(sessionId: string): Promise<PreviewSession> {
    const raw = await this.fetch<unknown>(
      `/api/preview-sessions/${sessionId}/touch`,
      { method: "POST" },
    );
    return parseWithFallback(raw, PreviewSessionSchema, EMPTY_PREVIEW_SESSION, {
      endpoint: "POST /api/preview-sessions/:id/touch",
    });
  }

  async switchPreviewSessionDevice(
    sessionId: string,
    serial: string,
    confirmed = false,
  ): Promise<PreviewSession> {
    const raw = await this.fetch<unknown>(
      `/api/preview-sessions/${sessionId}/device`,
      {
        method: "POST",
        body: JSON.stringify({ serial, confirmed }),
      },
    );
    return parseWithFallback(raw, PreviewSessionSchema, EMPTY_PREVIEW_SESSION, {
      endpoint: "POST /api/preview-sessions/:id/device",
    });
  }

  async stopPreviewSession(sessionId: string): Promise<PreviewSession> {
    const raw = await this.fetch<unknown>(
      `/api/preview-sessions/${sessionId}/stop`,
      { method: "POST" },
    );
    return parseWithFallback(raw, PreviewSessionSchema, EMPTY_PREVIEW_SESSION, {
      endpoint: "POST /api/preview-sessions/:id/stop",
    });
  }

  async createIssue(data: CreateIssueRequest): Promise<Issue> {
    return this.fetch("/api/issues", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async quickCreateIssue(data: {
    agent_id?: string;
    squad_id?: string;
    prompt: string;
    project_id?: string | null;
    parent_issue_id?: string | null;
    attachment_ids?: string[];
  }): Promise<{ task_id: string }> {
    return this.fetch("/api/issues/quick-create", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async createFeedback(data: {
    message: string;
    url?: string;
    workspace_id?: string;
  }): Promise<{ id: string; created_at: string }> {
    return this.fetch("/api/feedback", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateIssue(id: string, data: UpdateIssueRequest): Promise<Issue> {
    return this.fetch(`/api/issues/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async setIssueMetadataKey(
    id: string,
    key: string,
    value: string | number | boolean,
  ): Promise<{ metadata: Record<string, string | number | boolean> }> {
    return this.fetch(`/api/issues/${id}/metadata/${encodeURIComponent(key)}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    });
  }

  async listChildIssues(id: string): Promise<{ issues: Issue[] }> {
    const raw = await this.fetch<unknown>(`/api/issues/${id}/children`);
    return parseWithFallback(raw, ChildIssuesResponseSchema, { issues: [] }, {
      endpoint: "GET /api/issues/:id/children",
    });
  }

  /** Batched variant — returns children for multiple parents in one request.
   *  Avoids an N-request fan-out in Swimlane (one per visible parent lane).
   *  parentIds must be non-empty; pass a sorted, deduplicated list so the
   *  React Query cache key is stable across renders. */
  async listChildrenByParents(parentIds: string[]): Promise<{ issues: Issue[] }> {
    const raw = await this.fetch<unknown>(
      `/api/issues/children?parent_ids=${parentIds.join(",")}`,
    );
    return parseWithFallback(raw, ChildIssuesResponseSchema, { issues: [] }, {
      endpoint: "GET /api/issues/children",
    });
  }

  async getChildIssueProgress(): Promise<{ progress: { parent_issue_id: string; total: number; done: number }[] }> {
    return this.fetch("/api/issues/child-progress");
  }

  async deleteIssue(id: string): Promise<void> {
    await this.fetch(`/api/issues/${id}`, { method: "DELETE" });
  }

  async batchUpdateIssues(issueIds: string[], updates: UpdateIssueRequest): Promise<{ updated: number }> {
    return this.fetch("/api/issues/batch-update", {
      method: "POST",
      body: JSON.stringify({ issue_ids: issueIds, updates }),
    });
  }

  async batchDeleteIssues(issueIds: string[]): Promise<{ deleted: number }> {
    return this.fetch("/api/issues/batch-delete", {
      method: "POST",
      body: JSON.stringify({ issue_ids: issueIds }),
    });
  }

  // Comments
  async listComments(issueId: string): Promise<Comment[]> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/comments`);
    return parseWithFallback(raw, CommentsListSchema, [], {
      endpoint: "GET /api/issues/:id/comments",
    });
  }

  async createComment(
    issueId: string,
    content: string,
    type?: string,
    parentId?: string,
    attachmentIds?: string[],
    suppressAgentIds?: string[],
  ): Promise<Comment> {
    return this.fetch(`/api/issues/${issueId}/comments`, {
      method: "POST",
      body: JSON.stringify({
        content,
        type: type ?? "comment",
        ...(parentId ? { parent_id: parentId } : {}),
        ...(attachmentIds?.length ? { attachment_ids: attachmentIds } : {}),
        ...(suppressAgentIds?.length ? { suppress_agent_ids: suppressAgentIds } : {}),
      }),
    });
  }

  async previewCommentTriggers(issueId: string, content: string, parentId?: string, editingCommentId?: string): Promise<CommentTriggerPreview> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/comments/trigger-preview`, {
      method: "POST",
      body: JSON.stringify({
        content,
        ...(parentId ? { parent_id: parentId } : {}),
        ...(editingCommentId ? { editing_comment_id: editingCommentId } : {}),
      }),
    });
    return parseWithFallback(raw, CommentTriggerPreviewSchema, { agents: [] }, {
      endpoint: "POST /api/issues/:id/comments/trigger-preview",
    });
  }

  async listTimeline(issueId: string): Promise<TimelineEntry[]> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/timeline`,
    );
    return parseWithFallback(raw, TimelineEntriesSchema, EMPTY_TIMELINE_ENTRIES, {
      endpoint: "GET /api/issues/:id/timeline",
    });
  }

  async getAssigneeFrequency(): Promise<AssigneeFrequencyEntry[]> {
    return this.fetch("/api/assignee-frequency");
  }

  async updateComment(commentId: string, content: string, attachmentIds?: string[], suppressAgentIds?: string[]): Promise<Comment> {
    return this.fetch(`/api/comments/${commentId}`, {
      method: "PUT",
      body: JSON.stringify({
        content,
        attachment_ids: attachmentIds,
        ...(suppressAgentIds?.length ? { suppress_agent_ids: suppressAgentIds } : {}),
      }),
    });
  }

  async deleteComment(commentId: string): Promise<void> {
    await this.fetch(`/api/comments/${commentId}`, { method: "DELETE" });
  }

  async resolveComment(commentId: string): Promise<Comment> {
    return this.fetch(`/api/comments/${commentId}/resolve`, { method: "POST" });
  }

  async unresolveComment(commentId: string): Promise<Comment> {
    return this.fetch(`/api/comments/${commentId}/resolve`, { method: "DELETE" });
  }

  async addReaction(commentId: string, emoji: string): Promise<Reaction> {
    return this.fetch(`/api/comments/${commentId}/reactions`, {
      method: "POST",
      body: JSON.stringify({ emoji }),
    });
  }

  async removeReaction(commentId: string, emoji: string): Promise<void> {
    await this.fetch(`/api/comments/${commentId}/reactions`, {
      method: "DELETE",
      body: JSON.stringify({ emoji }),
    });
  }

  async addIssueReaction(issueId: string, emoji: string): Promise<IssueReaction> {
    return this.fetch(`/api/issues/${issueId}/reactions`, {
      method: "POST",
      body: JSON.stringify({ emoji }),
    });
  }

  async removeIssueReaction(issueId: string, emoji: string): Promise<void> {
    await this.fetch(`/api/issues/${issueId}/reactions`, {
      method: "DELETE",
      body: JSON.stringify({ emoji }),
    });
  }

  // Subscribers
  async listIssueSubscribers(issueId: string): Promise<IssueSubscriber[]> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/subscribers`);
    return parseWithFallback(raw, SubscribersListSchema, [], {
      endpoint: "GET /api/issues/:id/subscribers",
    });
  }

  async subscribeToIssue(issueId: string, userId?: string, userType?: string): Promise<void> {
    const body: Record<string, string> = {};
    if (userId) body.user_id = userId;
    if (userType) body.user_type = userType;
    await this.fetch(`/api/issues/${issueId}/subscribe`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async unsubscribeFromIssue(issueId: string, userId?: string, userType?: string): Promise<void> {
    const body: Record<string, string> = {};
    if (userId) body.user_id = userId;
    if (userType) body.user_type = userType;
    await this.fetch(`/api/issues/${issueId}/unsubscribe`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  // Agents
  async listAgents(params?: { workspace_id?: string; include_archived?: boolean }): Promise<Agent[]> {
    const search = new URLSearchParams();
    if (params?.workspace_id) search.set("workspace_id", params.workspace_id);
    if (params?.include_archived) search.set("include_archived", "true");
    return this.fetch(`/api/agents?${search}`);
  }

  async getAgent(id: string): Promise<Agent> {
    return this.fetch(`/api/agents/${id}`);
  }

  async createAgent(data: CreateAgentRequest): Promise<Agent> {
    return this.fetch("/api/agents", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async listAgentTemplates(): Promise<AgentTemplateSummary[]> {
    const raw = await this.fetch<unknown>("/api/agent-templates");
    return parseWithFallback(
      raw,
      AgentTemplateSummaryListSchema,
      EMPTY_AGENT_TEMPLATE_SUMMARY_LIST,
      { endpoint: "GET /api/agent-templates" },
    );
  }

  async getAgentTemplate(slug: string): Promise<AgentTemplate> {
    const raw = await this.fetch<unknown>(
      `/api/agent-templates/${encodeURIComponent(slug)}`,
    );
    // Round-trip the requested slug into the fallback so a malformed
    // detail response still produces a navigable record matching the URL
    // the user clicked.
    return parseWithFallback(
      raw,
      AgentTemplateSchema,
      { ...EMPTY_AGENT_TEMPLATE_DETAIL, slug },
      { endpoint: "GET /api/agent-templates/:slug" },
    );
  }

  /** Creates an agent from a curated template. The server fetches every
   *  referenced skill URL in parallel, materializes them into the workspace
   *  (find-or-create by name), and writes the agent + skill bindings in a
   *  single transaction. On any upstream fetch failure, the entire write is
   *  rolled back and the API returns 422 with `failed_urls`. */
  async createAgentFromTemplate(
    data: CreateAgentFromTemplateRequest,
  ): Promise<CreateAgentFromTemplateResponse> {
    const raw = await this.fetch<unknown>("/api/agents/from-template", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(
      raw,
      CreateAgentFromTemplateResponseSchema,
      EMPTY_CREATE_AGENT_FROM_TEMPLATE_RESPONSE,
      { endpoint: "POST /api/agents/from-template" },
    );
  }

  async updateAgent(id: string, data: UpdateAgentRequest): Promise<Agent> {
    return this.fetch(`/api/agents/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async archiveAgent(id: string): Promise<Agent> {
    return this.fetch(`/api/agents/${id}/archive`, { method: "POST" });
  }

  /**
   * Returns the plaintext `custom_env` map for an agent. Owner/admin
   * only; calls from agent-actor sessions get a 403. Every successful
   * call writes an `agent_env_revealed` activity_log row server-side.
   * MUL-2600.
   */
  async getAgentEnv(id: string): Promise<AgentEnvResponse> {
    return this.fetch(`/api/agents/${id}/env`);
  }

  /**
   * Replaces an agent's `custom_env` wholesale. Values equal to
   * `"****"` are preserved server-side (the **** guard) so a partial
   * UI edit doesn't overwrite real secrets with the masked
   * placeholder. Owner/admin only; agent actors get a 403. Every
   * successful call writes an `agent_env_updated` activity_log row.
   * MUL-2600.
   */
  async updateAgentEnv(id: string, data: UpdateAgentEnvRequest): Promise<AgentEnvResponse> {
    return this.fetch(`/api/agents/${id}/env`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async restoreAgent(id: string): Promise<Agent> {
    return this.fetch(`/api/agents/${id}/restore`, { method: "POST" });
  }

  // Bulk-cancel every active task (queued/dispatched/running) for the agent.
  // Permission: agent owner or workspace admin/owner. Server returns the
  // count of cancelled rows; broadcasts task:cancelled for each so other
  // surfaces can clear their live cards.
  async cancelAgentTasks(id: string): Promise<{ cancelled: number }> {
    return this.fetch(`/api/agents/${id}/cancel-tasks`, { method: "POST" });
  }

  async listRuntimes(params?: { workspace_id?: string; owner?: "me" }): Promise<AgentRuntime[]> {
    const search = new URLSearchParams();
    if (params?.workspace_id) search.set("workspace_id", params.workspace_id);
    if (params?.owner) search.set("owner", params.owner);
    return this.fetch(`/api/runtimes?${search}`);
  }

  async listRuntimeProfiles(workspaceId: string): Promise<RuntimeProfile[]> {
    const res = await this.fetch<{ runtime_profiles?: RuntimeProfile[] }>(
      `/api/workspaces/${workspaceId}/runtime-profiles`,
    );
    return res.runtime_profiles ?? [];
  }

  async getRuntimeProfile(workspaceId: string, profileId: string): Promise<RuntimeProfile> {
    return this.fetch(`/api/workspaces/${workspaceId}/runtime-profiles/${profileId}`);
  }

  async createRuntimeProfile(
    workspaceId: string,
    body: CreateRuntimeProfileRequest,
  ): Promise<RuntimeProfile> {
    return this.fetch(`/api/workspaces/${workspaceId}/runtime-profiles`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async updateRuntimeProfile(
    workspaceId: string,
    profileId: string,
    patch: UpdateRuntimeProfileRequest,
  ): Promise<RuntimeProfile> {
    return this.fetch(`/api/workspaces/${workspaceId}/runtime-profiles/${profileId}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    });
  }

  async deleteRuntimeProfile(workspaceId: string, profileId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/runtime-profiles/${profileId}`, {
      method: "DELETE",
    });
  }

  async listCloudRuntimeNodes(
    params?: ListCloudRuntimeNodesParams,
  ): Promise<CloudRuntimeNode[]> {
    const search = new URLSearchParams();
    if (params?.limit !== undefined) search.set("limit", String(params.limit));
    if (params?.offset !== undefined) search.set("offset", String(params.offset));
    const query = search.toString();
    const raw = await this.fetch<unknown>(
      `/api/cloud-runtime/nodes${query ? `?${query}` : ""}`,
    );
    return parseWithFallback(
      raw,
      CloudRuntimeNodeListSchema,
      EMPTY_CLOUD_RUNTIME_NODE_LIST,
      { endpoint: "GET /api/cloud-runtime/nodes" },
    );
  }

  async createCloudRuntimeNode(
    data: CreateCloudRuntimeNodeRequest,
  ): Promise<CloudRuntimeNode> {
    const res = await this.fetchRaw("/api/cloud-runtime/nodes", {
      method: "POST",
      body: JSON.stringify(data),
      extraHeaders: { "Content-Type": "application/json" },
    });
    const raw = await res.json() as unknown;
    return parseWithFallback(
      raw,
      CloudRuntimeNodeSchema,
      EMPTY_CLOUD_RUNTIME_NODE,
      { endpoint: "POST /api/cloud-runtime/nodes" },
    );
  }

  async deleteCloudRuntimeNode(instanceId: string): Promise<void> {
    await this.fetchRaw("/api/cloud-runtime/nodes", {
      method: "DELETE",
      body: JSON.stringify({ instance_id: instanceId }),
      extraHeaders: { "Content-Type": "application/json" },
    });
  }

  // ---------------------------------------------------------------------
  // Cloud Billing — proxies to multica-cloud /api/v1/billing/*. The
  // multica-api server stamps X-User-ID and forwards bytes; everything
  // here is upstream-shaped. See packages/core/types/billing.ts for the
  // response field documentation.
  // ---------------------------------------------------------------------

  async getCloudBillingBalance(): Promise<BillingBalance> {
    const raw = await this.fetch<unknown>("/api/cloud-billing/balance");
    return parseWithFallback(raw, BillingBalanceSchema, EMPTY_BILLING_BALANCE, {
      endpoint: "GET /api/cloud-billing/balance",
    });
  }

  async listCloudBillingTransactions(
    params?: { page?: number; page_size?: number },
  ): Promise<BillingTransactionsPage> {
    const search = new URLSearchParams();
    if (params?.page !== undefined) search.set("page", String(params.page));
    if (params?.page_size !== undefined) search.set("page_size", String(params.page_size));
    const query = search.toString();
    const raw = await this.fetch<unknown>(
      `/api/cloud-billing/transactions${query ? `?${query}` : ""}`,
    );
    return parseWithFallback(
      raw,
      BillingTransactionsPageSchema,
      EMPTY_BILLING_TRANSACTIONS_PAGE,
      { endpoint: "GET /api/cloud-billing/transactions" },
    );
  }

  async listCloudBillingBatches(
    params?: { page?: number; page_size?: number },
  ): Promise<BillingBatchesPage> {
    const search = new URLSearchParams();
    if (params?.page !== undefined) search.set("page", String(params.page));
    if (params?.page_size !== undefined) search.set("page_size", String(params.page_size));
    const query = search.toString();
    const raw = await this.fetch<unknown>(
      `/api/cloud-billing/batches${query ? `?${query}` : ""}`,
    );
    return parseWithFallback(
      raw,
      BillingBatchesPageSchema,
      EMPTY_BILLING_BATCHES_PAGE,
      { endpoint: "GET /api/cloud-billing/batches" },
    );
  }

  async listCloudBillingTopups(
    params?: { page?: number; page_size?: number },
  ): Promise<BillingTopupsPage> {
    const search = new URLSearchParams();
    if (params?.page !== undefined) search.set("page", String(params.page));
    if (params?.page_size !== undefined) search.set("page_size", String(params.page_size));
    const query = search.toString();
    const raw = await this.fetch<unknown>(
      `/api/cloud-billing/topups${query ? `?${query}` : ""}`,
    );
    return parseWithFallback(
      raw,
      BillingTopupsPageSchema,
      EMPTY_BILLING_TOPUPS_PAGE,
      { endpoint: "GET /api/cloud-billing/topups" },
    );
  }

  async listCloudBillingPriceTiers(): Promise<BillingPriceTier[]> {
    const raw = await this.fetch<unknown>("/api/cloud-billing/price-tiers");
    return parseWithFallback(
      raw,
      BillingPriceTierListSchema,
      EMPTY_BILLING_PRICE_TIER_LIST,
      { endpoint: "GET /api/cloud-billing/price-tiers" },
    );
  }

  async createCloudBillingCheckoutSession(
    data: CreateBillingCheckoutSessionRequest,
  ): Promise<CreateBillingCheckoutSessionResponse> {
    const res = await this.fetchRaw("/api/cloud-billing/checkout-sessions", {
      method: "POST",
      body: JSON.stringify(data),
      extraHeaders: { "Content-Type": "application/json" },
    });
    const raw = (await res.json()) as unknown;
    return parseWithFallback(
      raw,
      CreateBillingCheckoutSessionResponseSchema,
      EMPTY_CREATE_BILLING_CHECKOUT_SESSION_RESPONSE,
      { endpoint: "POST /api/cloud-billing/checkout-sessions" },
    );
  }

  async getCloudBillingCheckoutSession(
    sessionId: string,
  ): Promise<BillingCheckoutSessionStatus> {
    // Stripe session ids are `cs_<base62>` so they're URL-safe by
    // construction; encodeURIComponent is paranoia for the case where a
    // future Stripe format change adds a non-alphanumeric character. The
    // server has its own allow-list rejection for unsafe ids.
    const raw = await this.fetch<unknown>(
      `/api/cloud-billing/checkout-sessions/${encodeURIComponent(sessionId)}`,
    );
    return parseWithFallback(
      raw,
      BillingCheckoutSessionStatusSchema,
      EMPTY_BILLING_CHECKOUT_SESSION_STATUS,
      { endpoint: "GET /api/cloud-billing/checkout-sessions/{sessionId}" },
    );
  }

  async createCloudBillingPortalSession(): Promise<CreateBillingPortalSessionResponse> {
    const res = await this.fetchRaw("/api/cloud-billing/portal-sessions", {
      method: "POST",
      // Body is intentionally absent — the upstream endpoint requires no
      // payload today. fetchRaw with no body skips the Content-Type
      // default; that's fine because there's nothing to declare.
    });
    const raw = (await res.json()) as unknown;
    return parseWithFallback(
      raw,
      CreateBillingPortalSessionResponseSchema,
      EMPTY_CREATE_BILLING_PORTAL_SESSION_RESPONSE,
      { endpoint: "POST /api/cloud-billing/portal-sessions" },
    );
  }

  async deleteRuntime(runtimeId: string): Promise<void> {
    await this.fetch(`/api/runtimes/${runtimeId}`, { method: "DELETE" });
  }

  // Cascade variant of deleteRuntime. The strict DELETE refuses with
  // structured 409 (`code: "runtime_has_active_agents"`, body carries the
  // blocking agents) when active agents are bound; the front-end then opens
  // the cascade-mode confirmation dialog and submits the user-confirmed
  // active agent set here. Server compares the snapshot to the live set
  // inside the transaction and refuses with `code: "runtime_delete_plan_changed"`
  // (same shape, fresh `active_agents`) if they don't match — caller should
  // re-render the agent list and force the user to re-confirm.
  async archiveAgentsAndDeleteRuntime(
    runtimeId: string,
    expectedActiveAgentIds: string[],
  ): Promise<{ status: string; agents_archived: number; tasks_cancelled: number }> {
    return this.fetch(`/api/runtimes/${runtimeId}/archive-agents-and-delete`, {
      method: "POST",
      body: JSON.stringify({ expected_active_agent_ids: expectedActiveAgentIds }),
    });
  }

  async updateRuntime(
    runtimeId: string,
    patch: { visibility?: "private" | "public" },
  ): Promise<AgentRuntime> {
    return this.fetch(`/api/runtimes/${runtimeId}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    });
  }

  async getRuntimeUsage(
    runtimeId: string,
    params?: { days?: number; tz?: string },
  ): Promise<RuntimeUsage[]> {
    const search = new URLSearchParams();
    if (params?.days) search.set("days", String(params.days));
    // `tz` drives the calendar-day boundary for the trend chart (Viewing
    // layer). Caller-supplied; the backend falls back to user.timezone /
    // UTC if omitted.
    if (params?.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(
      `/api/runtimes/${runtimeId}/usage?${search}`,
    );
    return parseWithFallback<RuntimeUsage[]>(raw, RuntimeUsageListSchema, [], {
      endpoint: "GET /api/runtimes/:id/usage",
    });
  }

  async getRuntimeTaskActivity(
    runtimeId: string,
    params?: { tz?: string },
  ): Promise<RuntimeHourlyActivity[]> {
    // Hour-of-day heatmap follows the viewer's tz, like the other reports on
    // this page. Pass the viewer's IANA zone so the server buckets correctly.
    const search = new URLSearchParams();
    if (params?.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(
      `/api/runtimes/${runtimeId}/activity?${search}`,
    );
    return parseWithFallback<RuntimeHourlyActivity[]>(
      raw,
      RuntimeHourlyActivityListSchema,
      [],
      { endpoint: "GET /api/runtimes/:id/activity" },
    );
  }

  async getRuntimeUsageByAgent(
    runtimeId: string,
    params?: { days?: number; tz?: string },
  ): Promise<RuntimeUsageByAgent[]> {
    const search = new URLSearchParams();
    if (params?.days) search.set("days", String(params.days));
    if (params?.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(
      `/api/runtimes/${runtimeId}/usage/by-agent?${search}`,
    );
    return parseWithFallback<RuntimeUsageByAgent[]>(
      raw,
      RuntimeUsageByAgentListSchema,
      [],
      { endpoint: "GET /api/runtimes/:id/usage/by-agent" },
    );
  }

  async getRuntimeUsageByHour(
    runtimeId: string,
    params?: { days?: number; tz?: string },
  ): Promise<RuntimeUsageByHour[]> {
    const search = new URLSearchParams();
    if (params?.days) search.set("days", String(params.days));
    if (params?.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(
      `/api/runtimes/${runtimeId}/usage/by-hour?${search}`,
    );
    return parseWithFallback<RuntimeUsageByHour[]>(
      raw,
      RuntimeUsageByHourListSchema,
      [],
      { endpoint: "GET /api/runtimes/:id/usage/by-hour" },
    );
  }

  // ---------------------------------------------------------------------------
  // Workspace dashboard — three independent rollups for `/{slug}/dashboard`.
  // Each accepts an optional `project_id` to narrow the scope to one project.
  // Cost is computed client-side from the model pricing table (same contract
  // as the per-runtime endpoints above).
  // ---------------------------------------------------------------------------

  async getDashboardUsageDaily(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardUsageDaily[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/usage/daily?${search}`);
    return parseWithFallback<DashboardUsageDaily[]>(
      raw,
      DashboardUsageDailyListSchema,
      [],
      { endpoint: "GET /api/dashboard/usage/daily" },
    );
  }

  async getDashboardUsageByAgent(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardUsageByAgent[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/usage/by-agent?${search}`);
    return parseWithFallback<DashboardUsageByAgent[]>(
      raw,
      DashboardUsageByAgentListSchema,
      [],
      { endpoint: "GET /api/dashboard/usage/by-agent" },
    );
  }

  async getDashboardUsageByUser(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardUsageByUser[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/usage/by-user?${search}`);
    return parseWithFallback<DashboardUsageByUser[]>(
      raw,
      DashboardUsageByUserListSchema,
      [],
      { endpoint: "GET /api/dashboard/usage/by-user" },
    );
  }

  async getDashboardUsageMe(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardUsageByUser[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/usage/me?${search}`);
    return parseWithFallback<DashboardUsageByUser[]>(
      raw,
      DashboardUsageByUserListSchema,
      [],
      { endpoint: "GET /api/dashboard/usage/me" },
    );
  }

  async getDashboardUsageByUserDaily(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardUsageByUserDaily[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/usage/by-user/daily?${search}`);
    return parseWithFallback<DashboardUsageByUserDaily[]>(
      raw,
      DashboardUsageByUserDailyListSchema,
      [],
      { endpoint: "GET /api/dashboard/usage/by-user/daily" },
    );
  }

  async getDashboardUsageMeDaily(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardUsageByUserDaily[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/usage/me/daily?${search}`);
    return parseWithFallback<DashboardUsageByUserDaily[]>(
      raw,
      DashboardUsageByUserDailyListSchema,
      [],
      { endpoint: "GET /api/dashboard/usage/me/daily" },
    );
  }

  async getDashboardAgentRunTime(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardAgentRunTime[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    // `tz` aligns the "last N days" cutoff with the viewer's calendar,
    // matching the per-agent token card.
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/agent-runtime?${search}`);
    return parseWithFallback<DashboardAgentRunTime[]>(
      raw,
      DashboardAgentRunTimeListSchema,
      [],
      { endpoint: "GET /api/dashboard/agent-runtime" },
    );
  }

  async getDashboardRunTimeDaily(
    params: { days?: number; project_id?: string | null; tz?: string },
  ): Promise<DashboardRunTimeDaily[]> {
    const search = new URLSearchParams();
    if (params.days) search.set("days", String(params.days));
    if (params.project_id) search.set("project_id", params.project_id);
    // `tz` cuts the day buckets in the viewer's calendar so Time / Tasks
    // align with the Cost / Tokens charts.
    if (params.tz) search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/dashboard/runtime/daily?${search}`);
    return parseWithFallback<DashboardRunTimeDaily[]>(
      raw,
      DashboardRunTimeDailyListSchema,
      [],
      { endpoint: "GET /api/dashboard/runtime/daily" },
    );
  }

  async initiateUpdate(
    runtimeId: string,
    targetVersion: string,
  ): Promise<RuntimeUpdate> {
    return this.fetch(`/api/runtimes/${runtimeId}/update`, {
      method: "POST",
      body: JSON.stringify({ target_version: targetVersion }),
    });
  }

  async getUpdateResult(
    runtimeId: string,
    updateId: string,
  ): Promise<RuntimeUpdate> {
    return this.fetch(`/api/runtimes/${runtimeId}/update/${updateId}`);
  }

  async initiateListModels(runtimeId: string): Promise<RuntimeModelListRequest> {
    return this.fetch(`/api/runtimes/${runtimeId}/models`, { method: "POST" });
  }

  async getListModelsResult(
    runtimeId: string,
    requestId: string,
  ): Promise<RuntimeModelListRequest> {
    return this.fetch(`/api/runtimes/${runtimeId}/models/${requestId}`);
  }

  async initiateListLocalSkills(
    runtimeId: string,
  ): Promise<RuntimeLocalSkillListRequest> {
    return this.fetch(`/api/runtimes/${runtimeId}/local-skills`, {
      method: "POST",
    });
  }

  async getListLocalSkillsResult(
    runtimeId: string,
    requestId: string,
  ): Promise<RuntimeLocalSkillListRequest> {
    return this.fetch(`/api/runtimes/${runtimeId}/local-skills/${requestId}`);
  }

  async initiateImportLocalSkill(
    runtimeId: string,
    data: CreateRuntimeLocalSkillImportRequest,
  ): Promise<RuntimeLocalSkillImportRequest> {
    return this.fetch(`/api/runtimes/${runtimeId}/local-skills/import`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async getImportLocalSkillResult(
    runtimeId: string,
    requestId: string,
  ): Promise<RuntimeLocalSkillImportRequest> {
    return this.fetch(`/api/runtimes/${runtimeId}/local-skills/import/${requestId}`);
  }

  async listAgentTasks(agentId: string): Promise<AgentTask[]> {
    return this.fetch(`/api/agents/${agentId}/tasks`);
  }

  // Workspace-scoped agent task snapshot: every active task
  // (queued/dispatched/running) plus each agent's most recent terminal task.
  // Powers the front-end's "active wins, else latest terminal" presence
  // derivation; one fetch backs every per-agent presence read in the app.
  // Workspace is resolved server-side from the X-Workspace-Slug header.
  async getAgentTaskSnapshot(): Promise<AgentTask[]> {
    return this.fetch(`/api/agent-task-snapshot`);
  }

  // Per-agent daily activity for the last 30 days, anchored on
  // completed_at. One workspace-wide fetch backs both the Agents-list
  // sparkline (uses trailing 7 buckets) and the agent detail "Last 30
  // days" panel (uses all 30).
  async getWorkspaceAgentActivity30d(): Promise<AgentActivityBucket[]> {
    return this.fetch(`/api/agent-activity-30d`);
  }

  // Per-agent 30-day total run count for the Agents-list RUNS column.
  async getWorkspaceAgentRunCounts(): Promise<AgentRunCount[]> {
    return this.fetch(`/api/agent-run-counts`);
  }

  async getActiveTasksForIssue(issueId: string): Promise<{ tasks: AgentTask[] }> {
    return this.fetch(`/api/issues/${issueId}/active-task`);
  }

  async listTaskMessages(taskId: string): Promise<TaskMessagePayload[]> {
    return this.fetch(`/api/tasks/${taskId}/messages`);
  }

  async listTasksByIssue(issueId: string): Promise<AgentTask[]> {
    return this.fetch(`/api/issues/${issueId}/task-runs`);
  }

  async getIssueUsage(issueId: string): Promise<IssueUsageSummary> {
    return this.fetch(`/api/issues/${issueId}/usage`);
  }

  async cancelTask(issueId: string, taskId: string): Promise<AgentTask> {
    return this.fetch(`/api/issues/${issueId}/tasks/${taskId}/cancel`, {
      method: "POST",
    });
  }

  async rerunIssue(issueId: string, taskId?: string): Promise<AgentTask> {
    return this.fetch(`/api/issues/${issueId}/rerun`, {
      method: "POST",
      body: JSON.stringify(taskId ? { task_id: taskId } : {}),
    });
  }

  // Inbox
  async listInbox(): Promise<InboxItem[]> {
    return this.fetch("/api/inbox");
  }

  async markInboxRead(id: string): Promise<InboxItem> {
    return this.fetch(`/api/inbox/${id}/read`, { method: "POST" });
  }

  async archiveInbox(id: string): Promise<InboxItem> {
    return this.fetch(`/api/inbox/${id}/archive`, { method: "POST" });
  }

  async getUnreadInboxCount(): Promise<{ count: number }> {
    return this.fetch("/api/inbox/unread-count");
  }

  async markAllInboxRead(): Promise<{ count: number }> {
    return this.fetch("/api/inbox/mark-all-read", { method: "POST" });
  }

  async archiveAllInbox(): Promise<{ count: number }> {
    return this.fetch("/api/inbox/archive-all", { method: "POST" });
  }

  async archiveAllReadInbox(): Promise<{ count: number }> {
    return this.fetch("/api/inbox/archive-all-read", { method: "POST" });
  }

  async archiveCompletedInbox(): Promise<{ count: number }> {
    return this.fetch("/api/inbox/archive-completed", { method: "POST" });
  }

  // Notification preferences
  //
  // `workspaceSlug` overrides the default `X-Workspace-Slug` header (which
  // follows the active workspace) so a caller can read a SPECIFIC workspace's
  // preferences — e.g. honoring the mute setting of the workspace an inbox
  // notification came from while the user is viewing a different one (#3766).
  async getNotificationPreferences(workspaceSlug?: string): Promise<NotificationPreferenceResponse> {
    return this.fetch(
      "/api/notification-preferences",
      workspaceSlug ? { headers: { "X-Workspace-Slug": workspaceSlug } } : undefined,
    );
  }

  async updateNotificationPreferences(preferences: NotificationPreferences): Promise<NotificationPreferenceResponse> {
    return this.fetch("/api/notification-preferences", {
      method: "PUT",
      body: JSON.stringify({ preferences }),
    });
  }

  // App Config
  async getConfig(): Promise<AppConfigResponse> {
    const raw = await this.fetch<unknown>("/api/config");
    return parseWithFallback<AppConfigResponse>(raw, AppConfigSchema, EMPTY_APP_CONFIG, {
      endpoint: "GET /api/config",
    });
  }

  // Workspaces
  async listWorkspaces(): Promise<Workspace[]> {
    return this.fetch("/api/workspaces");
  }

  async getWorkspace(id: string): Promise<Workspace> {
    return this.fetch(`/api/workspaces/${id}`);
  }

  async createWorkspace(data: { name: string; slug: string; description?: string; context?: string }): Promise<Workspace> {
    return this.fetch("/api/workspaces", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateWorkspace(id: string, data: { name?: string; description?: string; context?: string; settings?: Record<string, unknown>; repos?: WorkspaceRepo[]; issue_prefix?: string; avatar_url?: string }): Promise<Workspace> {
    return this.fetch(`/api/workspaces/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  // Members
  async listMembers(workspaceId: string): Promise<MemberWithUser[]> {
    return this.fetch(`/api/workspaces/${workspaceId}/members`);
  }

  async createMember(workspaceId: string, data: CreateMemberRequest): Promise<Invitation> {
    return this.fetch(`/api/workspaces/${workspaceId}/members`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateMember(workspaceId: string, memberId: string, data: UpdateMemberRequest): Promise<MemberWithUser> {
    return this.fetch(`/api/workspaces/${workspaceId}/members/${memberId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteMember(workspaceId: string, memberId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/members/${memberId}`, {
      method: "DELETE",
    });
  }

  async leaveWorkspace(workspaceId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/leave`, {
      method: "POST",
    });
  }

  // Invitations
  async listWorkspaceInvitations(workspaceId: string): Promise<Invitation[]> {
    return this.fetch(`/api/workspaces/${workspaceId}/invitations`);
  }

  async revokeInvitation(workspaceId: string, invitationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/invitations/${invitationId}`, {
      method: "DELETE",
    });
  }

  async listMyInvitations(): Promise<Invitation[]> {
    return this.fetch("/api/invitations");
  }

  async getInvitation(invitationId: string): Promise<Invitation> {
    return this.fetch(`/api/invitations/${invitationId}`);
  }

  async acceptInvitation(invitationId: string): Promise<MemberWithUser> {
    return this.fetch(`/api/invitations/${invitationId}/accept`, {
      method: "POST",
    });
  }

  async declineInvitation(invitationId: string): Promise<void> {
    await this.fetch(`/api/invitations/${invitationId}/decline`, {
      method: "POST",
    });
  }

  async deleteWorkspace(workspaceId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}`, {
      method: "DELETE",
    });
  }

  // Skills
  async listSkills(): Promise<SkillSummary[]> {
    return this.fetch("/api/skills");
  }

  async getSkill(id: string): Promise<Skill> {
    return this.fetch(`/api/skills/${id}`);
  }

  async createSkill(data: CreateSkillRequest): Promise<Skill> {
    return this.fetch("/api/skills", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateSkill(id: string, data: UpdateSkillRequest): Promise<Skill> {
    return this.fetch(`/api/skills/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteSkill(id: string): Promise<void> {
    await this.fetch(`/api/skills/${id}`, { method: "DELETE" });
  }

  async importSkill(data: { url: string }): Promise<Skill> {
    return this.fetch("/api/skills/import", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async listAgentSkills(agentId: string): Promise<SkillSummary[]> {
    return this.fetch(`/api/agents/${agentId}/skills`);
  }

  async setAgentSkills(agentId: string, data: SetAgentSkillsRequest): Promise<void> {
    await this.fetch(`/api/agents/${agentId}/skills`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  // Incremental attach: POST /skills/add only inserts the given ids (the
  // server upserts with ON CONFLICT DO NOTHING), so callers don't need to
  // read the agent's current skill set first.
  async addAgentSkills(agentId: string, data: SetAgentSkillsRequest): Promise<void> {
    await this.fetch(`/api/agents/${agentId}/skills/add`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // Personal Access Tokens
  async listPersonalAccessTokens(): Promise<PersonalAccessToken[]> {
    return this.fetch("/api/tokens");
  }

  async createPersonalAccessToken(data: CreatePersonalAccessTokenRequest): Promise<CreatePersonalAccessTokenResponse> {
    return this.fetch("/api/tokens", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async revokePersonalAccessToken(id: string): Promise<void> {
    await this.fetch(`/api/tokens/${id}`, { method: "DELETE" });
  }

  // File Upload & Attachments
  async uploadFile(
    file: File,
    opts?: { issueId?: string; commentId?: string; chatSessionId?: string },
  ): Promise<Attachment> {
    const formData = new FormData();
    formData.append("file", file);
    if (opts?.issueId) formData.append("issue_id", opts.issueId);
    if (opts?.commentId) formData.append("comment_id", opts.commentId);
    if (opts?.chatSessionId) formData.append("chat_session_id", opts.chatSessionId);

    const rid = createRequestId();
    const start = Date.now();
    this.logger.info("→ POST /api/upload-file", { rid });

    const res = await fetch(`${this.baseUrl}/api/upload-file`, {
      method: "POST",
      headers: this.authHeaders(),
      body: formData,
      credentials: "include",
    });

    if (!res.ok) {
      if (res.status === 401) this.handleUnauthorized();
      const message = await this.parseErrorMessage(res, `Upload failed: ${res.status}`);
      this.logger.error(`← ${res.status} /api/upload-file`, { rid, duration: `${Date.now() - start}ms`, error: message });
      throw new Error(message);
    }

    this.logger.info(`← ${res.status} /api/upload-file`, { rid, duration: `${Date.now() - start}ms` });
    const raw = (await res.json()) as unknown;
    return parseWithFallback(raw, AttachmentResponseSchema, EMPTY_ATTACHMENT, {
      endpoint: "POST /api/upload-file",
    });
  }

  // Chat Sessions
  async listChatSessions(params?: { status?: string }): Promise<ChatSession[]> {
    const query = params?.status ? `?status=${params.status}` : "";
    return this.fetch(`/api/chat/sessions${query}`);
  }

  async getChatSession(id: string): Promise<ChatSession> {
    return this.fetch(`/api/chat/sessions/${id}`);
  }

  async createChatSession(data: { agent_id: string; title?: string }): Promise<ChatSession> {
    return this.fetch("/api/chat/sessions", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteChatSession(id: string): Promise<void> {
    await this.fetch(`/api/chat/sessions/${id}`, { method: "DELETE" });
  }

  async updateChatSession(id: string, data: { title: string }): Promise<ChatSession> {
    return this.fetch(`/api/chat/sessions/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async listChatMessages(sessionId: string): Promise<ChatMessage[]> {
    const raw = await this.fetch<unknown>(`/api/chat/sessions/${sessionId}/messages`);
    return parseWithFallback(raw, ChatMessageListSchema, EMPTY_CHAT_MESSAGE_LIST, {
      endpoint: "GET /api/chat/sessions/:sessionId/messages",
    });
  }

  async listChatMessagesPage(
    sessionId: string,
    params: { before?: { created_at: string; id: string } | null; limit?: number } = {},
  ): Promise<ChatMessagesPage> {
    const limit = params.limit ?? 50;
    const query = new URLSearchParams({ limit: String(limit) });
    if (params.before) {
      query.set("before_created_at", params.before.created_at);
      query.set("before_id", params.before.id);
    }
    try {
      const raw = await this.fetch<unknown>(
        `/api/chat/sessions/${sessionId}/messages/page?${query.toString()}`,
      );
      return parseWithFallback(
        raw,
        ChatMessagesPageSchema,
        { ...EMPTY_CHAT_MESSAGES_PAGE, limit },
        { endpoint: "GET /api/chat/sessions/:sessionId/messages/page" },
      );
    } catch (err) {
      // Deployment-order compatibility: a backend deployed before this endpoint
      // existed returns 404 for the unknown route. Fall back to the legacy
      // full-list endpoint so chat never white-screens regardless of whether
      // the server or the client deploys first. Only the initial (cursorless)
      // page falls back — the legacy endpoint returns every message at once, so
      // the fallback page reports has_more: false and there is no follow-up
      // request to translate. A 404 on a cursor request is an unexpected state
      // and propagates instead of duplicating the whole list.
      if (err instanceof ApiError && err.status === 404 && !params.before) {
        const messages = await this.listChatMessages(sessionId);
        return { messages, limit, has_more: false, next_cursor: null };
      }
      throw err;
    }
  }

  async sendChatMessage(
    sessionId: string,
    content: string,
    attachmentIds?: string[],
  ): Promise<SendChatMessageResponse> {
    const body: { content: string; attachment_ids?: string[] } = { content };
    if (attachmentIds && attachmentIds.length > 0) {
      body.attachment_ids = attachmentIds;
    }
    return this.fetch(`/api/chat/sessions/${sessionId}/messages`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async upsertChatMessageFeedback(
    sessionId: string,
    messageId: string,
    data: { sentiment: "positive" | "negative" | null; comment: string },
  ): Promise<void> {
    await this.fetch(
      `/api/chat/sessions/${sessionId}/messages/${messageId}/feedback`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      },
    );
  }

  async getPendingChatTask(sessionId: string): Promise<ChatPendingTask> {
    return this.fetch(`/api/chat/sessions/${sessionId}/pending-task`);
  }

  async listPendingChatTasks(): Promise<PendingChatTasksResponse> {
    return this.fetch(`/api/chat/pending-tasks`);
  }

  async markChatSessionRead(sessionId: string): Promise<void> {
    await this.fetch(`/api/chat/sessions/${sessionId}/read`, { method: "POST" });
  }

  async cancelTaskById(taskId: string): Promise<CancelTaskResponse> {
    const raw = await this.fetch<unknown>(`/api/tasks/${taskId}/cancel`, { method: "POST" });
    return parseWithFallback(raw, CancelTaskResponseSchema, EMPTY_CANCEL_TASK_RESPONSE, {
      endpoint: "POST /api/tasks/{taskId}/cancel",
    });
  }

  async listAttachments(issueId: string): Promise<Attachment[]> {
    return this.fetch(`/api/issues/${issueId}/attachments`);
  }

  // Fetches a fresh attachment metadata record. The server re-signs
  // `download_url` on every call (30 min expiry), so the click-time
  // download flow uses this endpoint to avoid handing the user a stale
  // signed URL cached in TanStack Query.
  async getAttachment(id: string): Promise<Attachment> {
    const raw = await this.fetch<unknown>(`/api/attachments/${id}`);
    return parseWithFallback(raw, AttachmentResponseSchema, EMPTY_ATTACHMENT, {
      endpoint: "GET /api/attachments/{id}",
    });
  }

  async listFavoriteCategories(): Promise<FavoriteCategory[]> {
    const raw = await this.fetch<unknown>("/api/favorite-categories");
    return parseWithFallback(
      raw,
      FavoriteCategoryListSchema,
      EMPTY_FAVORITE_CATEGORIES,
      { endpoint: "GET /api/favorite-categories" },
    );
  }

  async createFavoriteCategory(name: string): Promise<FavoriteCategory> {
    const raw = await this.fetch<unknown>("/api/favorite-categories", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    return parseWithFallback(
      raw,
      FavoriteCategoryResponseSchema,
      EMPTY_FAVORITE_CATEGORY,
      { endpoint: "POST /api/favorite-categories" },
    );
  }

  async updateFavoriteCategory(
    id: string,
    name: string,
  ): Promise<FavoriteCategory> {
    const raw = await this.fetch<unknown>(`/api/favorite-categories/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name }),
    });
    return parseWithFallback(
      raw,
      FavoriteCategoryResponseSchema,
      EMPTY_FAVORITE_CATEGORY,
      { endpoint: "PATCH /api/favorite-categories/{id}" },
    );
  }

  async deleteFavoriteCategory(id: string): Promise<void> {
    await this.fetch(`/api/favorite-categories/${id}`, {
      method: "DELETE",
    });
  }

  async listFavorites(): Promise<Favorite[]> {
    const raw = await this.fetch<unknown>("/api/favorites");
    return parseWithFallback(raw, FavoriteListSchema, EMPTY_FAVORITES, {
      endpoint: "GET /api/favorites",
    });
  }

  async putFavorite(
    itemType: FavoriteType,
    itemId: string,
  ): Promise<Favorite> {
    const raw = await this.fetch<unknown>(
      `/api/favorites/${itemType}/${itemId}`,
      { method: "PUT" },
    );
    return parseWithFallback(raw, FavoriteResponseSchema, EMPTY_FAVORITE, {
      endpoint: "PUT /api/favorites/{itemType}/{itemId}",
    });
  }

  async moveFavorite(
    itemType: FavoriteType,
    itemId: string,
    categoryId: string,
  ): Promise<Favorite> {
    const raw = await this.fetch<unknown>(
      `/api/favorites/${itemType}/${itemId}`,
      { method: "PATCH", body: JSON.stringify({ category_id: categoryId }) },
    );
    return parseWithFallback(raw, FavoriteResponseSchema, EMPTY_FAVORITE, {
      endpoint: "PATCH /api/favorites/{itemType}/{itemId}",
    });
  }

  async deleteFavorite(
    itemType: FavoriteType,
    itemId: string,
  ): Promise<void> {
    await this.fetch(`/api/favorites/${itemType}/${itemId}`, {
      method: "DELETE",
    });
  }

  async deleteAttachment(id: string): Promise<void> {
    await this.fetch(`/api/attachments/${id}`, { method: "DELETE" });
  }

  // Fetches the raw bytes of a text-previewable attachment.
  //
  // The endpoint sidesteps CloudFront CORS (not configured on the CDN) and
  // bypasses Content-Disposition: attachment for the `text/*` family, both
  // of which would otherwise prevent the renderer from getting the body.
  // The server always replies with `text/plain; charset=utf-8` for safety;
  // the original MIME ships back in the `X-Original-Content-Type` header so
  // the preview dispatcher can choose between markdown / html / plain code.
  //
  // Routes through `fetchRaw` so it inherits the standard auth headers,
  // 401 → handleUnauthorized recovery, request-id logging, and ApiError
  // shape. 413 / 415 are translated to typed `Preview*Error` instances so
  // the modal can render specific fallbacks instead of generic failure.
  async getAttachmentTextContent(
    id: string,
  ): Promise<{ text: string; originalContentType: string }> {
    let res: Response;
    try {
      res = await this.fetchRaw(`/api/attachments/${id}/content`);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 413) throw new PreviewTooLargeError();
        if (err.status === 415) throw new PreviewUnsupportedError();
      }
      throw err;
    }
    return {
      text: await res.text(),
      originalContentType: res.headers.get("X-Original-Content-Type") ?? "",
    };
  }

  // Projects
  async listProjects(params?: { status?: string; milestone_id?: string }): Promise<ListProjectsResponse> {
    const search = new URLSearchParams();
    if (params?.status) search.set("status", params.status);
    if (params?.milestone_id) search.set("milestone_id", params.milestone_id);
    return this.fetch(`/api/projects?${search}`);
  }

  async getProject(id: string): Promise<Project> {
    return this.fetch(`/api/projects/${id}`);
  }

  async createProject(data: CreateProjectRequest): Promise<Project> {
    return this.fetch("/api/projects", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateProject(id: string, data: UpdateProjectRequest): Promise<Project> {
    return this.fetch(`/api/projects/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteProject(id: string): Promise<void> {
    await this.fetch(`/api/projects/${id}`, { method: "DELETE" });
  }

  // Milestones
  async listMilestones(params?: { status?: string }): Promise<ListMilestonesResponse> {
    const search = new URLSearchParams();
    if (params?.status) search.set("status", params.status);
    return this.fetch(`/api/milestones?${search}`);
  }

  async getMilestone(id: string): Promise<Milestone> {
    return this.fetch(`/api/milestones/${id}`);
  }

  async createMilestone(data: CreateMilestoneRequest): Promise<Milestone> {
    return this.fetch("/api/milestones", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateMilestone(id: string, data: UpdateMilestoneRequest): Promise<Milestone> {
    return this.fetch(`/api/milestones/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteMilestone(id: string): Promise<void> {
    await this.fetch(`/api/milestones/${id}`, { method: "DELETE" });
  }

  async listKpiMetrics(workspaceId?: string): Promise<ListKpiMetricsResponse> {
    return this.fetch(`/api/kpi-metrics${workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : ""}`);
  }

  async getKpiMetric(id: string, workspaceId?: string): Promise<KpiMetric> {
    return this.fetch(`/api/kpi-metrics/${id}${workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : ""}`);
  }

  async createKpiMetric(data: CreateKpiMetricRequest, workspaceId?: string): Promise<KpiMetric> {
    return this.fetch(`/api/kpi-metrics${workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : ""}`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateKpiMetric(id: string, data: UpdateKpiMetricRequest, workspaceId?: string): Promise<KpiMetric> {
    return this.fetch(`/api/kpi-metrics/${id}${workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : ""}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteKpiMetric(id: string, workspaceId?: string): Promise<void> {
    await this.fetch(`/api/kpi-metrics/${id}${workspaceId ? `?workspace_id=${encodeURIComponent(workspaceId)}` : ""}`, { method: "DELETE" });
  }

  // Project resources
  async listProjectResources(
    projectId: string,
  ): Promise<ListProjectResourcesResponse> {
    return this.fetch(`/api/projects/${projectId}/resources`);
  }

  async createProjectResource(
    projectId: string,
    data: CreateProjectResourceRequest,
  ): Promise<ProjectResource> {
    return this.fetch(`/api/projects/${projectId}/resources`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateProjectResource(
    projectId: string,
    resourceId: string,
    data: UpdateProjectResourceRequest,
  ): Promise<ProjectResource> {
    return this.fetch(`/api/projects/${projectId}/resources/${resourceId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteProjectResource(
    projectId: string,
    resourceId: string,
  ): Promise<void> {
    await this.fetch(`/api/projects/${projectId}/resources/${resourceId}`, {
      method: "DELETE",
    });
  }

  // Labels
  async listLabels(params?: { resource_type?: LabelResourceType }): Promise<ListLabelsResponse> {
    const search = new URLSearchParams();
    if (params?.resource_type) search.set("resource_type", params.resource_type);
    const qs = search.toString();
    return this.fetch(`/api/labels${qs ? `?${qs}` : ""}`);
  }

  async getLabel(id: string): Promise<Label> {
    return this.fetch(`/api/labels/${id}`);
  }

  async createLabel(data: CreateLabelRequest): Promise<Label> {
    return this.fetch(`/api/labels`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateLabel(id: string, data: UpdateLabelRequest): Promise<Label> {
    return this.fetch(`/api/labels/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteLabel(id: string): Promise<void> {
    await this.fetch(`/api/labels/${id}`, { method: "DELETE" });
  }

  async listLabelsForIssue(issueId: string): Promise<IssueLabelsResponse> {
    return this.fetch(`/api/issues/${issueId}/labels`);
  }

  async attachLabel(issueId: string, labelId: string): Promise<IssueLabelsResponse> {
    return this.fetch(`/api/issues/${issueId}/labels`, {
      method: "POST",
      body: JSON.stringify({ label_id: labelId }),
    });
  }

  async detachLabel(issueId: string, labelId: string): Promise<IssueLabelsResponse> {
    return this.fetch(`/api/issues/${issueId}/labels/${labelId}`, {
      method: "DELETE",
    });
  }

  async listLabelsForProject(projectId: string): Promise<ProjectLabelsResponse> {
    return this.fetch(`/api/projects/${projectId}/labels`);
  }

  async attachProjectLabel(projectId: string, labelId: string): Promise<ProjectLabelsResponse> {
    return this.fetch(`/api/projects/${projectId}/labels`, {
      method: "POST",
      body: JSON.stringify({ label_id: labelId }),
    });
  }

  async detachProjectLabel(projectId: string, labelId: string): Promise<ProjectLabelsResponse> {
    return this.fetch(`/api/projects/${projectId}/labels/${labelId}`, {
      method: "DELETE",
    });
  }

  async listLabelsForAgent(agentId: string): Promise<AgentLabelsResponse> {
    return this.fetch(`/api/agents/${agentId}/labels`);
  }

  async attachAgentLabel(agentId: string, labelId: string): Promise<AgentLabelsResponse> {
    return this.fetch(`/api/agents/${agentId}/labels`, {
      method: "POST",
      body: JSON.stringify({ label_id: labelId }),
    });
  }

  async detachAgentLabel(agentId: string, labelId: string): Promise<AgentLabelsResponse> {
    return this.fetch(`/api/agents/${agentId}/labels/${labelId}`, {
      method: "DELETE",
    });
  }

  // Pins
  async listPins(): Promise<PinnedItem[]> {
    return this.fetch("/api/pins");
  }

  async createPin(data: CreatePinRequest): Promise<PinnedItem> {
    return this.fetch("/api/pins", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deletePin(itemType: PinnedItemType, itemId: string): Promise<void> {
    await this.fetch(`/api/pins/${itemType}/${itemId}`, { method: "DELETE" });
  }

  async reorderPins(data: ReorderPinsRequest): Promise<void> {
    await this.fetch("/api/pins/reorder", {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  // Squads
  async listSquads(): Promise<Squad[]> {
    const raw = await this.fetch<unknown>(`/api/squads`);
    return parseWithFallback(raw, SquadListSchema, EMPTY_SQUAD_LIST, {
      endpoint: "GET /api/squads",
    }) as Squad[];
  }

  async getSquad(id: string): Promise<Squad> {
    const raw = await this.fetch<unknown>(`/api/squads/${id}`);
    return parseWithFallback(raw, SquadSchema, EMPTY_SQUAD, {
      endpoint: "GET /api/squads/:id",
    }) as Squad;
  }

  async createSquad(data: { name: string; description?: string; leader_id: string; avatar_url?: string }): Promise<Squad> {
    const raw = await this.fetch<unknown>("/api/squads", { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, SquadSchema, EMPTY_SQUAD, {
      endpoint: "POST /api/squads",
    }) as Squad;
  }

  async updateSquad(id: string, data: { name?: string; description?: string; instructions?: string; leader_id?: string; avatar_url?: string }): Promise<Squad> {
    const raw = await this.fetch<unknown>(`/api/squads/${id}`, { method: "PUT", body: JSON.stringify(data) });
    return parseWithFallback(raw, SquadSchema, EMPTY_SQUAD, {
      endpoint: "PUT /api/squads/:id",
    }) as Squad;
  }

  async deleteSquad(id: string): Promise<void> {
    await this.fetch(`/api/squads/${id}`, { method: "DELETE" });
  }

  async listSquadMembers(squadId: string): Promise<SquadMember[]> {
    return this.fetch(`/api/squads/${squadId}/members`);
  }

  async addSquadMember(squadId: string, data: { member_type: string; member_id: string; role?: string }): Promise<SquadMember> {
    return this.fetch(`/api/squads/${squadId}/members`, { method: "POST", body: JSON.stringify(data) });
  }

  async removeSquadMember(squadId: string, data: { member_type: string; member_id: string }): Promise<void> {
    await this.fetch(`/api/squads/${squadId}/members`, { method: "DELETE", body: JSON.stringify(data) });
  }

  async updateSquadMemberRole(squadId: string, data: { member_type: string; member_id: string; role: string }): Promise<SquadMember> {
    return this.fetch(`/api/squads/${squadId}/members/role`, { method: "PATCH", body: JSON.stringify(data) });
  }

  // Per-squad members status snapshot: one row per member with derived
  // working/idle/offline/unstable plus the issues each agent is currently
  // running. Parsed with a lenient schema so a new server-side status
  // value or extra field can't white-screen the Squad page (#2143).
  async getSquadMemberStatus(squadId: string): Promise<SquadMemberStatusListResponse> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/members/status`);
    return parseWithFallback(raw, SquadMemberStatusListResponseSchema, EMPTY_SQUAD_MEMBER_STATUS_LIST, {
      endpoint: "GET /api/squads/:id/members/status",
    }) as SquadMemberStatusListResponse;
  }

  // Autopilots
  async listAutopilots(params?: { status?: string }): Promise<ListAutopilotsResponse> {
    const search = new URLSearchParams();
    if (params?.status) search.set("status", params.status);
    const raw = await this.fetch<unknown>(`/api/autopilots?${search}`);
    return parseWithFallback(
      raw,
      ListAutopilotsResponseSchema,
      EMPTY_LIST_AUTOPILOTS_RESPONSE as ListAutopilotsResponse,
      { endpoint: "GET /api/autopilots" },
    );
  }

  async getAutopilot(id: string): Promise<GetAutopilotResponse> {
    return this.fetch(`/api/autopilots/${id}`);
  }

  async createAutopilot(data: CreateAutopilotRequest): Promise<Autopilot> {
    return this.fetch("/api/autopilots", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateAutopilot(id: string, data: UpdateAutopilotRequest): Promise<Autopilot> {
    return this.fetch(`/api/autopilots/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteAutopilot(id: string): Promise<void> {
    await this.fetch(`/api/autopilots/${id}`, { method: "DELETE" });
  }

  async triggerAutopilot(id: string): Promise<AutopilotRun> {
    return this.fetch(`/api/autopilots/${id}/trigger`, { method: "POST" });
  }

  async listAutopilotRuns(id: string, params?: { limit?: number; offset?: number }): Promise<ListAutopilotRunsResponse> {
    const search = new URLSearchParams();
    if (params?.limit) search.set("limit", params.limit.toString());
    if (params?.offset) search.set("offset", params.offset.toString());
    return this.fetch(`/api/autopilots/${id}/runs?${search}`);
  }

  // Returns a single run including its full trigger_payload. List responses
  // omit trigger_payload to keep them small (a webhook envelope can be
  // up to 256 KiB × limit rows), so the detail view fetches via this route.
  async getAutopilotRun(autopilotId: string, runId: string): Promise<AutopilotRun> {
    return this.fetch(`/api/autopilots/${autopilotId}/runs/${runId}`);
  }

  async createAutopilotTrigger(autopilotId: string, data: CreateAutopilotTriggerRequest): Promise<AutopilotTrigger> {
    return this.fetch(`/api/autopilots/${autopilotId}/triggers`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateAutopilotTrigger(autopilotId: string, triggerId: string, data: UpdateAutopilotTriggerRequest): Promise<AutopilotTrigger> {
    return this.fetch(`/api/autopilots/${autopilotId}/triggers/${triggerId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteAutopilotTrigger(autopilotId: string, triggerId: string): Promise<void> {
    await this.fetch(`/api/autopilots/${autopilotId}/triggers/${triggerId}`, { method: "DELETE" });
  }

  async rotateAutopilotTriggerWebhookToken(
    autopilotId: string,
    triggerId: string,
  ): Promise<AutopilotTrigger> {
    return this.fetch(
      `/api/autopilots/${autopilotId}/triggers/${triggerId}/rotate-webhook-token`,
      { method: "POST" },
    );
  }

  // Webhook deliveries — list is slim (no raw_body / selected_headers /
  // response_body); detail returns the full row. Both responses are parsed
  // through a lenient schema so an unknown server-side `status` /
  // `signature_status` value degrades to a generic row instead of dropping
  // the whole list.
  async listAutopilotDeliveries(
    autopilotId: string,
    params?: { limit?: number; offset?: number },
  ): Promise<ListWebhookDeliveriesResponse> {
    const search = new URLSearchParams();
    if (params?.limit) search.set("limit", params.limit.toString());
    if (params?.offset) search.set("offset", params.offset.toString());
    const raw = await this.fetch<unknown>(
      `/api/autopilots/${autopilotId}/deliveries?${search}`,
    );
    return parseWithFallback(
      raw,
      ListWebhookDeliveriesResponseSchema,
      EMPTY_LIST_WEBHOOK_DELIVERIES_RESPONSE,
      { endpoint: "GET /api/autopilots/:id/deliveries" },
    );
  }

  async getAutopilotDelivery(
    autopilotId: string,
    deliveryId: string,
  ): Promise<WebhookDelivery> {
    const raw = await this.fetch<unknown>(
      `/api/autopilots/${autopilotId}/deliveries/${deliveryId}`,
    );
    return parseWithFallback(
      raw,
      WebhookDeliveryResponseSchema,
      { ...EMPTY_WEBHOOK_DELIVERY, id: deliveryId, autopilot_id: autopilotId },
      { endpoint: "GET /api/autopilots/:id/deliveries/:deliveryId" },
    );
  }

  // Replay creates a NEW delivery row referencing the original via
  // `replayed_from_delivery_id`. Server rejects replays of
  // signature-invalid / rejected deliveries with 400 — the UI keeps the
  // button disabled for those rows, but the server is the source of truth.
  async replayAutopilotDelivery(
    autopilotId: string,
    deliveryId: string,
  ): Promise<WebhookDelivery> {
    const raw = await this.fetch<unknown>(
      `/api/autopilots/${autopilotId}/deliveries/${deliveryId}/replay`,
      { method: "POST" },
    );
    return parseWithFallback(
      raw,
      WebhookDeliveryResponseSchema,
      { ...EMPTY_WEBHOOK_DELIVERY, autopilot_id: autopilotId },
      { endpoint: "POST /api/autopilots/:id/deliveries/:deliveryId/replay" },
    );
  }

  // Credential broker
  async listCredentialConnectors(): Promise<ListCredentialConnectorsResponse> {
    const raw = await this.fetch<unknown>("/api/credential-connectors");
    return parseWithFallback(
      raw,
      ListCredentialConnectorsResponseSchema,
      EMPTY_LIST_CREDENTIAL_CONNECTORS_RESPONSE,
      { endpoint: "GET /api/credential-connectors" },
    );
  }

  async listCredentialProfiles(): Promise<ListCredentialProfilesResponse> {
    const raw = await this.fetch<unknown>("/api/credential-profiles");
    return parseWithFallback(
      raw,
      ListCredentialProfilesResponseSchema,
      EMPTY_LIST_CREDENTIAL_PROFILES_RESPONSE,
      { endpoint: "GET /api/credential-profiles" },
    );
  }

  async startCredentialLoginSession(
    data: StartCredentialLoginSessionRequest,
  ): Promise<StartCredentialLoginSessionResponse> {
    const raw = await this.fetch<unknown>("/api/credential-login-sessions", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(
      raw,
      StartCredentialLoginSessionResponseSchema,
      EMPTY_START_CREDENTIAL_LOGIN_SESSION_RESPONSE,
      { endpoint: "POST /api/credential-login-sessions" },
    );
  }

  async deleteCredentialProfile(profileId: string): Promise<void> {
    const raw = await this.fetch<unknown>(`/api/credential-profiles/${profileId}`, {
      method: "DELETE",
    });
    parseWithFallback(raw, CredentialProfileSchema, null, {
      endpoint: "DELETE /api/credential-profiles/:id",
    });
  }

  async runCredentialCrawl(data: RunCredentialCrawlRequest): Promise<CredentialCrawlResult> {
    const raw = await this.fetch<unknown>("/api/credential-crawl", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(
      raw,
      CredentialCrawlResultSchema,
      EMPTY_CREDENTIAL_CRAWL_RESULT,
      { endpoint: "POST /api/credential-crawl" },
    );
  }

  async getWorkspaceCapabilities(workspaceId: string): Promise<WorkspaceCapabilitiesResponse> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/capabilities`);
    return parseWithFallback(raw, WorkspaceCapabilitiesSchema, EMPTY_WORKSPACE_CAPABILITIES, {
      endpoint: `GET /api/workspaces/${workspaceId}/capabilities`,
    });
  }

  async updateWorkspaceCapability(workspaceId: string, key: string, enabled: boolean, creativeFactory?: CreativeFactoryInitializationRequest): Promise<WorkspaceCapability> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/capabilities/${encodeURIComponent(key)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled, ...(creativeFactory ? { creative_factory: creativeFactory } : {}) }),
    });
    return parseWithFallback(raw, WorkspaceCapabilitySchema, EMPTY_WORKSPACE_CAPABILITY, {
      endpoint: `PATCH /api/workspaces/${workspaceId}/capabilities/${key}`,
    });
  }

  async addCredentialProfileManager(
    profileId: string,
    data: AddCredentialProfileManagerRequest,
  ): Promise<CredentialProfile> {
    const raw = await this.fetch<unknown>(`/api/credential-profiles/${profileId}/managers`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CredentialProfileSchema, {
      id: profileId,
      connector_id: "",
      label: "",
      status: "pending",
      scope: "workspace",
      can_manage: false,
      managers: [],
      created_at: "",
      updated_at: "",
    }, {
      endpoint: "POST /api/credential-profiles/:id/managers",
    });
  }

  async deleteCredentialProfileManager(profileId: string, userId: string): Promise<void> {
    await this.fetch<unknown>(
      `/api/credential-profiles/${profileId}/managers/${userId}`,
      { method: "DELETE" },
    );
  }

  async retryFailedAgentTasksBySource(
    agentId: string,
    triggerEvidenceKind: string,
    triggerEvidenceRefId: string,
  ): Promise<AgentTaskFanoutResponse> {
    const params = new URLSearchParams({
      trigger_evidence_kind: triggerEvidenceKind,
      trigger_evidence_ref_id: triggerEvidenceRefId,
    });
    const raw = await this.fetch<unknown>(
      `/api/agents/${encodeURIComponent(agentId)}/tasks/by-source/retry-failed?${params.toString()}`,
      { method: "POST" },
    );
    return parseWithFallback(raw, AgentTaskFanoutResponseSchema, EMPTY_AGENT_TASK_FANOUT_RESPONSE, {
      endpoint: "POST /api/agents/{agentId}/tasks/by-source/retry-failed",
    });
  }

  // Creative Studio resources
  async listCreativeMaterialLibrary(params?: CreativeMaterialLibraryQuery, signal?: AbortSignal): Promise<CreativeMaterialLibraryResponse> {
    const search = new URLSearchParams();
    if (params?.runId) search.set("run_id", params.runId);
    if (params?.includeEmptyRuns === false) search.set("include_empty_runs", "false");
    if (params?.limit !== undefined) search.set("limit", String(params.limit));
    if (params?.offset !== undefined) search.set("offset", String(params.offset));
    if (params?.query) search.set("query", params.query);
    if (params?.competitor) search.set("competitor", params.competitor);
    if (params?.area) search.set("area", params.area);
    if (params?.language) search.set("language", params.language);
    if (params?.media) search.set("media", params.media);
    if (params?.assetType) search.set("asset_type", params.assetType);
    if (params?.view && params.view !== "all") search.set("view", params.view);
    if (params?.sort && params.sort !== "recent") search.set("sort", params.sort);
    const query = search.toString();
    const raw = await this.fetch<unknown>(`/api/creative/materials${query ? `?${query}` : ""}`, { signal });
    return parseWithFallback(raw, CreativeMaterialLibrarySchema, EMPTY_CREATIVE_MATERIAL_LIBRARY, {
      endpoint: "GET /api/creative/materials",
    });
  }

  async importCreativeMaterialLibrary(
    data: ImportCreativeMaterialLibraryRequest,
  ): Promise<CreativeMaterialImportResult> {
    const raw = await this.fetch<unknown>("/api/creative/materials/import", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(
      raw,
      CreativeMaterialImportResultSchema,
      EMPTY_CREATIVE_MATERIAL_IMPORT_RESULT,
      { endpoint: "POST /api/creative/materials/import" },
    );
  }

  async retryCreativeMaterialReferenceAnalysis(id: string): Promise<CreativeMaterialImportResult> {
    const raw = await this.fetch<unknown>(`/api/creative/materials/${encodeURIComponent(id)}/analysis/retry`, {
      method: "POST",
    });
    return parseWithFallback(
      raw,
      CreativeMaterialImportResultSchema,
      EMPTY_CREATIVE_MATERIAL_IMPORT_RESULT,
      { endpoint: "POST /api/creative/materials/:id/analysis/retry" },
    );
  }

  async retryCreativeMaterialArchives(candidateIds: string[] = []): Promise<{ scheduled_count: number }> {
		return this.fetch<{ scheduled_count: number }>("/api/creative/materials/archive/retry", {
			method: "POST",
			body: JSON.stringify({ candidate_ids: candidateIds }),
		});
	}

  async listCreativeResources(kind?: CreativeResourceKind, signal?: AbortSignal): Promise<CreativeResourceListResponse> {
    const query = kind ? `?kind=${encodeURIComponent(kind)}` : "";
    const raw = await this.fetch<unknown>(`/api/creative/resources${query}`, { signal });
    return parseWithFallback(raw, CreativeResourceListSchema, EMPTY_CREATIVE_RESOURCE_LIST, {
      endpoint: "GET /api/creative/resources",
    });
  }

  async createCreativeResource(data: CreateCreativeResourceRequest): Promise<CreativeResource> {
    const raw = await this.fetch<unknown>("/api/creative/resources", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreativeResourceSchema, EMPTY_CREATIVE_RESOURCE, {
      endpoint: "POST /api/creative/resources",
    });
  }

  async updateCreativeResource(id: string, data: UpdateCreativeResourceRequest): Promise<CreativeResource> {
    const raw = await this.fetch<unknown>(`/api/creative/resources/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreativeResourceSchema, EMPTY_CREATIVE_RESOURCE, {
      endpoint: "PUT /api/creative/resources/:id",
    });
  }

  async publishCreativeResource(id: string): Promise<CreativeResource> {
    const raw = await this.fetch<unknown>(`/api/creative/resources/${id}/publish`, { method: "POST" });
    return parseWithFallback(raw, CreativeResourceSchema, EMPTY_CREATIVE_RESOURCE, {
      endpoint: "POST /api/creative/resources/:id/publish",
    });
  }

  async archiveCreativeResource(id: string): Promise<void> {
    await this.fetch<void>(`/api/creative/resources/${id}`, { method: "DELETE" });
  }

  async listCreativeResourceFiles(id: string, signal?: AbortSignal): Promise<CreativeResourceFileListResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/resources/${id}/files`, { signal });
    return parseWithFallback(raw, CreativeResourceFileListSchema, EMPTY_CREATIVE_RESOURCE_FILE_LIST, {
      endpoint: "GET /api/creative/resources/:id/files",
    });
  }

  async addCreativeResourceFile(
    id: string,
    data: { attachment_id: string; role: string; label?: string; metadata?: Record<string, unknown> },
  ): Promise<CreativeResourceFile> {
    const raw = await this.fetch<unknown>(`/api/creative/resources/${id}/files`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreativeResourceFileSchema, EMPTY_CREATIVE_RESOURCE_FILE, {
      endpoint: "POST /api/creative/resources/:id/files",
    });
  }

  async removeCreativeResourceFile(id: string, fileId: string): Promise<void> {
    await this.fetch<void>(`/api/creative/resources/${id}/files/${fileId}`, { method: "DELETE" });
  }

  async updateCreativeResourceFile(
    id: string,
    fileId: string,
    data: { attachment_id?: string; role: string; label?: string; metadata?: Record<string, unknown> },
  ): Promise<CreativeResourceFile> {
    const raw = await this.fetch<unknown>(`/api/creative/resources/${id}/files/${fileId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreativeResourceFileSchema, EMPTY_CREATIVE_RESOURCE_FILE, {
      endpoint: "PUT /api/creative/resources/:id/files/:fileId",
    });
  }

  // Creative issue workflow
  async getCreativeMaterials(issueId: string, signal?: AbortSignal): Promise<CreativeMaterialsResponse> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/creative-materials`, { signal });
    return parseWithFallback(
      raw,
      CreativeMaterialsResponseSchema,
      EMPTY_CREATIVE_MATERIALS_RESPONSE,
      { endpoint: "GET /api/issues/:id/creative-materials" },
    );
  }

  async importCreativeMaterials(
    issueId: string,
    data: ImportCreativeMaterialsRequest,
  ): Promise<CreativeImportSummary> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/creative-materials/import`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(
      raw,
      CreativeImportSummarySchema,
      EMPTY_CREATIVE_IMPORT_SUMMARY,
      { endpoint: "POST /api/issues/:id/creative-materials/import" },
    );
  }

  async updateCreativeMaterialCandidate(
    issueId: string,
    candidateId: string,
    data: UpdateCreativeMaterialCandidateRequest,
  ): Promise<CreativeMaterialsResponse> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/creative-materials/${candidateId}`,
      {
        method: "PATCH",
        body: JSON.stringify(data),
      },
    );
    return parseWithFallback(
      raw,
      CreativeMaterialsResponseSchema,
      EMPTY_CREATIVE_MATERIALS_RESPONSE,
      { endpoint: "PATCH /api/issues/:id/creative-materials/:candidateId" },
    );
  }

  async createCreativeFeedback(data: CreateCreativeFeedbackRequest): Promise<CreateCreativeFeedbackResponse> {
    const raw = await this.fetch<unknown>("/api/creative-feedback-events", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreateCreativeFeedbackResponseSchema, EMPTY_CREATIVE_FEEDBACK_RESPONSE, {
      endpoint: "POST /api/creative-feedback-events",
    });
  }

  async listCreativeFeedback(subjectType?: string, subjectId?: string, signal?: AbortSignal): Promise<CreativeFeedbackEventListResponse> {
    const params = new URLSearchParams();
    if (subjectType) params.set("subject_type", subjectType);
    if (subjectId) params.set("subject_id", subjectId);
    const query = params.size > 0 ? `?${params.toString()}` : "";
    const raw = await this.fetch<unknown>(`/api/creative-feedback-events${query}`, { signal });
    return parseWithFallback(raw, CreativeFeedbackEventListResponseSchema, EMPTY_CREATIVE_FEEDBACK_EVENT_LIST_RESPONSE, {
      endpoint: "GET /api/creative-feedback-events",
    });
  }

  async getCreativeFeedbackMetrics(signal?: AbortSignal): Promise<CreativeFeedbackMetrics> {
    const raw = await this.fetch<unknown>("/api/creative-feedback-events/metrics", { signal });
    return parseWithFallback(raw, CreativeFeedbackMetricsSchema, EMPTY_CREATIVE_FEEDBACK_METRICS, {
      endpoint: "GET /api/creative-feedback-events/metrics",
    });
  }

  async getCreativeFeedbackDashboard(signal?: AbortSignal): Promise<CreativeFeedbackDashboard> {
    const raw = await this.fetch<unknown>("/api/creative-feedback-events/dashboard", { signal });
    return parseWithFallback(raw, CreativeFeedbackDashboardSchema, EMPTY_CREATIVE_FEEDBACK_DASHBOARD, {
      endpoint: "GET /api/creative-feedback-events/dashboard",
    });
  }

  async undoCreativeFeedback(id: string): Promise<CreateCreativeFeedbackResponse> {
    const raw = await this.fetch<unknown>(`/api/creative-feedback-events/${id}/undo`, { method: "POST" });
    return parseWithFallback(raw, CreateCreativeFeedbackResponseSchema, EMPTY_CREATIVE_FEEDBACK_RESPONSE, {
      endpoint: "POST /api/creative-feedback-events/:id/undo",
    });
  }

  async confirmCreativeGalleryDelivery(data: { variant_id: string; revision: number; idempotency_key: string; qc_risk_acknowledged: boolean; qc_risk_reason: string }): Promise<CreateCreativeFeedbackResponse> {
    const raw = await this.fetch<unknown>("/api/creative-feedback-events/gallery", { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, CreateCreativeFeedbackResponseSchema, EMPTY_CREATIVE_FEEDBACK_RESPONSE, { endpoint: "POST /api/creative-feedback-events/gallery" });
  }

  async listCreativeOrders(signal?: AbortSignal): Promise<CreativeOrderListResponse> {
    const raw = await this.fetch<unknown>("/api/creative/orders", { signal });
    return parseWithFallback(raw, CreativeOrderListResponseSchema, EMPTY_CREATIVE_ORDER_LIST_RESPONSE, { endpoint: "GET /api/creative/orders" });
  }

  async getCreativeOrder(id: string, signal?: AbortSignal): Promise<CreativeOrder> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${id}`, { signal });
    return parseWithFallback(raw, CreativeOrderSchema, { id: "", workspace_id: "", issue_id: "", status: "draft", derived_status: "draft", delivery_status: "pending", production_status: "pending", input_snapshot: {}, trigger_evidence_kind: "", trigger_evidence_ref_id: "", created_by: "", created_at: "", updated_at: "", workflow_failures: [], items: [] }, { endpoint: "GET /api/creative/orders/:id" });
  }

  async retryCreativeOrderWorkflowFailure(orderId: string, taskId: string): Promise<CreativeOrderWorkflowRetryResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/workflow-failures/${encodeURIComponent(taskId)}/retry`, {
      method: "POST",
    });
    return parseWithFallback(raw, CreativeOrderWorkflowRetryResponseSchema, { task_id: "" }, {
      endpoint: "POST /api/creative/orders/:id/workflow-failures/:taskId/retry",
    });
  }

  async composeCreativeOrderPrime(orderId: string, variantId: string, options?: { async?: boolean }): Promise<CreativeOrderPrimeComposeResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/prime-compose`, {
      method: "POST",
      body: JSON.stringify({ variant_id: variantId, ...(options?.async ? { async: true } : {}) }),
    });
    return parseWithFallback(raw, CreativeOrderPrimeComposeResponseSchema, {
      variant_id: "", composed: false, completed: false,
    }, {
      endpoint: "POST /api/creative/orders/:id/prime-compose",
    });
  }

  async recoverCreativeOrderCandidates(orderId: string, itemId: string): Promise<CreativeOrderWorkflowRetryResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/items/${encodeURIComponent(itemId)}/candidate-recovery`, { method: "POST" });
    return parseWithFallback(raw, CreativeOrderWorkflowRetryResponseSchema, { task_id: "" }, { endpoint: "POST /api/creative/orders/:id/items/:itemId/candidate-recovery" });
  }

  async queueCreativeOrderAdjustment(orderId: string, data: QueueCreativeOrderAdjustmentRequest): Promise<QueueCreativeOrderAdjustmentResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/adjustments`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, QueueCreativeOrderAdjustmentResponseSchema, { task_id: "", revision: 1 }, {
      endpoint: "POST /api/creative/orders/:id/adjustments",
    });
  }

  async discardCreativeOrderVariantStaging(orderId: string, variantId: string): Promise<void> {
    await this.fetch(`/api/creative/orders/${encodeURIComponent(orderId)}/variants/${encodeURIComponent(variantId)}/staging/discard`, {
      method: "POST",
    });
  }

  async adoptCreativeOrderProcessImage(orderId: string, variantId: string, assetId: string): Promise<void> {
    await this.fetch(`/api/creative/orders/${encodeURIComponent(orderId)}/variants/${encodeURIComponent(variantId)}/process-images/${encodeURIComponent(assetId)}/adopt`, {
      method: "POST",
    });
  }

  async retryCreativeOrderVariantQC(orderId: string, variantId: string): Promise<CreativeOrderQCRetryResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/variants/${encodeURIComponent(variantId)}/qc/retry`, {
      method: "POST",
    });
    return parseWithFallback(raw, CreativeOrderQCRetryResponseSchema, {
      variant_id: "", revision: 1, attempt: 1, technical_task_id: "", visual_task_id: "",
    }, {
      endpoint: "POST /api/creative/orders/:id/variants/:variantId/qc/retry",
    });
  }

  async cancelCreativeOrder(id: string): Promise<CreativeOrder> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(id)}/cancel`, { method: "POST" });
    return parseWithFallback(raw, CreativeOrderSchema, { id: "", workspace_id: "", issue_id: "", status: "cancelled", derived_status: "cancelled", delivery_status: "cancelled", production_status: "cancelled", input_snapshot: {}, trigger_evidence_kind: "", trigger_evidence_ref_id: "", created_by: "", created_at: "", updated_at: "", workflow_failures: [], items: [] }, { endpoint: "POST /api/creative/orders/:id/cancel" });
  }

  async deleteCreativeOrder(id: string): Promise<void> {
    await this.fetch(`/api/creative/orders/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  async createCreativeOrder(data: CreateCreativeOrderRequest): Promise<CreativeOrder> {
    const raw = await this.fetch<unknown>("/api/creative/orders", { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, CreativeOrderSchema, { id: "", workspace_id: "", issue_id: "", status: "draft", derived_status: "draft", delivery_status: "pending", production_status: "pending", input_snapshot: {}, trigger_evidence_kind: "", trigger_evidence_ref_id: "", created_by: "", created_at: "", updated_at: "", workflow_failures: [], items: [] }, { endpoint: "POST /api/creative/orders" });
  }

  async adoptCreativeOrderVariant(orderId: string, itemId: string, data: AdoptCreativeOrderVariantRequest): Promise<CreativeOrderItem> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/items/${encodeURIComponent(itemId)}/adoption`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreativeOrderItemSchema, EMPTY_CREATIVE_ORDER_ITEM, {
      endpoint: "POST /api/creative/orders/:orderId/items/:itemId/adoption",
    });
  }

  async unadoptCreativeOrderVariant(orderId: string, itemId: string): Promise<CreativeOrderItem> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${encodeURIComponent(orderId)}/items/${encodeURIComponent(itemId)}/adoption`, {
      method: "DELETE",
    });
    return parseWithFallback(raw, CreativeOrderItemSchema, EMPTY_CREATIVE_ORDER_ITEM, {
      endpoint: "DELETE /api/creative/orders/:orderId/items/:itemId/adoption",
    });
  }

  async selectCreativeOrderVariantRevision(orderId: string, variantId: string, revision: number): Promise<void> {
    await this.fetch(`/api/creative/orders/${encodeURIComponent(orderId)}/variants/${encodeURIComponent(variantId)}/revisions/${encodeURIComponent(String(revision))}/select`, {
      method: "POST",
    });
  }

  async createCreativeDirectEdit(data: CreateCreativeDirectEditRequest): Promise<CreativeDirectEditResponse> {
    const raw = await this.fetch<unknown>("/api/creative/direct-edits", { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, CreativeDirectEditResponseSchema, EMPTY_CREATIVE_DIRECT_EDIT_RESPONSE, {
      endpoint: "POST /api/creative/direct-edits",
    });
  }

  async finalizeCreativeOrderQC(orderId: string, variantId: string, revision: number): Promise<CreativeOrderQCFinalizeResponse> {
    const raw = await this.fetch<unknown>(`/api/creative/orders/${orderId}/qc-finalize`, {
      method: "POST",
      body: JSON.stringify({ variant_id: variantId, revision }),
    });
    return parseWithFallback(raw, CreativeOrderQCFinalizeResponseSchema, EMPTY_CREATIVE_ORDER_QC_FINALIZE_RESPONSE, {
      endpoint: "POST /api/creative/orders/:id/qc-finalize",
    });
  }

  async listCreativeSourceAnalyses(candidateId?: string, signal?: AbortSignal) {
    const query = candidateId ? `?candidate_id=${encodeURIComponent(candidateId)}` : "";
    const raw = await this.fetch<unknown>(`/api/creative/source-analyses${query}`, { signal });
    return parseWithFallback(raw, CreativeSourceAnalysisListResponseSchema, EMPTY_CREATIVE_SOURCE_ANALYSIS_LIST_RESPONSE, { endpoint: "GET /api/creative/source-analyses" });
  }

  async retryCreativePreAdaptation(sourceAnalysisId: string, marketPackId: string): Promise<{ task_id: string; status: string }> {
    const raw = await this.fetch<unknown>(`/api/creative/source-analyses/${encodeURIComponent(sourceAnalysisId)}/pre-adaptation/retry`, {
      method: "POST",
      body: JSON.stringify({ market_pack_id: marketPackId }),
    });
    return parseWithFallback(raw, CreativePreAdaptationRetryResponseSchema, EMPTY_CREATIVE_PRE_ADAPTATION_RETRY_RESPONSE, {
      endpoint: "POST /api/creative/source-analyses/:id/pre-adaptation/retry",
    });
  }

  async putCreativeIssueContext(
    issueId: string,
    data: PutCreativeIssueContextRequest,
  ): Promise<CreativeIssueContext> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/creative-context`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CreativeIssueContextSchema, EMPTY_CREATIVE_ISSUE_CONTEXT, {
      endpoint: "PUT /api/issues/:id/creative-context",
    });
  }

  async putCreativeItemBrief(issueId: string, candidateId: string, creativeBrief: CreativeIssueItem["creative_brief"]): Promise<CreativeIssueItem> {
    const path = `/api/issues/${issueId}/creative-materials/${candidateId}/brief`;
    const parseItem = (raw: unknown) => parseWithFallback(raw, CreativeIssueItemSchema, EMPTY_CREATIVE_ISSUE_ITEM, {
      endpoint: "PUT /api/issues/:id/creative-materials/:candidateId/brief",
    });
    try {
      return parseItem(await this.fetch<unknown>(path, {
        method: "PUT",
        body: JSON.stringify(creativeBrief),
        suppressErrorLog: true,
      }));
    } catch (error) {
      if (!unsupportedCreativeBriefExtensionField(error)) {
        this.logger.error(`← brief save failed ${path}`, {
          error: error instanceof Error ? error.message : String(error),
        });
        throw error;
      }
      if (creativeBriefHasExtensionContent(creativeBrief)) {
        throw new Error("当前运行服务尚未支持保存补充创意想法或 App UI 引用；为避免丢失内容，本次未保存。请更新服务后重试。");
      }
      const {
        user_direction: _userDirection,
        app_ui_replacement_required: _appUIReplacementRequired,
        selected_app_ui_references: _selectedAppUIReferences,
        ...legacyBrief
      } = creativeBrief;
      return parseItem(await this.fetch<unknown>(path, {
        method: "PUT",
        body: JSON.stringify(legacyBrief),
      }));
    }
  }

  async putCreativeItemWorkIssue(issueId: string, candidateId: string, workIssueId: string): Promise<CreativeIssueItem> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/creative-materials/${candidateId}/work-issue`, {
      method: "PUT",
      body: JSON.stringify({ work_issue_id: workIssueId }),
    });
    return parseWithFallback(raw, CreativeIssueItemSchema, EMPTY_CREATIVE_ISSUE_ITEM, {
      endpoint: "PUT /api/issues/:id/creative-materials/:candidateId/work-issue",
    });
  }

  async registerCreativeDeliveries(
    issueId: string,
    data: RegisterCreativeDeliveriesRequest,
  ): Promise<RegisterCreativeDeliveriesResponse> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/creative-deliveries/register`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, RegisterCreativeDeliveriesResponseSchema, { deliveries: [] }, {
      endpoint: "POST /api/issues/:id/creative-deliveries/register",
    });
  }

  async createCreativeAdjustment(
    issueId: string,
    candidateId: string,
    data: CreateCreativeAdjustmentRequest,
  ): Promise<CreativeAdjustmentRequest> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/creative-materials/${candidateId}/adjustments`,
      { method: "POST", body: JSON.stringify(data) },
    );
    return parseWithFallback(raw, CreativeAdjustmentRequestSchema, EMPTY_CREATIVE_ADJUSTMENT_REQUEST, {
      endpoint: "POST /api/issues/:id/creative-materials/:candidateId/adjustments",
    });
  }

  async bindCreativeAdjustmentIssue(
    issueId: string,
    candidateId: string,
    adjustmentId: string,
    adjustmentIssueId: string,
  ): Promise<CreativeAdjustmentRequest> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/creative-materials/${candidateId}/adjustments/${adjustmentId}/issue`,
      { method: "PUT", body: JSON.stringify({ adjustment_issue_id: adjustmentIssueId }) },
    );
    return parseWithFallback(raw, CreativeAdjustmentRequestSchema, EMPTY_CREATIVE_ADJUSTMENT_REQUEST, {
      endpoint: "PUT /api/issues/:id/creative-materials/:candidateId/adjustments/:adjustmentId/issue",
    });
  }

  // GitHub integration
  async getGitHubConnectURL(workspaceId: string): Promise<GitHubConnectResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/github/connect`);
  }

  async listGitHubInstallations(workspaceId: string): Promise<ListGitHubInstallationsResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/github/installations`);
  }

  async deleteGitHubInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/github/installations/${installationId}`, {
      method: "DELETE",
    });
  }

  async listIssuePullRequests(issueId: string): Promise<{ pull_requests: GitHubPullRequest[] }> {
    return this.fetch(`/api/issues/${issueId}/pull-requests`);
  }

  // Lark integration
  async listLarkInstallations(workspaceId: string): Promise<ListLarkInstallationsResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/lark/installations`);
  }

  async beginLarkInstall(
    workspaceId: string,
    agentId: string,
    region: "feishu" | "lark",
  ): Promise<BeginLarkInstallResponse> {
    // The user picks the cloud explicitly in the UI ("Bind to Feishu"
    // vs "Bind to Lark"), and the backend POSTs the device-flow `begin`
    // against the corresponding accounts host (accounts.feishu.cn vs
    // accounts.larksuite.com) so the QR renders against the right
    // cloud up front. Empty / omitted region still resolves to Feishu
    // server-side (RegionOrDefault) — we surface region as a required
    // arg here so every call site is forced to make a deliberate
    // choice rather than silently defaulting to mainland.
    const search = new URLSearchParams({ agent_id: agentId, region });
    return this.fetch(`/api/workspaces/${workspaceId}/lark/install/begin?${search.toString()}`, {
      method: "POST",
    });
  }

  async getLarkInstallStatus(workspaceId: string, sessionId: string): Promise<LarkInstallStatusResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/lark/install/${sessionId}/status`);
  }

  async deleteLarkInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/lark/installations/${installationId}`, {
      method: "DELETE",
    });
  }

  async redeemLarkBindingToken(token: string): Promise<RedeemLarkBindingTokenResponse> {
    return this.fetch(`/api/lark/binding/redeem`, {
      method: "POST",
      body: JSON.stringify({ token }),
    });
  }

  async listComposioToolkits(): Promise<ComposioToolkit[]> {
    return this.fetch(`/api/integrations/composio/toolkits`);
  }

  async listComposioConnections(): Promise<ComposioConnection[]> {
    return this.fetch(`/api/integrations/composio/connections`);
  }

  async beginComposioConnect(toolkitSlug: string): Promise<ComposioConnectInitResponse> {
    return this.fetch(`/api/integrations/composio/connect/init`, {
      method: "POST",
      body: JSON.stringify({ toolkit_slug: toolkitSlug }),
    });
  }

  async deleteComposioConnection(connectionId: string): Promise<void> {
    await this.fetch(`/api/integrations/composio/connections/${connectionId}`, {
      method: "DELETE",
    });
  }
}
