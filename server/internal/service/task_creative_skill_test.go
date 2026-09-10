package service

import (
	"encoding/json"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCreativeTaskSkillCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     map[string]struct{}
	}{
		{
			name:     "production",
			workflow: "creative_production",
			want:     map[string]struct{}{"image_edit": {}, "prime_compose": {}},
		},
		{
			name:     "direct edit",
			workflow: "creative_direct_edit",
			want:     map[string]struct{}{"image_edit": {}, "direct_image_edit": {}, "prime_compose": {}},
		},
		{
			name:     "candidate selection",
			workflow: "creative_candidate_selection",
			want:     map[string]struct{}{"quality_control": {}},
		},
		{
			name:     "non creative task",
			workflow: "chat",
			want:     nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, err := json.Marshal(map[string]string{
				"type":     "creative_domain_task",
				"workflow": test.workflow,
			})
			if err != nil {
				t.Fatalf("marshal task context: %v", err)
			}
			got := creativeTaskSkillCapabilities(context)
			if len(got) != len(test.want) {
				t.Fatalf("capabilities = %#v, want %#v", got, test.want)
			}
			for capability := range test.want {
				if _, ok := got[capability]; !ok {
					t.Errorf("missing capability %q in %#v", capability, got)
				}
			}
		})
	}
}

func TestCreativeTaskSkillAllowedKeepsNonCreativeSkills(t *testing.T) {
	allowed := map[string]struct{}{"image_edit": {}}

	nonCreative := db.Skill{Config: []byte(`{"kind":"utility"}`)}
	if !creativeTaskSkillAllowed(nonCreative, allowed) {
		t.Fatal("non-creative skill should remain available")
	}

	direct := db.Skill{Config: []byte(`{"kind":"creative_role","capability":"direct_image_edit"}`)}
	if creativeTaskSkillAllowed(direct, allowed) {
		t.Fatal("unrelated creative skill should be filtered")
	}
}
