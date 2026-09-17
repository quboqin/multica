package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestDocumentsCannotEnqueueAgentRuns(t *testing.T) {
	ctx := context.Background()
	doc := db.Issue{Kind: "doc"}
	tasks := &TaskService{}
	for name, run := range map[string]func() error{
		"assignment": func() error { _, err := tasks.EnqueueTaskForIssue(ctx, doc); return err },
		"mention": func() error {
			_, err := tasks.EnqueueTaskForMention(ctx, doc, pgtype.UUID{}, pgtype.UUID{})
			return err
		},
		"squad": func() error {
			_, err := tasks.EnqueueTaskForSquadLeader(ctx, doc, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if run() == nil {
				t.Fatal("document run accepted")
			}
		})
	}
	issues := &IssueService{}
	if _, ok := issues.WillEnqueueRun(ctx, IssueTriggerInput{Issue: doc}, IssueTriggerProbe{}); ok {
		t.Fatal("document trigger accepted")
	}
}
