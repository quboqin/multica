package primecompose

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresManifest(t *testing.T) {
	if err := Run(context.Background(), "", &bytes.Buffer{}, &bytes.Buffer{}); err == nil || err.Error() != "--manifest is required" {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunRejectsMissingManifest(t *testing.T) {
	err := Run(context.Background(), filepath.Join(t.TempDir(), "missing.json"), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "read Prime compose manifest") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestHydrateManifestContractUsesFrozenOrderContract(t *testing.T) {
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "manifest.json")
	orderPath := filepath.Join(directory, "order.json")
	if err := os.WriteFile(manifestPath, []byte(`{"package_contract_version":3,"prime_template_set_validation":{"status":"draft"},"prime_layout_contract":{"layouts":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	order := []byte(`{
  "input_snapshot": {
    "market_pack": {
      "config": {
        "prime_template_set": {"schema_version":2,"selection_mode":"automatic_family_contrast","families":[{"id":"light_background","label":"Light","templates":{"10x10":{"source_role":"template"}}}]},
        "prime_template_set_validation": {"status":"passed","families":[{"id":"light_background","label":"Light","templates":{"10x10":{"source_role":"template"}}}]},
        "prime_layout_contract": {"layouts":{"10x10":{"hard_regions":[{"id":"template_01:header","kind":"template","x1":0,"y1":0,"x2":10,"y2":1}],"top_key_content_exclusion_end":1,"bottom_key_content_exclusion_start":9}}}
      }
    }
  }
}`)
	if err := os.WriteFile(orderPath, order, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := HydrateManifestContract(manifestPath, orderPath); err != nil {
		t.Fatal(err)
	}
	manifest, err := readJSONObject(manifestPath, "manifest")
	if err != nil {
		t.Fatal(err)
	}
	var validation struct {
		Status string `json:"status"`
	}
	var layout struct {
		Layouts map[string]struct {
			HardRegions []json.RawMessage `json:"hard_regions"`
		} `json:"layouts"`
	}
	if err := json.Unmarshal(manifest["prime_template_set_validation"], &validation); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifest["prime_layout_contract"], &layout); err != nil {
		t.Fatal(err)
	}
	if string(manifest["package_contract_version"]) != "6" || validation.Status != "passed" || len(layout.Layouts["10x10"].HardRegions) != 1 {
		t.Fatalf("hydrated manifest lost the frozen contract: %s", manifest)
	}
}

func TestRunCompositesAnIntactTemplate(t *testing.T) {
	directory := t.TempDir()
	writePNG(t, filepath.Join(directory, "body.png"), color.NRGBA{R: 12, G: 34, B: 56, A: 255}, false)
	writePNG(t, filepath.Join(directory, "template.png"), color.NRGBA{R: 255, G: 255, B: 255, A: 255}, true)
	manifest := map[string]any{
		"package_contract_version": 6, "variant_id": "variant-1", "variant_key": "V01", "revision": 1, "expected_sizes": []string{"10x10"},
		"prime_template_set":            map[string]any{"schema_version": 2, "selection_mode": "automatic_family_contrast", "families": []map[string]any{{"id": "light_background", "label": "Light", "templates": map[string]any{"10x10": map[string]any{"source_role": "template"}}}}},
		"prime_template_set_validation": map[string]any{"status": "passed", "families": []map[string]any{{"id": "light_background", "label": "Light", "templates": map[string]any{"10x10": map[string]any{"source_role": "template"}}}}},
		"prime_layout_contract":         map[string]any{"layouts": map[string]any{"10x10": map[string]any{"hard_regions": []map[string]any{}, "top_key_content_exclusion_end": 1, "bottom_key_content_exclusion_start": 9}}},
		"sources":                       map[string]string{"template": "template.png"},
		"jobs":                          []map[string]any{{"id": "V01-10x10", "variant_id": "variant-1", "variant_key": "V01", "size": "10x10", "revision": 1, "input": "body.png", "output": "final.png"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "final.png"), []byte("stale output"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), manifestPath, &stdout, &stderr)
	if err != nil || !strings.Contains(stdout.String(), "unchanged_full_canvas_alpha_composite") {
		t.Fatalf("compose must apply the uploaded full template without QR validation: err=%v output=%s", err, stdout.String())
	}
}

func TestRunFailsClosedWhenNoApprovedTemplateIsAdequate(t *testing.T) {
	directory := t.TempDir()
	writePNG(t, filepath.Join(directory, "body.png"), color.NRGBA{R: 250, G: 250, B: 250, A: 255}, false)
	writePNG(t, filepath.Join(directory, "template.png"), color.NRGBA{R: 255, G: 255, B: 255, A: 255}, true)
	manifest := map[string]any{
		"package_contract_version": 6, "variant_id": "variant-1", "variant_key": "V01", "revision": 1, "expected_sizes": []string{"10x10"},
		"prime_template_set":            map[string]any{"schema_version": 2, "selection_mode": "automatic_family_contrast", "families": []map[string]any{{"id": "light_glyph", "label": "Light glyph", "templates": map[string]any{"10x10": map[string]any{"source_role": "template"}}}}},
		"prime_template_set_validation": map[string]any{"status": "passed", "families": []map[string]any{{"id": "light_glyph", "label": "Light glyph", "templates": map[string]any{"10x10": map[string]any{"source_role": "template"}}}}},
		"prime_layout_contract":         map[string]any{"layouts": map[string]any{"10x10": map[string]any{"hard_regions": []map[string]any{}, "top_key_content_exclusion_end": 1, "bottom_key_content_exclusion_start": 9}}},
		"sources":                       map[string]string{"template": "template.png"},
		"jobs":                          []map[string]any{{"id": "V01-10x10", "size": "10x10", "input": "body.png", "output": "final.png"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), manifestPath, &stdout, &stderr); err == nil {
		t.Fatal("inadequate template must make the bundled composer fail")
	}
	var report struct {
		Succeeded int               `json:"succeeded"`
		Failed    int               `json:"failed"`
		Results   []json.RawMessage `json:"results"`
		Failures  []struct {
			Size      string `json:"size"`
			ErrorCode string `json:"error_code"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode failed compose report: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if report.Succeeded != 0 || report.Failed != 1 || len(report.Results) != 0 || len(report.Failures) != 1 || report.Failures[0].Size != "10x10" || report.Failures[0].ErrorCode != "prime_no_adequate_template_for_size" {
		t.Fatalf("unexpected fail-closed report: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(directory, "final.png")); !os.IsNotExist(err) {
		t.Fatal("fail-closed composition must not leave a final output")
	}
}

func writePNG(t *testing.T, path string, fill color.NRGBA, template bool) {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	if template {
		pixels.SetNRGBA(0, 0, fill)
		pixels.SetNRGBA(0, 9, fill)
	} else {
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				pixels.SetNRGBA(x, y, fill)
			}
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, pixels); err != nil {
		t.Fatal(err)
	}
}

func TestBundledComposerScriptIsExecutableSource(t *testing.T) {
	if !bytes.Contains(composerScript, []byte("def compose_manifest")) {
		t.Fatal("bundled Prime composer source is incomplete")
	}
	if len(engineSHA256) != 64 {
		t.Fatalf("engineSHA256 = %q", engineSHA256)
	}
}
