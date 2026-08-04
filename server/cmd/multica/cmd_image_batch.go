package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var imageEditBatchCmd = &cobra.Command{
	Use:   "edit-batch",
	Short: "Execute a dependency-aware batch of GPT Image edits",
	Args:  cobra.NoArgs,
	RunE:  runImageEditBatch,
}

type imageEditBatchManifest struct {
	MaxConcurrency int                 `json:"max_concurrency"`
	Jobs           []imageEditBatchJob `json:"jobs"`
}

type imageEditBatchJob struct {
	ID          string                `json:"id"`
	Inputs      []imageEditBatchInput `json:"inputs"`
	Mask        string                `json:"mask,omitempty"`
	Prompt      string                `json:"prompt,omitempty"`
	PromptFile  string                `json:"prompt_file,omitempty"`
	Model       string                `json:"model,omitempty"`
	Size        string                `json:"size"`
	Quality     string                `json:"quality,omitempty"`
	MaxAttempts int                   `json:"max_attempts,omitempty"`
	OutputFile  string                `json:"output_file"`
}

type imageEditBatchInput struct {
	Path string `json:"path,omitempty"`
	Job  string `json:"job,omitempty"`
}

type preparedImageEditBatch struct {
	MaxConcurrency int
	Jobs           []preparedImageEditJob
}

type preparedImageEditJob struct {
	ID          string
	Inputs      []imageEditBatchInput
	Mask        string
	Prompt      string
	Model       string
	Size        string
	Quality     string
	MaxAttempts int
	OutputFile  string
	DependsOn   []string
}

type imageEditBatchResult struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	Model           string  `json:"model"`
	Size            string  `json:"size"`
	Path            string  `json:"path,omitempty"`
	Bytes           int     `json:"bytes,omitempty"`
	RequestID       string  `json:"request_id,omitempty"`
	Attempts        int     `json:"attempts,omitempty"`
	DurationSeconds float64 `json:"duration_seconds"`
	Error           string  `json:"error,omitempty"`
}

type imageEditBatchSummary struct {
	MaxConcurrency    int                    `json:"max_concurrency"`
	ProviderSlotLimit int                    `json:"provider_slot_limit"`
	WallSeconds       float64                `json:"wall_seconds"`
	Succeeded         int                    `json:"succeeded"`
	Failed            int                    `json:"failed"`
	Skipped           int                    `json:"skipped"`
	Results           []imageEditBatchResult `json:"results"`
}

var imageEditBatchJobIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func init() {
	imageCmd.AddCommand(imageEditBatchCmd)
	imageEditBatchCmd.Flags().String("input-file", "", "UTF-8 JSON batch manifest")
	imageEditBatchCmd.Flags().String("output", "json", "Output format: json")
}

func runImageEditBatch(cmd *cobra.Command, _ []string) error {
	inputFile, _ := cmd.Flags().GetString("input-file")
	if strings.TrimSpace(inputFile) == "" {
		return fmt.Errorf("--input-file is required")
	}
	batch, err := loadImageEditBatchManifest(inputFile)
	if err != nil {
		return err
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required for image edit")
	}
	endpoint, err := openAIImageEditEndpoint(os.Getenv("OPENAI_BASE_URL"), os.Getenv("OPENAI_IMAGE_EDIT_PATH"))
	if err != nil {
		return err
	}
	imageField := strings.TrimSpace(os.Getenv("OPENAI_IMAGE_FILE_FIELD"))
	if imageField == "" {
		imageField = "image"
	}
	providerSlotLimit, err := configuredImageConcurrency()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(35*time.Minute))
	defer cancel()
	summary := executeImageEditBatch(ctx, http.DefaultClient, endpoint, apiKey, imageField, batch)
	summary.ProviderSlotLimit = providerSlotLimit
	if err := cli.PrintJSON(os.Stdout, summary); err != nil {
		return err
	}
	if summary.Failed > 0 || summary.Skipped > 0 {
		return fmt.Errorf("image edit batch completed with %d failed and %d skipped job(s)", summary.Failed, summary.Skipped)
	}
	return nil
}

