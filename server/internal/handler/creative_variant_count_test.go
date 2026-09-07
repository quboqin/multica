package handler

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCreativeVariantCountsFreezeCopyTargets(t *testing.T) {
	for _, target := range []int{1, 3, 10} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			input := creativeOrderInput{InputSnapshot: json.RawMessage(fmt.Sprintf(`{"target_variant_count":%d,"candidate_count":99}`, target)), Items: []creativeOrderItemInput{{SourceKind: "copy_library"}}}
			if err := freezeCreativeOrderVariantCount(&input); err != nil {
				t.Fatal(err)
			}
			counts, err := creativeOrderVariantCounts(input.InputSnapshot)
			if err != nil || counts.Target != target || counts.Candidates != target+2 {
				t.Fatalf("counts = %+v, %v", counts, err)
			}
			if !creativeReservePromotionAllowed(input.InputSnapshot) {
				t.Fatal("copy orders must enable bounded reserve promotion")
			}
			if err := counts.validateSelection(target, 2); err != nil {
				t.Fatal(err)
			}
			if err := counts.validateSelection(target-1, 2); err == nil {
				t.Fatal("incomplete selection accepted")
			}
			for rank := 1; rank <= target; rank++ {
				if err := counts.validateRank("selected", rank); err != nil {
					t.Fatal(err)
				}
			}
			if counts.validateRank("selected", target+1) == nil || counts.validateRank("reserve", target) == nil {
				t.Fatal("rank boundary accepted")
			}
			if !counts.allowsKey(fmt.Sprintf("C%02d", target+2)) || counts.allowsKey(fmt.Sprintf("C%02d", target+3)) {
				t.Fatal("candidate key bounds do not match target")
			}
		})
	}
}

func TestCreativeVariantCountsDefaultAndInvalidInputs(t *testing.T) {
	counts, err := creativeOrderVariantCounts(json.RawMessage(`{}`))
	if err != nil || counts.Target != 3 || counts.Candidates != 5 {
		t.Fatalf("historical default = %+v, %v", counts, err)
	}
	for _, raw := range []string{`{"target_variant_count":0}`, `{"target_variant_count":11}`, `{"target_variant_count":1.5}`, `{"target_variant_count":"10"}`, `{"target_variant_count":null}`, `{"target_variant_count":10,"candidate_count":5}`} {
		if _, err := creativeOrderVariantCounts(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid counts accepted: %s", raw)
		}
	}
	input := creativeOrderInput{InputSnapshot: json.RawMessage(`{"target_variant_count":10}`), Items: []creativeOrderItemInput{{SourceKind: "material"}}}
	if err := freezeCreativeOrderVariantCount(&input); err == nil {
		t.Fatal("material order accepted a custom target")
	}
}
