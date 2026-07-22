package main

import "testing"

func TestParseCreativeSizes(t *testing.T) {
	sizes, err := parseCreativeSizes([]string{"1080x1080", "800X1000"})
	if err != nil {
		t.Fatalf("parseCreativeSizes: %v", err)
	}
	if len(sizes) != 2 || sizes[1]["label"] != "800x1000" {
		t.Fatalf("sizes = %#v", sizes)
	}
}

func TestParseCreativeSizesRejectsUnsafeDimensions(t *testing.T) {
	if _, err := parseCreativeSizes([]string{"99999x1"}); err == nil {
		t.Fatal("expected invalid size to fail")
	}
}
