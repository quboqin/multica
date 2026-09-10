package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func withComposioMCPAppsFlag(t *testing.T, h *Handler, enabled bool) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.ComposioMCPApps, featureflag.Rule{Default: enabled})
	previousHandlerFlags := h.FeatureFlags
	flags := featureflag.NewService(provider)
	h.FeatureFlags = flags
	var previousTaskFlags *featureflag.Service
	if h.TaskService != nil {
		previousTaskFlags = h.TaskService.FeatureFlags
		h.TaskService.FeatureFlags = flags
	}
	t.Cleanup(func() {
		h.FeatureFlags = previousHandlerFlags
		if h.TaskService != nil {
			h.TaskService.FeatureFlags = previousTaskFlags
		}
	})
}
