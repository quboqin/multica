package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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
		return h.markCreativeArchiveFailed(ctx, candidate.ID, errors.New("creative material has no downloadable asset URL"))
	}
	if strings.HasPrefix(sourceURL, "/uploads/") {
		return h.completeCreativeArchive(ctx, candidate, sourceURL)
	}

	download, err := h.CreativeAssetDownloader.Fetch(ctx, sourceURL)
	if err != nil {
		_ = h.markCreativeArchiveFailed(ctx, candidate.ID, err)
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
		_ = h.markCreativeArchiveFailed(ctx, candidate.ID, err)
		return err
	}
	return h.completeCreativeArchive(ctx, candidate, archivedURL)
}

func (h *Handler) completeCreativeArchive(ctx context.Context, candidate creativeArchiveCandidate, archivedURL string) error {
	if _, err := h.DB.Exec(ctx, `
UPDATE creative_material_candidate
SET archived_url = $2, archive_status = 'completed', archive_error = '',
    archived_at = now(), updated_at = now()
WHERE id = $1
`, candidate.ID, archivedURL); err != nil {
		return err
	}
	h.publishCreativeArchiveCandidateIssues(ctx, candidate.WorkspaceID, candidate.ID)
	return nil
}

func (h *Handler) markCreativeArchiveFailed(ctx context.Context, candidateID pgtype.UUID, archiveErr error) error {
	_, err := h.DB.Exec(ctx, `
UPDATE creative_material_candidate
SET archive_status = CASE WHEN archive_attempts >= $2 THEN 'failed' ELSE 'pending' END,
    archive_error = $3,
    next_archive_at = now() + make_interval(
      secs => LEAST(900, GREATEST(15, (power(2, LEAST(archive_attempts, 6)) * 5)::int))
    ),
    updated_at = now()
WHERE id = $1
`, candidateID, maxCreativeArchiveAttempts, truncateCreativeArchiveError(archiveErr.Error()))
	return err
}

func (h *Handler) publishCreativeArchiveCandidateIssues(ctx context.Context, workspaceID, candidateID pgtype.UUID) {
	rows, err := h.DB.Query(ctx, `
SELECT issue_id
FROM creative_material_issue_candidate
WHERE workspace_id = $1 AND candidate_id = $2
`, workspaceID, candidateID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var issueID pgtype.UUID
		if rows.Scan(&issueID) == nil {
			h.publishCreativeMaterialsUpdated(workspaceID, issueID, "system", "")
		}
	}
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
