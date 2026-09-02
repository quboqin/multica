package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

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
