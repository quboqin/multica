package handler

import (
	"strings"
	"testing"
)

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
		UserDirection:        " 保持绿色版式，CTA 更突出 ",
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
	if brief.UserDirection != "保持绿色版式，CTA 更突出" {
		t.Fatalf("user_direction=%q", brief.UserDirection)
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

func TestNormalizeCreativeBriefKeepsStructuredAppUIReferences(t *testing.T) {
	brief, err := normalizeCreativeBrief(creativeBriefInput{
		SelectedAppUIReferences: []creativeBriefAppUIReferenceInput{
			{ResourceFileID: " file-1 ", AttachmentID: " attachment-1 ", Reason: " 首页结构最接近 "},
			{ResourceFileID: "file-1", AttachmentID: "attachment-1", Reason: "duplicate"},
		},
		Status: "draft",
		Source: "ai",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !brief.AppUIReplacementRequired {
		t.Fatal("selected App UI references must require replacement")
	}
	if len(brief.SelectedAppUIReferences) != 1 {
		t.Fatalf("selected_app_ui_references=%v", brief.SelectedAppUIReferences)
	}
	if got := brief.SelectedAppUIReferences[0]; got.ResourceFileID != "file-1" || got.AttachmentID != "attachment-1" || got.Reason != "首页结构最接近" {
		t.Fatalf("selected_app_ui_reference=%+v", got)
	}
}

func TestNormalizeCreativeBriefRequiresReferenceWhenReplacingAppUI(t *testing.T) {
	_, err := normalizeCreativeBrief(creativeBriefInput{
		AppUIReplacementRequired: true,
		Status:                   "draft",
		Source:                   "ai",
	})
	if err == nil || !strings.Contains(err.Error(), "at least one selected App UI reference") {
		t.Fatalf("err=%v", err)
	}
}

func TestDecodeCreativeBriefReportsUnknownField(t *testing.T) {
	_, err := decodeCreativeBrief(strings.NewReader(`{"status":"draft","source":"ai","layout_guidance":"hero left"}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "layout_guidance"`) {
		t.Fatalf("err=%v", err)
	}
}
