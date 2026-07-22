package broker

import "testing"

func TestRegistryWithJSONAddsDeclarativeConnector(t *testing.T) {
	registry, err := RegistryWithJSON(`[
  {
    "id": "example-ads",
    "display_name": "Example Ads",
    "login_url": "https://ads.example.com/login",
    "capabilities": ["profile_verify", "page_extract"]
  }
]`)
	if err != nil {
		t.Fatalf("RegistryWithJSON: %v", err)
	}
	connector, ok := registry.Get("example-ads")
	if !ok || connector.DisplayName != "Example Ads" {
		t.Fatalf("connector = %+v, ok = %v", connector, ok)
	}
	if _, ok := registry.Get(ConnectorAppGrowing); !ok {
		t.Fatal("built-in AppGrowing connector was removed")
	}
}

func TestRegistryWithJSONRejectsBuiltInOverride(t *testing.T) {
	_, err := RegistryWithJSON(`[{"id":"appgrowing","login_url":"https://example.com","capabilities":["profile_verify"]}]`)
	if err == nil {
		t.Fatal("expected built-in override to fail")
	}
}
