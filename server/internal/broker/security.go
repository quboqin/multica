package broker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

var forbiddenParamKeyFragments = []string{
	"authorization",
	"bearer",
	"cookie",
	"localstorage",
	"password",
	"secret",
	"sessionstorage",
	"storagestate",
	"token",
}

func ValidateSafeJSON(raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	return validateSafeValue("$", v)
}

func validateSafeValue(path string, v any) error {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if forbiddenParamKey(k) {
				return fmt.Errorf("%w: %s", ErrUnsafeParams, path+"."+k)
			}
			if err := validateSafeValue(path+"."+k, child); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range x {
			if err := validateSafeValue(fmt.Sprintf("%s[%d]", path, i), child); err != nil {
				return err
			}
		}
	case string:
		lower := strings.ToLower(x)
		if strings.Contains(lower, "authorization:") ||
			strings.Contains(lower, "cookie:") ||
			strings.Contains(lower, "bearer ") {
			return fmt.Errorf("%w: %s", ErrUnsafeParams, path)
		}
	}
	return nil
}

func forbiddenParamKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(strings.ToLower(key))
	for _, fragment := range forbiddenParamKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
