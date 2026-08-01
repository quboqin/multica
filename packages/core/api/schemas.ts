import { z } from "zod";
import type {
  Agent,
  AgentTemplate,
  AgentTemplateSummary,
  Attachment,
  BillingBalance,
  BillingBatchesPage,
  BillingCheckoutSessionStatus,
  BillingPriceTier,
  BillingTopupsPage,
  BillingTransactionsPage,
  CancelTaskResponse,
  CreateAgentFromTemplateResponse,
  CreateBillingCheckoutSessionResponse,
  CreateBillingPortalSessionResponse,
  CreativeCopyEntry,
  CreativeCopyEntryListResponse,
  CreativeCopyImportResult,
	CreativeAdjustmentRequest,
	CreativeDelivery,
  CreativeImportSummary,
  CreativeIssueContext,
  CreativeIssueItem,
  CreativeMaterialLibraryResponse,
  CreativeMaterialsResponse,
  CreativeResource,
  CreativeResourceFile,
  CreativeResourceFileListResponse,
  CreativeResourceListResponse,
  CredentialCrawlResult,
  GroupedIssuesResponse,
  ListCredentialConnectorsResponse,
  ListCredentialProfilesResponse,
  ListIssuesResponse,
  ListWebhookDeliveriesResponse,
  StartCredentialLoginSessionResponse,
  PreviewSession,
  PreviewSessionListResponse,
  Squad,
  TimelineEntry,
  User,
  WebhookDelivery,
} from "../types";
import type { CloudRuntimeNode } from "../runtimes/cloud-runtime";

export interface AppConfigResponse {
  cdn_domain: string;
  // True when the CDN domain serves private content via time-bounded signed
  // URLs (CloudFront signing) — raw storage URLs on that domain are NOT
  // publicly fetchable and must not be used as native media sources
  // (MUL-3254). Older servers omit the field; treat that as false.
  cdn_signed?: boolean;
  allow_signup: boolean;
  google_client_id?: string;
  posthog_key?: string;
  posthog_host?: string;
  analytics_environment?: string;
  daemon_server_url?: string;
  daemon_app_url?: string;
  workspace_creation_disabled?: boolean;
}

// ---------------------------------------------------------------------------
// Schemas for the highest-risk API endpoints — those whose responses drive
// the issue detail page (timeline, comments, subscribers) and the issues
// list. These are the surfaces that white-screened in #2143 / #2147 / #2192.
//
// These schemas are intentionally LENIENT:
//   - String enums are stored as `z.string()` rather than `z.enum([...])`.
//     A new server-side enum value should render as a generic fallback in
//     the UI, never crash a `safeParse`.
//   - Optional fields are unioned with `null` and given fallbacks where
//     existing UI code already coerces them.
//   - Arrays default to `[]` so a missing `reactions` / `attachments` /
//     `entries` field doesn't take the page down.
//   - Every object schema ends with `.loose()` so unknown server-side
//     fields pass through unchanged. zod 4's `.object()` defaults to STRIP,
//     which would silently delete fields the schema didn't explicitly list
//     — fine while the TS type doesn't claim them, but the moment a future
//     PR adds a TS field without updating the schema, the cast `as T` lies
//     and the field shows up as `undefined` at runtime. `.loose()` removes
//     that synchronisation hazard.
//
// These schemas are deliberately not typed as `z.ZodType<TimelineEntry>` /
// `z.ZodType<Issue>` etc. — the strict TS types narrow string fields to
// literal unions, which would defeat the leniency above. `parseWithFallback`
// returns the parsed value cast to the caller-supplied `T`, so the strict
// type still flows out at the call site; the schema only guards shape.
// ---------------------------------------------------------------------------

const ReactionSchema = z.object({
  id: z.string(),
  comment_id: z.string(),
  actor_type: z.string(),
  actor_id: z.string(),
  emoji: z.string(),
  created_at: z.string(),
});

// Nested attachments embedded in timeline/comment responses stay lenient on
// purpose: a single malformed attachment must not knock the whole timeline
// into the fallback `[]`.
const AttachmentSchema = z.object({
  id: z.string(),
}).loose();

// Standalone attachment lookup (`GET /api/attachments/{id}`) is the source of
// truth for click-time download URLs. The two fields the download flow opens
// in a new tab — `download_url` and `url` — must be strings, otherwise we'd
// happily `window.open(undefined)`. `filename` gates the toast/title and is
// also enforced so a missing value falls back to the empty record below.
//
// `markdown_url` is parsed lenient: a server old enough to predate
// MUL-3192 omits the field, in which case the schema defaults it to "".
// Callers that need to persist a URL into markdown should go through the
// `useFileUpload` helper (which falls back to the legacy
// `attachmentDownloadPath` shape when `markdown_url` is empty), so the
// empty-string default does not silently break any persistence path.
export const AttachmentResponseSchema = z.object({
  id: z.string(),
  url: z.string(),
  download_url: z.string(),
  markdown_url: z.string().optional().default(""),
  filename: z.string(),
  chat_session_id: z.string().nullable().optional(),
  chat_message_id: z.string().nullable().optional(),
}).loose();

export const EMPTY_ATTACHMENT: Attachment = {
  id: "",
  workspace_id: "",
  issue_id: null,
  comment_id: null,
  chat_session_id: null,
  chat_message_id: null,
  uploader_type: "",
  uploader_id: "",
  filename: "",
  url: "",
  download_url: "",
  markdown_url: "",
  content_type: "",
  size_bytes: 0,
  created_at: "",
};

// All object schemas use `.loose()` so unknown server-side fields pass
// through unchanged. zod 4's `.object()` defaults to STRIP, which would
// silently drop new fields and surface as a "field neither showed up in
// the UI" mystery the next time the TS type adopted them but the schema
// wasn't updated in lock-step. `.loose()` removes that synchronisation
// hazard — the schema validates the shape it knows about and leaves the
// rest alone.
const TimelineEntrySchema = z.object({
  type: z.string(),
  id: z.string(),
  actor_type: z.string(),
  actor_id: z.string(),
  created_at: z.string(),
  action: z.string().optional(),
  details: z.record(z.string(), z.unknown()).optional(),
  content: z.string().optional(),
  parent_id: z.string().nullable().optional(),
  updated_at: z.string().optional(),
  comment_type: z.string().optional(),
  reactions: z.array(ReactionSchema).optional(),
  attachments: z.array(AttachmentSchema).optional(),
  coalesced_count: z.number().optional(),
}).loose();

// /timeline returns a flat array of TimelineEntry, oldest first. The
// previously cursor-paginated wrapper was removed (#1929) — at observed data
// sizes (p99 ~30 entries per issue) paged delivery only created bugs.
export const TimelineEntriesSchema = z.array(TimelineEntrySchema);

export const EMPTY_TIMELINE_ENTRIES: TimelineEntry[] = [];

const OptionalStringSchema = z.preprocess(
  (value) => (typeof value === "string" ? value : undefined),
  z.string().optional(),
);

