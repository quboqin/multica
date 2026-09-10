package main

import (
	"context"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/scheduler"
)

func creativeReferenceAnalysisRecoveryJob(h *handler.Handler) scheduler.JobSpec {
	return scheduler.JobSpec{
		Name: "creative_reference_analysis_recovery", Cadence: time.Minute,
		CatchUpMode: scheduler.CatchUpLatestOnly, RunTimeout: 45 * time.Second,
		StaleTimeout: 2 * time.Minute, HeartbeatInterval: 15 * time.Second,
		MaxAttempts: 3, RetryBackoff: []time.Duration{time.Minute, 2 * time.Minute},
		Scopes: scheduler.StaticScopes(scheduler.ScopeGlobal),
		Handler: func(ctx context.Context, in scheduler.HandlerInput) (scheduler.HandlerResult, error) {
			count, err := h.RecoverCreativeReferenceAnalyses(ctx, 25)
			return scheduler.HandlerResult{RowsAffected: int64(count)}, err
		},
	}
}
