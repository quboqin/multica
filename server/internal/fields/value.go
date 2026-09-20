package fields

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Definition struct {
	Type   string
	Config []byte
}

const (
	maxPropertySelectOptions = 50
	maxPropertyTextValueLen  = 2000
	maxPropertyURLValueLen   = 2048
	maxPropertyActorValues   = 20
)

func propertyTypeHasOptions(t string) bool { return t == "select" || t == "multi_select" }

type PropertyOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type PropertyConfig struct {
	Options []PropertyOption `json:"options,omitempty"`
	// Relation is set only on collection relation fields. Issue properties
	// never carry it: their type whitelist has no relation.
	Relation *RelationConfig `json:"relation,omitempty"`
}

// TypeRelation is the collection-only field type whose values are edges in
// record_link rather than entries in a record's value bag.
const TypeRelation = "relation"

// Relation targets, stored as record_link.to_type.
const (
	RelationToIssue  = "issue"
	RelationToRecord = "record"
)

// RelationConfig names what a relation field points at: workspace tasks, or
// the records of one collection.
type RelationConfig struct {
	ToType       string `json:"to_type"`
	CollectionID string `json:"collection_id,omitempty"`
}

// ParseRelationConfig reads a stored relation field's target.
func ParseRelationConfig(raw []byte) (RelationConfig, bool) {
	cfg := parsePropertyConfig(raw)
	if cfg.Relation == nil {
		return RelationConfig{}, false
	}
	return *cfg.Relation, true
}

// ValidateRelationConfig canonicalizes a new relation field's target. Whether
// the named collection exists is the caller's check; this one is syntax only.
func ValidateRelationConfig(cfg *PropertyConfig) (RelationConfig, error) {
	if cfg == nil || cfg.Relation == nil {
		return RelationConfig{}, errors.New("relation fields require a target: config.relation.to_type must be \"issue\" or \"record\"")
	}
	if len(cfg.Options) > 0 {
		return RelationConfig{}, fmt.Errorf("type %q does not accept options", TypeRelation)
	}
	switch cfg.Relation.ToType {
	case RelationToIssue:
		if strings.TrimSpace(cfg.Relation.CollectionID) != "" {
			return RelationConfig{}, errors.New("a relation to tasks does not take a collection_id")
		}
		return RelationConfig{ToType: RelationToIssue}, nil
	case RelationToRecord:
		id, err := uuid.Parse(strings.TrimSpace(cfg.Relation.CollectionID))
		if err != nil {
			return RelationConfig{}, errors.New("a relation to records requires config.relation.collection_id")
		}
		return RelationConfig{ToType: RelationToRecord, CollectionID: id.String()}, nil
	}
	return RelationConfig{}, fmt.Errorf("unknown relation target %q; valid targets: %s, %s", cfg.Relation.ToType, RelationToIssue, RelationToRecord)
}

func parsePropertyConfig(raw []byte) PropertyConfig {
	var cfg PropertyConfig
	if len(raw) == 0 {
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return PropertyConfig{}
	}
	return cfg
}

