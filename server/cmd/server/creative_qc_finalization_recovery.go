package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
)

const creativeQCFinalizationRecoveryIdle = 15 * time.Second

func runCreativeQCFinalizationRecovery(ctx context.Context, h *handler.Handler) {
	ticker := time.NewTicker(creativeQCFinalizationRecoveryIdle)
	defer ticker.Stop()
	for {
		processed, err := h.RecoverPendingCreativeQCFinalizations(ctx, 1)
		if err != nil {
			slog.Warn("creative QC finalization recovery failed", "error", err)
		}
		if processed > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
