package imagemodel

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	Image2      = "gpt-image-2"
	Sunburst    = "gpt-image-2.5-sunburst"
	Flare       = "gpt-image-2.5-flare"
	SnapshotKey = "image_generation"
)

type Settings struct {
	Model   string `json:"model"`
	Quality string `json:"quality"`
}

func Default() Settings { return Settings{Model: Sunburst, Quality: "xhigh"} }

func Supported(model string) bool {
	return model == Image2 || model == Sunburst || model == Flare
}

func (s Settings) Validate() error {
	if !Supported(s.Model) {
		return fmt.Errorf("unsupported image model %q", s.Model)
	}
	switch s.Quality {
	case "low", "medium", "high", "auto":
		return nil
	case "xhigh", "max":
		if s.Model != Image2 {
			return nil
		}
	}
	return fmt.Errorf("image quality %q is not supported by %s", s.Quality, s.Model)
}

func Resolve(model, quality string) (Settings, error) {
	s := Settings{Model: strings.TrimSpace(model), Quality: strings.ToLower(strings.TrimSpace(quality))}
	if s.Model == "" {
		// Existing command lines omit --model. Platform tasks pass frozen settings explicitly.
		s.Model = Image2
	}
	if s.Quality == "" {
		s.Quality = Default().Quality
		if s.Model == Image2 {
			s.Quality = "high"
		}
	}
	return s, s.Validate()
}

// Orders created before image settings were frozen used Image 2 with high quality.
func FromSnapshot(raw json.RawMessage) (Settings, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return Settings{}, fmt.Errorf("image settings snapshot must be an object")
	}
	value, exists := envelope[SnapshotKey]
	if !exists {
		return Settings{Model: Image2, Quality: "high"}, nil
	}
	var s Settings
	if err := json.Unmarshal(value, &s); err != nil {
		return Settings{}, fmt.Errorf("invalid image generation settings: %w", err)
	}
	return s, s.Validate()
}

func Freeze(raw json.RawMessage) (json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return nil, fmt.Errorf("image settings snapshot must be an object")
	}
	envelope[SnapshotKey], _ = json.Marshal(Default())
	return json.Marshal(envelope)
}

func MarshalTask(snapshot json.RawMessage, fields map[string]any) ([]byte, error) {
	s, err := FromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	fields[SnapshotKey] = s
	return json.Marshal(fields)
}

// Older CLI receipts omitted quality; their model identity remains verifiable.
func ValidateReceipt(model, quality string) error {
	if model == Image2 && quality == "" {
		return nil
	}
	return (Settings{Model: model, Quality: quality}).Validate()
}

func MatchReceipt(snapshot json.RawMessage, model, quality string) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(snapshot, &envelope); err != nil {
		return err
	}
	if _, frozen := envelope[SnapshotKey]; !frozen {
		return nil
	}
	s, err := FromSnapshot(snapshot)
	if err != nil {
		return err
	}
	if s.Model != model || s.Quality != quality {
		return fmt.Errorf("image receipt model and quality do not match frozen settings")
	}
	return nil
}
