package daemon

import (
	"encoding/json"
	"strings"
)

func workspaceRuntimeProviders(settings json.RawMessage) map[string]struct{} {
	if len(settings) == 0 {
		return nil
	}
	var config struct {
		RuntimeProviders []string `json:"runtime_providers"`
	}
	if json.Unmarshal(settings, &config) != nil || len(config.RuntimeProviders) == 0 {
		return nil
	}
	providers := make(map[string]struct{}, len(config.RuntimeProviders))
	for _, provider := range config.RuntimeProviders {
		if normalized := strings.ToLower(strings.TrimSpace(provider)); normalized != "" {
			providers[normalized] = struct{}{}
		}
	}
	if len(providers) == 0 {
		return nil
	}
	return providers
}
