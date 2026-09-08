package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConfiguredImageConcurrency(t *testing.T) {
	for _, tt := range []struct {
		name, value string
		want        int
	}{
		{"default", "", 9},
		{"minimum", "1", 1},
		{"previous maximum", "20", 20},
		{"reported deployment", "30", 30},
		{"maximum", "50", 50},
		{"whitespace", " 50 ", 50},
		{"zero", "0", 0},
		{"negative", "-1", 0},
		{"above maximum", "51", 0},
		{"fraction", "1.5", 0},
		{"invalid", "many", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MULTICA_IMAGE_MAX_CONCURRENT", tt.value)
			got, err := configuredImageConcurrency()
			if tt.want == 0 {
				if err == nil || err.Error() != "MULTICA_IMAGE_MAX_CONCURRENT must be between 1 and 50" {
					t.Fatalf("invalid configuration returned %d, %v", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("configured concurrency = %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

func TestGlobalImageSlotsFiftyWaitsForReleasedCapacity(t *testing.T) {
	t.Setenv("MULTICA_IMAGE_SLOT_DIR", t.TempDir())
	t.Setenv("MULTICA_IMAGE_MAX_CONCURRENT", "50")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	releases := make([]func() error, 0, 50)
	defer func() {
		for _, release := range releases {
			if release != nil {
				if err := release(); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	for range 50 {
		release, err := acquireGlobalImageSlot(ctx, "https://provider.example/images", "test-key")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	waitCtx, stopWaiting := context.WithTimeout(ctx, 150*time.Millisecond)
	defer stopWaiting()
	release, err := acquireGlobalImageSlot(waitCtx, "https://provider.example/images", "test-key")
	if release != nil {
		_ = release()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("51st acquisition must wait: %v", err)
	}
	if err := releases[0](); err != nil {
		t.Fatal(err)
	}
	releases[0] = nil
	release, err = acquireGlobalImageSlot(ctx, "https://provider.example/images", "test-key")
	if err != nil {
		t.Fatal(err)
	}
	releases[0] = release
}
