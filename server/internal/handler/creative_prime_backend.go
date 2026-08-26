package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/creative/primecompose"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const maxCreativePrimeInputBytes = 64 << 20

const defaultCreativePrimeTemplateCacheDir = "/app/data/uploads/creative-prime-cache"
const defaultCreativePrimeComposeConcurrency = 2

var creativeProductionProcessLabels = []string{
	"Prime context",
	"模型原图",
	"规范化底图",
}

var creativePrimeCompositionProcessLabels = []string{"Prime 合成成图"}

type creativePrimeGeneratedAsset struct {
	ID            pgtype.UUID
	AssetFamilyID pgtype.UUID
	SizeKey       string
	AttachmentID  pgtype.UUID
}

type creativePrimeFrozenMarketPack struct {
	ID      string                         `json:"id"`
	Version int                            `json:"version"`
	Config  json.RawMessage                `json:"config"`
	Files   []creativeResourceFileResponse `json:"files"`
}

type creativePrimeComposeReport struct {
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	Results   []json.RawMessage `json:"results"`
	Failures  []json.RawMessage `json:"failures"`
}

type creativePrimeComposeResult struct {
	ID                string          `json:"id"`
	Size              string          `json:"size"`
	Template          json.RawMessage `json:"template"`
	TemplateSelection json.RawMessage `json:"template_selection"`
	Compose           json.RawMessage `json:"compose"`
	QR                json.RawMessage `json:"qr"`
}

type creativePrimeVariantLock struct {
	mu   sync.Mutex
	refs int
}

type creativeQCHandoffError struct {
	cause error
}

func (err *creativeQCHandoffError) Error() string {
	return err.cause.Error()
}

func (err *creativeQCHandoffError) Unwrap() error {
	return err.cause
}

func (h *Handler) lockCreativePrimeVariant(variantID pgtype.UUID) func() {
	key := uuidToString(variantID)
	h.creativePrimeLocksMu.Lock()
	if h.creativePrimeLocks == nil {
		h.creativePrimeLocks = make(map[string]*creativePrimeVariantLock)
	}
	lock := h.creativePrimeLocks[key]
	if lock == nil {
		lock = &creativePrimeVariantLock{}
		h.creativePrimeLocks[key] = lock
	}
	lock.refs++
	h.creativePrimeLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		h.creativePrimeLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(h.creativePrimeLocks, key)
		}
		h.creativePrimeLocksMu.Unlock()
	}
}

