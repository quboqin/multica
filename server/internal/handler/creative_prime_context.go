package handler

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Bind the final overlay to the context attachment used by this exact model
// operation. CLI-generated receipt metadata need not duplicate the guide data.
func bindCreativeGeneratedPrimeTemplate(snapshot json.RawMessage, size string, metadata json.RawMessage, contextRole string) (json.RawMessage, error) {
	var policy struct {
		Version int `json:"prime_context_policy_version"`
	}
	if err := json.Unmarshal(snapshot, &policy); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &fields); err != nil || fields == nil {
		return nil, errors.New("generated asset metadata must be an object")
	}
	var declaredRole string
	if raw, exists := fields["prime_template_source_role"]; exists {
		if err := json.Unmarshal(raw, &declaredRole); err != nil {
			return nil, errors.New("prime_template_source_role must be a string")
		}
	}
	if contextRole != "" && declaredRole != "" && contextRole != declaredRole {
		return nil, errors.New("generated asset Prime template differs from the model input context")
	}
	role := contextRole
	if role == "" {
		role = declaredRole
	}
	if policy.Version < 1 && role == "" {
		return metadata, nil // Preserve legacy receipt metadata byte-for-byte.
	}
	market, templates, files, err := frozenCreativePrimeMarketPack(snapshot)
	if err != nil {
		return nil, err
	}
	composition, err := parseFrozenPrimeCompositionConfig(market.Config, templates)
	if err != nil {
		return nil, err
	}
	if policy.Version >= 1 && composition.Mode == primeCompositionModeDeterministic && contextRole == "" {
		return nil, errors.New("Prime context template evidence is required: register the exact operation input attachment as Prime context with guide JSON metadata.prime_template_source_role; retry asset-put using the same model result")
	}
	if role == "" {
		return metadata, nil
	}
	approved := false
	for _, family := range templates.Families {
		if family.Templates[size].SourceRole == role {
			if file, ok := files[role]; ok && file.AttachmentID != "" {
				approved = true
			}
		}
	}
	if !approved {
		return nil, fmt.Errorf("Prime context source role %q is not approved for %s in the frozen market pack", role, size)
	}
	fields["prime_template_source_role"], _ = json.Marshal(role)
	return json.Marshal(fields)
}
