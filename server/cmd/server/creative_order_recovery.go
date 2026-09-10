package main

import (
	"context"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/scheduler"
)

func creativeOrderRecoveryJob(h *handler.Handler) scheduler.JobSpec {
	return scheduler.JobSpec{
		Name: "creative_order_recovery", Cadence: time.Minute, CatchUpMode: scheduler.CatchUpLatestOnly,
		RunTimeout: 90 * time.Second, StaleTimeout: 2 * time.Minute, HeartbeatInterval: 15 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 3, RetryBackoff: []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute},
		Scopes: scheduler.StaticScopes(scheduler.ScopeGlobal),
		Handler: func(ctx context.Context, in scheduler.HandlerInput) (scheduler.HandlerResult, error) {
			count, err := h.RecoverCreativeOrders(ctx, 25)
			return scheduler.HandlerResult{RowsAffected: int64(count)}, err
		},
	}
}