func (h *Handler) acquireCreativePrimeComposeSlot(ctx context.Context) (func(), error) {
	limit := configuredCreativePrimeComposeConcurrency()
	h.creativePrimeSlotsMu.Lock()
	if h.creativePrimeSlots == nil || h.creativePrimeSlotCap != limit {
		h.creativePrimeSlots = make(chan struct{}, limit)
		h.creativePrimeSlotCap = limit
	}
	slots := h.creativePrimeSlots
	h.creativePrimeSlotsMu.Unlock()

	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func configuredCreativePrimeComposeConcurrency() int {
	raw := strings.TrimSpace(os.Getenv("MULTICA_CREATIVE_PRIME_MAX_CONCURRENT"))
	if raw == "" {
		return defaultCreativePrimeComposeConcurrency
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return defaultCreativePrimeComposeConcurrency
	}
	if value > 8 {
		return 8
	}
	return value
}

// composeCreativeOrderVariantPrime owns the fixed full-template composition
// boundary. Image agents provide only generated bases; the API loads the
// frozen order contract and applies an unmodified transparent template. Direct
// adjustments preserve that deterministic boundary, then enter final visual
// validation before delivery.
func (h *Handler) composeCreativeOrderVariantPrime(
	ctx context.Context,
	workspaceID, orderID, variantID, requestedBy pgtype.UUID,
	force bool,
) (bool, error) {
	if h.Storage == nil {
		return false, errors.New("brand component storage is unavailable")
	}
	unlockPrime := h.lockCreativePrimeVariant(variantID)
	defer unlockPrime()

	var variantKey, triggerKind, inputSnapshot, brief string
	var revision int
	var createdBy pgtype.UUID
	if err := h.DB.QueryRow(ctx, `
SELECT variant.variant_key, variant.revision, order_row.trigger_evidence_kind,
       order_row.input_snapshot::text, variant.brief::text, order_row.created_by
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
`, variantID, orderID, workspaceID).Scan(&variantKey, &revision, &triggerKind, &inputSnapshot, &brief, &createdBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, errors.New("creative variant does not belong to this order")
		}
		return false, fmt.Errorf("load brand component input: %w", err)
	}
	directDelivery := parseCreativeDirectEditDeliveryConfig(json.RawMessage(brief))
	skipQC := creativePrimeSkipsQC(triggerKind, json.RawMessage(brief), directDelivery)

	if triggerKind == "creative_direct_edit" && directDelivery.DeliveryMode == "preview" {
		return false, nil
	}

	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		return false, err
	}

	generated, complete, err := h.loadCreativePrimeGeneratedAssets(ctx, variantID, revision, expectedSizes)
	if err != nil {
		return false, err
	}
	if !complete {
		return false, nil
	}

	primed, primedComplete, err := h.creativePrimePackageComplete(ctx, variantID, revision, expectedSizes)
	if err != nil {
		return false, err
	}
	if primedComplete && !force {
		missingProcess, processErr := h.creativePrimeProcessEvidenceMissing(ctx, variantID, revision, expectedSizes)
		if processErr != nil {
			return false, processErr
		}
		if len(missingProcess) > 0 {
			return false, fmt.Errorf("creative Prime process evidence is incomplete: %s", strings.Join(missingProcess, ", "))
		}
		if skipQC {
			tx, txErr := h.TxStarter.Begin(ctx)
			if txErr != nil {
				return false, fmt.Errorf("start direct adjustment delivery: %w", txErr)
			}
			defer tx.Rollback(ctx)
			if _, txErr := copyCreativePrimedAssetsToDelivered(ctx, tx, variantID, revision, expectedSizes); txErr != nil {
				return false, fmt.Errorf("register direct adjustment delivery: %w", txErr)
			}
			if _, txErr := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'completed',
    brief = brief - 'brand_composition_error' - 'creative_qc_handoff_error',
    updated_at = now()
WHERE id = $1
`, variantID); txErr != nil {
				return false, fmt.Errorf("complete direct adjustment variant: %w", txErr)
			}
			if txErr := tx.Commit(ctx); txErr != nil {
				return false, fmt.Errorf("save direct adjustment delivery: %w", txErr)
			}
			h.notifyCreativeDirectAdjustmentDelivery(ctx, workspaceID, orderID, variantID, revision, directDelivery.TargetSize)
			return len(primed) > 0, nil
		}
		if err := h.enqueueCreativeVariantQC(ctx, workspaceID, orderID, variantID, requestedBy); err != nil {
			return false, &creativeQCHandoffError{cause: err}
		}
		h.clearCreativeQCHandoffError(ctx, variantID)
		return len(primed) > 0, nil
	}

	marketPack, templateSet, filesByRole, err := frozenCreativePrimeMarketPack(json.RawMessage(inputSnapshot))
	if err != nil {
		return false, err
	}

	releasePrimeSlot, err := h.acquireCreativePrimeComposeSlot(ctx)
	if err != nil {
		return false, fmt.Errorf("wait for brand component compose slot: %w", err)
	}
	defer releasePrimeSlot()

	workDir, err := os.MkdirTemp("", "multica-creative-prime-")
	if err != nil {
		return false, fmt.Errorf("create brand component work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	manifest := map[string]any{
		"variant_id":     uuidToString(variantID),
		"variant_key":    variantKey,
		"revision":       revision,
		"expected_sizes": expectedSizes,
		"sources":        map[string]string{},
		"jobs":           make([]map[string]string, 0, len(expectedSizes)),
	}
	sources := manifest["sources"].(map[string]string)
	for _, family := range templateSet.Families {
		for _, template := range family.Templates {
			file := filesByRole[template.SourceRole]
			attachmentID, parseErr := parseUUIDString(file.AttachmentID)
			if parseErr != nil {
				return false, fmt.Errorf("frozen template %s has an invalid attachment", template.SourceRole)
			}
			data, downloadErr := h.readCreativePrimeTemplateAttachment(ctx, workspaceID, attachmentID)
			if downloadErr != nil {
				return false, fmt.Errorf("load frozen template %s: %w", template.SourceRole, downloadErr)
			}
			filename := filepath.ToSlash(filepath.Join("templates", template.SourceRole+".png"))
			if mkdirErr := os.MkdirAll(filepath.Dir(filepath.Join(workDir, filepath.FromSlash(filename))), 0o700); mkdirErr != nil {
				return false, fmt.Errorf("prepare frozen template directory: %w", mkdirErr)
			}
			if writeErr := os.WriteFile(filepath.Join(workDir, filepath.FromSlash(filename)), data, 0o600); writeErr != nil {
				return false, fmt.Errorf("write frozen template %s: %w", template.SourceRole, writeErr)
			}
			sources[template.SourceRole] = filename
		}
	}

	generatedBySize := make(map[string]creativePrimeGeneratedAsset, len(generated))
	for _, asset := range generated {
		generatedBySize[asset.SizeKey] = asset
	}
	for _, size := range expectedSizes {
		asset := generatedBySize[size]
		data, downloadErr := h.readCreativePrimeAttachment(ctx, workspaceID, asset.AttachmentID)
		if downloadErr != nil {
			return false, fmt.Errorf("load generated %s base: %w", size, downloadErr)
		}
		input := filepath.ToSlash(filepath.Join("bases", "generated-"+size+".png"))
		output := "final-" + size + ".png"
		if mkdirErr := os.MkdirAll(filepath.Dir(filepath.Join(workDir, filepath.FromSlash(input))), 0o700); mkdirErr != nil {
			return false, fmt.Errorf("prepare generated base directory: %w", mkdirErr)
		}
		if writeErr := os.WriteFile(filepath.Join(workDir, filepath.FromSlash(input)), data, 0o600); writeErr != nil {
			return false, fmt.Errorf("write generated %s base: %w", size, writeErr)
		}
		manifest["jobs"] = append(manifest["jobs"].([]map[string]string), map[string]string{
			"id":     uuidToString(asset.ID),
			"size":   size,
			"input":  input,
			"output": output,
		})
	}

	manifestPath := filepath.Join(workDir, "manifest.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode brand component manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		return false, fmt.Errorf("write brand component manifest: %w", err)
	}
	snapshotPath := filepath.Join(workDir, "order-snapshot.json")
	if err := os.WriteFile(snapshotPath, []byte(inputSnapshot), 0o600); err != nil {
		return false, fmt.Errorf("write frozen order snapshot: %w", err)
	}
	if err := primecompose.HydrateManifestContract(manifestPath, snapshotPath); err != nil {
		return false, fmt.Errorf("prepare frozen brand component contract: %w", err)
	}

	composeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if err := primecompose.Run(composeCtx, manifestPath, &stdout, &stderr); err != nil {
		return false, fmt.Errorf("compose brand components: %w%s", err, creativePrimeCommandDetail(stdout.String(), stderr.String()))
	}
	var report creativePrimeComposeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return false, fmt.Errorf("decode brand component result: %w", err)
	}
	if report.Failed != 0 || report.Succeeded != len(expectedSizes) || len(report.Results) != len(expectedSizes) {
		return false, errors.New("brand component composition did not complete every delivery size")
	}
	sanitizedReport, err := sanitizeCreativePrimeComposeReport(stdout.Bytes())
	if err != nil {
		return false, err
	}

	hydratedManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, fmt.Errorf("read hydrated brand component manifest: %w", err)
	}
	manifestAttachmentID, err := h.storeCreativePrimeAttachment(ctx, workspaceID, createdBy, orderID, variantID, revision, "brand-composition-manifest.json", hydratedManifest, "application/json")
	if err != nil {
		return false, err
	}
	composeAttachmentID, err := h.storeCreativePrimeAttachment(ctx, workspaceID, createdBy, orderID, variantID, revision, "brand-composition-result.json", sanitizedReport, "application/json")
	if err != nil {
		return false, err
	}

	resultByID := make(map[string]creativePrimeComposeResult, len(report.Results))
	for _, raw := range report.Results {
		var result creativePrimeComposeResult
		if err := json.Unmarshal(raw, &result); err != nil || result.ID == "" || result.Size == "" {
			return false, errors.New("brand component result contains an invalid output")
		}
		resultByID[result.ID] = result
	}

	type composedAsset struct {
		Generated  creativePrimeGeneratedAsset
		Attachment pgtype.UUID
		Metadata   json.RawMessage
		Evidence   json.RawMessage
	}
	composed := make([]composedAsset, 0, len(expectedSizes))
	for _, size := range expectedSizes {
		generatedAsset := generatedBySize[size]
		result, found := resultByID[uuidToString(generatedAsset.ID)]
		if !found || result.Size != size {
			return false, fmt.Errorf("brand component result is missing %s", size)
		}
		output, readErr := os.ReadFile(filepath.Join(workDir, "final-"+size+".png"))
		if readErr != nil {
			return false, fmt.Errorf("read composed %s image: %w", size, readErr)
		}
		attachmentID, uploadErr := h.storeCreativePrimeAttachment(ctx, workspaceID, createdBy, orderID, variantID, revision, "final-"+size+".png", output, "image/png")
		if uploadErr != nil {
			return false, uploadErr
		}
		metadata, metadataErr := json.Marshal(map[string]any{
			"composition": "backend_full_transparent_template",
			"market_pack": map[string]any{
				"id": marketPack.ID, "version": marketPack.Version,
			},
			"template": json.RawMessage(result.Template),
		})
		if metadataErr != nil {
			return false, fmt.Errorf("encode composed %s metadata: %w", size, metadataErr)
		}
		evidence, evidenceErr := json.Marshal(map[string]any{
			"package_contract_version":     6,
			"manifest_attachment_id":       uuidToString(manifestAttachmentID),
			"compose_result_attachment_id": uuidToString(composeAttachmentID),
			"template_selection":           json.RawMessage(result.TemplateSelection),
			"compose":                      json.RawMessage(result.Compose),
			"qr":                           json.RawMessage(result.QR),
		})
		if evidenceErr != nil {
			return false, fmt.Errorf("encode composed %s evidence: %w", size, evidenceErr)
		}
		composed = append(composed, composedAsset{Generated: generatedAsset, Attachment: attachmentID, Metadata: metadata, Evidence: evidence})
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("start brand component registration: %w", err)
	}
	defer tx.Rollback(ctx)
	var currentRevision int
	if err := tx.QueryRow(ctx, `SELECT revision FROM creative_order_variant WHERE id = $1 FOR UPDATE`, variantID).Scan(&currentRevision); err != nil {
		return false, fmt.Errorf("lock creative variant for brand component registration: %w", err)
	}
	if currentRevision != revision {
		return false, errors.New("creative variant revision changed during brand component composition")
	}
	for _, asset := range composed {
		if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
VALUES ($1,$2,$3,$4,'primed',$5,$6,$7::jsonb,$8::jsonb,'completed')
ON CONFLICT (variant_id, size_key, revision, stage) DO UPDATE SET
  asset_family_id = EXCLUDED.asset_family_id,
  attachment_id = EXCLUDED.attachment_id,
  derived_from_asset_id = EXCLUDED.derived_from_asset_id,
  metadata = EXCLUDED.metadata,
  evidence = EXCLUDED.evidence,
  status = EXCLUDED.status,
  updated_at = now()
`, variantID, asset.Generated.AssetFamilyID, asset.Generated.SizeKey, revision, asset.Attachment, asset.Generated.ID, asset.Metadata, asset.Evidence); err != nil {
			return false, fmt.Errorf("register composed %s asset: %w", asset.Generated.SizeKey, err)
		}
		processMetadata, metadataErr := json.Marshal(map[string]any{
			"process_stage":   "Prime 合成成图",
			"source_asset_id": uuidToString(asset.Generated.ID),
		})
		if metadataErr != nil {
			return false, fmt.Errorf("encode composed %s process evidence: %w", asset.Generated.SizeKey, metadataErr)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
VALUES ($1,NULL,$2,$3,$4,'brand_components','Prime 合成成图',$5,$6::jsonb)
ON CONFLICT (variant_id, revision, workflow, size_key, label, filename) DO UPDATE SET
  attachment_id = EXCLUDED.attachment_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
`, variantID, asset.Attachment, asset.Generated.SizeKey, revision, "final-"+asset.Generated.SizeKey+".png", processMetadata); err != nil {
			return false, fmt.Errorf("register composed %s process image: %w", asset.Generated.SizeKey, err)
		}
	}
	if skipQC {
		if _, err := copyCreativePrimedAssetsToDelivered(ctx, tx, variantID, revision, expectedSizes); err != nil {
			return false, fmt.Errorf("register direct adjustment delivery: %w", err)
		}
		if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'completed',
    brief = brief - 'brand_composition_error' - 'creative_qc_handoff_error',
    updated_at = now()
WHERE id = $1
`, variantID); err != nil {
			return false, fmt.Errorf("complete direct adjustment variant: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'running',
    brief = brief - 'brand_composition_error' - 'creative_qc_handoff_error',
    updated_at = now()
WHERE id = $1
`, variantID); err != nil {
		return false, fmt.Errorf("advance composed creative variant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("save composed creative assets: %w", err)
	}
	if skipQC {
		h.notifyCreativeDirectAdjustmentDelivery(ctx, workspaceID, orderID, variantID, revision, directDelivery.TargetSize)
		return true, nil
	}
	if err := h.enqueueCreativeVariantQC(ctx, workspaceID, orderID, variantID, requestedBy); err != nil {
		return true, &creativeQCHandoffError{cause: err}
	}
	return true, nil
}

// creativePrimeSkipsQC preserves the legacy direct-edit delivery mode while
// allowing an annotated direct adjustment to explicitly require final-image
// validation even if its parent order was created as a direct edit.
func creativePrimeSkipsQC(triggerKind string, brief json.RawMessage, delivery creativeDirectEditDeliveryConfig) bool {
	return (triggerKind == "creative_direct_edit" && !delivery.FinalVisualValidation) || creativeDirectEditSkipsQC(brief)
}

func creativePrimeCommandDetail(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	if value := strings.TrimSpace(stderr); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(stdout); value != "" {
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return ""
	}
	detail := strings.Join(parts, "; ")
	if len(detail) > 1200 {
		detail = detail[:1200]
	}
	return ": " + detail
}

func sanitizeCreativePrimeComposeReport(raw []byte) ([]byte, error) {
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("decode brand component result: %w", err)
	}
	if results, ok := report["results"].([]any); ok {
		for _, rawResult := range results {
			if result, ok := rawResult.(map[string]any); ok {
				delete(result, "input")
				delete(result, "output")
			}
		}
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode brand component result: %w", err)
	}
	return encoded, nil
}

func frozenCreativePrimeMarketPack(raw json.RawMessage) (creativePrimeFrozenMarketPack, *primeTemplateSetConfig, map[string]creativeResourceFileResponse, error) {
	var snapshot struct {
		MarketPack creativePrimeFrozenMarketPack `json:"market_pack"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil || strings.TrimSpace(snapshot.MarketPack.ID) == "" || len(snapshot.MarketPack.Config) == 0 {
		return creativePrimeFrozenMarketPack{}, nil, nil, errors.New("creative order is missing a frozen published market pack")
	}
	templateSet, err := parsePrimeTemplateSetConfig(snapshot.MarketPack.Config)
	if err != nil {
		return creativePrimeFrozenMarketPack{}, nil, nil, err
	}
	filesByRole, err := primeTemplateFilesByRole(templateSet, snapshot.MarketPack.Files)
	if err != nil {
		return creativePrimeFrozenMarketPack{}, nil, nil, err
	}
	return snapshot.MarketPack, templateSet, filesByRole, nil
}

func (h *Handler) loadCreativePrimeGeneratedAssets(ctx context.Context, variantID pgtype.UUID, revision int, expectedSizes []string) ([]creativePrimeGeneratedAsset, bool, error) {
	rows, err := h.DB.Query(ctx, `
SELECT id, asset_family_id, size_key, attachment_id
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'generated' AND status = 'completed'
  AND attachment_id IS NOT NULL AND size_key = ANY($3::text[])
ORDER BY size_key
`, variantID, revision, expectedSizes)
	if err != nil {
		return nil, false, fmt.Errorf("load generated creative assets: %w", err)
	}
	defer rows.Close()
	assets := make([]creativePrimeGeneratedAsset, 0, len(expectedSizes))
	sizes := make(map[string]struct{}, len(expectedSizes))
	for rows.Next() {
		var asset creativePrimeGeneratedAsset
		if err := rows.Scan(&asset.ID, &asset.AssetFamilyID, &asset.SizeKey, &asset.AttachmentID); err != nil {
			return nil, false, fmt.Errorf("read generated creative asset: %w", err)
		}
		assets = append(assets, asset)
		sizes[asset.SizeKey] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read generated creative assets: %w", err)
	}
	return assets, creativeSizesMatchExpected(sizes, expectedSizes), nil
}

func (h *Handler) creativePrimePackageComplete(ctx context.Context, variantID pgtype.UUID, revision int, expectedSizes []string) ([]pgtype.UUID, bool, error) {
	rows, err := h.DB.Query(ctx, `
SELECT attachment_id, size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
  AND attachment_id IS NOT NULL AND size_key = ANY($3::text[])
`, variantID, revision, expectedSizes)
	if err != nil {
		return nil, false, fmt.Errorf("load composed creative assets: %w", err)
	}
	defer rows.Close()
	attachments := make([]pgtype.UUID, 0, len(expectedSizes))
	sizes := make(map[string]struct{}, len(expectedSizes))
	for rows.Next() {
		var attachmentID pgtype.UUID
		var size string
		if err := rows.Scan(&attachmentID, &size); err != nil {
			return nil, false, fmt.Errorf("read composed creative asset: %w", err)
		}
		attachments = append(attachments, attachmentID)
		sizes[size] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read composed creative assets: %w", err)
	}
	return attachments, creativeSizesMatchExpected(sizes, expectedSizes), nil
}

func (h *Handler) creativeProcessEvidenceMissing(ctx context.Context, variantID pgtype.UUID, revision int, expectedSizes []string, workflow string, requiredLabels []string) ([]string, error) {
	rows, err := h.DB.Query(ctx, `
SELECT size_key, label
FROM creative_order_diagnostic_asset
WHERE variant_id = $1
  AND revision = $2
  AND workflow = $3
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
  AND label = ANY($5::text[])
`, variantID, revision, workflow, expectedSizes, requiredLabels)
	if err != nil {
		return nil, fmt.Errorf("load creative process evidence: %w", err)
	}
	defer rows.Close()
	present := make(map[string]struct{}, len(expectedSizes)*len(requiredLabels))
	for rows.Next() {
		var size, label string
		if err := rows.Scan(&size, &label); err != nil {
			return nil, fmt.Errorf("read creative process evidence: %w", err)
		}
		present[size+"\x00"+label] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read creative process evidence: %w", err)
	}
	missing := make([]string, 0)
	for _, size := range expectedSizes {
		for _, label := range requiredLabels {
			if _, ok := present[size+"\x00"+label]; !ok {
				missing = append(missing, size+"/"+label)
			}
		}
	}
	return missing, nil
}

func (h *Handler) creativePrimeProcessEvidenceMissing(ctx context.Context, variantID pgtype.UUID, revision int, expectedSizes []string) ([]string, error) {
	productionMissing, err := h.creativeProcessEvidenceMissing(ctx, variantID, revision, expectedSizes, "creative_production", creativeProductionProcessLabels)
	if err != nil {
		return nil, err
	}
	compositionMissing, err := h.creativeProcessEvidenceMissing(ctx, variantID, revision, expectedSizes, "brand_components", creativePrimeCompositionProcessLabels)
	if err != nil {
		return nil, err
	}
	return append(productionMissing, compositionMissing...), nil
}

func (h *Handler) loadCreativePrimeAttachment(ctx context.Context, workspaceID, attachmentID pgtype.UUID) (db.Attachment, string, error) {
	attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID {
		return db.Attachment{}, "", errors.New("attachment is unavailable")
	}
	key := h.Storage.KeyFromURL(attachment.Url)
	if key == "" {
		return db.Attachment{}, "", errors.New("attachment storage key is unavailable")
	}
	return attachment, key, nil
}

func (h *Handler) readCreativePrimeAttachment(ctx context.Context, workspaceID, attachmentID pgtype.UUID) ([]byte, error) {
	_, key, err := h.loadCreativePrimeAttachment(ctx, workspaceID, attachmentID)
	if err != nil {
		return nil, err
	}
	return h.readCreativePrimeStorageObject(ctx, key)
}

func (h *Handler) readCreativePrimeTemplateAttachment(ctx context.Context, workspaceID, attachmentID pgtype.UUID) ([]byte, error) {
	attachment, key, err := h.loadCreativePrimeAttachment(ctx, workspaceID, attachmentID)
	if err != nil {
		return nil, err
	}
	if data, ok := readCreativePrimeTemplateCache(attachment); ok {
		return data, nil
	}
	data, err := h.readCreativePrimeStorageObject(ctx, key)
	if err != nil {
		return nil, err
	}
	writeCreativePrimeTemplateCache(attachment, data)
	return data, nil
}

func (h *Handler) readCreativePrimeStorageObject(ctx context.Context, key string) ([]byte, error) {
	reader, err := h.Storage.GetReader(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxCreativePrimeInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCreativePrimeInputBytes {
		return nil, errors.New("attachment exceeds the brand component input limit")
	}
	return data, nil
}

func readCreativePrimeTemplateCache(attachment db.Attachment) ([]byte, bool) {
	path, ok := creativePrimeTemplateCachePath(attachment)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if attachment.SizeBytes > 0 && info.Size() != attachment.SizeBytes {
		os.Remove(path)
		return nil, false
	}
	if info.Size() > maxCreativePrimeInputBytes {
		os.Remove(path)
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if attachment.SizeBytes > 0 && int64(len(data)) != attachment.SizeBytes {
		os.Remove(path)
		return nil, false
	}
	if len(data) > maxCreativePrimeInputBytes {
		os.Remove(path)
		return nil, false
	}
	return data, true
}

func writeCreativePrimeTemplateCache(attachment db.Attachment, data []byte) {
	if len(data) == 0 || len(data) > maxCreativePrimeInputBytes {
		return
	}
	if attachment.SizeBytes > 0 && int64(len(data)) != attachment.SizeBytes {
		return
	}
	path, ok := creativePrimeTemplateCachePath(attachment)
	if !ok {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	file, err := os.CreateTemp(dir, ".prime-template-*.tmp")
	if err != nil {
		return
	}
	tmpPath := file.Name()
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
}

func creativePrimeTemplateCachePath(attachment db.Attachment) (string, bool) {
	attachmentID := uuidToString(attachment.ID)
	if attachmentID == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(attachmentID + "\x00" + attachment.Url))
	name := attachmentID + "-" + hex.EncodeToString(sum[:8]) + ".bin"
	return filepath.Join(creativePrimeTemplateCacheDir(), name), true
}

func creativePrimeTemplateCacheDir() string {
	if dir := strings.TrimSpace(os.Getenv("MULTICA_CREATIVE_PRIME_CACHE_DIR")); dir != "" {
		return dir
	}
	if uploadDir := strings.TrimSpace(os.Getenv("LOCAL_UPLOAD_DIR")); uploadDir != "" {
		return filepath.Join(uploadDir, "creative-prime-cache")
	}
	return defaultCreativePrimeTemplateCacheDir
}

func (h *Handler) storeCreativePrimeAttachment(
	ctx context.Context,
	workspaceID, createdBy, orderID, variantID pgtype.UUID,
	revision int,
	filename string,
	data []byte,
	contentType string,
) (pgtype.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("create brand component attachment id: %w", err)
	}
	attachmentID := pgtype.UUID{Bytes: id, Valid: true}
	key := filepath.ToSlash(filepath.Join(
		"workspaces", uuidToString(workspaceID), "creative-orders", uuidToString(orderID),
		"variants", uuidToString(variantID), fmt.Sprintf("r%d", revision), "brand-components", id.String()+"-"+filename,
	))
	url, err := h.Storage.Upload(ctx, key, data, contentType, filename)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("upload %s: %w", filename, err)
	}
	if !createdBy.Valid {
		h.Storage.Delete(ctx, key)
		return pgtype.UUID{}, errors.New("creative order is missing its owner")
	}
	if _, err := h.Queries.CreateAttachment(ctx, db.CreateAttachmentParams{
		ID:           attachmentID,
		WorkspaceID:  workspaceID,
		UploaderType: "member",
		UploaderID:   createdBy,
		Filename:     filename,
		Url:          url,
		ContentType:  contentType,
		SizeBytes:    int64(len(data)),
	}); err != nil {
		h.Storage.Delete(ctx, key)
		return pgtype.UUID{}, fmt.Errorf("register %s: %w", filename, err)
	}
	return attachmentID, nil
}

func (h *Handler) enqueueCreativeVariantQC(ctx context.Context, workspaceID, orderID, variantID, requestedBy pgtype.UUID) error {
	if h.TaskService == nil {
		return errors.New("creative QC task service is unavailable")
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("start creative QC handoff: %w", err)
	}
	defer tx.Rollback(ctx)

	var itemID, variantKey, triggerKind, inputSnapshot, brief string
	var revision int
	var issueID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT item.id::text, variant.variant_key, variant.revision, order_row.issue_id,
       order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
FOR UPDATE OF variant
`, variantID, orderID, workspaceID).Scan(&itemID, &variantKey, &revision, &issueID, &triggerKind, &inputSnapshot, &brief); err != nil {
		return fmt.Errorf("load creative QC handoff: %w", err)
	}
	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		return err
	}
	_, complete, err := h.creativePrimePackageComplete(ctx, variantID, revision, expectedSizes)
	if err != nil {
		return err
	}
	if !complete {
		return errors.New("brand component package is incomplete")
	}
	var alreadyResolved bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2)`, variantID, revision).Scan(&alreadyResolved); err != nil {
		return fmt.Errorf("check creative QC resolution: %w", err)
	}
	if alreadyResolved {
		return nil
	}
	missingProcess, err := h.creativePrimeProcessEvidenceMissing(ctx, variantID, revision, expectedSizes)
	if err != nil {
		return err
	}
	if len(missingProcess) > 0 {
		return fmt.Errorf("creative Prime process evidence is incomplete: %s", strings.Join(missingProcess, ", "))
	}

	var existingTasks bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM agent_task_queue
  WHERE trigger_evidence_kind = 'creative_order_variant_qc'
    AND trigger_evidence_ref_id = $1
    AND context->>'creative_order_id' = $2::text
    AND COALESCE(NULLIF(context->>'revision', '')::int, 1) = $3
    AND COALESCE(context->>'superseded_by_candidate', 'false') <> 'true'
)
`, variantID, orderID, revision).Scan(&existingTasks); err != nil {
		return fmt.Errorf("check creative QC handoff: %w", err)
	}
	if existingTasks {
		return nil
	}

	leaderID, reviewerID, err := creativeOrderQCAgentSnapshot(json.RawMessage(inputSnapshot))
	if err != nil {
		return err
	}
	reviewer, err := h.Queries.WithTx(tx).GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: reviewerID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || reviewer.ArchivedAt.Valid || !reviewer.RuntimeID.Valid {
		return errors.New("the frozen creative QC reviewer is unavailable")
	}
	if err != nil {
		return fmt.Errorf("load creative QC reviewer: %w", err)
	}
	var reviewerCapable bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill binding
  JOIN skill skill_row ON skill_row.id = binding.skill_id
  WHERE binding.agent_id = $1 AND binding.enabled
    AND skill_row.workspace_id = $2
    AND skill_row.config->>'kind' = 'creative_role'
    AND skill_row.config->>'capability' = 'quality_control'
)
`, reviewer.ID, workspaceID).Scan(&reviewerCapable); err != nil {
		return fmt.Errorf("validate creative QC reviewer: %w", err)
	}
	if !reviewerCapable {
		return errors.New("the frozen creative QC reviewer no longer provides quality_control")
	}

	manifestAttachmentID, composeAttachmentID, err := h.creativePrimeEvidenceAttachments(ctx, variantID, revision, expectedSizes)
	if err != nil {
		return err
	}
	attr := attribution.DirectHumanRun(requestedBy, attribution.EvidenceKind("creative_order_variant_qc"), variantID)
	created := make([]db.AgentTaskQueue, 0, 1)
	for _, lane := range []string{"visual"} {
		contextValue, marshalErr := json.Marshal(map[string]any{
			"type":                         "creative_domain_task",
			"workflow":                     "creative_qc_" + lane,
			"scope":                        "variant",
			"subject_id":                   uuidToString(variantID),
			"item_key":                     fmt.Sprintf("%s:%s:r%d", uuidToString(variantID), lane, revision),
			"creative_order_id":            uuidToString(orderID),
			"creative_order_item_id":       itemID,
			"variant_id":                   uuidToString(variantID),
			"revision":                     revision,
			"qc_attempt":                   defaultCreativeQCAttempt,
			"expected_sizes":               expectedSizes,
			"issue_id":                     uuidToString(issueID),
			"leader_agent_id":              uuidToString(leaderID),
			"manifest_attachment_id":       uuidToString(manifestAttachmentID),
			"compose_result_attachment_id": uuidToString(composeAttachmentID),
		})
		if marshalErr != nil {
			return fmt.Errorf("encode %s creative QC task: %w", lane, marshalErr)
		}
		if err := validateCreativeTaskFanoutContext("creative_order_variant_qc", variantID, []service.DirectTaskFanoutItem{{
			ItemKey: fmt.Sprintf("%s:%s:r%d", uuidToString(variantID), lane, revision), Context: contextValue,
		}}); err != nil {
			return err
		}
		task, createErr := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
			AgentID:              reviewer.ID,
			RuntimeID:            reviewer.RuntimeID,
			Priority:             0,
			ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
			RequestingUserID:     requestedBy,
			OriginatorUserID:     attr.UserID,
			AccountableUserID:    attr.AccountableUserID,
			OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
			TriggerEvidenceKind:  pgtype.Text{String: "creative_order_variant_qc", Valid: true},
			TriggerEvidenceRefID: variantID,
			Context:              contextValue,
		})
		if createErr != nil {
			return fmt.Errorf("queue %s creative QC: %w", lane, createErr)
		}
		created = append(created, task)
	}
	if _, err := tx.Exec(ctx, `UPDATE creative_order_variant SET status = 'running', updated_at = now() WHERE id = $1`, variantID); err != nil {
		return fmt.Errorf("advance creative QC variant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("save creative QC handoff: %w", err)
	}
	for _, task := range created {
		h.TaskService.NotifyTaskEnqueued(ctx, task)
	}
	return nil
}

