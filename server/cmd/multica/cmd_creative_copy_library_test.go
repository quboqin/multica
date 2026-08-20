package main

import "testing"

func TestAddCreativeCopyLibraryFragmentDefaults(t *testing.T) {
	config, err := addCreativeCopyLibraryFragment(map[string]any{}, creativeCopyFragmentPatch{
		Text:   "Ajukan pinjaman fleksibel hari ini",
		Group:  "core_benefit",
		Status: "approved",
		Tags:   []string{"benefit", "benefit", " homepage "},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragments := creativeCopyLibraryObjectSlice(config["fragments"])
	if len(fragments) != 1 {
		t.Fatalf("fragments = %#v", fragments)
	}
	fragment := fragments[0]
	if creativeCopyLibraryString(fragment["content_group"]) != "core_benefit" ||
		creativeCopyLibraryString(fragment["role"]) != "benefit" ||
		creativeCopyLibraryString(fragment["usage"]) != "core" ||
		creativeCopyLibraryString(fragment["status"]) != "approved" {
		t.Fatalf("fragment defaults = %#v", fragment)
	}
	types, ok := fragment["creative_types"].([]any)
	if !ok || len(types) != 2 || types[0] != "num" || types[1] != "repayment_plan" {
		t.Fatalf("creative types = %#v", fragment["creative_types"])
	}
	tags, ok := fragment["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "benefit" || tags[1] != "homepage" {
		t.Fatalf("tags = %#v", fragment["tags"])
	}
}

func TestUpdateCreativeCopyLibraryFragmentDoesNotResetGroupByDefault(t *testing.T) {
	config, err := updateCreativeCopyLibraryFragment(map[string]any{
		"fragments": []map[string]any{{
			"id": "fragment-1", "key": "headline-1", "name": "Headline", "content_group": "standard_headline",
			"creative_types": []any{"num", "repayment_plan"}, "role": "headline", "text": "Old headline",
			"tags": []any{}, "usage": "core", "status": "approved",
		}},
	}, "headline-1", creativeCopyFragmentPatch{Text: "New headline"})
	if err != nil {
		t.Fatal(err)
	}
	fragments := creativeCopyLibraryObjectSlice(config["fragments"])
	fragment := fragments[0]
	if creativeCopyLibraryString(fragment["text"]) != "New headline" {
		t.Fatalf("text = %#v", fragment["text"])
	}
	if creativeCopyLibraryString(fragment["content_group"]) != "standard_headline" || creativeCopyLibraryString(fragment["role"]) != "headline" {
		t.Fatalf("group was reset: %#v", fragment)
	}
}

func TestUpsertCreativeCopyLibraryRepaymentPlanUpdatesExistingPair(t *testing.T) {
	config, err := upsertCreativeCopyLibraryRepaymentPlan(map[string]any{
		"repayment_plan": map[string]any{
			"entries": []map[string]any{{
				"id": "plan-1", "key": "plan-1000000-6", "principal": 1000000, "tenor_months": 6,
				"monthly_installment": 180000, "total_interest": 80000, "total_repayment": 1080000,
				"source": "old source", "status": "approved",
			}},
		},
	}, creativeRepaymentPlanPatch{
		Principal: 1000000, TenorMonths: 6, MonthlyInstallment: 175000,
		Changed: map[string]bool{"monthly-installment": true},
		Status:  "approved",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := creativeCopyLibraryMap(config["repayment_plan"])
	entries := creativeCopyLibraryObjectSlice(plan["entries"])
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	if creativeCopyLibraryInt(entries[0]["monthly_installment"]) != 175000 {
		t.Fatalf("monthly_installment = %#v", entries[0]["monthly_installment"])
	}
	if creativeCopyLibraryString(entries[0]["id"]) != "plan-1" {
		t.Fatalf("id = %#v", entries[0]["id"])
	}
}
