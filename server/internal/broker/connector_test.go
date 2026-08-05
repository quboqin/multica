package broker

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

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

func TestDeploymentProfileUniqueViolationIsRecognizedPrecisely(t *testing.T) {
	if !isDeploymentProfileUniqueViolation(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "credential_profile_deployment_connector_idx",
	}) {
		t.Fatal("expected deployment credential unique violation to be recognized")
	}
	if isDeploymentProfileUniqueViolation(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "credential_profile_workspace_connector_idx",
	}) {
		t.Fatal("workspace-scoped unique violation must not be treated as a deployment race")
	}
	if isDeploymentProfileUniqueViolation(errors.New("duplicate key")) {
		t.Fatal("untyped error must not be treated as a deployment race")
	}
}
