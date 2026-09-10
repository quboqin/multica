export type CreativeMaterialStatus = "unseen" | "new" | "viewed" | "shortlisted" | "selected" | "rejected" | "archived" | string;
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
  source_issue_id: string;
  source_run_id: string;
  is_new_in_run: boolean;
  analysis_status?: "pending" | "running" | "completed" | "failed" | string;
  analysis_error?: string;
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
  autopilot_run_id: string;
  rerun_of_id: string;
  connector_id: string;
  query_summary: string;
  competitors: string[];
  status: string;
  error_code: string;
  error_message: string;
  diagnostics: {
    diagnosis?: {
      state?: string;
      classification?: string;
      summary?: string;
      automated_actions?: string[];
      requires_agent?: boolean;
      requires_user_action?: boolean;
      matched_total?: number | null;
    };
    strategy?: Record<string, unknown>;
    totals?: Record<string, unknown>;
    agent_diagnosis_state?: string;
    agent_diagnosis_task_id?: string;
  };
  imported_count: number;
  existing_count: number;
  total_count: number;
  candidate_metrics: {
    total: number;
    analyzed: number;
    analysis_failed: number;
    selected: number;
    rejected: number;
  };
  started_at: string;
  finished_at: string;
  created_at: string;
}

export interface CreativeMaterialLibraryResponse {
  candidates: CreativeMaterialCandidate[];
  crawl_runs?: CreativeMaterialCrawlRun[];
  total_count?: number;
  next_offset?: number | null;
}

