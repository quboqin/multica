package imagemodel

import (
	"encoding/json"
	"testing"
)

func TestModelQualityContract(t *testing.T) {
	for _, model := range []string{Image2, Sunburst, Flare, "unknown"} {
		for _, quality := range []string{"low", "medium", "high", "auto", "xhigh", "max", "invalid"} {
			want := Supported(model) && quality != "invalid" && (model != Image2 || (quality != "xhigh" && quality != "max"))
			if got := (Settings{model, quality}).Validate() == nil; got != want {
				t.Fatalf("%s/%s accepted=%v", model, quality, got)
			}
		}
	}
	if s, err := Resolve("", ""); err != nil || s != (Settings{Image2, "high"}) {
		t.Fatalf("default=%+v %v", s, err)
	}
	if s, err := Resolve(Image2, ""); err != nil || s.Quality != "high" {
		t.Fatalf("Image 2=%+v %v", s, err)
	}
}

func TestFrozenImageSettings(t *testing.T) {
	old := json.RawMessage(`{"target_variant_count":6}`)
	if s, err := FromSnapshot(old); err != nil || s != (Settings{Image2, "high"}) {
		t.Fatalf("old order=%+v %v", s, err)
	}
	current, err := Freeze(old)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := FromSnapshot(current); err != nil || s != Default() {
		t.Fatalf("new order=%+v %v", s, err)
	}
	if err := MatchReceipt(current, Sunburst, "high"); err == nil {
		t.Fatal("changed quality accepted")
	}
	if err := MatchReceipt(current, Flare, "xhigh"); err == nil {
		t.Fatal("changed model accepted")
	}
	if err := MatchReceipt(current, Sunburst, "xhigh"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceipt(Sunburst, ""); err == nil {
		t.Fatal("2.5 receipt without quality accepted")
	}
	if err := ValidateReceipt(Image2, ""); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`null`, `[]`, `{"image_generation":null}`, `{"image_generation":{"model":"gpt-image-2","quality":"xhigh"}}`} {
		if _, err := FromSnapshot([]byte(raw)); err == nil {
			t.Fatalf("accepted malformed snapshot %s", raw)
		}
	}
}
