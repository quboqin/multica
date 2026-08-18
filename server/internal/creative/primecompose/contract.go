package primecompose

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const packageContractVersion = 6

// HydrateManifestContract replaces the mutable Prime contract fields with the
// published market-pack contract frozen in a Creative Order snapshot.
func HydrateManifestContract(manifestPath, orderSnapshotPath string) error {
	manifestPath = strings.TrimSpace(manifestPath)
	orderSnapshotPath = strings.TrimSpace(orderSnapshotPath)
	if manifestPath == "" {
		return fmt.Errorf("--manifest is required")
	}
	if orderSnapshotPath == "" {
		return fmt.Errorf("--order-snapshot is required")
	}
	manifest, err := readJSONObject(manifestPath, "Prime compose manifest")
	if err != nil {
		return err
	}
	snapshot, err := readJSONObject(orderSnapshotPath, "Creative Order snapshot")
	if err != nil {
		return err
	}
	if raw, ok := snapshot["input_snapshot"]; ok {
		if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot == nil {
			return fmt.Errorf("Creative Order snapshot input_snapshot must be an object")
		}
	}
	marketPack, err := nestedJSONObject(snapshot, "market_pack")
	if err != nil {
		return err
	}
	config, err := nestedJSONObject(marketPack, "config")
	if err != nil {
		return err
	}
	for _, field := range []string{"prime_template_set", "prime_template_set_validation", "prime_layout_contract"} {
		value, ok := config[field]
		if !ok || len(value) == 0 || string(value) == "null" {
			return fmt.Errorf("Creative Order snapshot market_pack.config.%s is required", field)
		}
		manifest[field] = value
	}
	manifest["package_contract_version"] = json.RawMessage(fmt.Sprintf("%d", packageContractVersion))
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Prime compose manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write Prime compose manifest: %w", err)
	}
	return nil
}

func readJSONObject(path, label string) (map[string]json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return object, nil
}

func nestedJSONObject(object map[string]json.RawMessage, field string) (map[string]json.RawMessage, error) {
	raw, ok := object[field]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("Creative Order snapshot %s is required", field)
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil || nested == nil {
		return nil, fmt.Errorf("Creative Order snapshot %s must be an object", field)
	}
	return nested, nil
}