export interface CreativeMaterialLibraryQuery {
  runId?: string;
  includeEmptyRuns?: boolean;
  limit?: number;
  offset?: number;
  query?: string;
  competitor?: string;
  area?: string;
  language?: string;
  media?: string;
  assetType?: CreativeMaterialAssetType;
  view?: "available" | "analyze" | "generated" | "rejected" | "selected" | "all";
  sort?: "recent" | "impressions" | "duration";
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
  published_config?: Record<string, unknown>;
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

export type CreativeType = "num" | "repayment_plan";

export type CreativeCopyFragmentRole =
  | "headline"
  | "subheadline"
  | "benefit"
  | "supporting"
  | "cta"
  | "legal";

export type CreativeCopyFragmentUsage = "core" | "fallback" | "required";

/** Human-facing business grouping from the maintained copy workbook. */
export type CreativeCopyContentGroup =
  | "standard_headline"
  | "core_benefit"
  | "other_benefit"
  | "call_to_action"
  | "repayment_headline";

export interface CreativeCopyFragment {
  id: string;
  key: string;
  name: string;
  creative_types: CreativeType[];
  role: CreativeCopyFragmentRole;
  /** Business grouping shown to copy operators; recommendation metadata stays separate. */
  content_group?: CreativeCopyContentGroup;
  /** Business meaning used to keep one fact from each relevant information group. */
  semantic_group?: string;
  text: string;
  tags: string[];
  usage: CreativeCopyFragmentUsage;
  status: CreativeCopyStatus;
}

export interface CreativeCopyRecipe {
  id: string;
  key: string;
  name: string;
  creative_type: CreativeType;
  description: string;
  fragment_ids: Partial<Record<CreativeCopyFragmentRole, string[]>>;
  match_tags: string[];
  status: CreativeCopyStatus;
}

/** One approved amount-and-tenor outcome from the business repayment table. */
export interface CreativeRepaymentPlanEntry {
  id: string;
  key: string;
  principal: number;
  tenor_months: number;
  monthly_installment: number;
  total_interest: number;
  total_repayment: number;
  source: string;
  status: CreativeCopyStatus;
}

export interface CreativeRepaymentPlan {
  labels: {
    principal: string;
    tenor: string;
    monthly_installment: string;
    total_interest: string;
    total_repayment: string;
  };
  entries: CreativeRepaymentPlanEntry[];
}

export interface CreativeCopyLibraryConfig {
  schema_version: 4;
  market: string;
  locale: string;
  source: {
    name: string;
    url: string;
    sync_status: "synced" | "pending" | "failed";
    note: string;
  };
  fragments: CreativeCopyFragment[];
  recipes: CreativeCopyRecipe[];
  repayment_plan: CreativeRepaymentPlan;
}

export interface CreativeCopySnapshot {
  schema_version: 3;
  id: string;
  library_id: string;
  library_version: number;
  composition_id?: string;
  composition_key?: string;
  composition_engine_version?: string;
  recipe_id?: string;
  recipe_key?: string;
  creative_type: CreativeType;
  headline: string;
  subheadline: string;
  benefit: string;
  supporting: string;
  cta: string;
  legal_text: string;
  fragments: Array<{ id: string; key: string; role: CreativeCopyFragmentRole; text: string }>;
  repayment_plan_entries: Array<{
    key: string;
    principal: number;
    tenor_months: number;
    monthly_installment: number;
    total_interest: number;
    total_repayment: number;
    source: string;
  }>;
  recommendation: {
    score: number;
    reasons: string[];
    matched_signals: string[];
  };
  status: "approved" | "model_pre_adapted" | "user_custom";
  visual_direction?: CreativeVisualDirection;
  pre_adaptation?: {
    schema_version: 1;
    source_analysis_id: string;
    summary: string;
    analysis_highlights: string[];
    app_ui_replacement?: {
      required: boolean;
      selected: boolean;
      resource_file_id: string;
      attachment_id: string;
      label?: string;
      reason: string;
      source_screen: {
        app_ui_type: string;
        visual_characteristics?: string;
        bounds?: {
          x: number;
          y: number;
          width: number;
          height: number;
        };
      };
      constraints: string[];
    };
    additional_copy?: Array<{
      role: "benefit";
      text: string;
      source_kind: "manual";
      status: "ready";
    }>;
    text_replacements: Array<{
      block_id: string;
      location: string;
      role: string;
      semantic_kind?: string;
      source_text: string;
      replacement_text: string;
      source_kind: "library" | "manual" | "recommendation" | "calculation";
      source_keys: string[];
      status: "ready" | "missing";
      note: string;
      recommendation_basis?: string[];
      calculation?: {
        rule_key?: string;
        formula: string;
        inputs: string[];
        result: string;
      };
    }>;
    repayment_plan_selections: Array<{
      id: string;
      plan_key: string;
      principal: number;
      tenor_months: number;
      values: {
        principal: string;
        tenor: string;
        total_interest: string;
        total_repayment: string;
        monthly_installment: string;
      };
    }>;
    numeric_layouts: Array<{
      id: string;
      /** Only the source blocks that render approved repayment labels or values. */
      source_block_ids: string[];
      location: string;
      layout_kind: "table" | "card_grid" | "comparison" | "single_card" | "single_value" | "option_buttons" | "table_row" | "principal" | "tenor" | "repayment_table";
      scenario_ids: string[];
      target_columns: Array<"principal" | "tenor" | "monthly_installment" | "total_interest" | "total_repayment">;
      render_instruction: string;
    }>;
  };
}

export interface CreativeVisualDirection {
  schema_version: 1;
  theme: string;
  style_tags: string[];
  must_preserve: string[];
  avoid: string[];
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
  creative_brief: CreativeBrief;
  work_issue_id: string;
  revision: number;
  status: "ready" | "running" | "published" | "blocked" | string;
  updated_at: string;
}

export type CreativeBriefStatus = "requested" | "draft" | "confirmed" | string;
export type CreativeBriefSource = "ai" | "user" | "mixed" | string;

export interface CreativeBriefAppUIReference {
  resource_file_id: string;
  attachment_id: string;
  reason: string;
}

export interface CreativeBrief {
  theme: string;
  theme_elements: string[];
  primary_benefit: string;
  secondary_benefits: string[];
  benefit_value: string;
  source_semantics: string;
  information_mechanism: string;
  visual_anchors: string[];
  palette_anchors: string[];
  must_preserve: string[];
  allowed_variations: string[];
  evidence: string[];
  detected_text: string[];
  visual_type: string;
  analysis_summary: string;
  user_direction: string;
  app_ui_replacement_required: boolean;
  selected_app_ui_references: CreativeBriefAppUIReference[];
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

export type CreativeFeedbackSubjectType = "candidate" | "recommended_copy" | "variant" | "asset" | "qc" | string;
export type CreativeFeedbackDecision = "selected" | "rejected" | "accepted" | "replaced" | "needs_revision" | "abandoned" | "reported" | "" | string;
export type CreativeFeedbackScope = "size" | "variant" | "order" | string;

export interface CreativeFeedbackAnnotation {
  id: string;
  asset_id: string;
  kind: "point" | "rect" | string;
  issue_type: string;
  x: number;
  y: number;
  width: number;
  height: number;
  scope: CreativeFeedbackScope;
  comment: string;
}

export interface CreateCreativeFeedbackRequest {
  idempotency_key?: string;
  issue_id: string;
  subject_type: CreativeFeedbackSubjectType;
  subject_id: string;
  event_type: string;
  decision: CreativeFeedbackDecision;
  reason_codes?: string[];
  comment?: string;
  annotation?: CreativeFeedbackAnnotation;
  context_snapshot?: Record<string, unknown>;
}

export interface CreativeFeedbackEvent extends CreateCreativeFeedbackRequest {
  id: string;
  workspace_id: string;
  actor_id: string;
  created_at: string;
}

export interface CreateCreativeFeedbackResponse {
  id: string;
  idempotency_key: string;
  workspace_id: string;
  issue_id: string;
  actor_type: string;
  actor_id: string;
  subject_type: CreativeFeedbackSubjectType;
  subject_id: string;
  event_type: string;
  decision: CreativeFeedbackDecision;
  reason_codes: string[];
  comment: string;
  annotation?: CreativeFeedbackAnnotation;
  context_snapshot: Record<string, unknown>;
  undo_of_id: string;
  created_at: string;
}

export interface CreativeFeedbackEventListResponse {
  events: CreateCreativeFeedbackResponse[];
}

export interface CreativeFeedbackMetrics {
  candidate_selected: number;
  candidate_rejected: number;
  copy_accepted: number;
  copy_replaced: number;
  variant_accepted: number;
  variant_needs_revision: number;
  asset_accepted: number;
  asset_reported: number;
  qc_accepted: number;
  qc_missed_issue: number;
  qc_false_positive: number;
}

export interface CreativeFeedbackReasonSummary {
  code: string;
  count: number;
}

export interface CreativeFeedbackDashboard {
  workflow: {
    candidate_selected: number;
    candidate_rejected: number;
    copy_accepted: number;
    copy_replaced: number;
    asset_reported: number;
    qc_accepted: number;
    qc_missed_issue: number;
    qc_false_positive: number;
    image_generation_success: number;
    image_generation_total: number;
    image_generation_failed: number;
    image_generation_in_progress: number;
    image_generation_duration_seconds: number | null;
    image_generation_duration_package_count: number;
    three_size_qc_success: number;
    three_size_qc_total: number;
    first_delivery_count: number;
    first_delivery_total: number;
    production_adopted: number;
    production_adoption_eligible: number;
    feedback_reasons: CreativeFeedbackReasonSummary[];
  };
}

export interface CreativeOrder {
  recoveries?: CreativeOrderRecovery[];
  id: string;
  workspace_id: string;
  issue_id: string;
  status: string;
  derived_status: string;
  delivery_status?: string;
  production_status?: string;
  input_snapshot: Record<string, unknown>;
  trigger_evidence_kind: string;
  trigger_evidence_ref_id: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  workflow_failures: CreativeOrderWorkflowFailure[];
  items: CreativeOrderItem[];
}

export interface CreativeOrderWorkflowFailure {
  task_id: string;
  agent_id: string;
  workflow: string;
  scope: string;
  subject_id: string;
  item_key: string;
  trigger_evidence_kind: string;
  trigger_evidence_ref_id: string;
  failure_reason: string;
  error: string;
  failed_at: string;
  retryable: boolean;
}

export interface CreativeOrderWorkflowRetryResponse { task_id: string; }
export interface CreativeRecoveryAttempt {
  id: string; attempt: number; status: string; source_task_id: string; result_task_id: string;
  result_asset_id: string; reason_code: string; error_message: string; started_at: string; completed_at: string;
}
export interface CreativeOrderRecovery {
  id: string; order_item_id: string; variant_id: string; revision: number; stage: string; size_key: string;
  status: string; reason_code: string; source_task_id: string; result_task_id: string; resolved_asset_id: string;
  attempt: number; max_attempts: number; next_retry_at: string; last_error: string; created_at: string;
  updated_at: string; resolved_at: string; attempts: CreativeRecoveryAttempt[];
}
export interface CreativeOrderPrimeComposeResponse { variant_id: string; composed: boolean; completed: boolean; status?: string; }
export interface QueueCreativeOrderAdjustmentRequest {
  adjustment_issue_id: string;
  asset_id: string;
  size_key: CreativeDeliverySize;
  scope?: "size" | "variant";
  source_revision: number;
  annotation_guide_attachment_id?: string;
  comment: string;
  event_type: "annotation" | "decision";
  reason_codes: string[];
  annotation?: CreativeFeedbackAnnotation;
  context_snapshot?: Record<string, unknown>;
}
export interface QueueCreativeOrderAdjustmentResponse { task_id: string; revision: number; }
export interface CreativeOrderQCRetryResponse {
  variant_id: string;
  revision: number;
  attempt: number;
  technical_task_id: string;
  visual_task_id: string;
}

export interface CreativeOrderListResponse { orders: CreativeOrder[]; }
export interface CreateCreativeOrderRequest { issue_id: string; submission_key?: string; status: string; input_snapshot: Record<string, unknown>; trigger_evidence_kind: string; trigger_evidence_ref_id: string; items: { source_kind?: "material" | "copy_library"; copy_library_id?: string; candidate_id: string; source_analysis_id: string; copy_snapshot: Record<string, unknown>; direction: string }[]; }
export interface CreativeSourceAnalysis { id: string; candidate_id: string; analysis_version: number; status: string; summary: string; result: Record<string, unknown>; error_code: string; error_message: string; trigger_evidence_kind: string; trigger_evidence_ref_id: string; created_at: string; completed_at: string; }
export interface CreativeSourceAnalysisListResponse { analyses: CreativeSourceAnalysis[]; }
export interface CreativeCandidateProgress { state: string; target: number; expected: number; planned: number; generated: number; primed: number; settled: number; plan_task_id: string; plan_status: string; selection_task_id: string; selection_status: string; }
export interface CreativeOrderItem { candidate_progress?: CreativeCandidateProgress | null; id: string; order_id: string; source_kind?: string; copy_library_id?: string; candidate_id: string; source_analysis_id: string; copy_snapshot: Record<string, unknown>; direction: string; status: string; adopted_variant_id: string; adopted_at: string; adopted_by: string; created_at: string; updated_at: string; variants: CreativeOrderVariant[]; }
export interface AdoptCreativeOrderVariantRequest {
  variant_id: string;
  qc_risk_acknowledged?: boolean;
  qc_risk_reason?: string;
}
export interface CreativeOrderVariantBlocker { task_id: string; workflow: string; failure_reason: string; detail: string; failed_at: string; retryable: boolean; }
export interface CreativeOrderDiagnosticAsset { id: string; variant_id: string; task_id: string; attachment_id: string; size_key: CreativeDeliverySize | string; revision: number; workflow: string; label: string; filename: string; metadata: Record<string, unknown>; url: string; created_at: string; updated_at: string; }
export interface CreativeOrderVariantRevision { revision: number; brief: Record<string, unknown>; status: string; expected_sizes: (CreativeDeliverySize | string)[]; activated_at: string; created_at: string; updated_at: string; }
export interface CreativeImageOperationAttempt { id: string; attempt: number; status: string; runtime_id: string; task_id: string; provider_request_id: string; provider_status: string; http_status?: number | null; exit_code?: number | null; error_type: string; error_message: string; result_receipt: Record<string, unknown>; output_attachment_id: string; duration_ms?: number | null; started_at: string; completed_at: string; created_at: string; updated_at: string; }
export interface CreativeImageOperation { id: string; variant_id: string; size_key: CreativeDeliverySize | string; revision: number; operation_kind: string; idempotency_key: string; status: string; model: string; runtime_id: string; task_id: string; prompt_sha256: string; input_snapshot: Record<string, unknown>; provider_request_id: string; result_receipt: Record<string, unknown>; error_type: string; error_message: string; output_attachment_id: string; output_asset_id: string; started_at: string; completed_at: string; created_at: string; updated_at: string; attempts: CreativeImageOperationAttempt[]; }
export interface CreativeOrderVariant { id: string; order_item_id: string; variant_key: string; brief: Record<string, unknown>; revision: number; status: string; active_revision: number; staging_revision: number; candidate_state?: "candidate" | "selected" | "reserve" | "rejected" | string; selection_rank?: number; primary_size?: CreativeDeliverySize | string; qc_status: string; qc_recovery_used: boolean; qc_recovery_available: boolean; action_required?: CreativeOrderVariantBlocker; created_at: string; updated_at: string; assets: CreativeOrderAsset[]; revisions?: CreativeOrderVariantRevision[]; image_operations?: CreativeImageOperation[]; diagnostic_assets: CreativeOrderDiagnosticAsset[]; qc_reports: CreativeOrderQCReport[]; }
export interface CreativeOrderAsset { id: string; variant_id: string; asset_family_id: string; size_key: CreativeDeliverySize | string; revision: number; stage: "generated" | "primed" | "delivered" | string; attachment_id: string; derived_from_asset_id: string; operation_id?: string; metadata: Record<string, unknown>; evidence: Record<string, unknown>; status: string; created_at: string; updated_at: string; }
export interface CreativeOrderQCReport { id: string; variant_id: string; lane: "technical" | "visual" | string; revision: number; attempt: number; status: string; findings: Record<string, unknown>; trigger_evidence_kind: string; trigger_evidence_ref_id: string; created_at: string; updated_at: string; }
export interface CreativeOrderQCFinalizeResponse {
  created: boolean;
  finalized: boolean;
  outcome: "pending" | "delivered" | "action_required" | string;
  variant_id: string;
  revision: number;
  attempt: number;
  technical_status: string;
  visual_status: string;
  delivered_asset_count: number;
  order_aggregate_status: string;
  inbox_item_id?: string;
  rework_task_id?: string;
  rework_revision?: number;
}

export type CreativeDirectEditDeliveryMode = "preview" | "publish";

export interface CreateCreativeDirectEditRequest {
  issue_id: string;
  submission_key?: string;
  candidate_id: string;
  user_request: string;
  target_size: CreativeDeliverySize;
  delivery_mode: CreativeDirectEditDeliveryMode;
  squad_id: string;
}

export interface CreativeDirectEditResponse {
  order: CreativeOrder;
  item: CreativeOrderItem;
  variant: CreativeOrderVariant;
  source_asset: CreativeOrderAsset;
  task_id: string;
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
export type CreativeMaterialImportAnalysisAction =
  | "queued"
  | "already_queued"
  | "already_completed"
  | "unavailable"
  | "enqueue_failed"
  | string;
export interface CreativeMaterialImportAnalysis {
  action: CreativeMaterialImportAnalysisAction;
  status: "pending" | "running" | "completed" | "failed" | string;
  warning: string;
  crawl_run_id: string;
  analysis_agent_id: string;
  task_id: string;
}
export interface CreativeMaterialImportResult { id: string; analysis: CreativeMaterialImportAnalysis; }
export interface CreativeMaterialArchiveRetryResult { scheduled_count: number; }
