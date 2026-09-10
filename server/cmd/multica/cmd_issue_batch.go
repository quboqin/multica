package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var issueCreateBatchCmd = &cobra.Command{
	Use:   "create-batch",
	Short: "Create and assign several issues from one JSON manifest",
	Args:  cobra.NoArgs,
	RunE:  runIssueCreateBatch,
}

type issueCreateBatchManifest struct {
	MaxConcurrency int                    `json:"max_concurrency"`
	Defaults       issueCreateBatchItem   `json:"defaults"`
	Issues         []issueCreateBatchItem `json:"issues"`
}

type issueCreateBatchItem struct {
	Key            string                     `json:"key,omitempty"`
	Title          string                     `json:"title,omitempty"`
	Description    string                     `json:"description,omitempty"`
	Status         string                     `json:"status,omitempty"`
	Priority       string                     `json:"priority,omitempty"`
	Assignee       string                     `json:"assignee,omitempty"`
	AssigneeID     string                     `json:"assignee_id,omitempty"`
	Parent         string                     `json:"parent,omitempty"`
	Project        string                     `json:"project,omitempty"`
	StartDate      string                     `json:"start_date,omitempty"`
	DueDate        string                     `json:"due_date,omitempty"`
	Metadata       map[string]json.RawMessage `json:"metadata,omitempty"`
	AllowDuplicate bool                       `json:"allow_duplicate,omitempty"`
}

type preparedIssueCreateBatchItem struct {
	Key  string
	Body map[string]any
}

