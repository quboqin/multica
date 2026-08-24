package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/storage"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	creativeVisualInspectMaxImages     = 3
	creativeVisualInspectMaxImageBytes = 25 << 20
	creativeVisualInspectMaxTotalBytes = 60 << 20
	creativeVisualInspectSignedURLTTL  = 15 * time.Minute
)

var (
	errCreativeVisualInspectionRevisionStale   = errors.New("creative order visual inspection revision is stale")
	errCreativeVisualInspectionVariantNotFound = errors.New("variant does not belong to this creative order")
)

type creativeOrderVisualInspectInput struct {
	VariantID string `json:"variant_id"`
	Revision  int    `json:"revision"`
}

type creativeVisualInspectionFinding struct {
	Code      string `json:"code"`
	SizeKey   string `json:"size_key"`
	Diagnosis string `json:"diagnosis"`
}

type creativeVisualInspectionCheckedAsset struct {
	SizeKey      string   `json:"size_key"`
	AttachmentID string   `json:"attachment_id"`
	Filename     string   `json:"filename"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	SizeBytes    int64    `json:"size_bytes"`
	SHA256       string   `json:"sha256"`
	Observations []string `json:"observations,omitempty"`
}

type creativeVisualInspectionReport struct {
	VariantID                string                                 `json:"variant_id"`
	Lane                     string                                 `json:"lane"`
	Revision                 int                                    `json:"revision"`
	Status                   string                                 `json:"status"`
	PrimeAssetsReadable      bool                                   `json:"prime_assets_readable"`
	KeyContentPreserved      bool                                   `json:"key_content_preserved"`
	CheckedAssets            []creativeVisualInspectionCheckedAsset `json:"checked_assets"`
	QualityWarnings          []creativeVisualInspectionFinding      `json:"quality_warnings"`
	BlockingFailures         []creativeVisualInspectionFinding      `json:"blocking_failures"`
	Evidence                 creativeVisualInspectionEvidence       `json:"evidence"`
	TriggerEvidenceKind      string                                 `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string                                 `json:"trigger_evidence_ref_id"`
}

type creativeVisualInspectionEvidence struct {
	SchemaVersion                int                                     `json:"schema_version"`
	InspectionID                 string                                  `json:"inspection_id"`
	InspectedAt                  string                                  `json:"inspected_at"`
	OrderID                      string                                  `json:"creative_order_id"`
	VariantID                    string                                  `json:"variant_id"`
	Revision                     int                                     `json:"revision"`
	TaskID                       string                                  `json:"task_id,omitempty"`
	ExpectedSizes                []string                                `json:"expected_sizes"`
	ModelProvider                string                                  `json:"model_provider,omitempty"`
	Model                        string                                  `json:"model,omitempty"`
	ModelConfigured              bool                                    `json:"model_configured"`
	ModelRequestID               string                                  `json:"model_request_id,omitempty"`
	ModelErrorCode               string                                  `json:"model_error_code,omitempty"`
	ModelErrorMessage            string                                  `json:"model_error_message,omitempty"`
	ContactSheetAttachmentID     string                                  `json:"contact_sheet_attachment_id,omitempty"`
	InspectionReportAttachmentID string                                  `json:"inspection_report_attachment_id,omitempty"`
	Assets                       []creativeVisualInspectionAssetEvidence `json:"assets"`
}