const BooleanWithDefaultSchema = (fallback: boolean) =>
  z.preprocess(
    (value) => (typeof value === "boolean" ? value : undefined),
    z.boolean().default(fallback),
  );

export const AppConfigSchema = z.object({
  cdn_domain: z.string().default(""),
  cdn_signed: BooleanWithDefaultSchema(false),
  allow_signup: BooleanWithDefaultSchema(true),
  google_client_id: OptionalStringSchema,
  posthog_key: OptionalStringSchema,
  posthog_host: OptionalStringSchema,
  analytics_environment: OptionalStringSchema,
  daemon_server_url: OptionalStringSchema,
  daemon_app_url: OptionalStringSchema,
  workspace_creation_disabled: BooleanWithDefaultSchema(false).optional(),
}).loose();

export const EMPTY_APP_CONFIG: AppConfigResponse = {
  cdn_domain: "",
  cdn_signed: false,
  allow_signup: true,
  google_client_id: "",
  daemon_server_url: "",
  daemon_app_url: "",
  workspace_creation_disabled: false,
};

export const CommentSchema = z.object({
  id: z.string(),
  issue_id: z.string(),
  author_type: z.string(),
  author_id: z.string(),
  content: z.string(),
  type: z.string(),
  parent_id: z.string().nullable(),
  reactions: z.array(ReactionSchema).default([]),
  attachments: z.array(AttachmentSchema).default([]),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

export const CommentsListSchema = z.array(CommentSchema);

const CommentTriggerPreviewAgentSchema = z.object({
  id: z.string(),
  name: z.string().default(""),
  avatar_url: z.string().optional(),
  source: z.string().default(""),
  reason: z.string().default(""),
}).loose();

export const CommentTriggerPreviewSchema = z.object({
  agents: z.array(CommentTriggerPreviewAgentSchema).default([]),
}).loose();

// Metadata is primitive-only by API/DB contract. Stay lenient on shape:
// unknown keys land as `unknown` to a caller, but the field itself defaults
// to {} so consumers never need to nil-guard `issue.metadata`.
const IssueMetadataSchema = z.record(z.string(), z.union([z.string(), z.number(), z.boolean()])).default({});

export const IssueSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  number: z.number(),
  identifier: z.string(),
  title: z.string(),
  description: z.string().nullable(),
  status: z.string(),
  priority: z.string(),
  assignee_type: z.string().nullable(),
  assignee_id: z.string().nullable(),
  creator_type: z.string(),
  creator_id: z.string(),
  parent_issue_id: z.string().nullable(),
  project_id: z.string().nullable(),
  position: z.number(),
  start_date: z.string().nullable(),
  due_date: z.string().nullable(),
  metadata: IssueMetadataSchema,
  reactions: z.array(z.unknown()).optional(),
  labels: z.array(z.unknown()).optional(),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

export const ListIssuesResponseSchema = z.object({
  issues: z.array(IssueSchema).default([]),
  total: z.number().default(0),
}).loose();

export const EMPTY_LIST_ISSUES_RESPONSE: ListIssuesResponse = {
  issues: [],
  total: 0,
};

export const CredentialConnectorSchema = z.object({
  id: z.string().default(""),
  display_name: z.string().default(""),
  login_url: z.string().default(""),
  capabilities: z.array(z.string()).default([]),
}).loose();

export const CredentialProfileSchema = z.object({
  id: z.string().default(""),
  connector_id: z.string().default(""),
  label: z.string().default(""),
  status: z.string().default("pending"),
  last_used_at: z.string().nullable().optional(),
  expires_hint: z.string().nullable().optional(),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const CredentialLoginSessionSchema = z.object({
  id: z.string().default(""),
  profile_id: z.string().default(""),
  connector_id: z.string().default(""),
  browser_url: z.string().default(""),
  status: z.string().default("pending"),
  expires_at: z.string().default(""),
  created_at: z.string().default(""),
}).loose();

export const ListCredentialConnectorsResponseSchema = z.object({
  connectors: z.array(CredentialConnectorSchema).default([]),
}).loose();

export const ListCredentialProfilesResponseSchema = z.object({
  profiles: z.array(CredentialProfileSchema).default([]),
}).loose();

export const StartCredentialLoginSessionResponseSchema = z.object({
  profile: CredentialProfileSchema,
  session: CredentialLoginSessionSchema,
}).loose();

export const CredentialCrawlResultSchema = z.object({
  status: z.string().default(""),
  downloaded: z.number().default(0),
  output_prefix: z.string().default(""),
  message: z.string().default(""),
  raw: z.unknown().optional(),
}).loose();

export const EMPTY_LIST_CREDENTIAL_CONNECTORS_RESPONSE: ListCredentialConnectorsResponse = {
  connectors: [],
};

export const EMPTY_LIST_CREDENTIAL_PROFILES_RESPONSE: ListCredentialProfilesResponse = {
  profiles: [],
};

export const EMPTY_START_CREDENTIAL_LOGIN_SESSION_RESPONSE: StartCredentialLoginSessionResponse = {
  profile: {
    id: "",
    connector_id: "",
    label: "",
    status: "pending",
    created_at: "",
    updated_at: "",
  },
  session: {
    id: "",
    profile_id: "",
    connector_id: "",
    browser_url: "",
    status: "pending",
    expires_at: "",
    created_at: "",
  },
};

export const EMPTY_CREDENTIAL_CRAWL_RESULT: CredentialCrawlResult = {
  status: "",
  downloaded: 0,
  output_prefix: "",
  message: "",
};

const NullableNumberSchema = z.number().nullable().optional().transform((v) => v ?? null);
const NullableStringSchema = z.string().nullable().optional().transform((v) => v ?? null);

export const CreativeMaterialCandidateSchema = z.object({
  id: z.string().default(""),
  workspace_id: z.string().default(""),
  connector_id: z.string().default(""),
  external_id: z.string().default(""),
  dedupe_key: z.string().default(""),
  competitor: z.string().default(""),
  title: z.string().default(""),
  asset_type: z.string().default("unknown"),
  preview_url: z.string().default(""),
  resource_url: z.string().default(""),
  poster_url: z.string().default(""),
  original_url: z.string().default(""),
  archived_url: z.string().default(""),
  archive_status: z.string().default("pending"),
  archive_error: z.string().default(""),
  duration_days: NullableNumberSchema,
  impression_estimate: NullableNumberSchema,
  media_names: z.array(z.string()).default([]),
  area_names: z.array(z.string()).default([]),
  language_names: z.array(z.string()).default([]),
  platform_names: z.array(z.string()).default([]),
  status: z.string().default("new"),
  tags: z.array(z.string()).default([]),
  note: z.string().default(""),
  selected_at: NullableStringSchema,
  first_seen_at: z.string().default(""),
  last_seen_at: z.string().default(""),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
  source_attachment_id: z.string().default(""),
  raw: z.unknown().optional(),
}).loose();

export const CreativeMaterialLibrarySchema = z.object({
  candidates: z.array(CreativeMaterialCandidateSchema).default([]),
}).loose();

export const EMPTY_CREATIVE_MATERIAL_LIBRARY: CreativeMaterialLibraryResponse = { candidates: [] };
export const CreativeMaterialImportResultSchema = z.object({ id: z.string().default("") });
export const EMPTY_CREATIVE_MATERIAL_IMPORT_RESULT = { id: "" };

export const CreativeMaterialSummarySchema = z.object({
  total: z.number().default(0),
  new: z.number().default(0),
  selected: z.number().default(0),
  rejected: z.number().default(0),
  sent_to_edit: z.number().default(0),
  edited: z.number().default(0),
  approved: z.number().default(0),
  archived: z.number().default(0),
}).loose();

export const CreativeMaterialCrawlRunSchema = z.object({
  id: z.string().default(""),
  workspace_id: z.string().default(""),
  issue_id: z.string().default(""),
  connector_id: z.string().default(""),
  query_summary: z.string().default(""),
  status: z.string().default(""),
  imported_count: z.number().default(0),
  existing_count: z.number().default(0),
  total_count: z.number().default(0),
  created_at: z.string().default(""),
}).loose();

export const CreativeIssueContextSchema = z.object({
  issue_id: z.string().default(""),
  workspace_id: z.string().default(""),
  market_pack_id: z.string().default(""),
  squad_id: z.string().default(""),
  snapshot: z.record(z.string(), z.unknown()).default({}),
  updated_at: z.string().default(""),
}).loose();

export const CreativeBriefSchema = z.object({
  theme: z.string().default(""),
  theme_elements: z.array(z.string()).default([]),
  primary_benefit: z.string().default(""),
  secondary_benefits: z.array(z.string()).default([]),
  benefit_value: z.string().default(""),
  evidence: z.array(z.string()).default([]),
  detected_text: z.array(z.string()).default([]),
  visual_type: z.string().default(""),
  analysis_summary: z.string().default(""),
  status: z.string().default(""),
  source: z.string().default(""),
  confidence: z.number().min(0).max(1).nullable().default(null),
  analysis_issue_id: z.string().default(""),
}).loose();

export const CreativeIssueItemSchema = z.object({
  issue_id: z.string().default(""),
  candidate_id: z.string().default(""),
  copy_entry_id: z.string().default(""),
  copy_snapshot: z.record(z.string(), z.unknown()).default({}),
  creative_brief: CreativeBriefSchema.default({
    theme: "", theme_elements: [], primary_benefit: "", secondary_benefits: [],
    benefit_value: "", evidence: [], detected_text: [], visual_type: "",
    analysis_summary: "", status: "", source: "", confidence: null,
    analysis_issue_id: "",
  }),
  work_issue_id: z.string().default(""),
  revision: z.number().default(1),
  status: z.string().default("ready"),
  updated_at: z.string().default(""),
}).loose();

export const CreativeDeliverySchema = z.object({
  id: z.string().default(""),
  issue_id: z.string().default(""),
  candidate_id: z.string().default(""),
  work_issue_id: z.string().default(""),
  variant: z.number().int().min(1).max(3),
  size: z.enum(["1080x1080", "1200x628", "800x1000"]),
  revision: z.number().int().positive(),
  base_attachment_id: z.string().default(""),
  final_attachment_id: z.string().default(""),
  prime_evidence_attachment_id: z.string().default(""),
  qc_issue_id: z.string().default(""),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const CreativeAdjustmentRequestSchema = z.object({
  id: z.string().default(""),
  issue_id: z.string().default(""),
  candidate_id: z.string().default(""),
  work_issue_id: z.string().default(""),
  adjustment_issue_id: z.string().default(""),
  variant: z.number().int().min(1).max(3),
  scope: z.enum(["size", "variant"]),
  size: z.union([z.enum(["1080x1080", "1200x628", "800x1000"]), z.literal("")]),
  revision: z.number().int().positive(),
  instruction: z.string().default(""),
  target_attachment_ids: z.array(z.string()).default([]),
  base_attachment_ids: z.array(z.string()).default([]),
  status: z.string().default("pending"),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const RegisterCreativeDeliveriesResponseSchema = z.object({
  deliveries: z.array(CreativeDeliverySchema).default([]),
}).loose();

export const EMPTY_CREATIVE_DELIVERY: CreativeDelivery = {
  id: "", issue_id: "", candidate_id: "", work_issue_id: "", variant: 1,
  size: "1080x1080", revision: 1, base_attachment_id: "", final_attachment_id: "",
  prime_evidence_attachment_id: "", qc_issue_id: "", created_at: "", updated_at: "",
};

export const EMPTY_CREATIVE_ADJUSTMENT_REQUEST: CreativeAdjustmentRequest = {
  id: "", issue_id: "", candidate_id: "", work_issue_id: "", adjustment_issue_id: "",
  variant: 1, scope: "size", size: "1080x1080", revision: 1, instruction: "",
  target_attachment_ids: [], base_attachment_ids: [], status: "pending", created_at: "", updated_at: "",
};

export const CreativeMaterialsResponseSchema = z.object({
  enabled: z.boolean().default(false),
  summary: CreativeMaterialSummarySchema.default({
    total: 0,
    new: 0,
    selected: 0,
    rejected: 0,
    sent_to_edit: 0,
    edited: 0,
    approved: 0,
    archived: 0,
  }),
  candidates: z.array(CreativeMaterialCandidateSchema).default([]),
  crawl_runs: z.array(CreativeMaterialCrawlRunSchema).default([]),
  context: CreativeIssueContextSchema.nullable().optional().transform((value) => value ?? null),
  items: z.array(CreativeIssueItemSchema).default([]),
	deliveries: z.array(CreativeDeliverySchema).default([]),
	adjustments: z.array(CreativeAdjustmentRequestSchema).default([]),
}).loose();

export const EMPTY_CREATIVE_MATERIALS_RESPONSE: CreativeMaterialsResponse = {
  enabled: false,
  summary: {
    total: 0,
    new: 0,
    selected: 0,
    rejected: 0,
    sent_to_edit: 0,
    edited: 0,
    approved: 0,
    archived: 0,
  },
  candidates: [],
  crawl_runs: [],
  context: null,
  items: [],
	deliveries: [],
	adjustments: [],
};

export const CreativeResourceSchema = z.object({
  id: z.string().default(""),
  workspace_id: z.string().default(""),
  kind: z.enum(["copy_library", "market_pack"]).default("copy_library"),
  name: z.string().default(""),
  description: z.string().default(""),
  status: z.enum(["draft", "published", "archived"]).default("draft"),
  version: z.number().default(1),
  published_version: z.number().default(0),
  config: z.record(z.string(), z.unknown()).default({}),
  created_by: z.string().default(""),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const CreativeResourceListSchema = z.object({
  resources: z.array(CreativeResourceSchema).default([]),
}).loose();

export const CreativeResourceFileSchema = z.object({
  id: z.string().default(""),
  resource_id: z.string().default(""),
  attachment_id: z.string().default(""),
  role: z.string().default(""),
  label: z.string().default(""),
  metadata: z.record(z.string(), z.unknown()).default({}),
  filename: z.string().default(""),
  url: z.string().default(""),
  content_type: z.string().default("application/octet-stream"),
  size_bytes: z.number().default(0),
  created_version: z.number().default(1),
  created_at: z.string().default(""),
}).loose();

export const CreativeResourceFileListSchema = z.object({
  files: z.array(CreativeResourceFileSchema).default([]),
}).loose();

export const CreativeCopyEntrySchema = z.object({
  id: z.string().default(""),
  workspace_id: z.string().default(""),
  library_id: z.string().default(""),
  external_key: z.string().default(""),
  headline: z.string().default(""),
  subheadline: z.string().default(""),
  benefit: z.string().default(""),
  cta: z.string().default(""),
  legal_text: z.string().default(""),
  copy_role: z.string().default(""),
  market: z.string().default(""),
  locale: z.string().default(""),
  tags: z.array(z.string()).default([]),
  status: z.enum(["draft", "approved", "disabled"]).default("draft"),
  version: z.number().default(1),
  metadata: z.record(z.string(), z.unknown()).default({}),
  created_by: z.string().default(""),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const CreativeCopyEntryListSchema = z.object({
  entries: z.array(CreativeCopyEntrySchema).default([]),
}).loose();

export const CreativeCopyImportResultSchema = z.object({
  created: z.number().default(0),
  updated: z.number().default(0),
  skipped: z.number().default(0),
  resource: CreativeResourceSchema,
}).loose();

export const EMPTY_CREATIVE_RESOURCE: CreativeResource = {
  id: "", workspace_id: "", kind: "copy_library", name: "", description: "",
  status: "draft", version: 1, published_version: 0, config: {}, created_by: "",
  created_at: "", updated_at: "",
};

export const EMPTY_CREATIVE_RESOURCE_LIST: CreativeResourceListResponse = { resources: [] };
export const EMPTY_CREATIVE_RESOURCE_FILE: CreativeResourceFile = {
  id: "", resource_id: "", attachment_id: "", role: "", label: "", metadata: {},
  filename: "", url: "", content_type: "application/octet-stream", size_bytes: 0,
  created_version: 1, created_at: "",
};
export const EMPTY_CREATIVE_RESOURCE_FILE_LIST: CreativeResourceFileListResponse = { files: [] };
export const EMPTY_CREATIVE_COPY_ENTRY: CreativeCopyEntry = {
  id: "", workspace_id: "", library_id: "", external_key: "", headline: "",
  subheadline: "", benefit: "", cta: "", legal_text: "", copy_role: "",
  market: "", locale: "", tags: [], status: "draft", version: 1, metadata: {},
  created_by: "", created_at: "", updated_at: "",
};
export const EMPTY_CREATIVE_COPY_ENTRY_LIST: CreativeCopyEntryListResponse = { entries: [] };
export const EMPTY_CREATIVE_COPY_IMPORT_RESULT: CreativeCopyImportResult = {
  created: 0, updated: 0, skipped: 0, resource: EMPTY_CREATIVE_RESOURCE,
};
export const EMPTY_CREATIVE_ISSUE_CONTEXT: CreativeIssueContext = {
  issue_id: "", workspace_id: "", market_pack_id: "",
  squad_id: "", snapshot: {}, updated_at: "",
};
export const EMPTY_CREATIVE_ISSUE_ITEM: CreativeIssueItem = {
  issue_id: "", candidate_id: "", copy_entry_id: "", copy_snapshot: {},
  creative_brief: {
    theme: "", theme_elements: [], primary_benefit: "", secondary_benefits: [],
    benefit_value: "", evidence: [], detected_text: [], visual_type: "",
    analysis_summary: "", status: "", source: "", confidence: null,
    analysis_issue_id: "",
  },
  work_issue_id: "", revision: 1, status: "ready", updated_at: "",
};

export const CreativeImportSummarySchema = z.object({
  run_id: z.string().default(""),
  imported_count: z.number().default(0),
  existing_count: z.number().default(0),
  total_count: z.number().default(0),
  skipped_count: z.number().default(0),
}).loose();

export const EMPTY_CREATIVE_IMPORT_SUMMARY: CreativeImportSummary = {
  run_id: "",
  imported_count: 0,
  existing_count: 0,
  total_count: 0,
  skipped_count: 0,
};

const IssueAssigneeGroupSchema = z.object({
  id: z.string(),
  assignee_type: z.string().nullable(),
  assignee_id: z.string().nullable(),
  issues: z.array(IssueSchema).default([]),
  total: z.number().default(0),
}).loose();

export const GroupedIssuesResponseSchema = z.object({
  groups: z.array(IssueAssigneeGroupSchema).default([]),
}).loose();

export const EMPTY_GROUPED_ISSUES_RESPONSE: GroupedIssuesResponse = {
  groups: [],
};

const SubscriberSchema = z.object({
  issue_id: z.string(),
  user_type: z.string(),
  user_id: z.string(),
  reason: z.string(),
  created_at: z.string(),
}).loose();

export const SubscribersListSchema = z.array(SubscriberSchema);

export const ChildIssuesResponseSchema = z.object({
  issues: z.array(IssueSchema).default([]),
}).loose();

// Preview session responses are new, but desktop clients can outlive the
// server version they were built against. Keep enum-like values as strings,
// default display-only fields, and convert the wire's snake_case shape at the
// API boundary so the rest of TypeScript only sees camelCase.
const PreviewSessionWireSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  issue_id: z.string(),
  task_id: z.string().nullable().optional().default(null),
  platform: z.string().default("unknown"),
  provider: z.string().default("unknown"),
  title: z.string().default(""),
  preview_url: z.string().default(""),
  status: z.string().default("unknown"),
  creator_type: z.string().default(""),
  creator_id: z.string().default(""),
  error_message: z.string().nullable().optional().default(null),
  expires_at: z.string().nullable().optional().default(null),
  last_active_at: z.string().nullable().optional().default(null),
  lease_expires_at: z.string().nullable().optional().default(null),
  started_at: z.string().nullable().optional().default(null),
  stopped_at: z.string().nullable().optional().default(null),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const PreviewSessionSchema = PreviewSessionWireSchema.transform(
  (session): PreviewSession => ({
    id: session.id,
    workspaceId: session.workspace_id,
    issueId: session.issue_id,
    taskId: session.task_id,
    platform: session.platform,
    provider: session.provider,
    title: session.title,
    previewUrl: session.preview_url,
    status: session.status,
    creatorType: session.creator_type,
    creatorId: session.creator_id,
    errorMessage: session.error_message,
    expiresAt: session.expires_at,
    lastActiveAt: session.last_active_at,
    leaseExpiresAt: session.lease_expires_at,
    startedAt: session.started_at,
    stoppedAt: session.stopped_at,
    createdAt: session.created_at,
    updatedAt: session.updated_at,
  }),
);

export const PreviewSessionListResponseSchema = z.object({
  preview_sessions: z.array(PreviewSessionSchema).default([]),
  total: z.number().default(0),
}).loose().transform(
  (response): PreviewSessionListResponse => ({
    previewSessions: response.preview_sessions,
    total: response.total,
  }),
);

export const EMPTY_PREVIEW_SESSION: PreviewSession = {
  id: "",
  workspaceId: "",
  issueId: "",
  taskId: null,
  platform: "unknown",
  provider: "unknown",
  title: "",
  previewUrl: "",
  status: "unknown",
  creatorType: "",
  creatorId: "",
  errorMessage: null,
  expiresAt: null,
  lastActiveAt: null,
  leaseExpiresAt: null,
  startedAt: null,
  stoppedAt: null,
  createdAt: "",
  updatedAt: "",
};

export const EMPTY_PREVIEW_SESSION_LIST_RESPONSE: PreviewSessionListResponse = {
  previewSessions: [],
  total: 0,
};

export const CloudRuntimeNodeSchema = z.object({
  id: z.string(),
  owner_id: z.string(),
  instance_id: z.string(),
  region: z.string(),
  instance_type: z.string(),
  image_id: z.string(),
  subnet_id: z.string(),
  name: z.string(),
  status: z.string(),
  tags: z.record(z.string(), z.string()).default({}),
  metadata: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

export const CloudRuntimeNodeListSchema = z.array(CloudRuntimeNodeSchema);

export const EMPTY_CLOUD_RUNTIME_NODE_LIST: CloudRuntimeNode[] = [];

export const EMPTY_CLOUD_RUNTIME_NODE: CloudRuntimeNode = {
  id: "",
  owner_id: "",
  instance_id: "",
  region: "",
  instance_type: "",
  image_id: "",
  subnet_id: "",
  name: "",
  status: "",
  tags: {},
  metadata: {},
  created_at: "",
  updated_at: "",
};

// ---------------------------------------------------------------------------
// Workspace dashboard schemas
//
// The dashboard hits three independent rollup endpoints. Each returns a flat
// array, and every field is consumed by chart / KPI math — a missing number
// silently degrades to NaN downstream, so we coerce missing numbers to 0.
// String fields default to "" (no enum narrowing) to survive future model /
// agent ID drift, and so a single null from tz-aware SQL bucketing fails
// only that row instead of dropping the whole array to the `[]` fallback.
// ---------------------------------------------------------------------------

const DashboardUsageDailySchema = z.object({
  date: z.string().default(""),
  model: z.string().default(""),
  input_tokens: z.number().default(0),
  output_tokens: z.number().default(0),
  cache_read_tokens: z.number().default(0),
  cache_write_tokens: z.number().default(0),
  task_count: z.number().default(0),
}).loose();

export const DashboardUsageDailyListSchema = z.array(DashboardUsageDailySchema);

const DashboardUsageByAgentSchema = z.object({
  agent_id: z.string().default(""),
  model: z.string().default(""),
  input_tokens: z.number().default(0),
  output_tokens: z.number().default(0),
  cache_read_tokens: z.number().default(0),
  cache_write_tokens: z.number().default(0),
  task_count: z.number().default(0),
}).loose();

export const DashboardUsageByAgentListSchema = z.array(DashboardUsageByAgentSchema);

const DashboardUsageByUserSchema = z.object({
  user_id: z.string().default(""),
  model: z.string().default(""),
  input_tokens: z.number().default(0),
  output_tokens: z.number().default(0),
  cache_read_tokens: z.number().default(0),
  cache_write_tokens: z.number().default(0),
  task_count: z.number().default(0),
}).loose();

export const DashboardUsageByUserListSchema = z.array(DashboardUsageByUserSchema);

const DashboardUsageByUserDailySchema = DashboardUsageByUserSchema.extend({
  date: z.string().default(""),
}).loose();

export const DashboardUsageByUserDailyListSchema = z.array(DashboardUsageByUserDailySchema);

const DashboardAgentRunTimeSchema = z.object({
  agent_id: z.string().default(""),
  total_seconds: z.number().default(0),
  task_count: z.number().default(0),
  failed_count: z.number().default(0),
}).loose();

export const DashboardAgentRunTimeListSchema = z.array(DashboardAgentRunTimeSchema);

const DashboardRunTimeDailySchema = z.object({
  date: z.string().default(""),
  total_seconds: z.number().default(0),
  task_count: z.number().default(0),
  failed_count: z.number().default(0),
}).loose();

export const DashboardRunTimeDailyListSchema = z.array(DashboardRunTimeDailySchema);

// ---------------------------------------------------------------------------
// Runtime usage schemas — the runtime-detail page's four usage endpoints
// (`/api/runtimes/:id/usage*`). Same leniency rules as the dashboard
// schemas above: numbers default to 0, strings to "", `.loose()` passes
// unknown fields.
// ---------------------------------------------------------------------------

const RuntimeUsageSchema = z.object({
  runtime_id: z.string().default(""),
  date: z.string().default(""),
  provider: z.string().default(""),
  model: z.string().default(""),
  input_tokens: z.number().default(0),
  output_tokens: z.number().default(0),
  cache_read_tokens: z.number().default(0),
  cache_write_tokens: z.number().default(0),
}).loose();

export const RuntimeUsageListSchema = z.array(RuntimeUsageSchema);

const RuntimeHourlyActivitySchema = z.object({
  hour: z.number().default(0),
  count: z.number().default(0),
}).loose();

export const RuntimeHourlyActivityListSchema = z.array(RuntimeHourlyActivitySchema);

const RuntimeUsageByAgentSchema = z.object({
  agent_id: z.string().default(""),
  model: z.string().default(""),
  input_tokens: z.number().default(0),
  output_tokens: z.number().default(0),
  cache_read_tokens: z.number().default(0),
  cache_write_tokens: z.number().default(0),
  task_count: z.number().default(0),
}).loose();

export const RuntimeUsageByAgentListSchema = z.array(RuntimeUsageByAgentSchema);

const RuntimeUsageByHourSchema = z.object({
  hour: z.number().default(0),
  model: z.string().default(""),
  input_tokens: z.number().default(0),
  output_tokens: z.number().default(0),
  cache_read_tokens: z.number().default(0),
  cache_write_tokens: z.number().default(0),
  task_count: z.number().default(0),
}).loose();

export const RuntimeUsageByHourListSchema = z.array(RuntimeUsageByHourSchema);

// ---------------------------------------------------------------------------
// Task cancellation (`POST /api/tasks/:id/cancel`)
//
// This response is consumed directly by chat recovery. The embedded task
// object stays loose so daemon/runtime fields can drift, but the optional
// `cancelled_chat_message` payload must be well-formed before the UI deletes
// a message from cache or restores text into the input.
// ---------------------------------------------------------------------------

const AttributionUserSchema = z.object({
  id: z.string(),
  name: z.string().optional(),
  email: z.string().optional(),
  avatar_url: z.string().optional(),
}).loose();

const TaskEvidenceSchema = z.object({
  kind: z.string(),
  ref_id: z.string(),
}).loose();

const TaskAttributionSchema = z.object({
  source: z.string().default("unattributed"),
  precise: z.boolean().default(false),
  initiator: AttributionUserSchema.optional(),
  originator: AttributionUserSchema.optional(),
  evidence: TaskEvidenceSchema.optional(),
  rule_version_id: z.string().optional(),
  delegated_from_task_id: z.string().optional(),
  retry_of_task_id: z.string().optional(),
  rerun_of_task_id: z.string().optional(),
}).loose();

const AgentTaskResponseSchema = z.object({
  id: z.string(),
  agent_id: z.string().default(""),
  runtime_id: z.string().default(""),
  issue_id: z.string().default(""),
  status: z.string().default("cancelled"),
  priority: z.number().default(0),
  dispatched_at: z.string().nullable().default(null),
  started_at: z.string().nullable().default(null),
  completed_at: z.string().nullable().default(null),
  result: z.unknown().default(null),
  error: z.string().nullable().default(null),
  failure_reason: z.string().optional(),
  created_at: z.string().default(""),
  chat_session_id: z.string().optional(),
  autopilot_run_id: z.string().optional(),
  parent_task_id: z.string().optional(),
  attempt: z.number().optional(),
  trigger_comment_id: z.string().optional(),
  trigger_summary: z.string().optional(),
  kind: z.string().optional(),
  work_dir: z.string().optional(),
  relative_work_dir: z.string().optional(),
  attribution: TaskAttributionSchema.optional(),
}).loose();

const CancelledChatMessageSchema = z.object({
  chat_session_id: z.string(),
  message_id: z.string(),
  content: z.string(),
  restore_to_input: z.boolean().default(false),
}).loose();

export const CancelTaskResponseSchema = AgentTaskResponseSchema.extend({
  cancelled_chat_message: CancelledChatMessageSchema.nullish()
    .transform((value) => value ?? undefined),
}).loose();

export const EMPTY_CANCEL_TASK_RESPONSE: CancelTaskResponse = {
  id: "",
  agent_id: "",
  runtime_id: "",
  issue_id: "",
  status: "cancelled",
  priority: 0,
  dispatched_at: null,
  started_at: null,
  completed_at: null,
  result: null,
  error: null,
  created_at: "",
};

// ---------------------------------------------------------------------------
// Agent template catalog — `/api/agent-templates*` and the
// create-from-template response. The desktop app's create-agent picker
// reaches these endpoints, and a future server change to the template shape
// would white-screen older installed builds (#2192 pattern) without these
// parsers. Lenient by the same rules as IssueSchema above: arrays default to
// `[]`, optional fields stay optional, `.loose()` lets unknown fields pass
// through unchanged.
// ---------------------------------------------------------------------------

const AgentTemplateSkillRefSchema = z.object({
  source_url: z.string(),
  cached_name: z.string().default(""),
  cached_description: z.string().default(""),
}).loose();

const AgentTemplateSummarySchemaBase = z.object({
  slug: z.string(),
  name: z.string(),
  description: z.string().default(""),
  category: z.string().optional(),
  icon: z.string().optional(),
  accent: z.string().optional(),
  // skills MUST default to [] — picker code reads `template.skills.length`
  // and `.map(...)`, both of which crash on `undefined`. The most common
  // future drift (field renamed / wrapped) lands here.
  skills: z.array(AgentTemplateSkillRefSchema).default([]),
}).loose();

export const AgentTemplateSummarySchema = AgentTemplateSummarySchemaBase;

// List endpoint historically returns a bare array. Server could legitimately
// migrate to `{templates: [...]}` later — we accept either shape so an old
// desktop survives the upgrade.
export const AgentTemplateSummaryListSchema = z.union([
  z.array(AgentTemplateSummarySchemaBase),
  z.object({ templates: z.array(AgentTemplateSummarySchemaBase).default([]) })
    .loose()
    .transform((v) => v.templates),
]);

export const EMPTY_AGENT_TEMPLATE_SUMMARY_LIST: AgentTemplateSummary[] = [];

export const AgentTemplateSchema = AgentTemplateSummarySchemaBase.extend({
  // Detail-only field. Default "" so a malformed detail still renders the
  // header + skill list; the user just sees an empty Instructions block.
  instructions: z.string().default(""),
}).loose();

// Used as the parse fallback for `GET /api/agent-templates/:slug`. Slug comes
// from the URL, so we round-trip the requested one back into the fallback
// at the call site (see `getAgentTemplate` in client.ts).
export const EMPTY_AGENT_TEMPLATE_DETAIL: AgentTemplate = {
  slug: "",
  name: "",
  description: "",
  skills: [],
  instructions: "",
};

// `agent` is a full Agent record — schematising every field would duplicate
// a 50-field interface and bit-rot fast. We keep it loose and require only
// `id`, the one field the create-from-template flow consumes (used to
// navigate to the new agent's detail page). Downstream code already
// optional-chains the rest.
const MinimalAgentSchema = z.object({
  id: z.string(),
}).loose();

export const CreateAgentFromTemplateResponseSchema = z.object({
  agent: MinimalAgentSchema,
  imported_skill_ids: z.array(z.string()).default([]),
  reused_skill_ids: z.array(z.string()).default([]),
}).loose();

// Fallback when the success response fails to parse. The agent server-side
// has likely been created already, so we can't pretend nothing happened —
// the caller (`create-agent-dialog.tsx`) is responsible for noticing
// `agent.id === ""` and skipping navigation while keeping the list
// invalidation, so the user finds their new agent in the list.
export const EMPTY_CREATE_AGENT_FROM_TEMPLATE_RESPONSE: CreateAgentFromTemplateResponse = {
  agent: { id: "" } as Agent,
  imported_skill_ids: [],
  reused_skill_ids: [],
};

// Squad list responses carry lightweight membership previews used by hover
// cards. The preview fields are additive API fields, so older backends default
// cleanly to no preview instead of breaking newer frontends.
const SquadMemberPreviewSchema = z.object({
  member_type: z.string(),
  member_id: z.string(),
  role: z.string().default(""),
}).loose();

export const SquadSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  name: z.string(),
  description: z.string().default(""),
  instructions: z.string().default(""),
  avatar_url: z.string().nullable().optional().transform((v) => v ?? null),
  leader_id: z.string(),
  creator_id: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
  archived_at: z.string().nullable().optional().transform((v) => v ?? null),
  archived_by: z.string().nullable().optional().transform((v) => v ?? null),
  member_count: z.number().default(0),
  member_preview: z.array(SquadMemberPreviewSchema).default([]),
}).loose();

export const SquadListSchema = z.array(SquadSchema);
export const EMPTY_SQUAD_LIST: Squad[] = [];
export const EMPTY_SQUAD: Squad = {
  id: "",
  workspace_id: "",
  name: "",
  description: "",
  instructions: "",
  avatar_url: null,
  leader_id: "",
  creator_id: "",
  created_at: "",
  updated_at: "",
  archived_at: null,
  archived_by: null,
  member_count: 0,
  member_preview: [],
};

// Squad member status — backs the Squad detail page's Members tab. status
// is `string | null` (not the narrow `SquadMemberStatusValue` union) so a
// new server-side status doesn't fail the parse; the UI defaults to a
// neutral pill for unknown values.
const SquadActiveIssueBriefSchema = z.object({
  issue_id: z.string(),
  identifier: z.string(),
  title: z.string(),
  issue_status: z.string(),
}).loose();

const SquadMemberStatusSchema = z.object({
  member_type: z.string(),
  member_id: z.string(),
  status: z.string().nullable().optional().transform((v) => v ?? null),
  active_issues: z.array(SquadActiveIssueBriefSchema).default([]),
  last_active_at: z.string().nullable().optional().transform((v) => v ?? null),
}).loose();

export const SquadMemberStatusListResponseSchema = z.object({
  members: z.array(SquadMemberStatusSchema).default([]),
}).loose();

export const EMPTY_SQUAD_MEMBER_STATUS_LIST = { members: [] };

// ---------------------------------------------------------------------------
// Structured error body — POST /api/workspaces/:wsId/issues 409 conflict.
//
// When the server detects an active issue with the same title in the same
// workspace, it returns `{ code: "active_duplicate_issue", error, issue }`
// instead of letting the create through. The UI uses the embedded issue ref
// to offer "view existing" rather than dropping the user into a generic
// "create failed" toast.
//
// Strict guarantees:
//   - `code` is a literal so a future server rename (e.g. `duplicate_issue`)
//     fails the parse and falls back to a normal error toast — drift never
//     ships as a broken duplicate UI.
//   - `issue` is required; without an id/identifier/title the "view existing"
//     button has nothing to point at, so we'd rather fall back than guess.
//   - `issue.status` is intentionally OMITTED: the duplicate toast doesn't
//     render a StatusIcon (which has no fallback for unknown enum values),
//     so a future server-side rename of `status` must not knock this branch
//     out. `.loose()` lets the field pass through unchanged for any other
//     consumer.
// ---------------------------------------------------------------------------

export const DuplicateIssueErrorBodySchema = z.object({
  code: z.literal("active_duplicate_issue"),
  error: z.string().optional(),
  issue: z.object({
    id: z.string(),
    identifier: z.string(),
    title: z.string(),
  }).loose(),
}).loose();

export interface DuplicateIssueErrorBody {
  code: "active_duplicate_issue";
  error?: string;
  issue: {
    id: string;
    identifier: string;
    title: string;
  };
}

// ---------------------------------------------------------------------------
// Webhook delivery schemas — backing the Autopilot Deliveries section. Enums
// (`status`, `signature_status`, `provider`) are kept as `z.string()` so a
// future server-side value (e.g. a Stripe provider, a new dedupe state)
// degrades to a generic UI fallback rather than collapsing the list into
// the empty array. `.loose()` lets unknown fields pass through, matching
// the rule used by every other endpoint here.
// ---------------------------------------------------------------------------

const WebhookDeliverySchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  autopilot_id: z.string(),
  trigger_id: z.string(),
  provider: z.string(),
  event: z.string(),
  dedupe_key: z.string().nullable(),
  dedupe_source: z.string().nullable(),
  signature_status: z.string(),
  status: z.string(),
  attempt_count: z.number().default(0),
  content_type: z.string().nullable(),
  response_status: z.number().nullable(),
  autopilot_run_id: z.string().nullable(),
  replayed_from_delivery_id: z.string().nullable(),
  error: z.string().nullable(),
  received_at: z.string(),
  last_attempt_at: z.string(),
  created_at: z.string(),
  // Detail-only fields. The list endpoint omits them; the detail endpoint
  // populates raw_body / selected_headers / response_body.
  selected_headers: z.record(z.string(), z.unknown()).nullable().optional(),
  raw_body: z.string().nullable().optional(),
  response_body: z.string().nullable().optional(),
}).loose();

export const ListWebhookDeliveriesResponseSchema = z.object({
  deliveries: z.array(WebhookDeliverySchema).default([]),
  total: z.number().default(0),
}).loose();

export const WebhookDeliveryResponseSchema = WebhookDeliverySchema;

export const EMPTY_LIST_WEBHOOK_DELIVERIES_RESPONSE: ListWebhookDeliveriesResponse = {
  deliveries: [],
  total: 0,
};

// ---------------------------------------------------------------------------
// Autopilot list schema. Enums (`status`, `execution_mode`, `trigger_kinds`,
// `last_run_status`) stay `z.string()` so future server-side values degrade
// to a generic UI fallback. The three derived fields (trigger_kinds /
// next_run_at / last_run_status) are list-endpoint-only and absent on older
// servers — optional by contract, the list renders "—" without them.
// ---------------------------------------------------------------------------

const AutopilotListItemSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  title: z.string(),
  description: z.string().nullable().optional(),
  project_id: z.string().nullable().optional(),
  // Older servers (pre-MUL-2429) omit assignee_type; "agent" is the
  // documented default.
  assignee_type: z.string().default("agent"),
  assignee_id: z.string(),
  status: z.string(),
  execution_mode: z.string(),
  issue_title_template: z.string().nullable().optional(),
  created_by_type: z.string(),
  created_by_id: z.string(),
  last_run_at: z.string().nullable().optional(),
  created_at: z.string(),
  updated_at: z.string(),
  trigger_kinds: z.array(z.string()).optional(),
  next_run_at: z.string().nullable().optional(),
  last_run_status: z.string().nullable().optional(),
}).loose();

export const ListAutopilotsResponseSchema = z.object({
  autopilots: z.array(AutopilotListItemSchema).default([]),
  total: z.number().default(0),
}).loose();

export const EMPTY_LIST_AUTOPILOTS_RESPONSE = {
  autopilots: [],
  total: 0,
};

export const EMPTY_WEBHOOK_DELIVERY: WebhookDelivery = {
  id: "",
  workspace_id: "",
  autopilot_id: "",
  trigger_id: "",
  provider: "",
  event: "",
  dedupe_key: null,
  dedupe_source: null,
  signature_status: "not_required",
  status: "queued",
  attempt_count: 0,
  content_type: null,
  response_status: null,
  autopilot_run_id: null,
  replayed_from_delivery_id: null,
  error: null,
  received_at: "",
  last_attempt_at: "",
  created_at: "",
};

// ---------------------------------------------------------------------------
// User (`/api/me` GET + PATCH). The auth store and Settings → Account both
// trust this shape — a drift here would knock both surfaces out. Kept
// lenient by the same rules as IssueSchema: enums stay `z.string()`,
// nullable fields are unioned with `null`, unknown server fields pass
// through via `.loose()`. `profile_description` is the field added in
// MUL-2406; the server emits `""` when unset (NOT NULL DEFAULT ''), so
// the schema defaults to `""` too — keeps the type tight without
// breaking older backends that don't return the column yet.
// ---------------------------------------------------------------------------

export const UserSchema = z.object({
  id: z.string(),
  name: z.string().default(""),
  email: z.string().default(""),
  avatar_url: z.string().nullable().default(null),
  onboarded_at: z.string().nullable().default(null),
  onboarding_questionnaire: z.record(z.string(), z.unknown()).default({}),
  starter_content_state: z.string().nullable().default(null),
  language: z.string().nullable().default(null),
  profile_description: z.string().default(""),
  timezone: z.string().nullable().default(null),
  integration_tokens: z.object({
    git_token: z.string().optional(),
    feishu_mcp_token: z.string().optional(),
    paones_token: z.string().optional(),
    jingwei_token: z.string().optional(),
  }).catchall(z.string()).default({}),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const EMPTY_USER: User = {
  id: "",
  name: "",
  email: "",
  avatar_url: null,
  onboarded_at: null,
  onboarding_questionnaire: {},
  starter_content_state: null,
  language: null,
  profile_description: "",
  timezone: null,
  integration_tokens: {},
  created_at: "",
  updated_at: "",
};

const EmptyUserSchemaValue = {
  id: "",
  name: "",
  email: "",
  avatar_url: null,
  onboarded_at: null,
  onboarding_questionnaire: {},
  starter_content_state: null,
  language: null,
  profile_description: "",
  timezone: null,
  integration_tokens: {},
  created_at: "",
  updated_at: "",
};

const LoginUserSchema = z.preprocess(
  (value) => value ?? EmptyUserSchemaValue,
  UserSchema.catch(EmptyUserSchemaValue),
);

export const LoginResponseSchema = z.object({
  token: z.string().default(""),
  user: LoginUserSchema,
}).loose();

export const EMPTY_LOGIN_RESPONSE = {
  token: "",
  user: EMPTY_USER,
};

export const LarkLoginStateResponseSchema = z.object({
  state: z.string().default(""),
  authorize_url: z.string().optional(),
}).loose();

export const EMPTY_LARK_LOGIN_STATE_RESPONSE = {
  state: "",
};

export const LarkLoginResponseSchema = LoginResponseSchema.extend({
  workspace_id: z.string().optional(),
  workspace_slug: z.string().optional(),
  next: z.string().optional(),
}).loose();

export const EMPTY_LARK_LOGIN_RESPONSE = {
  token: "",
  user: EMPTY_USER,
};

// ---------------------------------------------------------------------------
// Billing schemas (cloud-billing proxy surface)
//
// All billing JSON we receive comes from multica-cloud verbatim — we proxy
// the bytes without re-shaping. These schemas use `loose()` so a future
// non-breaking field addition on the cloud side doesn't crash us; required
// fields are still strictly enforced. EMPTY_* constants supply the
// fallback parseWithFallback uses when the upstream response is malformed
// or unparseable.

export const BillingBalanceSchema = z.object({
  owner_id: z.string(),
  balance_micro: z.number(),
  balance_credit: z.number(),
  updated_at: z.string(),
}).loose();

export const EMPTY_BILLING_BALANCE: BillingBalance = {
  owner_id: "",
  balance_micro: 0,
  balance_credit: 0,
  updated_at: "",
};

// `tx_type` and `source` are kept as plain strings here; the cloud doc
// enumerates the canonical values but the frontend display tolerates
// unknown ones gracefully. Strict enums would crash the page on a future
// addition (e.g. a new `topup` source kind).
export const BillingTransactionSchema = z.object({
  id: z.string(),
  owner_id: z.string(),
  idempotency_key: z.string().default(""),
  tx_type: z.string(),
  source: z.string(),
  amount_micro: z.number(),
  balance_after: z.number(),
  reference_id: z.string().default(""),
  description: z.string().default(""),
  metadata: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
}).loose();

export const BillingTransactionsPageSchema = z.object({
  items: z.array(BillingTransactionSchema).default([]),
  total: z.number().default(0),
  page: z.number().default(1),
  page_size: z.number().default(20),
}).loose();

export const EMPTY_BILLING_TRANSACTIONS_PAGE: BillingTransactionsPage = {
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
};

export const BillingBatchSchema = z.object({
  id: z.string(),
  owner_id: z.string(),
  source_tx_id: z.string().default(""),
  source_type: z.string(),
  total_micro: z.number(),
  remaining_micro: z.number(),
  // Cloud either omits the key (never expires) or sends a string
  // timestamp. Null is also tolerated since some serializers emit
  // explicit nulls for absent timestamps.
  expires_at: z.string().nullable().optional(),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

export const BillingBatchesPageSchema = z.object({
  items: z.array(BillingBatchSchema).default([]),
  total: z.number().default(0),
  page: z.number().default(1),
  page_size: z.number().default(20),
}).loose();

export const EMPTY_BILLING_BATCHES_PAGE: BillingBatchesPage = {
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
};

export const BillingTopupSchema = z.object({
  id: z.string(),
  owner_id: z.string(),
  amount_cents: z.number(),
  currency: z.string().default("usd"),
  credits: z.number(),
  bonus_credits: z.number().default(0),
  status: z.string(),
  tier_id: z.string().default(""),
  stripe_checkout_id: z.string().default(""),
  // Only set after status reaches `credited` — leave optional rather
  // than coerce to "" so a UI can branch on existence.
  purchase_batch_id: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

export const BillingTopupsPageSchema = z.object({
  items: z.array(BillingTopupSchema).default([]),
  total: z.number().default(0),
  page: z.number().default(1),
  page_size: z.number().default(20),
}).loose();

export const EMPTY_BILLING_TOPUPS_PAGE: BillingTopupsPage = {
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
};

export const BillingPriceTierSchema = z.object({
  id: z.string(),
  // Cloud doc says display_name falls back to id; tolerate empty too.
  display_name: z.string().default(""),
  amount_cents: z.number(),
  credits: z.number(),
  bonus_credits: z.number().optional(),
  bonus_expires_in: z.string().optional(),
}).loose();

export const BillingPriceTierListSchema = z.array(BillingPriceTierSchema);

export const EMPTY_BILLING_PRICE_TIER_LIST: BillingPriceTier[] = [];

export const CreateBillingCheckoutSessionResponseSchema = z.object({
  order_id: z.string(),
  session_id: z.string(),
  url: z.string(),
}).loose();

export const EMPTY_CREATE_BILLING_CHECKOUT_SESSION_RESPONSE: CreateBillingCheckoutSessionResponse = {
  order_id: "",
  session_id: "",
  url: "",
};

export const BillingCheckoutSessionStatusSchema = z.object({
  order_id: z.string(),
  status: z.string(),
  amount_cents: z.number(),
  credits: z.number(),
  bonus_credits: z.number().default(0),
  currency: z.string().default("usd"),
  tier_id: z.string().default(""),
}).loose();

export const EMPTY_BILLING_CHECKOUT_SESSION_STATUS: BillingCheckoutSessionStatus = {
  order_id: "",
  status: "pending",
  amount_cents: 0,
  credits: 0,
  bonus_credits: 0,
  currency: "usd",
  tier_id: "",
};

export const CreateBillingPortalSessionResponseSchema = z.object({
  url: z.string(),
}).loose();

export const EMPTY_CREATE_BILLING_PORTAL_SESSION_RESPONSE: CreateBillingPortalSessionResponse = {
  url: "",
};
