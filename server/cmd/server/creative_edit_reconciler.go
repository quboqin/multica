package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
)

const (
	defaultCreativeEditReconcileInterval = 2 * time.Second
	defaultCreativeEditReconcileBatch    = 20
)

func runCreativeEditReconciler(ctx context.Context, h *handler.Handler) {
	interval := envDuration("MULTICA_CREATIVE_RECONCILE_INTERVAL", defaultCreativeEditReconcileInterval)
	batch := defaultCreativeEditReconcileBatch
	if raw := strings.TrimSpace(os.Getenv("MULTICA_CREATIVE_RECONCILE_BATCH")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			batch = parsed
		} else {
			slog.Warn("invalid MULTICA_CREATIVE_RECONCILE_BATCH, using default", "value", raw, "default", batch)
		}
	}

	tick := func() {
		count, err := h.ReconcileDueCreativeEditJobs(ctx, batch)
		if err != nil && ctx.Err() == nil {
			slog.Warn("creative edit reconciler tick failed", "claimed", count, "error", err)
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
