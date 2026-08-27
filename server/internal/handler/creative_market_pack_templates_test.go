package handler

import (
	"encoding/json"
	"image"
	"image/color"
	"testing"
)

func primeTemplateSetJSON() json.RawMessage {
	return json.RawMessage(`{
  "prime_template_set": {
    "schema_version": 2,
    "selection_mode": "automatic_family_contrast",
    "families": [
      {"id":"light_background","label":"Light","templates":{"1080x1080":{"source_role":"square_light"},"1200x628":{"source_role":"landscape_light"},"800x1000":{"source_role":"portrait_light"}}},
      {"id":"dark_background","label":"Dark","templates":{"1080x1080":{"source_role":"square_dark"},"1200x628":{"source_role":"landscape_dark"},"800x1000":{"source_role":"portrait_dark"}}}
    ]
  }
}`)
}

func TestParsePrimeTemplateSetRequiresEverySizeAndUniqueSourceFiles(t *testing.T) {
	templateSet, err := parsePrimeTemplateSetConfig(primeTemplateSetJSON())
	if err != nil {
		t.Fatalf("parse template set: %v", err)
	}
	if got := len(templateSet.Families); got != 2 {
		t.Fatalf("template families = %d, want 2", got)
	}

	missingSize := json.RawMessage(`{"prime_template_set":{"schema_version":2,"selection_mode":"automatic_family_contrast","families":[{"id":"green_full","label":"Green","templates":{"1080x1080":{"source_role":"square_green"}}}]}}`)
	if _, err := parsePrimeTemplateSetConfig(missingSize); err == nil {
		t.Fatal("incomplete family must be rejected")
	}

	duplicateRole := json.RawMessage(`{"prime_template_set":{"schema_version":2,"selection_mode":"automatic_family_contrast","families":[{"id":"green_full","label":"Green","templates":{"1080x1080":{"source_role":"shared"},"1200x628":{"source_role":"landscape_green"},"800x1000":{"source_role":"portrait_green"}}},{"id":"dark_full","label":"Dark","templates":{"1080x1080":{"source_role":"shared"},"1200x628":{"source_role":"landscape_dark"},"800x1000":{"source_role":"portrait_dark"}}}]}}`)
	if _, err := parsePrimeTemplateSetConfig(duplicateRole); err == nil {
		t.Fatal("reusing a source file across output sizes must be rejected")
	}
}

