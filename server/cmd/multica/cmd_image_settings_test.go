package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/multica-ai/multica/server/pkg/imagemodel"
	"github.com/spf13/cobra"
)

func TestImageSettingsReadsFrozenFiles(t *testing.T) {
	for _, raw := range []string{`{"input_snapshot":{"image_generation":{"model":"gpt-image-2.5-sunburst","quality":"xhigh"}}}`, `{"context":{"image_generation":{"model":"gpt-image-2.5-sunburst","quality":"xhigh"}}}`} {
		if s, err := imageSettingsFromFile([]byte(raw)); err != nil || s != imagemodel.Default() {
			t.Fatalf("settings=%+v %v", s, err)
		}
	}
	if s, err := imageSettingsFromFile([]byte(`{"input_snapshot":{}}`)); err != nil || s.Model != imagemodel.Image2 {
		t.Fatalf("old order=%+v %v", s, err)
	}
}

func TestImage25InvocationAndAssetReceipt(t *testing.T) {
	for _, model := range []string{imagemodel.Sunburst, imagemodel.Flare} {
		t.Run(model, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name string, data []byte) string {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				return path
			}
			input := write("input.png", pngImageBytes(16, 16))
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					http.Error(w, "bad form", 400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				for field, want := range map[string]string{"model": model, "quality": "xhigh", "size": "1088x1088", "output_format": "png"} {
					if r.FormValue(field) != want {
						t.Errorf("%s=%q want %q", field, r.FormValue(field), want)
					}
				}
				w.Header().Set("x-request-id", "image25-request")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(pngImageBytes(1088, 1088))}}})
			}))
			defer provider.Close()
			t.Setenv("OPENAI_API_KEY", "test-key")
			t.Setenv("OPENAI_BASE_URL", provider.URL)
			t.Setenv("OPENAI_IMAGE_EDIT_PATH", "/images/edits")
			t.Setenv("MULTICA_IMAGE_SLOT_DIR", filepath.Join(dir, "slots"))
			t.Setenv("MULTICA_IMAGE_MAX_CONCURRENT", "1")
			cmd := &cobra.Command{}
			for key, value := range map[string]string{"model": model, "quality": "xhigh", "size": "1080x1080", "prompt": "Orange circle", "prompt-file": "", "mask": "", "output-file": filepath.Join(dir, "out.png"), "result-file": filepath.Join(dir, "receipt.json"), "operation-id": "", "output": "json"} {
				cmd.Flags().String(key, value, "")
			}
			cmd.Flags().StringSlice("input", []string{input}, "")
			cmd.Flags().Bool("prompt-stdin", false, "")
			cmd.Flags().Int("max-attempts", 1, "")
			cmd.Flags().Int("operation-attempt", 0, "")
			if err := runImageEdit(cmd, nil); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "receipt.json"))
			if err != nil {
				t.Fatal(err)
			}
			var settings imagemodel.Settings
			if err := json.Unmarshal(raw, &settings); err != nil || settings != (imagemodel.Settings{Model: model, Quality: "xhigh"}) {
				t.Fatalf("receipt=%s %v", raw, err)
			}
			asset := &cobra.Command{}
			for key, value := range map[string]string{
				"input-file":        write("asset.json", []byte(`{"variant_id":"variant","size_key":"1080x1080","revision":1,"stage":"generated","status":"completed","attachment_id":"attachment"}`)),
				"model-result-file": filepath.Join(dir, "receipt.json"), "model-result-id": "", "copy-validation-file": "",
				"prompt-contract-file":        write("contract.json", []byte(`{"prompt_sha256":"`+imagePromptSHA256("Orange circle")+`"}`)),
				"normalization-evidence-file": write("normalization.json", []byte(`{"target_size":{"width":1080,"height":1080}}`)),
			} {
				asset.Flags().String(key, value, "")
			}
			payload, err := creativeOrderAssetPayload(asset)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Metadata imagemodel.Settings `json:"metadata"`
			}
			if err := json.Unmarshal(payload, &result); err != nil || result.Metadata != settings {
				t.Fatalf("asset metadata=%s %v", payload, err)
			}
			manifest, _ := json.Marshal(imageEditBatchManifest{Jobs: []imageEditBatchJob{{ID: "square", Inputs: []imageEditBatchInput{{Path: input}}, Model: model, Quality: "xhigh", Prompt: "Orange circle", Size: "1080x1080", OutputFile: filepath.Join(dir, "batch.png")}}})
			batch, err := loadImageEditBatchManifest(write("batch.json", manifest))
			if err != nil {
				t.Fatal(err)
			}
			summary := executeImageEditBatch(t.Context(), provider.Client(), provider.URL, "test-key", "image", batch)
			if summary.Succeeded != 1 || summary.Results[0].Quality != "xhigh" || summary.Results[0].Model != model || summary.Results[0].ProviderSize != "1088x1088" {
				t.Fatalf("batch=%+v", summary)
			}
		})
	}
}
