package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCreativePrimeTemplateCacheRoundTrip(t *testing.T) {
	t.Setenv("MULTICA_CREATIVE_PRIME_CACHE_DIR", t.TempDir())
	attachmentID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	attachment := db.Attachment{
		ID:        attachmentID,
		Url:       "https://static.example.test/workspaces/ws/templates/square.png",
		SizeBytes: 4,
	}

	writeCreativePrimeTemplateCache(attachment, []byte("body"))
	got, ok := readCreativePrimeTemplateCache(attachment)
	if !ok {
		t.Fatal("expected cached template")
	}
	if string(got) != "body" {
		t.Fatalf("cached template = %q, want body", string(got))
	}

	attachment.SizeBytes = 5
	if _, ok := readCreativePrimeTemplateCache(attachment); ok {
		t.Fatal("size mismatch must invalidate cached template")
	}
}

func TestCreativePrimeVariantLockSerializesOnlySameVariant(t *testing.T) {
	var h Handler
	first := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	second := pgtype.UUID{Bytes: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Valid: true}

	unlockFirst := h.lockCreativePrimeVariant(first)
	sameVariantLocked := make(chan struct{})
	sameVariantDone := make(chan struct{})
	go func() {
		unlock := h.lockCreativePrimeVariant(first)
		close(sameVariantLocked)
		unlock()
		close(sameVariantDone)
	}()
	select {
	case <-sameVariantLocked:
		t.Fatal("same variant lock was not serialized")
	case <-time.After(20 * time.Millisecond):
	}

	otherVariantDone := make(chan struct{})
	go func() {
		unlock := h.lockCreativePrimeVariant(second)
		unlock()
		close(otherVariantDone)
	}()
	select {
	case <-otherVariantDone:
	case <-time.After(time.Second):
		t.Fatal("different variant lock was blocked")
	}

	unlockFirst()
	select {
	case <-sameVariantLocked:
	case <-time.After(time.Second):
		t.Fatal("same variant lock did not release")
	}
	select {
	case <-sameVariantDone:
	case <-time.After(time.Second):
		t.Fatal("same variant lock did not finish")
	}

	h.creativePrimeLocksMu.Lock()
	defer h.creativePrimeLocksMu.Unlock()
	if len(h.creativePrimeLocks) != 0 {
		t.Fatalf("creative Prime lock table retained %d entries", len(h.creativePrimeLocks))
	}
}

func TestCreativePrimeComposeSlotLimit(t *testing.T) {
	t.Setenv("MULTICA_CREATIVE_PRIME_MAX_CONCURRENT", "1")
	var h Handler
	releaseFirst, err := h.acquireCreativePrimeComposeSlot(context.Background())
	if err != nil {
		t.Fatalf("acquire first compose slot: %v", err)
	}

	secondAcquired := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		releaseSecond, err := h.acquireCreativePrimeComposeSlot(context.Background())
		if err != nil {
			close(secondDone)
			return
		}
		close(secondAcquired)
		releaseSecond()
		close(secondDone)
	}()
	select {
	case <-secondAcquired:
		t.Fatal("second compose slot acquired despite limit 1")
	case <-time.After(20 * time.Millisecond):
	}

	releaseFirst()
	select {
	case <-secondAcquired:
	case <-time.After(time.Second):
		t.Fatal("second compose slot did not acquire after release")
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second compose slot did not finish")
	}
}

func TestCreativePrimeCompositionFailurePayloadKeepsPerSizeEvidence(t *testing.T) {
	manifestID := pgtype.UUID{Bytes: uuid.MustParse("33333333-3333-3333-3333-333333333333"), Valid: true}
	resultID := pgtype.UUID{Bytes: uuid.MustParse("44444444-4444-4444-4444-444444444444"), Valid: true}
	failure := json.RawMessage(`{
  "size":"800x1000",
  "error_code":"prime_no_adequate_template_for_size",
  "template_selection":{"visual_adequacy":{"failure_code":"prime_no_adequate_template_for_size","inadequacy_codes":["prime_background_polarity_mismatch","prime_background_too_textured"]}}
}`)
	cause := &creativePrimeCompositionError{
		cause:                errors.New("brand component composition rejected"),
		manifestAttachmentID: manifestID, composeResultAttachmentID: resultID,
		failures: []json.RawMessage{failure},
	}
	payload := creativePrimeCompositionFailurePayload(cause, time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	var decoded struct {
		ManifestAttachmentID      string                       `json:"manifest_attachment_id"`
		ComposeResultAttachmentID string                       `json:"compose_result_attachment_id"`
		FailureReasons            []creativePrimeFailureReason `json:"failure_reasons"`
		Retryable                 bool                         `json:"retryable"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode failure payload: %v", err)
	}
	if decoded.ManifestAttachmentID != uuidToString(manifestID) || decoded.ComposeResultAttachmentID != uuidToString(resultID) {
		t.Fatalf("failure evidence attachment ids were lost: %s", payload)
	}
	if len(decoded.FailureReasons) != 1 || decoded.FailureReasons[0].Size != "800x1000" || decoded.FailureReasons[0].ErrorCode != "prime_no_adequate_template_for_size" {
		t.Fatalf("failure reasons were not preserved: %s", payload)
	}
	if len(decoded.FailureReasons[0].InadequacyCodes) != 2 || !decoded.Retryable {
		t.Fatalf("failure quality evidence is incomplete: %s", payload)
	}
}
