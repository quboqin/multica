package deviceruntime

import "testing"

func TestParseCurrentDisplaySize(t *testing.T) {
	output := []byte("init=576x1280 1dpi cur=576x1280 app=576x1280\ninit=1080x2400 420dpi cur=2400x1080 app=2274x1080")
	width, height, err := parseCurrentDisplaySize(output)
	if err != nil {
		t.Fatal(err)
	}
	if width != 2400 || height != 1080 {
		t.Fatalf("got %dx%d", width, height)
	}
}

func TestScaleCoordinate(t *testing.T) {
	if got := scaleCoordinate(288, 576, 1080); got != 540 {
		t.Fatalf("scaled x = %d", got)
	}
	if got := scaleCoordinate(384, 1280, 2400); got != 720 {
		t.Fatalf("scaled y = %d", got)
	}
}