func loadImageEditBatchManifest(path string) (preparedImageEditBatch, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return preparedImageEditBatch{}, fmt.Errorf("read image batch manifest: %w", err)
	}
	var manifest imageEditBatchManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return preparedImageEditBatch{}, fmt.Errorf("decode image batch manifest: %w", err)
	}
	if manifest.MaxConcurrency == 0 {
		manifest.MaxConcurrency = 2
	}
	if manifest.MaxConcurrency < 1 || manifest.MaxConcurrency > 20 {
		return preparedImageEditBatch{}, fmt.Errorf("max_concurrency must be between 1 and 20")
	}
	if len(manifest.Jobs) == 0 || len(manifest.Jobs) > 50 {
		return preparedImageEditBatch{}, fmt.Errorf("jobs must contain between 1 and 50 entries")
	}
	absManifest, err := filepath.Abs(path)
	if err != nil {
		absManifest = path
	}
	baseDir := filepath.Dir(absManifest)
	resolvePath := func(value string) string {
		if value == "" || filepath.IsAbs(value) {
			return value
		}
		return filepath.Join(baseDir, value)
	}

	prepared := preparedImageEditBatch{MaxConcurrency: manifest.MaxConcurrency, Jobs: make([]preparedImageEditJob, 0, len(manifest.Jobs))}
	ids := make(map[string]struct{}, len(manifest.Jobs))
	outputs := make(map[string]string, len(manifest.Jobs))
	for _, job := range manifest.Jobs {
		job.ID = strings.TrimSpace(job.ID)
		if !imageEditBatchJobIDPattern.MatchString(job.ID) {
			return preparedImageEditBatch{}, fmt.Errorf("job id %q must match %s", job.ID, imageEditBatchJobIDPattern.String())
		}
		if _, exists := ids[job.ID]; exists {
			return preparedImageEditBatch{}, fmt.Errorf("duplicate job id %q", job.ID)
		}
		ids[job.ID] = struct{}{}
		if len(job.Inputs) == 0 || len(job.Inputs) > 16 {
			return preparedImageEditBatch{}, fmt.Errorf("job %q inputs must contain between 1 and 16 entries", job.ID)
		}
		if strings.TrimSpace(job.Prompt) != "" && strings.TrimSpace(job.PromptFile) != "" {
			return preparedImageEditBatch{}, fmt.Errorf("job %q must use prompt or prompt_file, not both", job.ID)
		}
		prompt := strings.TrimSpace(job.Prompt)
		if strings.TrimSpace(job.PromptFile) != "" {
			promptBytes, readErr := os.ReadFile(resolvePath(job.PromptFile))
			if readErr != nil {
				return preparedImageEditBatch{}, fmt.Errorf("read prompt for job %q: %w", job.ID, readErr)
			}
			prompt = strings.TrimSpace(string(promptBytes))
		}
		if prompt == "" {
			return preparedImageEditBatch{}, fmt.Errorf("job %q requires prompt or prompt_file", job.ID)
		}
		model := strings.TrimSpace(job.Model)
		if model == "" {
			model = "gpt-image-2"
		}
		if model != "gpt-image-2" {
			return preparedImageEditBatch{}, fmt.Errorf("job %q: only gpt-image-2 is supported", job.ID)
		}
		size := strings.TrimSpace(job.Size)
		if size == "" {
			size = "auto"
		}
		if err := validateGPTImageSize(size); err != nil {
			return preparedImageEditBatch{}, fmt.Errorf("job %q: %w", job.ID, err)
		}
		quality := strings.ToLower(strings.TrimSpace(job.Quality))
		if quality != "" && !map[string]bool{"low": true, "medium": true, "high": true, "auto": true}[quality] {
			return preparedImageEditBatch{}, fmt.Errorf("job %q: quality must be low, medium, high, or auto", job.ID)
		}
		maxAttempts := job.MaxAttempts
		if maxAttempts == 0 {
			maxAttempts = 3
		}
		if maxAttempts < 1 || maxAttempts > 5 {
			return preparedImageEditBatch{}, fmt.Errorf("job %q: max_attempts must be between 1 and 5", job.ID)
		}
		outputFile := resolvePath(strings.TrimSpace(job.OutputFile))
		if outputFile == "" {
			return preparedImageEditBatch{}, fmt.Errorf("job %q requires output_file", job.ID)
		}
		outputKey := strings.ToLower(filepath.Clean(outputFile))
		if previous, exists := outputs[outputKey]; exists {
			return preparedImageEditBatch{}, fmt.Errorf("jobs %q and %q use the same output_file", previous, job.ID)
		}
		outputs[outputKey] = job.ID

		inputs := make([]imageEditBatchInput, len(job.Inputs))
		dependencies := make([]string, 0, len(job.Inputs))
		for index, input := range job.Inputs {
			input.Path = strings.TrimSpace(input.Path)
			input.Job = strings.TrimSpace(input.Job)
			if (input.Path == "") == (input.Job == "") {
				return preparedImageEditBatch{}, fmt.Errorf("job %q input %d must set exactly one of path or job", job.ID, index+1)
			}
			if input.Path != "" {
				input.Path = resolvePath(input.Path)
				if _, statErr := os.Stat(input.Path); statErr != nil {
					return preparedImageEditBatch{}, fmt.Errorf("job %q input %d: %w", job.ID, index+1, statErr)
				}
			} else {
				dependencies = append(dependencies, input.Job)
			}
			inputs[index] = input
		}
		prepared.Jobs = append(prepared.Jobs, preparedImageEditJob{
			ID: job.ID, Inputs: inputs, Mask: resolvePath(strings.TrimSpace(job.Mask)), Prompt: prompt,
			Model: model, Size: size, Quality: quality, MaxAttempts: maxAttempts,
			OutputFile: outputFile, DependsOn: uniqueCLIStrings(dependencies),
		})
	}
	for _, job := range prepared.Jobs {
		for _, dependency := range job.DependsOn {
			if _, exists := ids[dependency]; !exists {
				return preparedImageEditBatch{}, fmt.Errorf("job %q references unknown job %q", job.ID, dependency)
			}
			if dependency == job.ID {
				return preparedImageEditBatch{}, fmt.Errorf("job %q cannot depend on itself", job.ID)
			}
		}
	}
	if err := validateImageEditBatchDAG(prepared.Jobs); err != nil {
		return preparedImageEditBatch{}, err
	}
	return prepared, nil
}

