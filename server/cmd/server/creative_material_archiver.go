package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
)

const defaultCreativeMaterialArchiveInterval = 3 * time.Second

func runCreativeMaterialArchiver(ctx context.Context, h *handler.Handler) {
	interval := envDuration("MULTICA_CREATIVE_ARCHIVE_INTERVAL", defaultCreativeMaterialArchiveInterval)
	batch := envPositiveInt("MULTICA_CREATIVE_ARCHIVE_BATCH", 5)
	if batch > 100 {
		batch = 100
	}
	tick := func() {
		count, err := h.ArchiveDueCreativeMaterials(ctx, batch)
		if err != nil && ctx.Err() == nil {
			slog.Warn("creative material archiver tick failed", "claimed", count, "error", err)
		}
	}
	tick()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
