package attribution

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestDelegatedRunCopiesHumanAttribution(t *testing.T) {
	parent := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	human := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	evidence := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	got := DelegatedRun(parent, human, pgtype.UUID{}, EvidenceKind("creative_variant"), evidence)
	if got.Source != SourceDelegation || got.DelegatedFromTaskID != parent || got.UserID != human || got.AccountableUserID != human || got.EvidenceRefID != evidence {
		t.Fatalf("delegated attribution = %#v", got)
	}
}
