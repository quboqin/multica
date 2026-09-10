package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxCreativeArchiveAttempts = 8

type creativeArchiveCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	AssetType   string
	PreviewURL  string
	ResourceURL string
	PosterURL   string
}

func (h *Handler) ArchiveDueCreativeMaterials(ctx context.Context, limit int) (int, error) {
	if h.DB == nil {
		return 0, errors.New("database executor not configured")
	}
	if h.Storage == nil {
		return 0, errors.New("creative material storage not configured")
	}
	if h.CreativeAssetDownloader == nil {
		return 0, errors.New("creative material downloader not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	// Recover claims left behind by a stopped server before looking for new work.
	// A final-attempt claim becomes a visible failure; earlier claims are retried.
	if _, err := h.DB.Exec(ctx, `
UPDATE creative_material_candidate
SET archive_status = CASE WHEN archive_attempts >= $1 THEN 'failed' ELSE 'pending' END,
    archive_error = CASE
      WHEN archive_error <> '' THEN archive_error
      ELSE 'archive worker stopped before completing this attempt'
    END,
    next_archive_at = now(), updated_at = now()
WHERE archived_url = '' AND archive_status = 'running' AND next_archive_at <= now()
`, maxCreativeArchiveAttempts); err != nil {
		return 0, err
	}
	if _, err := h.DB.Exec(ctx, `
UPDATE creative_material_candidate
SET archive_status = 'failed',
    archive_error = CASE
      WHEN archive_error <> '' THEN archive_error
      ELSE 'archive retry limit reached before a stable file was saved'
    END,
    updated_at = now()
WHERE archived_url = '' AND archive_status = 'pending' AND archive_attempts >= $1
`, maxCreativeArchiveAttempts); err != nil {
		return 0, err
	}
	rows, err := h.DB.Query(ctx, `
WITH due AS (
  SELECT id
  FROM creative_material_candidate
  WHERE archived_url = ''
    AND archive_status = 'pending'
    AND archive_attempts < $2
    AND next_archive_at <= now()
  ORDER BY next_archive_at, created_at
  FOR UPDATE SKIP LOCKED
  LIMIT $1
)
UPDATE creative_material_candidate c
SET archive_status = 'running', archive_attempts = archive_attempts + 1,
    next_archive_at = now() + interval '2 minutes', updated_at = now()
FROM due
WHERE c.id = due.id
RETURNING c.id, c.workspace_id, c.asset_type, c.preview_url, c.resource_url, c.poster_url
`, limit, maxCreativeArchiveAttempts)
	if err != nil {
		return 0, err
	}
	candidates := []creativeArchiveCandidate{}
	for rows.Next() {
		var candidate creativeArchiveCandidate
		if err := rows.Scan(
			&candidate.ID, &candidate.WorkspaceID, &candidate.AssetType,
			&candidate.PreviewURL, &candidate.ResourceURL, &candidate.PosterURL,
		); err != nil {
			rows.Close()
			return len(candidates), err
		}
		candidates = append(candidates, candidate)
	}
	rows.Close()

	var archiveErr error
	for _, candidate := range candidates {
		if err := h.archiveCreativeMaterial(ctx, candidate); err != nil {
			slog.Warn("creative material archiver: archive failed", "candidate_id", uuidToString(candidate.ID), "error", err)
			archiveErr = errors.Join(archiveErr, err)
		}
	}
	return len(candidates), archiveErr
}

func (h *Handler) archiveCreativeMaterial(ctx context.Context, candidate creativeArchiveCandidate) error {
	sourceURL := creativeArchiveSource(candidate)
	if sourceURL == "" {
		return h.markCreativeArchiveFailed(ctx, candidate, errors.New("creative material has no downloadable asset URL"))
	}
	if strings.HasPrefix(sourceURL, "/uploads/") {
		return h.completeCreativeArchive(ctx, candidate, sourceURL, 0)
	}

	download, err := h.CreativeAssetDownloader.Fetch(ctx, sourceURL)
	if err != nil {
		_ = h.markCreativeArchiveFailed(ctx, candidate, err)
		return err
	}
	key := fmt.Sprintf(
		"creative-materials/%s/%s/source%s",
		uuidToString(candidate.WorkspaceID),
		uuidToString(candidate.ID),
		download.Extension,
	)
	archivedURL, err := h.Storage.Upload(ctx, key, download.Data, download.ContentType, "source"+download.Extension)
	if err != nil {
		_ = h.markCreativeArchiveFailed(ctx, candidate, err)
		return err
	}
	return h.completeCreativeArchive(ctx, candidate, archivedURL, int64(len(download.Data)))
}

func (h *Handler) completeCreativeArchive(ctx context.Context, candidate creativeArchiveCandidate, archivedURL string, sizeBytes int64) error {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var sourceAttachmentID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT source_attachment_id
FROM creative_material_candidate
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, candidate.ID, candidate.WorkspaceID).Scan(&sourceAttachmentID); err != nil {
		return err
	}
	if !sourceAttachmentID.Valid {
		var uploaderID pgtype.UUID
		if err := tx.QueryRow(ctx, `
SELECT user_id
FROM member
WHERE workspace_id = $1
ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, created_at
LIMIT 1
`, candidate.WorkspaceID).Scan(&uploaderID); err != nil {
			return fmt.Errorf("find creative archive attachment owner: %w", err)
		}
		attachmentUUID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("create creative archive attachment id: %w", err)
		}
		sourceAttachmentID = pgtype.UUID{Bytes: attachmentUUID, Valid: true}
		if _, err := tx.Exec(ctx, `
INSERT INTO attachment (id, workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, $6, $7)
`, sourceAttachmentID, candidate.WorkspaceID, uploaderID,
			creativeCandidateArchiveFilename(archivedURL, candidate.AssetType), archivedURL,
			creativeArchiveContentType(archivedURL, candidate.AssetType), sizeBytes); err != nil {
			return fmt.Errorf("register creative archive attachment: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_material_candidate
SET archived_url = $2, archive_status = 'completed', archive_error = '',
    archived_at = now(), source_attachment_id = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $4
`, candidate.ID, archivedURL, sourceAttachmentID, candidate.WorkspaceID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.publishCreativeMaterialsUpdated(candidate.WorkspaceID, pgtype.UUID{}, "system", "")
	return nil
}

func creativeCandidateArchiveFilename(archivedURL, assetType string) string {
	filename := strings.TrimSpace(archivedURL)
	if slash := strings.LastIndex(filename, "/"); slash >= 0 && slash < len(filename)-1 {
		filename = filename[slash+1:]
	}
	if question := strings.IndexByte(filename, '?'); question >= 0 {
		filename = filename[:question]
	}
	if filename != "" {
		return filename
	}
	if strings.EqualFold(assetType, "video") {
		return "source.mp4"
	}
	return "source.jpg"
}

func (h *Handler) markCreativeArchiveFailed(ctx context.Context, candidate creativeArchiveCandidate, archiveErr error) error {
	_, err := h.DB.Exec(ctx, `
UPDATE creative_material_candidate
SET archive_status = CASE WHEN archive_attempts >= $2 THEN 'failed' ELSE 'pending' END,
    archive_error = $3,
    next_archive_at = now() + make_interval(
      secs => LEAST(900, GREATEST(15, (power(2, LEAST(archive_attempts, 6)) * 5)::int))
    ),
    updated_at = now()
WHERE id = $1
	`, candidate.ID, maxCreativeArchiveAttempts, truncateCreativeArchiveError(archiveErr.Error()))
	if err == nil {
		h.publishCreativeMaterialsUpdated(candidate.WorkspaceID, pgtype.UUID{}, "system", "")
	}
	return err
}

func creativeArchiveSource(candidate creativeArchiveCandidate) string {
	if candidate.AssetType == "video" {
		return firstNonEmpty(candidate.ResourceURL, candidate.PreviewURL, candidate.PosterURL)
	}
	return firstNonEmpty(candidate.PreviewURL, candidate.ResourceURL, candidate.PosterURL)
}

func truncateCreativeArchiveError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}
