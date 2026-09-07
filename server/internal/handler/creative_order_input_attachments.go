package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const creativeOrderAttachmentSnapshotSchemaVersion = 1

type creativeOrderAttachmentValidationError struct {
	Code    string
	Message string
}

func (e *creativeOrderAttachmentValidationError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "creative order attachment validation failed"
	}
	return e.Message
}

func creativeAttachmentFailureCode(err error) string {
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(err)))
	switch {
	case strings.Contains(text, "expired"), strings.Contains(text, "unauthorized"), strings.Contains(text, "forbidden"), strings.Contains(text, "login"):
		return "auth_expired"
	case strings.Contains(text, "timeout"), strings.Contains(text, "deadline exceeded"), strings.Contains(text, "temporarily unavailable"), strings.Contains(text, "connection reset"):
		return "storage_timeout"
	case strings.Contains(text, "unknown flag"), strings.Contains(text, "unknown command"), strings.Contains(text, "invalid argument"), strings.Contains(text, "cli contract"):
		return "cli_contract_mismatch"
	default:
		return "attachment_not_found"
	}
}

func creativeAttachmentFailureMentioned(value string) bool {
	text := strings.ToLower(value)
	return strings.Contains(text, "attachment") || strings.Contains(text, "download") ||
		strings.Contains(text, "storage") || strings.Contains(text, "object key") ||
		strings.Contains(text, "oss") || strings.Contains(text, "s3")
}

func creativeOrderAttachmentValidationFailure(scope string, err error) error {
	code := creativeAttachmentFailureCode(err)
	return &creativeOrderAttachmentValidationError{
		Code:    code,
		Message: fmt.Sprintf("%s is unavailable (%s)", scope, code),
	}
}

type creativeOrderFrozenAttachment struct {
	CandidateID    string `json:"candidate_id,omitempty"`
	ResourceFileID string `json:"resource_file_id,omitempty"`
	Role           string `json:"role,omitempty"`
	AttachmentID   string `json:"attachment_id"`
}

type creativeOrderAppUIAttachmentReference struct {
	CandidateID    string `json:"candidate_id"`
	ResourceFileID string `json:"resource_file_id"`
	AttachmentID   string `json:"attachment_id"`
}

func (h *Handler) freezeCreativeOrderInputAttachments(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID pgtype.UUID,
	raw json.RawMessage,
	items []creativeOrderItemInput,
) (json.RawMessage, error) {
	var snapshot map[string]json.RawMessage
	if json.Unmarshal(raw, &snapshot) != nil || snapshot == nil {
		return nil, errors.New("input_snapshot must be an object")
	}
	marketPackRaw, hasMarketPack := snapshot["market_pack"]
	if !hasMarketPack || len(marketPackRaw) == 0 || string(marketPackRaw) == "null" {
		// Legacy API clients did not freeze a market pack. Do not make their
		// idempotent recovery requests unreadable; all current UI submissions
		// include one and therefore receive the stronger validation below.
		return raw, nil
	}

	marketPack, _, filesByRole, err := frozenCreativePrimeMarketPack(raw)
	if err != nil {
		return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: err.Error()}
	}
	marketPackID, err := parseUUIDString(marketPack.ID)
	if err != nil || marketPack.Version < 1 {
		return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "frozen market pack version is invalid"}
	}
	var publishedVersion int
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(published_version, 0)
FROM creative_resource
WHERE id = $1 AND workspace_id = $2 AND kind = 'market_pack' AND status <> 'archived'
FOR SHARE
`, marketPackID, workspaceID).Scan(&publishedVersion); err != nil || publishedVersion != marketPack.Version {
		return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "frozen market pack is no longer the published version"}
	}

	filesByID := make(map[string]creativeResourceFileResponse, len(marketPack.Files))
	for _, file := range marketPack.Files {
		filesByID[file.ID] = file
	}
	primeAttachments := make([]creativeOrderFrozenAttachment, 0, len(filesByRole))
	roles := make([]string, 0, len(filesByRole))
	for role := range filesByRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		file := filesByRole[role]
		if err := h.validateCreativeOrderResourceAttachment(ctx, tx, workspaceID, marketPackID, marketPack.Version, file, role); err != nil {
			return nil, err
		}
		primeAttachments = append(primeAttachments, creativeOrderFrozenAttachment{
			ResourceFileID: file.ID,
			Role:           role,
			AttachmentID:   strings.TrimSpace(file.AttachmentID),
		})
	}

	candidateSources := make([]creativeOrderFrozenAttachment, 0, len(items))
	appUIReferences := make([]creativeOrderAppUIAttachmentReference, 0, len(items))
	for _, item := range items {
		if item.SourceKind == "copy_library" {
			continue
		}
		candidateID, parseErr := parseUUIDString(item.CandidateID)
		if parseErr != nil {
			return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "candidate source attachment is invalid"}
		}
		var sourceAttachmentID pgtype.UUID
		if err := tx.QueryRow(ctx, `
