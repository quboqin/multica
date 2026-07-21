package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxCreativePackageBytes = 300 << 20

type creativePackageAsset struct {
	ID             string `json:"id"`
	CandidateID    string `json:"candidate_id"`
	Competitor     string `json:"competitor"`
	Variant        int    `json:"variant"`
	QCStatus       string `json:"qc_status"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Label          string `json:"label"`
	AssetURL       string `json:"asset_url"`
	SourceAssetURL string `json:"-"`
	ContentType    string `json:"content_type"`
	StorageKey     string `json:"-"`
	Filename       string `json:"filename"`
}

type creativePackageSource struct {
	CandidateID string `json:"candidate_id"`
	Competitor  string `json:"competitor"`
	Title       string `json:"title"`
	AssetType   string `json:"asset_type"`
	SourceURL   string `json:"source_url"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
}

type creativePackageVariant struct {
	CandidateID string `json:"candidate_id"`
	Variant     int    `json:"variant"`
	Title       string `json:"title"`
	Description string `json:"description"`
	QCStatus    string `json:"qc_status"`
}

type creativePackageProcess struct {
	Status           string `json:"status"`
	Stage            string `json:"stage"`
	Progress         int    `json:"progress"`
	ExternalProvider string `json:"external_provider"`
	MCPConnectionID  string `json:"mcp_connection_id,omitempty"`
	ExternalJobID    string `json:"external_job_id"`
	ExternalStatus   string `json:"external_status"`
	PollAttempts     int    `json:"poll_attempts"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	CompletedAt      string `json:"completed_at,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

var creativeDeliveryFilenamePattern = regexp.MustCompile(
	`(?i)^\d{2}_P_[A-Z0-9-]+_[A-Z]{2,3}_\d{8}_[A-Z0-9-]+_[A-Z0-9-]+_(?:11|169|191|916|45|\d+x\d+)\.(?:png|jpe?g|webp)$`,
)
var creativePackageDatePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func (h *Handler) DownloadCreativeEditJob(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	jobID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "jobId"), "job_id")
	if !ok {
		return
	}
	selectedAssetIDs := r.URL.Query()["asset_id"]
	if selectedAssetIDs == nil {
		selectedAssetIDs = []string{}
	}
	for _, assetID := range selectedAssetIDs {
		if _, ok := parseUUIDOrBadRequest(w, assetID, "asset_id"); !ok {
			return
		}
	}
	includeOriginal := r.URL.Query().Get("include_original") != "false"
	archive, err := h.buildCreativeEditJobArchive(
		r.Context(), issue.ID, issue.WorkspaceID, jobID, selectedAssetIDs, includeOriginal,
	)
	if err != nil {
		slog.Error("build creative asset package failed", "job_id", uuidToString(jobID), "error", err)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, http.StatusNotFound, "creative edit job not found")
		case errors.Is(err, errCreativeJobNotReady):
			writeError(w, http.StatusConflict, "creative edit job has no deliverable results")
		case errors.Is(err, errCreativeJobNoAssets):
			writeError(w, http.StatusConflict, "creative edit job has no deliverable assets")
		default:
			writeError(w, http.StatusInternalServerError, "failed to build creative asset package")
		}
		return
	}
	filename, err := h.creativePackageDownloadFilename(r.Context(), issue.ID, issue.WorkspaceID, jobID)
	if err != nil {
		slog.Warn("derive creative package filename failed", "job_id", uuidToString(jobID), "error", err)
		filename = "creative-job-" + shortCreativeID(uuidToString(jobID)) + ".zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", archive.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = archive.WriteTo(w)
}

