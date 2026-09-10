package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
)

const (
	creativePrimeRecoveryWorkers = 2
	creativePrimeRecoveryIdle    = 15 * time.Second
)

func runCreativePrimeRecovery(ctx context.Context, h *handler.Handler) {
	var workers sync.WaitGroup
	workers.Add(creativePrimeRecoveryWorkers)
	for lane := 0; lane < creativePrimeRecoveryWorkers; lane++ {
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(creativePrimeRecoveryIdle)
			defer ticker.Stop()
			for {
				processed, err := h.RecoverPendingCreativePrimeCompositions(ctx, 1)
				if err != nil {
					slog.Warn("creative Prime recovery failed", "error", err)
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
		}()
	}
	workers.Wait()
}
