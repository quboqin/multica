# Creative Flow Diagnosis Source Map

| Contract | Source |
|---|---|
| Order detail exposes scoped recovery reasons, limits, task/asset lineage and attempt history without exposing lease tokens | `server/internal/handler/creative_recovery_response.go`, `server/internal/handler/creative_order_recovery.go` |
| Crawl diagnosis tasks are automatically assigned through the `crawl_diagnosis` creative role capability | `server/internal/handler/creative_crawl_diagnosis.go` |
| Creative Order detail exposes items, variants, current revision assets, QC reports, diagnostic assets, and workflow failures | `server/internal/handler/creative_domain.go:GetCreativeOrder`, `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderGet` |
| Business users can ask the `素材_诊断` agent with an order short ID, Variant label, card copy, or error text; the agent first resolves the current order item, Variant, revision, and latest task before explaining the stopped step | `scripts/bootstrap-creative-platform-demo.ps1:素材_诊断`, `scripts/creative-platform-skills/creative-flow-diagnostician/SKILL.md:入口识别` |
| Workflow failures are scoped to the current Variant revision before the order response exposes retry state | `server/internal/handler/creative_domain.go:listCreativeOrderWorkflowFailures` |
| Native task recovery is grouped by trigger evidence kind/ref and target Agent | `server/internal/handler/task_fanout.go`, `server/cmd/multica/cmd_task.go` |
| Production copy validation selects the current order item by `creative_order_item_id` and `variant_id` | `scripts/creative-platform-skills/ad-creative-production/references/validate_copy_snapshot.py` |
| Process images are exposed from registered diagnostic assets, backed by attachment storage, and stay separate from generated/primed/delivered creative order assets | `server/internal/handler/creative_domain.go:listCreativeOrderVariantDiagnosticAssets`, `server/migrations/265_creative_order_diagnostic_assets.up.sql`, `packages/views/creative/components/creative-order-delivery.tsx:CreativeProcessImageDialog` |

| Reference analysis dispatch belongs to the backend; direct analysis fanout is disabled | `server/internal/handler/creative_manual_analysis.go`, `server/internal/handler/creative_reference_analysis_recovery.go`, `server/internal/handler/task_fanout.go` |