type creativeVisualInspectionAssetEvidence struct {
	SizeKey           string `json:"size_key"`
	AttachmentID      string `json:"attachment_id"`
	Filename          string `json:"filename"`
	ContentType       string `json:"content_type"`
	SizeBytes         int64  `json:"size_bytes"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	SHA256            string `json:"sha256"`
	SignedURLProvided bool   `json:"signed_url_provided"`
}

type creativeVisualInspectionProviderInput struct {
	OrderID       string
	VariantID     string
	Revision      int
	ExpectedSizes []string
	CopySnapshot  json.RawMessage
	Brief         json.RawMessage
	Assets        []creativeVisualInspectionProviderAsset
}

type creativeVisualInspectionProviderAsset struct {
	SizeKey     string
	Filename    string
	Width       int
	Height      int
	SizeBytes   int64
	SHA256      string
	SignedURL   string
	ContentType string
}

type creativeVisualInspectionModelResult struct {
	Status              string                                 `json:"status"`
	Summary             string                                 `json:"summary"`
	PrimeAssetsReadable bool                                   `json:"prime_assets_readable"`
	KeyContentPreserved bool                                   `json:"key_content_preserved"`
	CheckedAssets       []creativeVisualInspectionCheckedAsset `json:"checked_assets"`
	QualityWarnings     []creativeVisualInspectionFinding      `json:"quality_warnings"`
	BlockingFailures    []creativeVisualInspectionFinding      `json:"blocking_failures"`
	RequestID           string                                 `json:"request_id"`
}

type creativeVisualInspector interface {
	InspectCreativeVisual(ctx context.Context, input creativeVisualInspectionProviderInput) (creativeVisualInspectionModelResult, error)
	ProviderName() string
	ModelName() string
}

type creativeVisualInspectionAsset struct {
	ID           pgtype.UUID
	SizeKey      string
	AttachmentID pgtype.UUID
	Filename     string
	URL          string
	ContentType  string
	SizeBytes    int64
}

type creativeVisualInspectionOrderContext struct {
	CreatedBy     pgtype.UUID
	IssueID       pgtype.UUID
	TriggerKind   string
	InputSnapshot json.RawMessage
	Brief         json.RawMessage
	Revision      int
	Status        string
}

func (h *Handler) InspectCreativeOrderVisual(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var input creativeOrderVisualInspectInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative visual inspection request")
		return
	}
	input.VariantID = strings.TrimSpace(input.VariantID)
	if input.Revision < 1 {
		input.Revision = 1
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	task, ok := h.creativeQCActiveTask(w, r, orderID, variantID, input.Revision, "visual")
	if !ok {
		return
	}

	report, err := h.runCreativeOrderVisualInspection(r.Context(), workspaceID, orderID, variantID, input.Revision, task)
	if err != nil {
		slog.Error("creative visual inspection failed", "order_id", uuidToString(orderID), "variant_id", input.VariantID, "error", err)
		if errors.Is(err, errCreativeVisualInspectionRevisionStale) {
			writeError(w, http.StatusConflict, "creative order visual inspection revision is stale")
			return
		}
		if errors.Is(err, errCreativeVisualInspectionVariantNotFound) {
			writeError(w, http.StatusNotFound, "creative order variant not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to run creative visual inspection")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) runCreativeOrderVisualInspection(ctx context.Context, workspaceID, orderID, variantID pgtype.UUID, revision int, task db.AgentTaskQueue) (creativeVisualInspectionReport, error) {
	if h.Storage == nil {
		return creativeVisualInspectionReport{}, errors.New("creative visual inspection storage is unavailable")
	}
	order, err := h.loadCreativeVisualInspectionOrder(ctx, workspaceID, orderID, variantID)
	if err != nil {
		return creativeVisualInspectionReport{}, err
	}
	if order.Revision != revision {
		return creativeVisualInspectionReport{}, errCreativeVisualInspectionRevisionStale
	}
	expectedSizes, err := expectedCreativeVariantSizes(order.TriggerKind, order.InputSnapshot, order.Brief)
	if err != nil {
		return creativeVisualInspectionReport{}, err
	}
	report := newCreativeVisualInspectionReport(orderID, variantID, revision, expectedSizes)
	report.Evidence.ModelConfigured = h.CreativeVisualInspector != nil
	if h.CreativeVisualInspector != nil {
		report.Evidence.ModelProvider = h.CreativeVisualInspector.ProviderName()
		report.Evidence.Model = h.CreativeVisualInspector.ModelName()
	}
	if len(expectedSizes) > creativeVisualInspectMaxImages {
		report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
			Code:      "visual_inspection_input_limit_exceeded",
			SizeKey:   "all",
			Diagnosis: fmt.Sprintf("视觉检查最多支持 %d 张成图，本次 expected_sizes=%d。", creativeVisualInspectMaxImages, len(expectedSizes)),
		})
		return h.finalizeCreativeVisualInspectionReport(ctx, workspaceID, order.CreatedBy, orderID, variantID, revision, task, report)
	}

	assets, err := h.loadCreativeVisualInspectionAssets(ctx, variantID, revision, expectedSizes)
	if err != nil {
		return creativeVisualInspectionReport{}, err
	}
	packageFailures := creativeVisualInspectionPackageFailures(assets, expectedSizes)
	for _, failure := range packageFailures {
		report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
			Code:      "creative_prime_package_incomplete",
			SizeKey:   failure.sizeKey,
			Diagnosis: failure.message,
		})
	}
	if len(report.BlockingFailures) > 0 {
		return h.finalizeCreativeVisualInspectionReport(ctx, workspaceID, order.CreatedBy, orderID, variantID, revision, task, report)
	}

	tempDir, err := os.MkdirTemp("", "multica-visual-inspect-*")
	if err != nil {
		return creativeVisualInspectionReport{}, fmt.Errorf("create visual inspection temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	providerAssets := make([]creativeVisualInspectionProviderAsset, 0, len(assets))
	localImages := make([]creativeVisualInspectionLocalImage, 0, len(assets))
	presigner, canPresign := h.Storage.(storage.Presigner)
	totalBytes := int64(0)
	for _, asset := range assets {
		imageInfo, readErr := h.readCreativeVisualInspectionImage(ctx, tempDir, asset)
		if readErr != nil {
			report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
				Code:      "visual_inspection_asset_unavailable",
				SizeKey:   asset.SizeKey,
				Diagnosis: fmt.Sprintf("%s：无法从 OSS 读取当前 Prime 成图，%s。", asset.SizeKey, readErr.Error()),
			})
			continue
		}
		totalBytes += imageInfo.SizeBytes
		if totalBytes > creativeVisualInspectMaxTotalBytes {
			report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
				Code:      "visual_inspection_input_limit_exceeded",
				SizeKey:   asset.SizeKey,
				Diagnosis: fmt.Sprintf("%s：视觉输入总大小超过 %d MB。", asset.SizeKey, creativeVisualInspectMaxTotalBytes>>20),
			})
			continue
		}
		checked := creativeVisualInspectionCheckedAsset{
			SizeKey: asset.SizeKey, AttachmentID: uuidToString(asset.AttachmentID), Filename: asset.Filename,
			Width: imageInfo.Width, Height: imageInfo.Height, SizeBytes: imageInfo.SizeBytes, SHA256: imageInfo.SHA256,
		}
		report.CheckedAssets = append(report.CheckedAssets, checked)
		report.Evidence.Assets = append(report.Evidence.Assets, creativeVisualInspectionAssetEvidence{
			SizeKey: asset.SizeKey, AttachmentID: uuidToString(asset.AttachmentID), Filename: asset.Filename,
			ContentType: asset.ContentType, SizeBytes: imageInfo.SizeBytes, Width: imageInfo.Width, Height: imageInfo.Height,
			SHA256: imageInfo.SHA256, SignedURLProvided: h.CreativeVisualInspector != nil && canPresign,
		})
		localImages = append(localImages, creativeVisualInspectionLocalImage{SizeKey: asset.SizeKey, Path: imageInfo.Path})
		providerAsset := creativeVisualInspectionProviderAsset{
			SizeKey: asset.SizeKey, Filename: asset.Filename, Width: imageInfo.Width, Height: imageInfo.Height,
			SizeBytes: imageInfo.SizeBytes, SHA256: imageInfo.SHA256, ContentType: asset.ContentType,
		}
		if h.CreativeVisualInspector != nil {
			if !canPresign {
				report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
					Code:      "visual_inspection_signed_url_unavailable",
					SizeKey:   asset.SizeKey,
					Diagnosis: fmt.Sprintf("%s：当前存储不支持短时签名 URL，后端不能把图片安全传给视觉模型。", asset.SizeKey),
				})
			} else {
				key := h.Storage.KeyFromURL(asset.URL)
				signedURL, signErr := presigner.PresignGet(ctx, key, creativeVisualInspectSignedURLTTL)
				if signErr != nil {
					report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
						Code:      "visual_inspection_signed_url_unavailable",
						SizeKey:   asset.SizeKey,
						Diagnosis: fmt.Sprintf("%s：生成短时签名 URL 失败，%s。", asset.SizeKey, signErr.Error()),
					})
				} else {
					providerAsset.SignedURL = signedURL
				}
			}
		}
		providerAssets = append(providerAssets, providerAsset)
	}
	if len(localImages) > 0 {
		contactSheet, sheetErr := makeCreativeVisualInspectionContactSheet(localImages)
		if sheetErr == nil && len(contactSheet) > 0 {
			attachmentID, _, uploadErr := h.storeCreativeOrderInternalAttachment(ctx, workspaceID, order.CreatedBy, orderID, variantID, revision, "visual-inspection", "contact-sheet.png", contactSheet, "image/png")
			if uploadErr != nil {
				return creativeVisualInspectionReport{}, uploadErr
			}
			report.Evidence.ContactSheetAttachmentID = uuidToString(attachmentID)
		} else if sheetErr != nil {
			report.QualityWarnings = append(report.QualityWarnings, creativeVisualInspectionFinding{
				Code:      "visual_inspection_contact_sheet_unavailable",
				SizeKey:   "all",
				Diagnosis: "视觉诊断联系表生成失败：" + sheetErr.Error(),
			})
		}
	}
	if len(report.BlockingFailures) == 0 {
		report.PrimeAssetsReadable = true
	}

	if len(report.BlockingFailures) == 0 {
		switch {
		case h.CreativeVisualInspector == nil:
			report.Evidence.ModelErrorCode = "visual_inspection_model_unconfigured"
			report.Evidence.ModelErrorMessage = "backend visual inspection model is not configured"
			report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
				Code:      "visual_inspection_model_unconfigured",
				SizeKey:   "all",
				Diagnosis: "后端视觉模型未配置，无法对最终品牌成图做真实视觉验收。",
			})
		default:
			modelResult, modelErr := h.CreativeVisualInspector.InspectCreativeVisual(ctx, creativeVisualInspectionProviderInput{
				OrderID: uuidToString(orderID), VariantID: uuidToString(variantID), Revision: revision,
				ExpectedSizes: expectedSizes, CopySnapshot: order.InputSnapshot, Brief: order.Brief, Assets: providerAssets,
			})
			if modelErr != nil {
				report.Evidence.ModelErrorCode = "visual_inspection_model_error"
				report.Evidence.ModelErrorMessage = truncateCreativeVisualInspectionMessage(modelErr.Error(), 500)
				report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
					Code:      "visual_inspection_model_error",
					SizeKey:   "all",
					Diagnosis: "视觉模型调用失败：" + truncateCreativeVisualInspectionMessage(modelErr.Error(), 220),
				})
			} else {
				normalized, normalizeErr := normalizeCreativeVisualInspectionModelResult(modelResult, expectedSizes, report.CheckedAssets)
				if normalizeErr != nil {
					report.Evidence.ModelErrorCode = "visual_inspection_model_invalid_report"
					report.Evidence.ModelErrorMessage = truncateCreativeVisualInspectionMessage(normalizeErr.Error(), 500)
					report.BlockingFailures = append(report.BlockingFailures, creativeVisualInspectionFinding{
						Code:      "visual_inspection_model_invalid_report",
						SizeKey:   "all",
						Diagnosis: "视觉模型返回的结构化验收报告无效：" + truncateCreativeVisualInspectionMessage(normalizeErr.Error(), 220),
					})
				} else {
					report.Status = normalized.Status
					report.PrimeAssetsReadable = normalized.PrimeAssetsReadable
					report.KeyContentPreserved = normalized.KeyContentPreserved
					report.CheckedAssets = normalized.CheckedAssets
					report.QualityWarnings = normalized.QualityWarnings
					report.BlockingFailures = normalized.BlockingFailures
					report.Evidence.ModelRequestID = normalized.RequestID
				}
			}
		}
	}
	return h.finalizeCreativeVisualInspectionReport(ctx, workspaceID, order.CreatedBy, orderID, variantID, revision, task, report)
}

func newCreativeVisualInspectionReport(orderID, variantID pgtype.UUID, revision int, expectedSizes []string) creativeVisualInspectionReport {
	inspectionID := uuid.NewString()
	return creativeVisualInspectionReport{
		VariantID: uuidToString(variantID),
		Lane:      "visual",
		Revision:  revision,
		Status:    "passed",
		Evidence: creativeVisualInspectionEvidence{
			SchemaVersion: 2,
			InspectionID:  inspectionID,
			InspectedAt:   time.Now().UTC().Format(time.RFC3339),
			OrderID:       uuidToString(orderID),
			VariantID:     uuidToString(variantID),
			Revision:      revision,
			ExpectedSizes: append([]string(nil), expectedSizes...),
		},
		TriggerEvidenceKind: "creative_order_variant_qc_visual_inspection",
	}
}

func (h *Handler) loadCreativeVisualInspectionOrder(ctx context.Context, workspaceID, orderID, variantID pgtype.UUID) (creativeVisualInspectionOrderContext, error) {
	var result creativeVisualInspectionOrderContext
	var inputSnapshot, brief string
	err := h.DB.QueryRow(ctx, `
SELECT order_row.created_by, order_row.issue_id, order_row.trigger_evidence_kind,
       order_row.input_snapshot::text, variant.brief::text, variant.revision, variant.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
`, variantID, orderID, workspaceID).Scan(&result.CreatedBy, &result.IssueID, &result.TriggerKind, &inputSnapshot, &brief, &result.Revision, &result.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return result, errCreativeVisualInspectionVariantNotFound
		}
		return result, fmt.Errorf("load creative visual inspection order: %w", err)
	}
	result.InputSnapshot = json.RawMessage(inputSnapshot)
	result.Brief = json.RawMessage(brief)
	return result, nil
}

func (h *Handler) loadCreativeVisualInspectionAssets(ctx context.Context, variantID pgtype.UUID, revision int, expectedSizes []string) ([]creativeVisualInspectionAsset, error) {
	rows, err := h.DB.Query(ctx, `
SELECT asset.id, asset.size_key, asset.attachment_id, attachment.filename, attachment.url,
       attachment.content_type, attachment.size_bytes
FROM creative_order_asset asset
JOIN attachment ON attachment.id = asset.attachment_id
WHERE asset.variant_id = $1
  AND asset.revision = $2
  AND asset.stage = 'primed'
  AND asset.status = 'completed'
ORDER BY asset.size_key
`, variantID, revision)
	if err != nil {
		return nil, fmt.Errorf("load creative visual inspection assets: %w", err)
	}
	defer rows.Close()
	assets := []creativeVisualInspectionAsset{}
	for rows.Next() {
		var asset creativeVisualInspectionAsset
		if err := rows.Scan(&asset.ID, &asset.SizeKey, &asset.AttachmentID, &asset.Filename, &asset.URL, &asset.ContentType, &asset.SizeBytes); err != nil {
			return nil, fmt.Errorf("read creative visual inspection assets: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read creative visual inspection assets: %w", err)
	}
	order := make(map[string]int, len(expectedSizes))
	for index, size := range expectedSizes {
		order[size] = index
	}
	sortCreativeVisualInspectionAssets(assets, order)
	return assets, nil
}

type creativeVisualInspectionPackageFailure struct {
	sizeKey string
	message string
}

func creativeVisualInspectionPackageFailures(assets []creativeVisualInspectionAsset, expectedSizes []string) []creativeVisualInspectionPackageFailure {
	failures := []creativeVisualInspectionPackageFailure{}
	expected := map[string]struct{}{}
	seen := map[string]struct{}{}
	for _, size := range expectedSizes {
		expected[size] = struct{}{}
	}
	for _, asset := range assets {
		if _, ok := expected[asset.SizeKey]; !ok {
			failures = append(failures, creativeVisualInspectionPackageFailure{
				sizeKey: asset.SizeKey,
				message: fmt.Sprintf("%s：当前 revision 存在未声明尺寸的 Prime 成图，不能进入视觉验收。", asset.SizeKey),
			})
			continue
		}
		if _, duplicate := seen[asset.SizeKey]; duplicate {
			failures = append(failures, creativeVisualInspectionPackageFailure{
				sizeKey: asset.SizeKey,
				message: fmt.Sprintf("%s：当前 revision 存在重复 Prime 成图，不能进入视觉验收。", asset.SizeKey),
			})
			continue
		}
		seen[asset.SizeKey] = struct{}{}
	}
	for _, size := range expectedSizes {
		if _, ok := seen[size]; !ok {
			failures = append(failures, creativeVisualInspectionPackageFailure{
				sizeKey: size,
				message: fmt.Sprintf("%s：当前 revision 缺少 completed primed 成图，不能进入视觉验收。", size),
			})
		}
	}
	return failures
}

func sortCreativeVisualInspectionAssets(assets []creativeVisualInspectionAsset, order map[string]int) {
	for i := 1; i < len(assets); i++ {
		for j := i; j > 0 && creativeVisualInspectionAssetOrder(assets[j], order) < creativeVisualInspectionAssetOrder(assets[j-1], order); j-- {
			assets[j], assets[j-1] = assets[j-1], assets[j]
		}
	}
}

func creativeVisualInspectionAssetOrder(asset creativeVisualInspectionAsset, order map[string]int) int {
	if index, ok := order[asset.SizeKey]; ok {
		return index
	}
	return len(order) + creativeAssetSizeOrder(asset.SizeKey)
}

type creativeVisualInspectionImageInfo struct {
	Path      string
	Width     int
	Height    int
	SizeBytes int64
	SHA256    string
}

func (h *Handler) readCreativeVisualInspectionImage(ctx context.Context, tempDir string, asset creativeVisualInspectionAsset) (creativeVisualInspectionImageInfo, error) {
	contentType := strings.TrimSpace(strings.Split(asset.ContentType, ";")[0])
	if contentType == "" {
		if detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(asset.Filename))); detected != "" {
			contentType = strings.TrimSpace(strings.Split(detected, ";")[0])
		}
	}
	if !strings.HasPrefix(contentType, "image/") {
		return creativeVisualInspectionImageInfo{}, fmt.Errorf("attachment content type %q is not an image", asset.ContentType)
	}
	if asset.SizeBytes > creativeVisualInspectMaxImageBytes {
		return creativeVisualInspectionImageInfo{}, fmt.Errorf("image exceeds %d MB limit", creativeVisualInspectMaxImageBytes>>20)
	}
	key := h.Storage.KeyFromURL(asset.URL)
	if key == "" {
		return creativeVisualInspectionImageInfo{}, errors.New("attachment storage key is empty")
	}
	reader, err := h.Storage.GetReader(ctx, key)
	if err != nil {
		return creativeVisualInspectionImageInfo{}, err
	}
	defer reader.Close()

	outputPath := filepath.Join(tempDir, asset.SizeKey+"-"+uuidToString(asset.AttachmentID)+filepath.Ext(asset.Filename))
	output, err := os.Create(outputPath)
	if err != nil {
		return creativeVisualInspectionImageInfo{}, err
	}
	hasher := sha256.New()
	limited := io.LimitReader(reader, creativeVisualInspectMaxImageBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(output, hasher), limited)
	closeErr := output.Close()
	if copyErr != nil {
		return creativeVisualInspectionImageInfo{}, copyErr
	}
	if closeErr != nil {
		return creativeVisualInspectionImageInfo{}, closeErr
	}
	if written > creativeVisualInspectMaxImageBytes {
		return creativeVisualInspectionImageInfo{}, fmt.Errorf("image exceeds %d MB limit", creativeVisualInspectMaxImageBytes>>20)
	}
	file, err := os.Open(outputPath)
	if err != nil {
		return creativeVisualInspectionImageInfo{}, err
	}
	cfg, _, err := image.DecodeConfig(file)
	_ = file.Close()
	if err != nil {
		return creativeVisualInspectionImageInfo{}, fmt.Errorf("image is not decodable: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return creativeVisualInspectionImageInfo{}, errors.New("image has invalid dimensions")
	}
	return creativeVisualInspectionImageInfo{
		Path: outputPath, Width: cfg.Width, Height: cfg.Height, SizeBytes: written, SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

type creativeVisualInspectionLocalImage struct {
	SizeKey string
	Path    string
}

func makeCreativeVisualInspectionContactSheet(images []creativeVisualInspectionLocalImage) ([]byte, error) {
	if len(images) == 0 {
		return nil, nil
	}
	const (
		cellWidth  = 420
		cellHeight = 340
		gap        = 16
	)
	columns := len(images)
	if columns > creativeVisualInspectMaxImages {
		columns = creativeVisualInspectMaxImages
	}
	rows := (len(images) + columns - 1) / columns
	canvas := image.NewRGBA(image.Rect(0, 0, gap+columns*(cellWidth+gap), gap+rows*(cellHeight+gap)))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(image.White), image.Point{}, draw.Src)
	for index, item := range images {
		source, err := decodeImageFile(item.Path)
		if err != nil {
			return nil, err
		}
		preview := containImage(source, cellWidth, cellHeight)
		row, col := index/columns, index%columns
		x := gap + col*(cellWidth+gap) + (cellWidth-preview.Bounds().Dx())/2
		y := gap + row*(cellHeight+gap) + (cellHeight-preview.Bounds().Dy())/2
		draw.Draw(canvas, image.Rect(x, y, x+preview.Bounds().Dx(), y+preview.Bounds().Dy()), preview, preview.Bounds().Min, draw.Over)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decodeImageFile(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func containImage(source image.Image, maxWidth, maxHeight int) *image.RGBA {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	scaleX := float64(maxWidth) / float64(width)
	scaleY := float64(maxHeight) / float64(height)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale > 1 {
		scale = 1
	}
	dstWidth := max(1, int(float64(width)*scale))
	dstHeight := max(1, int(float64(height)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := 0; y < dstHeight; y++ {
		srcY := bounds.Min.Y + y*height/dstHeight
		for x := 0; x < dstWidth; x++ {
			srcX := bounds.Min.X + x*width/dstWidth
			dst.Set(x, y, source.At(srcX, srcY))
		}
	}
	return dst
}

func normalizeCreativeVisualInspectionModelResult(result creativeVisualInspectionModelResult, expectedSizes []string, fallbackChecked []creativeVisualInspectionCheckedAsset) (creativeVisualInspectionModelResult, error) {
	result.Status = strings.TrimSpace(result.Status)
	if result.Status == "" {
		result.Status = "passed"
	}
	if !validCreativeQCStatus(result.Status) || result.Status == "pending" {
		return result, errors.New("status must be passed, warning, or failed")
	}
	result.QualityWarnings = normalizeCreativeVisualInspectionFindings(result.QualityWarnings)
	result.BlockingFailures = normalizeCreativeVisualInspectionFindings(result.BlockingFailures)
	if err := validateCreativeVisualInspectionFindings(result.QualityWarnings, expectedSizes, false); err != nil {
		return result, fmt.Errorf("quality_warnings: %w", err)
	}
	if err := validateCreativeVisualInspectionFindings(result.BlockingFailures, expectedSizes, true); err != nil {
		return result, fmt.Errorf("blocking_failures: %w", err)
	}
	if len(result.BlockingFailures) == 0 {
		if !result.PrimeAssetsReadable {
			return result, errors.New("prime_assets_readable=false requires blocking_failures")
		}
		if !result.KeyContentPreserved {
			return result, errors.New("key_content_preserved=false requires blocking_failures")
		}
	}
	if len(result.BlockingFailures) > 0 {
		result.Status = "failed"
	} else if result.Status == "passed" && len(result.QualityWarnings) > 0 {
		result.Status = "warning"
	}
	if len(result.CheckedAssets) == 0 {
		result.CheckedAssets = fallbackChecked
	}
	result.CheckedAssets = normalizeCreativeVisualInspectionCheckedAssets(result.CheckedAssets, fallbackChecked)
	return result, nil
}

func normalizeCreativeVisualInspectionFindings(findings []creativeVisualInspectionFinding) []creativeVisualInspectionFinding {
	result := make([]creativeVisualInspectionFinding, 0, len(findings))
	for _, finding := range findings {
		finding.Code = strings.TrimSpace(finding.Code)
		finding.SizeKey = strings.TrimSpace(finding.SizeKey)
		finding.Diagnosis = strings.TrimSpace(finding.Diagnosis)
		if finding.Code == "" && finding.SizeKey == "" && finding.Diagnosis == "" {
			continue
		}
		result = append(result, finding)
	}
	return result
}

func validateCreativeVisualInspectionFindings(findings []creativeVisualInspectionFinding, expectedSizes []string, blocking bool) error {
	expected := make(map[string]struct{}, len(expectedSizes))
	for _, size := range expectedSizes {
		expected[size] = struct{}{}
	}
	for _, finding := range findings {
		if finding.Code == "" || finding.SizeKey == "" || finding.Diagnosis == "" {
			return errors.New("code, size_key, and diagnosis are required")
		}
		if finding.SizeKey != "all" {
			if _, ok := expected[finding.SizeKey]; !ok {
				return fmt.Errorf("unexpected size_key %q", finding.SizeKey)
			}
		}
		if len([]rune(finding.Diagnosis)) > 500 {
			return errors.New("diagnosis is too long")
		}
		if blocking {
			switch finding.Code {
			case "actual_prime_obstruction", "official_prime_text_unreadable", "generated_content_missing",
				"visual_inspection_uncertain", "visual_inspection_model_error", "visual_inspection_model_unconfigured",
				"visual_inspection_model_invalid_report", "visual_inspection_asset_unavailable",
				"visual_inspection_input_limit_exceeded", "visual_inspection_signed_url_unavailable",
				"creative_prime_package_incomplete":
			default:
				return fmt.Errorf("unsupported blocking code %q", finding.Code)
			}
		}
	}
	return nil
}

func normalizeCreativeVisualInspectionCheckedAssets(assets, fallback []creativeVisualInspectionCheckedAsset) []creativeVisualInspectionCheckedAsset {
	fallbackBySize := map[string]creativeVisualInspectionCheckedAsset{}
	for _, item := range fallback {
		fallbackBySize[item.SizeKey] = item
	}
	result := make([]creativeVisualInspectionCheckedAsset, 0, len(assets))
	for _, item := range assets {
		item.SizeKey = strings.TrimSpace(item.SizeKey)
		item.AttachmentID = strings.TrimSpace(item.AttachmentID)
		item.Filename = strings.TrimSpace(item.Filename)
		item.SHA256 = strings.TrimSpace(item.SHA256)
		if base, ok := fallbackBySize[item.SizeKey]; ok {
			if item.AttachmentID == "" {
				item.AttachmentID = base.AttachmentID
			}
			if item.Filename == "" {
				item.Filename = base.Filename
			}
			if item.Width == 0 {
				item.Width = base.Width
			}
			if item.Height == 0 {
				item.Height = base.Height
			}
			if item.SizeBytes == 0 {
				item.SizeBytes = base.SizeBytes
			}
			if item.SHA256 == "" {
				item.SHA256 = base.SHA256
			}
		}
		if item.SizeKey != "" {
			result = append(result, item)
		}
	}
	return result
}

func (h *Handler) finalizeCreativeVisualInspectionReport(
	ctx context.Context,
	workspaceID, createdBy, orderID, variantID pgtype.UUID,
	revision int,
	task db.AgentTaskQueue,
	report creativeVisualInspectionReport,
) (creativeVisualInspectionReport, error) {
	if len(report.BlockingFailures) > 0 {
		report.Status = "failed"
	} else if report.Status == "passed" && len(report.QualityWarnings) > 0 {
		report.Status = "warning"
	}
	if report.Status == "" {
		report.Status = "passed"
	}
	report.TriggerEvidenceKind = "creative_order_variant_qc_visual_inspection"
	if task.ID.Valid {
		report.Evidence.TaskID = uuidToString(task.ID)
	}
	attachmentID, _, err := h.storeCreativeOrderInternalAttachmentWithID(ctx, workspaceID, createdBy, orderID, variantID, revision, "visual-inspection", "visual-inspection.json", func(id pgtype.UUID) ([]byte, error) {
		report.TriggerEvidenceReference = uuidToString(id)
		report.Evidence.InspectionReportAttachmentID = uuidToString(id)
		report.Evidence.ModelConfigured = h.CreativeVisualInspector != nil
		reportJSON, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		return reportJSON, nil
	}, "application/json")
	if err != nil {
		return creativeVisualInspectionReport{}, err
	}
	report.TriggerEvidenceReference = uuidToString(attachmentID)
	report.Evidence.InspectionReportAttachmentID = uuidToString(attachmentID)
	return report, nil
}

func (h *Handler) storeCreativeOrderInternalAttachment(
	ctx context.Context,
	workspaceID, createdBy, orderID, variantID pgtype.UUID,
	revision int,
	subdir, filename string,
	data []byte,
	contentType string,
) (pgtype.UUID, string, error) {
	return h.storeCreativeOrderInternalAttachmentWithID(ctx, workspaceID, createdBy, orderID, variantID, revision, subdir, filename, func(pgtype.UUID) ([]byte, error) {
		return data, nil
	}, contentType)
}

func (h *Handler) storeCreativeOrderInternalAttachmentWithID(
	ctx context.Context,
	workspaceID, createdBy, orderID, variantID pgtype.UUID,
	revision int,
	subdir, filename string,
	data func(pgtype.UUID) ([]byte, error),
	contentType string,
) (pgtype.UUID, string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("create creative order attachment id: %w", err)
	}
	attachmentID := pgtype.UUID{Bytes: id, Valid: true}
	key := filepath.ToSlash(filepath.Join(
		"workspaces", uuidToString(workspaceID), "creative-orders", uuidToString(orderID),
		"variants", uuidToString(variantID), fmt.Sprintf("r%d", revision), subdir, id.String()+"-"+filename,
	))
	body, err := data(attachmentID)
	if err != nil {
		return pgtype.UUID{}, "", err
	}
	url, err := h.Storage.Upload(ctx, key, body, contentType, filename)
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("upload %s: %w", filename, err)
	}
	if !createdBy.Valid {
		h.Storage.Delete(ctx, key)
		return pgtype.UUID{}, "", errors.New("creative order is missing its owner")
	}
	if _, err := h.Queries.CreateAttachment(ctx, db.CreateAttachmentParams{
		ID:           attachmentID,
		WorkspaceID:  workspaceID,
		UploaderType: "member",
		UploaderID:   createdBy,
		Filename:     filename,
		ContentType:  contentType,
		SizeBytes:    int64(len(body)),
		Url:          url,
	}); err != nil {
		h.Storage.Delete(ctx, key)
		return pgtype.UUID{}, "", fmt.Errorf("record %s attachment: %w", filename, err)
	}
	return attachmentID, key, nil
}

func truncateCreativeVisualInspectionMessage(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return ""
	}
	return string(runes[:limit-1]) + "…"
}

type openAICompatibleCreativeVisualInspector struct {
	apiKey     string
	baseURL    string
	path       string
	model      string
	httpClient *http.Client
}

func newCreativeVisualInspectorFromEnv() creativeVisualInspector {
	apiKey := strings.TrimSpace(os.Getenv("MULTICA_VISUAL_INSPECT_API_KEY"))
	model := strings.TrimSpace(os.Getenv("MULTICA_VISUAL_INSPECT_MODEL"))
	if apiKey == "" || model == "" {
		return nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MULTICA_VISUAL_INSPECT_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	path := strings.TrimSpace(os.Getenv("MULTICA_VISUAL_INSPECT_PATH"))
	if path == "" {
		path = "/v1/responses"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	timeout := 90 * time.Second
	if raw := strings.TrimSpace(os.Getenv("MULTICA_VISUAL_INSPECT_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	return &openAICompatibleCreativeVisualInspector{
		apiKey: apiKey, baseURL: baseURL, path: path, model: model,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (v *openAICompatibleCreativeVisualInspector) ProviderName() string {
	return "openai_compatible_responses"
}

func (v *openAICompatibleCreativeVisualInspector) ModelName() string {
	return v.model
}

func (v *openAICompatibleCreativeVisualInspector) InspectCreativeVisual(ctx context.Context, input creativeVisualInspectionProviderInput) (creativeVisualInspectionModelResult, error) {
	content := []map[string]any{{
		"type": "input_text",
		"text": creativeVisualInspectionPrompt(input),
	}}
	for _, asset := range input.Assets {
		if strings.TrimSpace(asset.SignedURL) == "" {
			return creativeVisualInspectionModelResult{}, fmt.Errorf("missing signed URL for %s", asset.SizeKey)
		}
		content = append(content, map[string]any{
			"type": "input_text",
			"text": fmt.Sprintf("Final branded Prime image for size %s (%dx%d).", asset.SizeKey, asset.Width, asset.Height),
		})
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": asset.SignedURL,
		})
	}
	body, err := json.Marshal(map[string]any{
		"model": v.model,
		"input": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
		"max_output_tokens": 1800,
	})
	if err != nil {
		return creativeVisualInspectionModelResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+v.path, bytes.NewReader(body))
	if err != nil {
		return creativeVisualInspectionModelResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+v.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return creativeVisualInspectionModelResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return creativeVisualInspectionModelResult{}, err
	}
	if resp.StatusCode >= 400 {
		return creativeVisualInspectionModelResult{}, fmt.Errorf("visual model returned HTTP %d", resp.StatusCode)
	}
	outputText := creativeVisualInspectionOutputText(raw)
	if outputText == "" {
		return creativeVisualInspectionModelResult{}, errors.New("visual model response has no output text")
	}
	var result creativeVisualInspectionModelResult
	if err := decodeCreativeVisualInspectionJSON(outputText, &result); err != nil {
		return creativeVisualInspectionModelResult{}, err
	}
	if result.RequestID == "" {
		result.RequestID = resp.Header.Get("x-request-id")
	}
	return result, nil
}

func creativeVisualInspectionPrompt(input creativeVisualInspectionProviderInput) string {
	brief := truncateCreativeVisualInspectionMessage(string(input.Brief), 6000)
	copySnapshot := truncateCreativeVisualInspectionMessage(string(input.CopySnapshot), 6000)
	return fmt.Sprintf(`你是广告最终视觉质检。只基于随后的最终品牌成图下结论，不要凭空假设图片内容。

验收对象：
- creative_order_id: %s
- variant_id: %s
- revision: %d
- expected_sizes: %s

必须检查三道闸门：
1. 冻结文案、关键金额/表格/CTA、主要组件是否在最终图中完整出现；空框、缺字、明显事实缺失算失败。
2. 正文、金额、表格、CTA 是否被顶部或底部官方 Prime 组件实际遮挡或进入 Prime 禁区导致不可读。
3. Logo、条款、底部官方组件是否局部可读，不能用整条平均颜色代替局部判断。

只输出一个 JSON object，不要 Markdown，不要解释。结构：
{
  "status": "passed|warning|failed",
  "summary": "一句中文摘要",
  "prime_assets_readable": true,
  "key_content_preserved": true,
  "checked_assets": [{"size_key":"1080x1080","observations":["..."]}],
  "quality_warnings": [{"code":"...","size_key":"1080x1080","diagnosis":"..."}],
  "blocking_failures": [{"code":"actual_prime_obstruction|official_prime_text_unreadable|generated_content_missing|visual_inspection_uncertain","size_key":"1200x628","diagnosis":"1200x628：..."}]
}

blocking_failures 规则：
- 真实 Prime 遮挡用 actual_prime_obstruction，diagnosis 必须以 size_key 开头，并包含“期望移动到 safe_content_frame ...”。
- 官方 Prime 文字不可读用 official_prime_text_unreadable，diagnosis 必须以 size_key 开头，并包含“期望调整为官方模板文字下方连续、低细节的浅色背景”。
- 冻结关键内容缺失用 generated_content_missing，diagnosis 必须说明“期望补齐冻结文案 copy_snapshot”。
- 看不清或无法确认时不要猜 passed，写 visual_inspection_uncertain。

brief:
%s

copy_snapshot:
%s`, input.OrderID, input.VariantID, input.Revision, strings.Join(input.ExpectedSizes, ","), brief, copySnapshot)
}

func creativeVisualInspectionOutputText(raw []byte) string {
	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return ""
	}
	if strings.TrimSpace(response.OutputText) != "" {
		return response.OutputText
	}
	var parts []string
	for _, output := range response.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	for _, choice := range response.Choices {
		if strings.TrimSpace(choice.Message.Content) != "" {
			parts = append(parts, choice.Message.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func decodeCreativeVisualInspectionJSON(text string, out any) error {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return errors.New("visual model output does not contain a JSON object")
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), out); err != nil {
		return fmt.Errorf("decode visual model JSON: %w", err)
	}
	return nil
}