func (h *Handler) PreviewCreativeEditAsset(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	assetID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "assetId"), "asset_id")
	if !ok {
		return
	}
	asset, err := h.loadCreativePreviewAsset(r.Context(), issue.ID, issue.WorkspaceID, assetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "creative edit asset not found")
			return
		}
		slog.Error("load creative preview asset failed", "asset_id", uuidToString(assetID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load creative edit asset")
		return
	}
	data, err := h.readCreativePackageAsset(r.Context(), asset)
	if err != nil {
		slog.Error("read creative preview asset failed", "asset_id", uuidToString(assetID), "error", err)
		writeError(w, http.StatusBadGateway, "failed to read creative edit asset")
		return
	}
	contentType := strings.TrimSpace(asset.ContentType)
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) loadCreativePreviewAsset(
	ctx context.Context,
	issueID, workspaceID, assetID pgtype.UUID,
) (creativePackageAsset, error) {
	var asset creativePackageAsset
	err := h.DB.QueryRow(ctx, `
SELECT a.id::text, c.id::text, c.competitor, v.variant_index, v.qc_status,
       a.width, a.height, a.label, a.asset_url, COALESCE(a.source_asset_url, ''),
       a.content_type, COALESCE(a.storage_key, '')
FROM creative_edit_asset a
JOIN creative_edit_variant v ON v.id = a.variant_id
JOIN creative_material_candidate c ON c.id = v.candidate_id
JOIN creative_edit_job j ON j.id = v.job_id
WHERE a.id = $1 AND j.issue_id = $2 AND j.workspace_id = $3
	`, assetID, issueID, workspaceID).Scan(
		&asset.ID, &asset.CandidateID, &asset.Competitor, &asset.Variant, &asset.QCStatus,
		&asset.Width, &asset.Height, &asset.Label, &asset.AssetURL, &asset.SourceAssetURL, &asset.ContentType, &asset.StorageKey,
	)
	return asset, err
}

var errCreativeJobNotReady = errors.New("creative edit job has no deliverable results")
var errCreativeJobNoAssets = errors.New("creative edit job has no deliverable assets")

func (h *Handler) creativePackageDownloadFilename(
	ctx context.Context, issueID, workspaceID, jobID pgtype.UUID,
) (string, error) {
	var asset creativePackageAsset
	err := h.DB.QueryRow(ctx, `
SELECT a.width, a.height, a.label, a.asset_url, a.content_type
FROM creative_edit_asset a
JOIN creative_edit_variant v ON v.id = a.variant_id
JOIN creative_edit_job j ON j.id = v.job_id
WHERE j.id = $1 AND j.issue_id = $2 AND j.workspace_id = $3
ORDER BY v.variant_index, a.width, a.height
LIMIT 1
`, jobID, issueID, workspaceID).Scan(
		&asset.Width, &asset.Height, &asset.Label, &asset.AssetURL, &asset.ContentType,
	)
	if err != nil {
		return "", err
	}
	return creativePackageDownloadName(creativeDeliveryFilename(asset)), nil
}