func (h *Handler) creativePrimeEvidenceAttachments(ctx context.Context, variantID pgtype.UUID, revision int, expectedSizes []string) (pgtype.UUID, pgtype.UUID, error) {
	rows, err := h.DB.Query(ctx, `
SELECT evidence::text
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
  AND size_key = ANY($3::text[])
`, variantID, revision, expectedSizes)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("load brand component evidence: %w", err)
	}
	defer rows.Close()
	manifestIDs := map[string]pgtype.UUID{}
	composeIDs := map[string]pgtype.UUID{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("read brand component evidence: %w", err)
		}
		var evidence struct {
			ManifestAttachmentID      string `json:"manifest_attachment_id"`
			ComposeResultAttachmentID string `json:"compose_result_attachment_id"`
		}
		if json.Unmarshal([]byte(raw), &evidence) != nil {
			return pgtype.UUID{}, pgtype.UUID{}, errors.New("brand component evidence is invalid")
		}
		manifestID, parseErr := parseUUIDString(evidence.ManifestAttachmentID)
		if parseErr != nil {
			return pgtype.UUID{}, pgtype.UUID{}, errors.New("brand component manifest evidence is missing")
		}
		composeID, parseErr := parseUUIDString(evidence.ComposeResultAttachmentID)
		if parseErr != nil {
			return pgtype.UUID{}, pgtype.UUID{}, errors.New("brand component result evidence is missing")
		}
		manifestIDs[uuidToString(manifestID)] = manifestID
		composeIDs[uuidToString(composeID)] = composeID
	}
	if err := rows.Err(); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("read brand component evidence: %w", err)
	}
	if len(manifestIDs) != 1 || len(composeIDs) != 1 {
		return pgtype.UUID{}, pgtype.UUID{}, errors.New("brand component package evidence is inconsistent")
	}
	manifestKeys := make([]string, 0, 1)
	for key := range manifestIDs {
		manifestKeys = append(manifestKeys, key)
	}
	composeKeys := make([]string, 0, 1)
	for key := range composeIDs {
		composeKeys = append(composeKeys, key)
	}
	sort.Strings(manifestKeys)
	sort.Strings(composeKeys)
	return manifestIDs[manifestKeys[0]], composeIDs[composeKeys[0]], nil
}

