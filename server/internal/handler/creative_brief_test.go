package handler

import "testing"

func TestNormalizeCreativeBriefSeparatesThemeAndBenefit(t *testing.T) {
	confidence := 0.92
	brief, err := normalizeCreativeBrief(creativeBriefInput{
		Theme:                " 世界杯 / 足球赛事 ",
		ThemeElements:        []string{"足球", "足球", " 球场 "},
		PrimaryBenefit:       " 费用减免 ",
		SecondaryBenefits:    []string{"低利率", "低利率"},
		SourceSemantics:      " 分期还款计划 ",
		InformationMechanism: " 多档月供表格 ",
		VisualAnchors:        []string{"月供表", "月供表", "中央数字卡片"},
		PaletteAnchors:       []string{"蓝白"},
		MustPreserve:         []string{"还款计划语义"},
		AllowedVariations:    []string{"表格布局"},
		Evidence:             []string{"主标题出现 biaya 25%"},
		Status:               "draft",
		Source:               "ai",
		Confidence:           &confidence,
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
	if brief.SourceSemantics != "分期还款计划" || brief.InformationMechanism != "多档月供表格" {
		t.Fatalf("source_semantics=%q information_mechanism=%q", brief.SourceSemantics, brief.InformationMechanism)
	}
	if len(brief.VisualAnchors) != 2 || len(brief.MustPreserve) != 1 || len(brief.AllowedVariations) != 1 {
		t.Fatalf("anchors=%v preserve=%v variations=%v", brief.VisualAnchors, brief.MustPreserve, brief.AllowedVariations)
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
