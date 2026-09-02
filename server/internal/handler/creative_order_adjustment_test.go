package handler

import "testing"

func TestCreativeOrderAdjustmentPromptProfile(t *testing.T) {
	tests := []struct {
		name    string
		request string
		want    string
	}{
		{name: "layout adjustment", request: "所有元素缩小一点向中间聚拢，下面预留一定空间，不要出现文字", want: "layout_micro_adjustment"},
		{name: "english layout adjustment", request: "Slightly scale down and center the content group", want: "layout_micro_adjustment"},
		{name: "bottom watermark removal", request: "移除底部水印", want: "standard_direct_edit"},
		{name: "background replacement", request: "把背景换成更明亮的办公室", want: "standard_direct_edit"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := creativeOrderAdjustmentPromptProfile(test.request); got != test.want {
				t.Fatalf("creativeOrderAdjustmentPromptProfile(%q) = %q, want %q", test.request, got, test.want)
			}
		})
	}
}