func (h *Handler) markCreativePrimeCompositionFailed(ctx context.Context, variantID pgtype.UUID, cause error) {
	if cause == nil {
		return
	}
	detail := strings.TrimSpace(cause.Error())
	if len(detail) > 1200 {
		detail = detail[:1200]
	}
	_, _ = h.DB.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'action_required',
    brief = jsonb_set(
      brief - 'creative_qc_handoff_error',
      '{brand_composition_error}',
      jsonb_build_object('message', $2::text, 'retryable', true, 'updated_at', now()::text),
      true
    ),
    updated_at = now()
WHERE id = $1
`, variantID, detail)
}

func (h *Handler) markCreativeQCHandoffFailed(ctx context.Context, variantID pgtype.UUID, cause error) {
	if cause == nil {
		return
	}
	detail := strings.TrimSpace(cause.Error())
	if len(detail) > 1200 {
		detail = detail[:1200]
	}
	_, _ = h.DB.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'action_required',
    brief = jsonb_set(
      brief - 'brand_composition_error',
      '{creative_qc_handoff_error}',
      jsonb_build_object('message', $2::text, 'retryable', true, 'updated_at', now()::text),
      true
    ),
    updated_at = now()
WHERE id = $1
`, variantID, detail)
}

func (h *Handler) clearCreativeQCHandoffError(ctx context.Context, variantID pgtype.UUID) {
	_, _ = h.DB.Exec(ctx, `
UPDATE creative_order_variant
SET brief = brief - 'creative_qc_handoff_error',
    updated_at = now()
WHERE id = $1
`, variantID)
}