func TestNormalizeMarketPackPrimeTemplateSetUsesCompleteStandardFiles(t *testing.T) {
	files := []creativeResourceFileResponse{
		{Role: "prime_light_square"}, {Role: "prime_light_landscape"}, {Role: "prime_light_portrait"},
		{Role: "prime_dark_square"}, {Role: "prime_dark_landscape"}, {Role: "prime_dark_portrait"},
	}
	normalized, err := normalizeMarketPackPrimeTemplateSet(json.RawMessage(`{"origin":"creative_factory_seed"}`), files)
	if err != nil {
		t.Fatalf("normalize legacy market pack: %v", err)
	}
	templateSet, err := parsePrimeTemplateSetConfig(normalized)
	if err != nil {
		t.Fatalf("parse normalized market pack: %v", err)
	}
	if len(templateSet.Families) != 2 {
		t.Fatalf("template families = %d, want 2", len(templateSet.Families))
	}

	incomplete, err := normalizeMarketPackPrimeTemplateSet(json.RawMessage(`{"origin":"creative_factory_seed"}`), files[:5])
	if err != nil {
		t.Fatalf("normalize incomplete market pack: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(incomplete, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, exists := envelope["prime_template_set"]; exists {
		t.Fatal("incomplete standard files must not receive a publishable template set")
	}
}

func TestTemplateBandBoundsAndLayoutContractUseTransparentTemplateFootprints(t *testing.T) {
	template := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for x := 0; x < 100; x++ {
		template.SetNRGBA(x, 0, color.NRGBA{R: 1, A: 255})
		template.SetNRGBA(x, 12, color.NRGBA{R: 1, A: 255})
		template.SetNRGBA(x, 88, color.NRGBA{R: 1, A: 255})
	}
	headerEnd, footerStart := templateBandBounds(template, 1000)
	if headerEnd != 130 || footerStart != 880 {
		t.Fatalf("template bands = %d,%d; want 130,880", headerEnd, footerStart)
	}
	contract := compilePrimeTemplateLayoutContract([]primeTemplateFamilyValidation{
		{ID: "light_background", Templates: map[string]primeTemplateValidation{
			"1080x1080": {HeaderEnd: 100, FooterStart: 980}, "1200x628": {HeaderEnd: 80, FooterStart: 580}, "800x1000": {HeaderEnd: headerEnd, FooterStart: footerStart},
		}},
	})
	layouts := contract["layouts"].(map[string]map[string]any)
	portrait := layouts["800x1000"]
	if portrait["top_key_content_exclusion_end"] != 130 || portrait["bottom_key_content_exclusion_start"] != 880 {
		t.Fatalf("unexpected portrait layout: %#v", portrait)
	}
	regions := portrait["hard_regions"].([]map[string]any)
	if len(regions) != 2 || regions[0]["kind"] != "template" || regions[1]["y1"] != 880 {
		t.Fatalf("unexpected template hard regions: %#v", regions)
	}
}

func TestPrimeTemplateMatchesItsTargetCanvasWithoutCropping(t *testing.T) {
	if !primeTemplateMatchesCanvas(image.NewNRGBA(image.Rect(0, 0, 5000, 2617)), primeCanvasSizes["1200x628"]) {
		t.Fatal("rounded high-resolution landscape template must preserve the target canvas ratio")
	}
	if primeTemplateMatchesCanvas(image.NewNRGBA(image.Rect(0, 0, 1080, 1920)), primeCanvasSizes["1200x628"]) {
		t.Fatal("portrait template must not be accepted for a landscape canvas")
	}
}

func TestPrimeTemplateForegroundEvidenceRecordsActualGlyphPolarity(t *testing.T) {
	darkGlyphs := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	lightGlyphs := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	for x := 0; x < 14; x++ {
		// This saturated brand-like green is above the midpoint in gamma-coded
		// luma but remains a dark glyph under relative luminance.
		darkGlyphs.SetNRGBA(x, 1, color.NRGBA{R: 0, G: 170, B: 100, A: 255})
		lightGlyphs.SetNRGBA(x, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	// Opposite-polarity QR-like pixels must not change the dominant component polarity.
	for x := 16; x < 20; x++ {
		darkGlyphs.SetNRGBA(x, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		lightGlyphs.SetNRGBA(x, 1, color.NRGBA{A: 255})
	}

	polarity, support, visible := primeTemplateForegroundEvidence(darkGlyphs)
	if polarity != "dark" || support != "light_low_texture" || visible != 14 {
		t.Fatalf("dark glyph evidence = %q,%q,%d", polarity, support, visible)
	}
	polarity, support, visible = primeTemplateForegroundEvidence(lightGlyphs)
	if polarity != "light" || support != "dark_low_texture" || visible != 14 {
		t.Fatalf("light glyph evidence = %q,%q,%d", polarity, support, visible)
	}
}

func TestPrimeTemplateForegroundEvidenceRejectsTransparentTemplate(t *testing.T) {
	polarity, support, visible := primeTemplateForegroundEvidence(image.NewNRGBA(image.Rect(0, 0, 10, 10)))
	if polarity != "unknown" || support != "unknown" || visible != 0 {
		t.Fatalf("transparent template evidence = %q,%q,%d", polarity, support, visible)
	}
}

func TestLegacyPrimeQRConfigurationIsRemovedWhenPublishing(t *testing.T) {
	config := map[string]any{
		"qr_payload":           "https://example.com/old",
		"qr_canonical_payload": "https://example.com/old",
		"qr_allowed_domains":   []any{"example.com"},
		"qr_approval_status":   "approved",
		"qr_approval_note":     "legacy",
		"market":               "Indonesia",
	}
	stripLegacyPrimeQRConfigFields(config)
	if _, exists := config["qr_payload"]; exists {
		t.Fatal("legacy QR configuration must not remain in the published config")
	}
	if config["market"] != "Indonesia" {
		t.Fatal("unrelated market configuration must be preserved")
	}
}
