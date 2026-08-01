export type CreativeMaterialStatus = "new" | "selected" | "rejected" | "archived" | string;
export type CreativeMaterialAssetType = "image" | "video" | "unknown" | string;

export interface CreativeMaterialCandidate {
  id: string;
  workspace_id: string;
  connector_id: string;
  external_id: string;
  dedupe_key: string;
  competitor: string;
  title: string;
  asset_type: CreativeMaterialAssetType;
  preview_url: string;
  resource_url: string;
  poster_url: string;
  original_url: string;
  archived_url: string;
  archive_status: string;
  archive_error: string;
  duration_days: number | null;
  impression_estimate: number | null;
  media_names: string[];
  area_names: string[];
  language_names: string[];
  platform_names: string[];
  status: CreativeMaterialStatus;
  tags: string[];
  note: string;
  selected_at: string | null;
  first_seen_at: string;
  last_seen_at: string;
  created_at: string;
  updated_at: string;
  source_attachment_id: string;
}

export interface CreativeMaterialSummary {
  total: number;
  new: number;
  selected: number;
  rejected: number;
  sent_to_edit: number;
  edited: number;
  approved: number;
  archived: number;
}

export interface CreativeMaterialCrawlRun {
  id: string;
  workspace_id: string;
  issue_id: string;
  connector_id: string;
  query_summary: string;
  status: string;
  imported_count: number;
  existing_count: number;
  total_count: number;
  created_at: string;
}

export interface CreativeMaterialLibraryResponse {
  candidates: CreativeMaterialCandidate[];
}

export interface CreativeMaterialsResponse {
  enabled: boolean;
  summary: CreativeMaterialSummary;
  candidates: CreativeMaterialCandidate[];
  crawl_runs: CreativeMaterialCrawlRun[];
  context: CreativeIssueContext | null;
  items: CreativeIssueItem[];
  deliveries: CreativeDelivery[];
  adjustments: CreativeAdjustmentRequest[];
}

export type CreativeDeliverySize = "1080x1080" | "1200x628" | "800x1000";

export interface CreativeDelivery {
  id: string;
  issue_id: string;
  candidate_id: string;
  work_issue_id: string;
  variant: number;
  size: CreativeDeliverySize;
  revision: number;
  base_attachment_id: string;
  final_attachment_id: string;
  prime_evidence_attachment_id: string;
  qc_issue_id: string;
  created_at: string;
  updated_at: string;
}

export interface CreativeAdjustmentRequest {
  id: string;
  issue_id: string;
  candidate_id: string;
  work_issue_id: string;
  adjustment_issue_id: string;
  variant: number;
  scope: "size" | "variant";
  size: CreativeDeliverySize | "";
  revision: number;
  instruction: string;
  target_attachment_ids: string[];
  base_attachment_ids: string[];
  status: string;
  created_at: string;
  updated_at: string;
}

export type RegisterCreativeDeliveryInput = Pick<
  CreativeDelivery,
  | "candidate_id"
  | "work_issue_id"
  | "variant"
  | "size"
  | "revision"
  | "base_attachment_id"
  | "final_attachment_id"
  | "prime_evidence_attachment_id"
  | "qc_issue_id"
>;

export interface RegisterCreativeDeliveriesRequest {
  deliveries: RegisterCreativeDeliveryInput[];
}

export interface RegisterCreativeDeliveriesResponse {
  deliveries: CreativeDelivery[];
}

export interface CreateCreativeAdjustmentRequest {
  variant: number;
  scope: "size" | "variant";
  size?: CreativeDeliverySize;
  instruction: string;
  target_attachment_ids: string[];
  base_attachment_ids: string[];
}

export type CreativeResourceKind = "copy_library" | "market_pack";
export type CreativeResourceStatus = "draft" | "published" | "archived";