SELECT source_attachment_id
FROM creative_material_candidate
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, candidateID, workspaceID).Scan(&sourceAttachmentID); err != nil {
			return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "candidate source is unavailable"}
		}
		if !sourceAttachmentID.Valid {
			return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "candidate source attachment is unavailable"}
		}
		if _, err := h.readCreativePrimeAttachment(ctx, workspaceID, sourceAttachmentID); err != nil {
			return nil, creativeOrderAttachmentValidationFailure("candidate source attachment", err)
		}
		candidateSources = append(candidateSources, creativeOrderFrozenAttachment{
			CandidateID:  strings.TrimSpace(item.CandidateID),
			AttachmentID: uuidToString(sourceAttachmentID),
		})

		appUI, selected, appUIErr := creativeOrderSelectedAppUIAttachment(item.CopySnapshot)
		if appUIErr != nil {
			return nil, appUIErr
		}
		if !selected {
			continue
		}
		file, found := filesByID[appUI.ResourceFileID]
		if !found || strings.TrimSpace(file.AttachmentID) != appUI.AttachmentID || file.Role != "app_ui_reference" {
			return nil, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "selected App UI reference is not in the frozen market pack"}
		}
		if err := h.validateCreativeOrderResourceAttachment(ctx, tx, workspaceID, marketPackID, marketPack.Version, file, "selected App UI reference"); err != nil {
			return nil, err
		}
		appUIReferences = append(appUIReferences, creativeOrderAppUIAttachmentReference{
			CandidateID: strings.TrimSpace(item.CandidateID), ResourceFileID: appUI.ResourceFileID, AttachmentID: appUI.AttachmentID,
		})
	}

	attachmentSnapshot, err := json.Marshal(map[string]any{
		"schema_version":    creativeOrderAttachmentSnapshotSchemaVersion,
		"candidate_sources": candidateSources,
		"prime_templates":   primeAttachments,
		"app_ui_references": appUIReferences,
	})
	if err != nil {
		return nil, errors.New("failed to freeze creative order attachments")
	}
	snapshot["attachment_snapshot"] = attachmentSnapshot
	return json.Marshal(snapshot)
}

func (h *Handler) validateCreativeOrderResourceAttachment(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, resourceID pgtype.UUID,
	version int,
	file creativeResourceFileResponse,
	scope string,
) error {
	resourceFileID, err := parseUUIDString(file.ID)
	if err != nil {
		return &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: fmt.Sprintf("%s resource file is invalid", scope)}
	}
	attachmentID, err := parseUUIDString(file.AttachmentID)
	if err != nil {
		return &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: fmt.Sprintf("%s attachment is invalid", scope)}
	}
	var valid bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM creative_resource_file
  WHERE id = $1 AND resource_id = $2 AND workspace_id = $3 AND attachment_id = $4
    AND created_version <= $5
    AND (removed_version IS NULL OR removed_version > $5)
)
`, resourceFileID, resourceID, workspaceID, attachmentID, version).Scan(&valid); err != nil || !valid {
		return &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: fmt.Sprintf("%s attachment is no longer in the frozen market pack", scope)}
	}
	if _, err := h.readCreativePrimeAttachment(ctx, workspaceID, attachmentID); err != nil {
		return creativeOrderAttachmentValidationFailure(scope+" attachment", err)
	}
	return nil
}

func creativeOrderSelectedAppUIAttachment(raw json.RawMessage) (creativeOrderAppUIAttachmentReference, bool, error) {
	var snapshot struct {
		PreAdaptation struct {
			AppUIReplacement struct {
				Required       bool   `json:"required"`
				Selected       bool   `json:"selected"`
				ResourceFileID string `json:"resource_file_id"`
				AttachmentID   string `json:"attachment_id"`
			} `json:"app_ui_replacement"`
		} `json:"pre_adaptation"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &snapshot) != nil {
		return creativeOrderAppUIAttachmentReference{}, false, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "copy snapshot App UI selection is invalid"}
	}
	selection := snapshot.PreAdaptation.AppUIReplacement
	if !selection.Required && !selection.Selected {
		return creativeOrderAppUIAttachmentReference{}, false, nil
	}
	if !selection.Required || !selection.Selected || strings.TrimSpace(selection.ResourceFileID) == "" || strings.TrimSpace(selection.AttachmentID) == "" {
		return creativeOrderAppUIAttachmentReference{}, false, &creativeOrderAttachmentValidationError{Code: "attachment_not_found", Message: "requested App UI replacement requires a selected frozen reference"}
	}
	return creativeOrderAppUIAttachmentReference{
		ResourceFileID: strings.TrimSpace(selection.ResourceFileID),
		AttachmentID:   strings.TrimSpace(selection.AttachmentID),
	}, true, nil
}