func validateImageEditBatchDAG(jobs []preparedImageEditJob) error {
	dependencies := make(map[string][]string, len(jobs))
	for _, job := range jobs {
		dependencies[job.ID] = job.DependsOn
	}
	state := make(map[string]int, len(jobs))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("image batch contains a dependency cycle at job %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range dependencies[id] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range dependencies {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func executeImageEditBatch(ctx context.Context, client *http.Client, endpoint, apiKey, imageField string, batch preparedImageEditBatch) imageEditBatchSummary {
	startedAt := time.Now()
	results := make([]imageEditBatchResult, len(batch.Jobs))
	indexes := make(map[string]int, len(batch.Jobs))
	done := make(map[string]chan struct{}, len(batch.Jobs))
	for index, job := range batch.Jobs {
		indexes[job.ID] = index
		done[job.ID] = make(chan struct{})
	}
	semaphore := make(chan struct{}, batch.MaxConcurrency)
	var wait sync.WaitGroup
	for index := range batch.Jobs {
		index := index
		job := batch.Jobs[index]
		wait.Add(1)
		go func() {
			defer wait.Done()
			defer close(done[job.ID])
			for _, dependency := range job.DependsOn {
				select {
				case <-ctx.Done():
					results[index] = imageEditBatchResult{ID: job.ID, Status: "skipped", Model: job.Model, Size: job.Size, Error: ctx.Err().Error()}
					return
				case <-done[dependency]:
				}
				dependencyResult := results[indexes[dependency]]
				if dependencyResult.Status != "succeeded" {
					results[index] = imageEditBatchResult{ID: job.ID, Status: "skipped", Model: job.Model, Size: job.Size, Error: fmt.Sprintf("dependency %s did not succeed", dependency)}
					return
				}
			}
			select {
			case <-ctx.Done():
				results[index] = imageEditBatchResult{ID: job.ID, Status: "skipped", Model: job.Model, Size: job.Size, Error: ctx.Err().Error()}
				return
			case semaphore <- struct{}{}:
			}
			defer func() { <-semaphore }()
			jobStartedAt := time.Now()
			inputs := make([]string, 0, len(job.Inputs))
			for _, input := range job.Inputs {
				if input.Path != "" {
					inputs = append(inputs, input.Path)
				} else {
					inputs = append(inputs, batch.Jobs[indexes[input.Job]].OutputFile)
				}
			}
			image, requestID, attempts, err := requestGPTImageEditWithRetryAndSlots(ctx, client, endpoint, apiKey, job.Model, imageField, inputs, job.Mask, job.Prompt, job.Size, job.Quality, job.MaxAttempts)
			duration := time.Since(jobStartedAt).Seconds()
			if err != nil {
				results[index] = imageEditBatchResult{ID: job.ID, Status: "failed", Model: job.Model, Size: job.Size, RequestID: requestID, Attempts: attempts, DurationSeconds: duration, Error: err.Error()}
				return
			}
			if directory := filepath.Dir(job.OutputFile); directory != "." {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					results[index] = imageEditBatchResult{ID: job.ID, Status: "failed", Model: job.Model, Size: job.Size, RequestID: requestID, Attempts: attempts, DurationSeconds: duration, Error: fmt.Sprintf("create output directory: %v", err)}
					return
				}
			}
			if err := os.WriteFile(job.OutputFile, image, 0o644); err != nil {
				results[index] = imageEditBatchResult{ID: job.ID, Status: "failed", Model: job.Model, Size: job.Size, RequestID: requestID, Attempts: attempts, DurationSeconds: duration, Error: fmt.Sprintf("write image output: %v", err)}
				return
			}
			abs, err := filepath.Abs(job.OutputFile)
			if err != nil {
				abs = job.OutputFile
			}
			results[index] = imageEditBatchResult{ID: job.ID, Status: "succeeded", Model: job.Model, Size: job.Size, Path: abs, Bytes: len(image), RequestID: requestID, Attempts: attempts, DurationSeconds: duration}
		}()
	}
	wait.Wait()
	sort.SliceStable(results, func(left, right int) bool { return indexes[results[left].ID] < indexes[results[right].ID] })
	summary := imageEditBatchSummary{MaxConcurrency: batch.MaxConcurrency, WallSeconds: time.Since(startedAt).Seconds(), Results: results}
	for _, result := range results {
		switch result.Status {
		case "succeeded":
			summary.Succeeded++
		case "failed":
			summary.Failed++
		case "skipped":
			summary.Skipped++
		}
	}
	return summary
}
