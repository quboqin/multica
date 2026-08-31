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

type assertionError string

func (e assertionError) Error() string { return string(e) }
