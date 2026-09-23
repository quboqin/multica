// Package documentaccess applies document and collection audiences outside HTTP handlers.
package documentaccess

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// EventPolicy applies to every producer, so a new comment or attachment event
// cannot accidentally return private document content to the workspace room.
func EventPolicy(q *db.Queries) func(events.Event) (events.Event, bool) {
	return func(e events.Event) (events.Event, bool) {
		if e.RecipientUserIDs != nil {
			return e, true
		}
		// Pins are personal, even when the referenced table is shared.
		if strings.HasPrefix(e.Type, "pin:") && e.ActorType == "member" {
			if e.ActorID == "" {
				return e, false
			}
			e.RecipientUserIDs = []string{e.ActorID}
			return e, true
		}
		var payload map[string]json.RawMessage
		raw, err := json.Marshal(e.Payload)
		if err != nil {
			return e, false
		}
		if json.Unmarshal(raw, &payload) != nil {
			return e, true
		}

		collectionID := ""
		_ = json.Unmarshal(payload["collection_id"], &collectionID)
		if collectionID == "" {
			for _, key := range []string{"record", "view"} {
				var item struct {
					CollectionID string `json:"collection_id"`
				}
				if json.Unmarshal(payload[key], &item) == nil && item.CollectionID != "" {
					collectionID = item.CollectionID
					break
				}
			}
		}
		if collectionID != "" {
			id, err := util.ParseUUID(collectionID)
			if err != nil {
				return e, false
			}
			ws, err := util.ParseUUID(e.WorkspaceID)
			if err != nil {
				return e, false
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			readers, err := q.ListCollectionReaders(ctx, db.ListCollectionReadersParams{CollectionID: id, WorkspaceID: ws})
			if err != nil {
				return e, false
			}
			e.RecipientUserIDs = []string{}
			for _, reader := range readers {
				e.RecipientUserIDs = append(e.RecipientUserIDs, util.UUIDToString(reader))
			}
			return e, true
		}
		id := ""
		_ = json.Unmarshal(payload["issue_id"], &id)
		if id == "" {
			for _, key := range []string{"issue", "comment", "item", "activity", "attachment", "task"} {
				var item struct {
					ID      string `json:"id"`
					IssueID string `json:"issue_id"`
				}
				if json.Unmarshal(payload[key], &item) == nil {
					id = item.IssueID
					if key == "issue" {
						id = item.ID
					}
					if id != "" {
						break
					}
				}
			}
		}
		if id == "" {
			return e, true
		}
		uid, err := util.ParseUUID(id)
		if err != nil {
			return e, false
		}
		ws, err := util.ParseUUID(e.WorkspaceID)
		if err != nil {
			return e, false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		issue, err := q.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: uid, WorkspaceID: ws})
		if err != nil {
			// Deletion notifications contain IDs only; drop other unresolved
			// resource events rather than broadcast cached document content.
			return e, e.Type == "issue:deleted"
		}
		if issue.Kind != "doc" {
			return e, true
		}
		readers, err := q.ListDocumentReaders(ctx, db.ListDocumentReadersParams{IssueID: uid, WorkspaceID: ws})
		if err != nil {
			return e, false
		}
		e.RecipientUserIDs = []string{}
		for _, reader := range readers {
			e.RecipientUserIDs = append(e.RecipientUserIDs, util.UUIDToString(reader))
		}
		return e, true
	}
}
