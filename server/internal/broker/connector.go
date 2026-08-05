package broker

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	ConnectorAppGrowing = "appgrowing"
	ScopeWorkspace      = "workspace"
	ScopeDeployment     = "deployment"

	StatusPending    = "pending"
	StatusActive     = "active"
	StatusNeedReauth = "need_reauth"
	StatusRevoked    = "revoked"
)

type Connector struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	LoginURL     string   `json:"login_url"`
	Capabilities []string `json:"capabilities"`
	Scope        string   `json:"scope"`
}

type Registry struct {
	connectors map[string]Connector
}

var connectorIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func DefaultRegistry() Registry {
	return Registry{connectors: map[string]Connector{
		ConnectorAppGrowing: {
			ID:          ConnectorAppGrowing,
			DisplayName: "AppGrowing",
			LoginURL:    "https://auth.youcloud.com/login?app_id=en_appgrowing&goto=https%3A%2F%2Fappgrowing-global.youcloud.com%2Fleaflet",
			Scope:       ScopeDeployment,
			Capabilities: []string{
				"profile_verify",
				"material_search",
				"material_download",
			},
		},
	}}
}

// RegistryWithJSON adds deployment-owned declarative connectors to the built-in
// registry. The JSON may contain worker-only fields; encoding/json ignores them
// here while the crawler worker consumes the same document in full.
func RegistryWithJSON(raw string) (Registry, error) {
	registry := DefaultRegistry()
	if strings.TrimSpace(raw) == "" {
		return registry, nil
	}
	var connectors []Connector
	if err := json.Unmarshal([]byte(raw), &connectors); err != nil {
		return Registry{}, fmt.Errorf("parse MULTICA_CREDENTIAL_CONNECTORS_JSON: %w", err)
	}
	for _, connector := range connectors {
		connector.ID = strings.ToLower(strings.TrimSpace(connector.ID))
		connector.DisplayName = strings.TrimSpace(connector.DisplayName)
		connector.LoginURL = strings.TrimSpace(connector.LoginURL)
		connector.Capabilities = uniqueConnectorStrings(connector.Capabilities)
		connector.Scope = strings.ToLower(strings.TrimSpace(connector.Scope))
		if connector.Scope == "" {
			connector.Scope = ScopeWorkspace
		}
		if !connectorIDPattern.MatchString(connector.ID) {
			return Registry{}, fmt.Errorf("invalid connector id %q", connector.ID)
		}
		if _, exists := registry.connectors[connector.ID]; exists {
			return Registry{}, fmt.Errorf("connector id %q is already registered", connector.ID)
		}
		parsed, err := url.Parse(connector.LoginURL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" {
			return Registry{}, fmt.Errorf("connector %q has invalid login_url", connector.ID)
		}
		if connector.DisplayName == "" {
			connector.DisplayName = connector.ID
		}
		if len(connector.Capabilities) == 0 {
			return Registry{}, fmt.Errorf("connector %q must declare capabilities", connector.ID)
		}
		if connector.Scope != ScopeWorkspace && connector.Scope != ScopeDeployment {
			return Registry{}, fmt.Errorf("connector %q has invalid scope %q", connector.ID, connector.Scope)
		}
		registry.connectors[connector.ID] = connector
	}
	return registry, nil
}

func (r Registry) Get(id string) (Connector, bool) {
	c, ok := r.connectors[id]
	return c, ok
}

func (r Registry) List() []Connector {
	out := make([]Connector, 0, len(r.connectors))
	for _, c := range r.connectors {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func uniqueConnectorStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
