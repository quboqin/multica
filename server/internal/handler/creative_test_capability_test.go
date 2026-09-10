package handler

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"testing"
)

func enableCreativeFactoryForTest(t *testing.T) {
	t.Helper()
	var prior pgtype.Bool
	if err := testPool.QueryRow(t.Context(), `SELECT (SELECT enabled FROM workspace_capability WHERE workspace_id=$1 AND capability_key='creative_factory')`, testWorkspaceID).Scan(&prior); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `INSERT INTO workspace_capability(workspace_id,capability_key,enabled) VALUES($1,'creative_factory',true) ON CONFLICT(workspace_id,capability_key) DO UPDATE SET enabled=true`, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if prior.Valid {
			_, _ = testPool.Exec(context.Background(), `UPDATE workspace_capability SET enabled=$2 WHERE workspace_id=$1 AND capability_key='creative_factory'`, testWorkspaceID, prior.Bool)
		} else {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace_capability WHERE workspace_id=$1 AND capability_key='creative_factory'`, testWorkspaceID)
		}
	})
}
