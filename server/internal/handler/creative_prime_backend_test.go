package handler

import (
	"context"
	"encoding/json"
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

func TestCreativePrimeSkipsQCHonorsDirectAdjustmentFinalValidation(t *testing.T) {
	legacy := json.RawMessage(`{"creative_direct_edit_delivery":{"skip_qc":true}}`)
	if !creativePrimeSkipsQC("creative_direct_edit", legacy, creativeDirectEditDeliveryConfig{}) {
		t.Fatal("legacy direct edit should retain its explicit QC skip")
	}
	validated := json.RawMessage(`{"creative_direct_edit_delivery":{"skip_qc":false,"final_visual_validation":true}}`)
	if creativePrimeSkipsQC("creative_direct_edit", validated, parseCreativeDirectEditDeliveryConfig(validated)) {
		t.Fatal("final-validated direct adjustment must enter visual QC")
	}
}
