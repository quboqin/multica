package handler

import "testing"

func TestCreativeAttachmentFailureCodeClassifiesRecoveryPaths(t *testing.T) {
	for _, test := range []struct {
		message string
		want    string
	}{
		{message: "attachment download failed: session expired", want: "auth_expired"},
		{message: "object storage request timeout", want: "storage_timeout"},
		{message: "attachment key not found", want: "attachment_not_found"},
		{message: "unknown flag: --output-file", want: "cli_contract_mismatch"},
	} {
		if got := creativeAttachmentFailureCode(assertionError(test.message)); got != test.want {
			t.Fatalf("creativeAttachmentFailureCode(%q) = %q, want %q", test.message, got, test.want)
		}
	}
}

func TestNormalizeCreativeImageOperationClassifiesAttachmentFailure(t *testing.T) {
	normalized, err := normalizeCreativeImageOperation(creativeImageOperationInput{
		VariantID:      "3e93979d-7562-4b15-9cd8-357a215a7c0a",
		SizeKey:        "1080x1080",
		Revision:       1,
		OperationKind:  "generation",
		IdempotencyKey: "attachment-classification",
		Status:         "failed",
		Attempt:        1,
		ErrorType:      "attachment_download_failed",
		ErrorMessage:   "attachment download failed: session expired",
		InputSnapshot:  []byte(`{}`),
		ResultReceipt:  []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ErrorType != "auth_expired" {
		t.Fatalf("error type = %q, want auth_expired", normalized.ErrorType)
	}
}

func TestBindArchivedCreativeCandidateSourceAttachmentRepairsHistoricalArchive(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	handler := *testHandler
	store := &mockStorage{}
	handler.Storage = store
	const key = "creative-materials/historical/source.jpeg"
	const archivedURL = "https://cdn.example.com/" + key
	store.put(key, []byte("archived source"))

	ctx := t.Context()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var candidateID string
	if err := tx.QueryRow(ctx, `
INSERT INTO creative_material_candidate (
  workspace_id, connector_id, dedupe_key, title, asset_type, archived_url, archive_status, raw
)
VALUES ($1, 'test', $2, 'Historical archive', 'image', $3, 'completed', '{}'::jsonb)
RETURNING id::text
`, testWorkspaceID, "historical-source-attachment-"+testWorkspaceID, archivedURL).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := handler.bindArchivedCreativeCandidateSourceAttachment(
		ctx, tx, parseUUID(testWorkspaceID), parseUUID(testUserID), parseUUID(candidateID), archivedURL, "completed", "image",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE id = $1`, attachmentID)
	})

	var boundID, attachmentURL, filename, contentType string
	var sizeBytes int64
	if err := testPool.QueryRow(ctx, `
SELECT c.source_attachment_id::text, a.url, a.filename, a.content_type, a.size_bytes
FROM creative_material_candidate c
JOIN attachment a ON a.id = c.source_attachment_id
WHERE c.id = $1
`, candidateID).Scan(&boundID, &attachmentURL, &filename, &contentType, &sizeBytes); err != nil {
		t.Fatal(err)
	}
	if boundID != uuidToString(attachmentID) || attachmentURL != archivedURL || filename != "source.jpeg" || contentType != "image/jpeg" || sizeBytes != int64(len("archived source")) {
		t.Fatalf("bound source = id=%q url=%q filename=%q content_type=%q bytes=%d", boundID, attachmentURL, filename, contentType, sizeBytes)
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
