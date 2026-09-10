package daemon

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceRuntimeProviders(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     map[string]struct{}
	}{
		{name: "unset keeps all providers", settings: `{}`, want: nil},
		{name: "normalizes configured providers", settings: `{"runtime_providers":[" Codex ","codex"]}`, want: map[string]struct{}{"codex": {}}},
		{name: "empty list keeps all providers", settings: `{"runtime_providers":[]}`, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workspaceRuntimeProviders(json.RawMessage(tt.settings))
			if len(got) != len(tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
			for provider := range tt.want {
				if _, ok := got[provider]; !ok {
					t.Fatalf("provider %q missing from %#v", provider, got)
				}
			}
		})
	}
}
