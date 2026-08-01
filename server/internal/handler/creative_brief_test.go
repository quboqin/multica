package handler

import "testing"

func TestNormalizeCreativeBriefSeparatesThemeAndBenefit(t *testing.T) {
	confidence := 0.92
	brief, err := normalizeCreativeBrief(creativeBriefInput{
		Theme:             " 世界杯 / 足球赛事 ",
		ThemeElements:     []string{"足球", "足球", " 球场 "},
		PrimaryBenefit:    " 费用减免 ",
		SecondaryBenefits: []string{"低利率", "低利率"},
		Evidence:          []string{"主标题出现 biaya 25%"},
		Status:            "draft",
		Source:            "ai",
		Confidence:        &confidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if brief.Theme != "世界杯 / 足球赛事" || brief.PrimaryBenefit != "费用减免" {
		t.Fatalf("theme=%q primary_benefit=%q", brief.Theme, brief.PrimaryBenefit)
	}
	if len(brief.ThemeElements) != 2 || len(brief.SecondaryBenefits) != 1 {
		t.Fatalf("theme_elements=%v secondary_benefits=%v", brief.ThemeElements, brief.SecondaryBenefits)
	}
}

func TestNormalizeCreativeBriefRejectsInvalidConfidence(t *testing.T) {
	confidence := 1.1
	_, err := normalizeCreativeBrief(creativeBriefInput{
		Status: "draft", Source: "ai", Confidence: &confidence,
	})
	if err == nil {
		t.Fatal("expected invalid confidence to fail")
	}
}