export interface CreativeResource {
  id: string;
  workspace_id: string;
  kind: CreativeResourceKind;
  name: string;
  description: string;
  status: CreativeResourceStatus;
  version: number;
  published_version: number;
  config: Record<string, unknown>;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreativeResourceListResponse {
  resources: CreativeResource[];
}

export interface CreativeResourceFile {
  id: string;
  resource_id: string;
  attachment_id: string;
  role: string;
  label: string;
  metadata: Record<string, unknown>;
  filename: string;
  url: string;
  content_type: string;
  size_bytes: number;
  created_version: number;
  created_at: string;
}

export interface CreativeResourceFileListResponse {
  files: CreativeResourceFile[];
}

export interface CreateCreativeResourceRequest {
  kind: CreativeResourceKind;
  name: string;
  description?: string;
  config?: Record<string, unknown>;
}

export interface UpdateCreativeResourceRequest {
  name: string;
  description?: string;
  config: Record<string, unknown>;
}

export type CreativeCopyStatus = "draft" | "approved" | "disabled";

export interface CreativeCopyEntry {
  id: string;
  workspace_id: string;
  library_id: string;
  external_key: string;
  headline: string;
  subheadline: string;
  benefit: string;
  cta: string;
  legal_text: string;
  copy_role: string;
  market: string;
  locale: string;
  tags: string[];
  status: CreativeCopyStatus;
  version: number;
  metadata: Record<string, unknown>;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreativeCopyEntryListResponse {
  entries: CreativeCopyEntry[];
}

export type CreativeCopyEntryInput = Omit<CreativeCopyEntry,
  "id" | "workspace_id" | "library_id" | "version" | "created_by" | "created_at" | "updated_at"
>;

export interface ImportCreativeCopyEntriesRequest {
  mode: "append" | "upsert" | "replace";
  source_filename: string;
  mapping: Record<string, string>;
  entries: CreativeCopyEntryInput[];
}

export interface CreativeCopyImportResult {
  created: number;
  updated: number;
  skipped: number;
  resource: CreativeResource;
}

export interface CreativeIssueContext {
  issue_id: string;
  workspace_id: string;
  market_pack_id: string;
  squad_id: string;
  snapshot: Record<string, unknown>;
  updated_at: string;
}

export interface CreativeIssueItem {
  issue_id: string;
  candidate_id: string;
  copy_entry_id: string;
  copy_snapshot: Record<string, unknown>;
  creative_brief: CreativeBrief;
  work_issue_id: string;
  revision: number;
  status: "ready" | "running" | "published" | "blocked" | string;
  updated_at: string;
}

export type CreativeBriefStatus = "requested" | "draft" | "confirmed" | string;
export type CreativeBriefSource = "ai" | "user" | "mixed" | string;

export interface CreativeBrief {
  theme: string;
  theme_elements: string[];
  primary_benefit: string;
  secondary_benefits: string[];
  benefit_value: string;
  evidence: string[];
  detected_text: string[];
  visual_type: string;
  analysis_summary: string;
  status: CreativeBriefStatus;
  source: CreativeBriefSource;
  confidence: number | null;
  analysis_issue_id: string;
}

export interface PutCreativeIssueContextRequest {
  market_pack_id: string;
  squad_id: string;
}

export interface UpdateCreativeMaterialCandidateRequest {
  status: CreativeMaterialStatus;
  note?: string;
}

export interface CreativeImportSummary { run_id: string; imported_count: number; existing_count: number; total_count: number; skipped_count: number; }
export interface CreativeMaterialInput {
  external_id?: string;
  dedupe_key?: string;
  competitor?: string;
  title?: string;
  asset_type?: CreativeMaterialAssetType;
  preview_url?: string;
  resource_url?: string;
  poster_url?: string;
  original_url?: string;
  media_names?: string[];
  area_names?: string[];
  language_names?: string[];
  platform_names?: string[];
  raw?: Record<string, unknown>;
}
export interface ImportCreativeMaterialsRequest { connector_id?: string; query_summary?: string; params?: Record<string, unknown>; materials: CreativeMaterialInput[]; }
export interface ImportCreativeMaterialLibraryRequest {
  attachment_id?: string;
  source_url?: string;
  title?: string;
  competitor?: string;
  asset_type?: CreativeMaterialAssetType;
  area_names?: string[];
  language_names?: string[];
  platform_names?: string[];
  tags?: string[];
  note?: string;
}
export interface CreativeMaterialImportResult { id: string; }
export interface CreativeMaterialArchiveRetryResult { scheduled_count: number; }