func ValidateConfig(propType string, cfg *PropertyConfig, validateLabelName func(string) (string, error), normalizeColor func(string) (string, error)) ([]byte, error) {
	if cfg != nil && cfg.Relation != nil {
		return nil, fmt.Errorf("type %q does not accept a relation target", propType)
	}
	if !propertyTypeHasOptions(propType) {
		if cfg != nil && len(cfg.Options) > 0 {
			return nil, fmt.Errorf("type %q does not accept options", propType)
		}
		return []byte(`{}`), nil
	}
	if cfg == nil || len(cfg.Options) == 0 {
		return nil, errors.New("select properties require at least one option")
	}
	if len(cfg.Options) > maxPropertySelectOptions {
		return nil, fmt.Errorf("a property cannot have more than %d options", maxPropertySelectOptions)
	}
	seenIDs := make(map[string]struct{}, len(cfg.Options))
	seenNames := make(map[string]struct{}, len(cfg.Options))
	out := PropertyConfig{Options: make([]PropertyOption, 0, len(cfg.Options))}
	for _, opt := range cfg.Options {
		name, err := validateLabelName(opt.Name)
		if err != nil {
			return nil, fmt.Errorf("option %w", err)
		}
		lower := strings.ToLower(name)
		if _, dup := seenNames[lower]; dup {
			return nil, fmt.Errorf("duplicate option name %q", name)
		}
		seenNames[lower] = struct{}{}
		color, err := normalizeColor(opt.Color)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", name, err)
		}
		id := strings.TrimSpace(opt.ID)
		if id == "" {
			id = uuid.NewString()
		} else if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("option %q: id must be a UUID", name)
		}
		if _, dup := seenIDs[id]; dup {
			return nil, fmt.Errorf("duplicate option id %q", id)
		}
		seenIDs[id] = struct{}{}
		out.Options = append(out.Options, PropertyOption{ID: id, Name: name, Color: color})
	}
	return json.Marshal(out)
}

func propertyOptionIDs(cfg PropertyConfig) map[string]int {
	ids := make(map[string]int, len(cfg.Options))
	for i, opt := range cfg.Options {
		ids[opt.ID] = i
	}
	return ids
}

func OptionsHint(cfg PropertyConfig) string {
	parts := make([]string, len(cfg.Options))
	for i, opt := range cfg.Options {
		parts[i] = fmt.Sprintf("%s (%s)", opt.ID, opt.Name)
	}
	return strings.Join(parts, ", ")
}

var actorPropertyKinds = []string{"member"}

// ActorRef is a parsed "<kind>:<uuid>" property value.
type ActorRef struct {
	Kind string
	ID   string
}

func (a ActorRef) String() string { return a.Kind + ":" + a.ID }

func propertyTypeIsActor(t string) bool {
	return t == "actor" || t == "multi_actor"
}

func actorKindsHint() string {
	return strings.Join(actorPropertyKinds, " / ")
}

// ParseActorRef splits a stored actor value. Members are referenced by
// user_id — the same id the assignee pair uses — so "who is this" resolves
// identically everywhere in the product.
func ParseActorRef(s string) (ActorRef, error) {
	kind, id, found := strings.Cut(s, ":")
	if !found {
		return ActorRef{}, fmt.Errorf("value must look like \"<kind>:<uuid>\" where kind is one of: %s", actorKindsHint())
	}
	valid := false
	for _, k := range actorPropertyKinds {
		if kind == k {
			valid = true
			break
		}
	}
	if !valid {
		return ActorRef{}, fmt.Errorf("unknown actor kind %q; valid kinds: %s", kind, actorKindsHint())
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return ActorRef{}, fmt.Errorf("actor id in %q must be a UUID", s)
	}
	// Store the canonical lowercase-hyphenated form. uuid.Parse also accepts
	// uppercase, braces and the urn: prefix; every consumer downstream (the
	// member directory lookup in the client, the "= me" filter, @> containment)
	// compares reference strings exactly, so an unnormalized id would store
	// fine and then render as Unknown and never match a filter.
	return ActorRef{Kind: kind, ID: parsed.String()}, nil
}

// ParseActorRefList validates a multi_actor array: every element must parse,
// duplicates are dropped, and the caller's order is preserved. Unlike
// multi_select there is no config order to canonicalize against, and sorting
// by id would make the avatar row reshuffle on every edit. @> containment is
// order-insensitive, so filtering is unaffected either way.
func ParseActorRefList(items []any) ([]ActorRef, error) {
	if len(items) == 0 {
		return nil, errors.New("value must be a non-empty array of actor references")
	}
	if len(items) > maxPropertyActorValues {
		return nil, fmt.Errorf("value cannot list more than %d actors", maxPropertyActorValues)
	}
	seen := make(map[string]struct{}, len(items))
	refs := make([]ActorRef, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, errors.New("value must be an array of actor reference strings")
		}
		ref, err := ParseActorRef(s)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[ref.String()]; dup {
			continue
		}
		seen[ref.String()] = struct{}{}
		refs = append(refs, ref)
	}
	return refs, nil
}