func (h *Handler) buildCreativeEditJobArchive(
	ctx context.Context, issueID, workspaceID, jobID pgtype.UUID, selectedAssetIDs []string, includeOriginal bool,
) (*bytes.Buffer, error) {
	var process creativePackageProcess
	var prompt string
	var rulesRaw string
	var processDataRaw string
	if err := h.DB.QueryRow(ctx, `
SELECT status, stage, progress, external_provider,
       COALESCE(mcp_connection_id::text, ''), external_job_id, external_status,
       poll_attempts, created_at::text, updated_at::text,
       COALESCE(completed_at::text, ''), error_message, prompt, rules::text, process_data::text
FROM creative_edit_job
WHERE id = $1 AND issue_id = $2 AND workspace_id = $3
`, jobID, issueID, workspaceID).Scan(
		&process.Status, &process.Stage, &process.Progress, &process.ExternalProvider,
		&process.MCPConnectionID, &process.ExternalJobID, &process.ExternalStatus,
		&process.PollAttempts, &process.CreatedAt, &process.UpdatedAt,
		&process.CompletedAt, &process.ErrorMessage, &prompt, &rulesRaw, &processDataRaw,
	); err != nil {
		return nil, err
	}
	if process.Status != "completed" && process.Status != "partial" {
		return nil, errCreativeJobNotReady
	}
	sources, err := h.listCreativePackageSources(ctx, jobID, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	variants, err := h.listCreativePackageVariants(ctx, jobID, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := h.DB.Query(ctx, `
SELECT a.id::text, c.id::text, c.competitor, v.variant_index, v.qc_status,
       a.width, a.height, a.label, a.asset_url, COALESCE(a.source_asset_url, ''),
       a.content_type, a.storage_key
FROM creative_edit_asset a
JOIN creative_edit_variant v ON v.id = a.variant_id
JOIN creative_material_candidate c ON c.id = v.candidate_id
JOIN creative_edit_job j ON j.id = v.job_id
WHERE j.id = $1 AND j.issue_id = $2 AND j.workspace_id = $3
  AND (COALESCE(cardinality($4::text[]), 0) = 0 OR a.id::text = ANY($4::text[]))
ORDER BY c.competitor, c.id, v.variant_index, a.width, a.height
`, jobID, issueID, workspaceID, selectedAssetIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := []creativePackageAsset{}
	for rows.Next() {
		var asset creativePackageAsset
		if err := rows.Scan(
			&asset.ID, &asset.CandidateID, &asset.Competitor, &asset.Variant, &asset.QCStatus,
			&asset.Width, &asset.Height, &asset.Label, &asset.AssetURL, &asset.SourceAssetURL, &asset.ContentType, &asset.StorageKey,
		); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	naming := creativePackageNamingFromJob(rulesRaw, process.CreatedAt)
	for index := range assets {
		assets[index].Filename = creativePackageAssetFilename(assets[index], naming)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, errCreativeJobNoAssets
	}
	selectedCandidates := map[string]bool{}
	selectedVariants := map[string]bool{}
	for _, asset := range assets {
		selectedCandidates[asset.CandidateID] = true
		selectedVariants[fmt.Sprintf("%s:%d", asset.CandidateID, asset.Variant)] = true
	}
	if includeOriginal {
		filteredSources := sources[:0]
		for _, source := range sources {
			if selectedCandidates[source.CandidateID] {
				filteredSources = append(filteredSources, source)
			}
		}
		sources = filteredSources
	} else {
		sources = nil
	}
	filteredVariants := variants[:0]
	for _, variant := range variants {
		if selectedVariants[fmt.Sprintf("%s:%d", variant.CandidateID, variant.Variant)] {
			filteredVariants = append(filteredVariants, variant)
		}
	}
	variants = filteredVariants

	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	cache := map[string][]byte{}
	var totalBytes int64
	for _, source := range sources {
		data, err := h.readCreativePackageAsset(ctx, creativePackageAsset{
			AssetURL: source.SourceURL, ContentType: source.ContentType,
		})
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("read original candidate %s: %w", source.CandidateID, err)
		}
		totalBytes += int64(len(data))
		if totalBytes > maxCreativePackageBytes {
			_ = writer.Close()
			return nil, errors.New("creative asset package exceeds maximum size")
		}
		entryName := fmt.Sprintf(
			"%s-%s/original/%s",
			safeCreativeZipPart(source.Competitor),
			shortCreativeID(source.CandidateID),
			source.Filename,
		)
		entry, err := writer.Create(entryName)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := entry.Write(data); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	for _, asset := range assets {
		cacheKey := firstNonEmpty(asset.StorageKey, asset.AssetURL)
		data, ok := cache[cacheKey]
		if !ok {
			var err error
			data, err = h.readCreativePackageAsset(ctx, asset)
			if err != nil {
				_ = writer.Close()
				return nil, fmt.Errorf(
					"read candidate %s variant %d asset %dx%d: %w",
					asset.CandidateID, asset.Variant, asset.Width, asset.Height, err,
				)
			}
			cache[cacheKey] = data
		}
		totalBytes += int64(len(data))
		if totalBytes > maxCreativePackageBytes {
			_ = writer.Close()
			return nil, errors.New("creative asset package exceeds maximum size")
		}
		entryName := fmt.Sprintf(
			"%s-%s/variant-%d/%s",
			safeCreativeZipPart(asset.Competitor),
			shortCreativeID(asset.CandidateID),
			asset.Variant,
			asset.Filename,
		)
		entry, err := writer.Create(entryName)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := entry.Write(data); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	manifest := map[string]any{
		"job_id":        uuidToString(jobID),
		"issue_id":      uuidToString(issueID),
		"prompt":        prompt,
		"rules":         json.RawMessage(rulesRaw),
		"process":       process,
		"process_data":  json.RawMessage(processDataRaw),
		"sources":       sources,
		"variants":      variants,
		"assets":        assets,
		"source_count":  len(sources),
		"variant_count": len(variants),
		"asset_count":   len(assets),
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	manifestEntry, err := writer.Create("manifest.json")
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	if _, err := manifestEntry.Write(manifestBytes); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer, nil
}

func (h *Handler) listCreativePackageSources(
	ctx context.Context,
	jobID, issueID, workspaceID pgtype.UUID,
) ([]creativePackageSource, error) {
	rows, err := h.DB.Query(ctx, `
SELECT c.id::text, c.competitor, c.title, c.asset_type,
       c.archived_url, c.preview_url, c.resource_url, c.poster_url
FROM creative_edit_job_candidate jc
JOIN creative_edit_job j ON j.id = jc.job_id
JOIN creative_material_candidate c ON c.id = jc.candidate_id
WHERE j.id = $1 AND j.issue_id = $2 AND j.workspace_id = $3
ORDER BY c.competitor, c.id
`, jobID, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []creativePackageSource{}
	for rows.Next() {
		var source creativePackageSource
		var archivedURL, previewURL, resourceURL, posterURL string
		if err := rows.Scan(
			&source.CandidateID, &source.Competitor, &source.Title, &source.AssetType,
			&archivedURL, &previewURL, &resourceURL, &posterURL,
		); err != nil {
			return nil, err
		}
		if source.AssetType == "video" {
			source.SourceURL = firstNonEmpty(archivedURL, resourceURL, previewURL, posterURL)
		} else {
			source.SourceURL = firstNonEmpty(archivedURL, previewURL, resourceURL, posterURL)
		}
		if source.SourceURL == "" {
			return nil, fmt.Errorf("creative source %s has no downloadable URL", source.CandidateID)
		}
		source.ContentType = creativeSourceContentType(source.AssetType, source.SourceURL)
		source.Filename = "original" + creativePackageExtension(source.SourceURL, source.ContentType)
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (h *Handler) listCreativePackageVariants(
	ctx context.Context,
	jobID, issueID, workspaceID pgtype.UUID,
) ([]creativePackageVariant, error) {
	rows, err := h.DB.Query(ctx, `
SELECT v.candidate_id::text, v.variant_index, v.title, v.description, v.qc_status
FROM creative_edit_variant v
JOIN creative_edit_job j ON j.id = v.job_id
WHERE j.id = $1 AND j.issue_id = $2 AND j.workspace_id = $3
ORDER BY v.candidate_id, v.variant_index
`, jobID, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variants := []creativePackageVariant{}
	for rows.Next() {
		var variant creativePackageVariant
		if err := rows.Scan(
			&variant.CandidateID, &variant.Variant, &variant.Title,
			&variant.Description, &variant.QCStatus,
		); err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}
	return variants, rows.Err()
}

func creativeDeliveryFilename(asset creativePackageAsset) string {
	assetURL := firstNonEmpty(asset.SourceAssetURL, asset.AssetURL)
	extension := creativePackageExtension(assetURL, asset.ContentType)
	if parsed, err := url.Parse(assetURL); err == nil {
		if decoded, decodeErr := url.PathUnescape(path.Base(parsed.Path)); decodeErr == nil &&
			creativeDeliveryFilenamePattern.MatchString(decoded) {
			return decoded
		}
	}
	return safeCreativeZipPart(firstNonEmpty(
		asset.Label,
		fmt.Sprintf("%dx%d", asset.Width, asset.Height),
	)) + extension
}

type creativePackageNaming struct {
	month   string
	brand   string
	country string
	date    string
	picType string
}

func creativePackageNamingFromJob(rulesRaw, createdAt string) creativePackageNaming {
	naming := creativePackageNaming{month: "00", brand: "AK", country: "ID", date: "19700101", picType: "NUM"}
	match := creativePackageDatePattern.FindString(createdAt)
	if match != "" {
		naming.month = match[5:7]
		naming.date = strings.ReplaceAll(match, "-", "")
	}
	var rules map[string]any
	if json.Unmarshal([]byte(rulesRaw), &rules) == nil {
		market := strings.ToLower(stringFromAny(rules["market"]))
		switch market {
		case "id-adakami", "idn-adakami":
			naming.brand, naming.country = "AK", "ID"
		case "my-adakami", "mys-adakami":
			naming.brand, naming.country = "AK", "MY"
		}
	}
	return naming
}

func creativePackageAssetFilename(asset creativePackageAsset, naming creativePackageNaming) string {
	if filename := creativeDeliveryFilename(asset); creativeDeliveryFilenamePattern.MatchString(filename) {
		return filename
	}
	variant := asset.Variant
	if variant < 1 {
		variant = 1
	}
	sizeCode := map[string]string{
		"1080x1080": "11",
		"800x1000":  "45",
		"1200x628":  "191",
	}[fmt.Sprintf("%dx%d", asset.Width, asset.Height)]
	if sizeCode == "" {
		sizeCode = fmt.Sprintf("%dx%d", asset.Width, asset.Height)
	}
	return fmt.Sprintf(
		"%s_P_%s_%s_%s_%s_AI%02d_%s%s",
		naming.month, naming.brand, naming.country, naming.date, naming.picType,
		variant, sizeCode, creativePackageExtension(firstNonEmpty(asset.SourceAssetURL, asset.AssetURL), asset.ContentType),
	)
}

func creativePackageDownloadName(assetFilename string) string {
	name := strings.TrimSuffix(assetFilename, filepath.Ext(assetFilename))
	parts := strings.Split(name, "_")
	if len(parts) >= 8 && parts[1] == "P" && parts[6] != "" {
		return strings.Join(parts[:6], "_") + "_AI_PACKAGE.zip"
	}
	return "creative-results.zip"
}

func creativeSourceContentType(assetType, rawURL string) string {
	extension := strings.ToLower(filepath.Ext(strings.Split(rawURL, "?")[0]))
	switch extension {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	}
	if strings.EqualFold(assetType, "video") {
		return "video/mp4"
	}
	return "application/octet-stream"
}

func (h *Handler) readCreativePackageAsset(ctx context.Context, asset creativePackageAsset) ([]byte, error) {
	downloadURL := firstNonEmpty(asset.SourceAssetURL, asset.AssetURL)
	storageKey := strings.TrimSpace(asset.StorageKey)
	if storageKey == "" && strings.Contains(downloadURL, "/uploads/") && h.Storage != nil {
		storageKey = h.Storage.KeyFromURL(downloadURL)
	}
	if storageKey != "" && h.Storage != nil {
		reader, err := h.Storage.GetReader(ctx, storageKey)
		if err == nil {
			defer reader.Close()
			return readLimitedCreativeAsset(reader)
		}
	}
	if h.CreativeAssetDownloader == nil {
		return nil, errors.New("creative asset downloader not configured")
	}
	download, err := h.CreativeAssetDownloader.Fetch(ctx, downloadURL)
	if err != nil {
		return nil, err
	}
	return download.Data, nil
}

func readLimitedCreativeAsset(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxCreativePackageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCreativePackageBytes {
		return nil, errors.New("creative asset exceeds maximum package size")
	}
	return data, nil
}

func creativePackageExtension(rawURL, contentType string) string {
	if ext := strings.ToLower(filepath.Ext(strings.Split(rawURL, "?")[0])); len(ext) >= 2 && len(ext) <= 6 {
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	default:
		return ".bin"
	}
}

func safeCreativeZipPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "material"
	}
	var builder strings.Builder
	lastWasSeparator := false
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_' {
			builder.WriteRune(char)
			lastWasSeparator = false
		} else if builder.Len() > 0 && !lastWasSeparator {
			builder.WriteByte('-')
			lastWasSeparator = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "material"
	}
	return result
}

func shortCreativeID(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
