package handler

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreativeDirectEditProcessingSizes(t *testing.T) {
	expected := []string{"1080x1080", "1200x628", "800x1000"}
	targetOnly, err := creativeDirectEditProcessingSizes([]byte(`{
  "creative_direct_edit_delivery": {
    "scope": "size",
    "target_size": "1080x1080",
    "edit_sizes": ["1080x1080"],
    "final_visual_validation": true
  }
}`), expected)
	if err != nil || !reflect.DeepEqual(targetOnly, []string{"1080x1080"}) {
		t.Fatalf("size-scoped processing sizes = %v, %v", targetOnly, err)
	}
	all, err := creativeDirectEditProcessingSizes([]byte(`{
  "creative_direct_edit_delivery": {
    "scope": "variant",
    "target_size": "1080x1080",
    "final_visual_validation": true
  }
}`), expected)
	if err != nil || !reflect.DeepEqual(all, expected) {
		t.Fatalf("variant-scoped processing sizes = %v, %v", all, err)
	}
}

func TestCreativeDirectAdjustmentPreviewCommentIncludesUnadoptedImages(t *testing.T) {
	content := creativeDirectAdjustmentDeliveryCommentContent(
		3,
		creativeDirectAdjustmentDeliveryTarget{TargetSize: "1080x1080"},
		nil,
		nil,
		[]creativeDirectAdjustmentProcessAsset{{
			SizeKey: "1080x1080", Revision: 3, Workflow: "creative_direct_edit",
			Label: "直接改图尝试 2 · 未采用", Filename: "preview.png", AttachmentID: "attachment-1",
			UpdatedAt: pgtype.Timestamptz{Time: time.Date(2026, time.September, 2, 6, 16, 34, 0, time.UTC), Valid: true},
		}},
		"<!-- marker -->",
		true,
	)
	if !strings.Contains(content, "预览图已生成，但未通过交付验收") {
		t.Fatalf("preview comment header = %q", content)
	}
	if !strings.Contains(content, "![过程图片 直接改图尝试 2 · 未采用 1080x1080 r3](/api/attachments/attachment-1/download)") {
		t.Fatalf("preview comment did not include image markdown: %q", content)
	}
}
