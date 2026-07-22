export type CreativeMaterialStatus =
  | "new"
  | "selected"
  | "rejected"
  | "sent_to_edit"
  | "edited"
  | "approved"
  | "archived"
  | string;

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
  raw?: unknown;
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

export interface CreativeEditAsset {
  id: string;
  variant_id: string;
  width: number;
  height: number;
  label: string;
  asset_url: string;
  content_type: string;
  created_at: string;
}

export type CreativeEditFeedbackDecision =
  | "accepted"
  | "rejected"
  | "needs_revision";

export type CreativeEditFeedbackReason =
  | "ready_to_publish"
  | "copy_accurate"
  | "benefit_clear"
  | "layout_match"
  | "brand_complete"
  | "copy_error"
  | "copy_too_long"
  | "benefit_mismatch"
  | "layout_mismatch"
  | "missing_content"
  | "brand_compliance"
  | "visual_quality"
  | "other";

export interface CreativeEditProcessSnapshot {
  job_status?: string;
  job_stage?: string;
  job_progress?: number;
  job_prompt?: string;
  process_data?: Record<string, unknown>;
  dynamic_rules?: {
    market?: string;
    strategy?: string;
    variant_count?: number;
    sizes?: { width?: number; height?: number; label?: string }[];
    [key: string]: unknown;
  };
  external_status?: string;
  external_provider?: string;
  poll_attempts?: number;
  job_created_at?: string;
  job_updated_at?: string;
  job_completed_at?: string | null;
  last_poll_at?: string | null;
  source_candidate?: {
    competitor?: string;
    title?: string;
    asset_type?: string;
    [key: string]: unknown;
  };
  variant_index?: number;
  variant_qc_status?: string;
  variant_created_at?: string;
  asset_count?: number;
  assets?: { width?: number; height?: number; label?: string }[];
  [key: string]: unknown;
}

export interface CreativeEditFeedback {
  id: string;
  workspace_id: string;
  issue_id: string;
  job_id: string;
  candidate_id: string;
  variant_id: string;
  decision: CreativeEditFeedbackDecision | string;
  reason_codes: (CreativeEditFeedbackReason | string)[];
  suggestion: string;
  process_snapshot: CreativeEditProcessSnapshot;
  created_by: string;
  created_by_name: string;
  created_at: string;
}

export interface CreativeEditVariant {
  id: string;
  job_id: string;
  candidate_id: string;
  variant_index: number;
  title: string;
  description: string;
  qc_status: string;
  created_at: string;
  assets: CreativeEditAsset[];
  feedback: CreativeEditFeedback[];
}

export interface CreativeEditJob {
  id: string;
  workspace_id: string;
  issue_id: string;
  status: string;
  prompt: string;
  rules: unknown;
  process_data: Record<string, unknown>;
  external_provider: string;
  mcp_connection_id: string;
  external_job_id: string;
  external_status: string;
  stage: string;
  progress: number;
  last_poll_at: string;
  next_poll_at: string;
  completed_at: string;
  error_message: string;
  poll_attempts: number;
  created_at: string;
  updated_at: string;
  candidate_ids: string[];
  variants: CreativeEditVariant[];
}

export interface CreativeMaterialsResponse {
  enabled: boolean;
  summary: CreativeMaterialSummary;
  candidates: CreativeMaterialCandidate[];
  crawl_runs: CreativeMaterialCrawlRun[];
  edit_jobs: CreativeEditJob[];
}

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
  duration_days?: number | null;
  impression_estimate?: number | null;
  media_names?: string[];
  area_names?: string[];
  language_names?: string[];
  platform_names?: string[];
  raw?: unknown;
}

export interface ImportCreativeMaterialsRequest {
  connector_id?: string;
  query_summary?: string;
  params?: Record<string, unknown>;
  materials: CreativeMaterialInput[];
}

export interface CreativeImportSummary {
  run_id: string;
  imported_count: number;
  existing_count: number;
  total_count: number;
  skipped_count: number;
}

export interface UpdateCreativeMaterialCandidateRequest {
  status: CreativeMaterialStatus;
  note?: string;
}

export interface CreateCreativeEditJobRequest {
  candidate_ids?: string[];
  prompt?: string;
  rules?: Record<string, unknown>;
}

export interface CreateCreativeEditFeedbackRequest {
  decision: CreativeEditFeedbackDecision;
  reason_codes: CreativeEditFeedbackReason[];
  suggestion?: string;
}