func ValidateValue(def Definition, raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("value is required")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("value must be valid JSON: %w", err)
	}
	if v == nil {
		return nil, errors.New("value cannot be null (use DELETE to unset a property)")
	}

	cfg := parsePropertyConfig(def.Config)
	switch def.Type {
	case "text":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("value must be a string")
		}
		if strings.TrimSpace(s) == "" {
			return nil, errors.New("value cannot be empty (use DELETE to unset a property)")
		}
		if utf8.RuneCountInString(s) > maxPropertyTextValueLen {
			return nil, fmt.Errorf("value must be %d characters or fewer", maxPropertyTextValueLen)
		}
		return json.Marshal(strings.ReplaceAll(s, "\x00", ""))
	case "url":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("value must be a URL string")
		}
		s = strings.TrimSpace(s)
		if len(s) > maxPropertyURLValueLen {
			return nil, fmt.Errorf("value must be %d characters or fewer", maxPropertyURLValueLen)
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("value must be an http(s) URL")
		}
		return json.Marshal(s)
	case "number":
		if _, ok := v.(float64); !ok {
			return nil, errors.New("value must be a number")
		}
		return json.Marshal(v)
	case "checkbox":
		if _, ok := v.(bool); !ok {
			return nil, errors.New("value must be true or false")
		}
		return json.Marshal(v)
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("value must be a date string in YYYY-MM-DD format")
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, errors.New("value must be a date string in YYYY-MM-DD format")
		}
		return json.Marshal(s)
	case "select":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("value must be one of the option ids: %s", OptionsHint(cfg))
		}
		if _, exists := propertyOptionIDs(cfg)[s]; !exists {
			return nil, fmt.Errorf("value must be one of the option ids: %s", OptionsHint(cfg))
		}
		return json.Marshal(s)
	case "multi_select":
		items, ok := v.([]any)
		if !ok || len(items) == 0 {
			return nil, fmt.Errorf("value must be a non-empty array of option ids: %s", OptionsHint(cfg))
		}
		order := propertyOptionIDs(cfg)
		seen := make(map[string]struct{}, len(items))
		ids := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("value must be a non-empty array of option ids: %s", OptionsHint(cfg))
			}
			if _, exists := order[s]; !exists {
				return nil, fmt.Errorf("unknown option id %q; valid option ids: %s", s, OptionsHint(cfg))
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			ids = append(ids, s)
		}
		// Canonicalize to config order so equal selections serialize equally
		// (stable @> containment filtering and change detection).
		sort.SliceStable(ids, func(a, b int) bool { return order[ids[a]] < order[ids[b]] })
		return json.Marshal(ids)
	case "actor":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("value must be an actor reference string like \"member:<uuid>\" (kinds: %s)", actorKindsHint())
		}
		ref, err := ParseActorRef(s)
		if err != nil {
			return nil, err
		}
		return json.Marshal(ref.String())
	case "multi_actor":
		items, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("value must be an array of actor reference strings like \"member:<uuid>\" (kinds: %s)", actorKindsHint())
		}
		refs, err := ParseActorRefList(items)
		if err != nil {
			return nil, err
		}
		out := make([]string, len(refs))
		for i, ref := range refs {
			out[i] = ref.String()
		}
		return json.Marshal(out)
	case TypeRelation:
		// A relation has no entry in the value bag; its edges have their own
		// write path and their own table.
		return nil, errors.New("a relation field is edited through record links, not as a value")
	default:
		return nil, fmt.Errorf("unsupported property type %q", def.Type)
	}
}
