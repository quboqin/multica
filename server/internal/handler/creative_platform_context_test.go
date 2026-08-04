package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCreativeContextAgentCanBindOnlyAssignedSquadAsLeader(t *testing.T) {
	workspaceID := util.MustParseUUID("cf7160d6-8bc3-4d04-8226-6d771bd94ae7")
	squadID := util.MustParseUUID("4ff69424-c0cd-4cb2-894b-32bc17132739")
	leaderID := util.MustParseUUID("b3460435-3bec-4921-8693-94a5e2e0d59c")
	issue := db.Issue{
		WorkspaceID:  workspaceID,
		AssigneeType: pgtype.Text{String: "squad", Valid: true},
		AssigneeID:   squadID,
	}
	squad := db.Squad{ID: squadID, WorkspaceID: workspaceID, LeaderID: leaderID}

	if !creativeContextAgentCanBind(issue, squadID, util.UUIDToString(leaderID), squad) {
		t.Fatal("assigned squad leader should be allowed to bind creative context")
	}
	if creativeContextAgentCanBind(issue, squadID, "242aef75-795e-415c-b0d5-b548e2c75550", squad) {
		t.Fatal("non-leader agent must not bind creative context")
	}
	otherSquadID := util.MustParseUUID("518d2f40-fac3-4dc2-8e18-d78fc2a0f8c4")
	if creativeContextAgentCanBind(issue, otherSquadID, util.UUIDToString(leaderID), squad) {
		t.Fatal("leader must not bind a different squad")
	}
	otherWorkspace := util.MustParseUUID("50de7709-657e-4d25-b072-f61831cd75b6")
	if creativeContextAgentCanBind(issue, squadID, util.UUIDToString(leaderID), db.Squad{
		ID: squadID, WorkspaceID: otherWorkspace, LeaderID: leaderID,
	}) {
		t.Fatal("leader must not bind a squad from another workspace")
	}
}