type issueCreateBatchResult struct {
	Key        string `json:"key,omitempty"`
	Status     string `json:"status"`
	ID         string `json:"id,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Title      string `json:"title,omitempty"`
	Error      string `json:"error,omitempty"`
}

type issueCreateBatchSummary struct {
	Created int                      `json:"created"`
	Failed  int                      `json:"failed"`
	Results []issueCreateBatchResult `json:"results"`
}

func init() {
	issueCmd.AddCommand(issueCreateBatchCmd)
	issueCreateBatchCmd.Flags().String("input-file", "", "UTF-8 JSON batch manifest")
	issueCreateBatchCmd.Flags().String("output", "json", "Output format: json")
}

func runIssueCreateBatch(cmd *cobra.Command, _ []string) error {
	inputFile, _ := cmd.Flags().GetString("input-file")
	if strings.TrimSpace(inputFile) == "" {
		return fmt.Errorf("--input-file is required")
	}
	raw, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("read issue batch manifest: %w", err)
	}
	var manifest issueCreateBatchManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("decode issue batch manifest: %w", err)
	}
	if manifest.MaxConcurrency == 0 {
		manifest.MaxConcurrency = 6
	}
	if manifest.MaxConcurrency < 1 || manifest.MaxConcurrency > 20 {
		return fmt.Errorf("max_concurrency must be between 1 and 20")
	}
	if len(manifest.Issues) == 0 || len(manifest.Issues) > 50 {
		return fmt.Errorf("issues must contain between 1 and 50 entries")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(2*time.Minute))
	defer cancel()
	prepared, err := prepareIssueCreateBatch(ctx, client, manifest)
	if err != nil {
		return err
	}
	summary := executeIssueCreateBatch(ctx, client, manifest.MaxConcurrency, prepared)
	if err := cli.PrintJSON(os.Stdout, summary); err != nil {
		return err
	}
	if summary.Failed > 0 {
		return fmt.Errorf("issue batch completed with %d failed create(s)", summary.Failed)
	}
	return nil
}

func prepareIssueCreateBatch(ctx context.Context, client *cli.APIClient, manifest issueCreateBatchManifest) ([]preparedIssueCreateBatchItem, error) {
	prepared := make([]preparedIssueCreateBatchItem, 0, len(manifest.Issues))
	keys := make(map[string]struct{}, len(manifest.Issues))
	for index, item := range manifest.Issues {
		merged := mergeIssueCreateBatchItem(manifest.Defaults, item)
		merged.Key = strings.TrimSpace(merged.Key)
		if merged.Key == "" {
			merged.Key = fmt.Sprintf("issue-%d", index+1)
		}
		if _, exists := keys[merged.Key]; exists {
			return nil, fmt.Errorf("duplicate issue batch key %q", merged.Key)
		}
		keys[merged.Key] = struct{}{}
		if strings.TrimSpace(merged.Title) == "" {
			return nil, fmt.Errorf("issue %q requires title", merged.Key)
		}
		if strings.TrimSpace(merged.Assignee) != "" && strings.TrimSpace(merged.AssigneeID) != "" {
			return nil, fmt.Errorf("issue %q must use assignee or assignee_id, not both", merged.Key)
		}

		body := map[string]any{"title": merged.Title}
		if merged.Description != "" {
			body["description"] = merged.Description
		}
		if merged.Status != "" {
			body["status"] = merged.Status
		}
		if merged.Priority != "" {
			body["priority"] = merged.Priority
		}
		if merged.StartDate != "" {
			body["start_date"] = merged.StartDate
		}
		if merged.DueDate != "" {
			body["due_date"] = merged.DueDate
		}
		if len(merged.Metadata) > 0 {
			body["metadata"] = merged.Metadata
		}
		if merged.AllowDuplicate {
			body["allow_duplicate"] = true
		}
		if merged.Parent != "" {
			parent, err := resolveIssueRef(ctx, client, merged.Parent)
			if err != nil {
				return nil, fmt.Errorf("issue %q resolve parent: %w", merged.Key, err)
			}
			body["parent_issue_id"] = parent.ID
		}
		if merged.Project != "" {
			projectID, err := resolveProjectID(ctx, client, merged.Project)
			if err != nil {
				return nil, fmt.Errorf("issue %q resolve project: %w", merged.Key, err)
			}
			body["project_id"] = projectID
		}
		if merged.Assignee != "" {
			assigneeType, assigneeID, err := resolveAssignee(ctx, client, merged.Assignee, issueAssigneeKinds)
			if err != nil {
				return nil, fmt.Errorf("issue %q resolve assignee: %w", merged.Key, err)
			}
			body["assignee_type"] = assigneeType
			body["assignee_id"] = assigneeID
		} else if merged.AssigneeID != "" {
			assigneeType, assigneeID, err := resolveAssigneeByID(ctx, client, merged.AssigneeID, issueAssigneeKinds)
			if err != nil {
				return nil, fmt.Errorf("issue %q resolve assignee_id: %w", merged.Key, err)
			}
			body["assignee_type"] = assigneeType
			body["assignee_id"] = assigneeID
		}
		prepared = append(prepared, preparedIssueCreateBatchItem{Key: merged.Key, Body: body})
	}
	return prepared, nil
}

func mergeIssueCreateBatchItem(defaults, item issueCreateBatchItem) issueCreateBatchItem {
	merged := defaults
	if item.Key != "" {
		merged.Key = item.Key
	}
	if item.Title != "" {
		merged.Title = item.Title
	}
	if item.Description != "" {
		merged.Description = item.Description
	}
	if item.Status != "" {
		merged.Status = item.Status
	}
	if item.Priority != "" {
		merged.Priority = item.Priority
	}
	if item.Assignee != "" || item.AssigneeID != "" {
		merged.Assignee, merged.AssigneeID = item.Assignee, item.AssigneeID
	}
	if item.Parent != "" {
		merged.Parent = item.Parent
	}
	if item.Project != "" {
		merged.Project = item.Project
	}
	if item.StartDate != "" {
		merged.StartDate = item.StartDate
	}
	if item.DueDate != "" {
		merged.DueDate = item.DueDate
	}
	if item.Metadata != nil {
		merged.Metadata = item.Metadata
	}
	if item.AllowDuplicate {
		merged.AllowDuplicate = true
	}
	return merged
}

func executeIssueCreateBatch(ctx context.Context, client *cli.APIClient, maxConcurrency int, items []preparedIssueCreateBatchItem) issueCreateBatchSummary {
	results := make([]issueCreateBatchResult, len(items))
	semaphore := make(chan struct{}, maxConcurrency)
	var wait sync.WaitGroup
	for index := range items {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case <-ctx.Done():
				results[index] = issueCreateBatchResult{Key: items[index].Key, Status: "failed", Error: ctx.Err().Error()}
				return
			case semaphore <- struct{}{}:
			}
			defer func() { <-semaphore }()
			var created map[string]any
			if err := client.PostJSON(ctx, "/api/issues", items[index].Body, &created); err != nil {
				results[index] = issueCreateBatchResult{Key: items[index].Key, Status: "failed", Title: fmt.Sprint(items[index].Body["title"]), Error: err.Error()}
				return
			}
			results[index] = issueCreateBatchResult{Key: items[index].Key, Status: "created", ID: strVal(created, "id"), Identifier: strVal(created, "identifier"), Title: strVal(created, "title")}
		}()
	}
	wait.Wait()
	summary := issueCreateBatchSummary{Results: results}
	for _, result := range results {
		if result.Status == "created" {
			summary.Created++
		} else {
			summary.Failed++
		}
	}
	return summary
}
